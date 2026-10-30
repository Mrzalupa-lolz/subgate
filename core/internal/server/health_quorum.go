package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/subgate/subgate/core/internal/panel"
	"github.com/subgate/subgate/pkg/wire"
)

const healthVoteTTL = 90 * time.Second

var errHealthCredential = errors.New("health credential changed")

type healthVote struct {
	Vantage        string            `json:"vantage"`
	CredentialHash string            `json:"credential_hash"`
	ReceivedAt     time.Time         `json:"received_at"`
	Result         wire.HealthResult `json:"result"`
}

func healthTarget(n panel.Node) wire.HealthTarget {
	t := wire.HealthTarget{NodeID: n.ID, Address: n.IPAddress}
	for _, in := range n.Inbounds {
		t.Inbounds = append(t.Inbounds, wire.HealthInbound{Protocol: in.Type, Network: in.Network, Security: in.Security, Port: in.Port, HTTP: in.HTTP})
	}
	return t
}

func (s *Server) agentHealthTargets(w http.ResponseWriter, r *http.Request) {
	// Take the health lock before opening DB rows to avoid inverting the
	// writer's healthMu -> DB transaction lock order.
	states := s.committedHealthSnapshot()
	rows, err := s.db.Query("SELECT data FROM sg_objects WHERE kind='nodes' ORDER BY id LIMIT 65")
	if err != nil {
		fail(w, 500, "health targets unavailable")
		return
	}
	defer rows.Close()
	targets := []wire.HealthTarget{}
	for rows.Next() {
		var raw string
		var n panel.Node
		if rows.Scan(&raw) != nil || json.Unmarshal([]byte(raw), &n) != nil {
			fail(w, 500, "health targets unavailable")
			return
		}
		if s.inMaintenance(n.ID, time.Now()) {
			continue
		}
		if state, ok := states[n.ID]; ok && time.Now().Before(state.NextProbeAt) {
			continue
		}
		t := healthTarget(n)
		// Preserve original endpoint count for incomplete-coverage detection.
		if len(t.Inbounds) > 16 {
			continue
		}
		targets = append(targets, t)
	}
	if rows.Err() != nil || len(targets) > 64 {
		fail(w, 503, "health fleet exceeds agent cycle limit")
		return
	}
	writeJSON(w, 200, map[string]any{"data": targets})
}

func (s *Server) agentHealthResults(w http.ResponseWriter, r *http.Request) {
	var report wire.HealthReport
	if !decode(w, r, &report) {
		return
	}
	if len(report.Results) > 64 {
		fail(w, 400, "too many health results")
		return
	}
	seen := map[string]bool{}
	now := time.Now()
	for i, result := range report.Results {
		var n panel.Node
		if result.NodeID == "" || seen[result.NodeID] || s.object("nodes", result.NodeID, &n) != nil || result.Timestamp.IsZero() || now.Sub(result.Timestamp) > healthVoteTTL || result.Timestamp.After(now.Add(5*time.Second)) || (result.Verified != "tcp" && result.Verified != "tls" && result.Verified != "ws" && result.Verified != "httpupgrade" && result.Verified != "grpc" && result.Verified != "xhttp-options" && result.Verified != "xhttp-h3-options" && result.Verified != "unverified") || len(result.Error) > 256 || len(result.Protocol) > 64 || len(result.Transport) > 64 || len(result.Security) > 64 {
			fail(w, 400, "invalid health result")
			return
		}
		if !result.CompletedAt.IsZero() && (result.CompletedAt.Before(result.Timestamp) || result.CompletedAt.After(now.Add(5*time.Second))) {
			fail(w, 400, "invalid health observation interval")
			return
		}
		seen[result.NodeID] = true
		if !http3VoteCompatible(n, result.Verified, result.Success) {
			report.Results[i].Verified = "unverified"
			report.Results[i].Success = false
		}
	}
	// source cannot be supplied in JSON; bind it to the authenticated credential.
	vantage := node(r)
	credential := hash(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	err := s.enrollmentTransaction(func(tx *sql.Tx) error {
		var current string
		if err := tx.QueryRow("SELECT token_hash FROM agents WHERE node_id=? AND token_hash=? AND status!='revoked'", vantage, credential).Scan(&current); err != nil {
			return errHealthCredential
		}
		for _, result := range report.Results {
			key := hash(vantage + "\x00" + result.NodeID)
			var oldRaw string
			if err := tx.QueryRow("SELECT data FROM sg_objects WHERE kind='health-vote' AND id=?", key).Scan(&oldRaw); err == nil {
				var previous healthVote
				if err := json.Unmarshal([]byte(oldRaw), &previous); err != nil {
					return err
				}
				if previous.CredentialHash == credential && previous.Result.Timestamp.After(result.Timestamp) {
					continue
				}
			} else if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			// Store only the evidence used by quorum, not attacker-supplied details.
			result = wire.HealthResult{NodeID: result.NodeID, Timestamp: result.Timestamp, CompletedAt: result.CompletedAt, Verified: result.Verified, Success: result.Success}
			vote := healthVote{Vantage: vantage, CredentialHash: credential, ReceivedAt: now, Result: result}
			raw, err := json.Marshal(vote)
			if err != nil {
				return err
			}
			if _, err := tx.Exec("INSERT INTO sg_objects(kind,id,data,updated_at) VALUES('health-vote',?,?,?) ON CONFLICT(kind,id) DO UPDATE SET data=excluded.data,updated_at=excluded.updated_at", key, string(raw), now.Unix()); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errHealthCredential) {
			fail(w, 401, "invalid agent token")
			return
		}
		fail(w, 500, "health evidence persistence failed")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) probeQuorum(tx *sql.Tx, core nodeProbe) (bool, bool, int, error) {
	var n panel.Node
	var nodeRaw string
	if err := tx.QueryRow("SELECT data FROM sg_objects WHERE kind='nodes' AND id=?", core.NodeID).Scan(&nodeRaw); err != nil {
		return false, false, 0, err
	}
	if err := json.Unmarshal([]byte(nodeRaw), &n); err != nil {
		return false, false, 0, err
	}
	positive, negative := 0, 0
	if core.Verified != "unverified" && !s.overlapsMaintenance(core.NodeID, core.Timestamp, core.CompletedAt) {
		if core.Success {
			positive++
		} else {
			negative++
		}
	}
	rows, err := tx.Query(`SELECT o.data FROM sg_objects o JOIN agents a ON a.node_id=json_extract(o.data,'$.vantage') AND a.token_hash=json_extract(o.data,'$.credential_hash') AND a.status!='revoked' WHERE o.kind='health-vote' AND json_extract(o.data,'$.result.node_id')=?`, core.NodeID)
	if err != nil {
		return false, false, 0, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	now := time.Now()
	for rows.Next() {
		var raw string
		var vote healthVote
		if err := rows.Scan(&raw); err != nil {
			return false, false, 0, err
		}
		if err := json.Unmarshal([]byte(raw), &vote); err != nil {
			return false, false, 0, err
		}
		completedAt := vote.Result.CompletedAt
		if completedAt.IsZero() {
			// Legacy Agents do not report the observation end. Receive time is
			// a conservative bound, including reports already saved on disk.
			completedAt = vote.ReceivedAt
		}
		if seen[vote.Vantage] || !http3VoteCompatible(n, vote.Result.Verified, vote.Result.Success) || s.overlapsMaintenance(core.NodeID, vote.Result.Timestamp, completedAt) || vote.Result.Verified == "unverified" || now.Sub(vote.Result.Timestamp) > healthVoteTTL || vote.Result.Timestamp.After(now.Add(5*time.Second)) || now.Sub(vote.ReceivedAt) > healthVoteTTL {
			continue
		}
		seen[vote.Vantage] = true
		if vote.Result.Success {
			positive++
		} else {
			negative++
		}
	}
	if err := rows.Err(); err != nil {
		return false, false, 0, err
	}
	q := s.config.Health.Quorum
	// Positive evidence wins a tie to preserve fail-open behavior.
	if positive >= q {
		return true, true, positive + negative, nil
	}
	return false, negative >= q, positive + negative, nil
}

// Older Agents ignore the HTTP3 field and may report TCP failure for a UDP
// listener. Recheck persisted votes against the current node as well. Mixed
// endpoints lack a coverage/capability claim, so their negative Agent votes
// remain unverified; positive reachability still preserves fail-open behavior.
func http3VoteCompatible(n panel.Node, verified string, success bool) bool {
	h3, other := false, false
	for _, in := range n.Inbounds {
		if in.HTTP != nil && in.HTTP.HTTP3 {
			h3 = true
		} else {
			other = true
		}
	}
	if !h3 {
		return true
	}
	if success {
		return other || verified == "xhttp-h3-options"
	}
	return !other && verified == "xhttp-h3-options"
}
