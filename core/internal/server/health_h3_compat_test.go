package server

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/subgate/subgate/core/internal/panel"
	"github.com/subgate/subgate/pkg/wire"
)

func TestHTTP3LegacyVotesCannotQuarantine(t *testing.T) {
	s := newTestServer(t)
	s.config.Health.Quorum = 2
	n := panel.Node{ID: "h3", IPAddress: "127.0.0.1", Inbounds: []panel.Inbound{{Type: "vless", Network: "xhttp", Security: "tls", Port: 443, HTTP: &wire.HealthHTTP{Path: "/probe/", HTTP3: true}}}}
	if err := s.saveObject("nodes", n.ID, n); err != nil {
		t.Fatal(err)
	}
	session := loginTest(t, s)
	for _, vantage := range []string{"demo-node-1", "demo-node-2"} {
		token, _ := enrollTest(t, s, session, vantage)
		legacy := wire.HealthResult{NodeID: n.ID, Timestamp: time.Now(), Verified: "xhttp-options", Success: false}
		if w := perform(s, "PUT", "/api/v1/agents/health-results", wire.HealthReport{Results: []wire.HealthResult{legacy}}, nil, token, false); w.Code != 200 {
			t.Fatal(w.Code)
		}
		var vote healthVote
		if err := s.object("health-vote", hash(vantage+"\x00"+n.ID), &vote); err != nil || vote.Result.Verified != "unverified" {
			t.Fatal("legacy TCP result accepted for H3-only node", vote, err)
		}
		// Simulate a vote saved before Core upgrade: read-time validation must
		// exclude it even though ingestion no longer allows it.
		vote.Result = legacy
		if err := s.saveObject("health-vote", hash(vantage+"\x00"+n.ID), vote); err != nil {
			t.Fatal(err)
		}
	}
	core := nodeProbe{NodeID: n.ID, Timestamp: time.Now(), CompletedAt: time.Now(), Verified: "xhttp-h3-options", Success: true}
	for i := 0; i < 4; i++ {
		if err := s.enrollmentTransaction(func(tx *sql.Tx) error {
			_, quorum, vantages, err := s.probeQuorum(tx, core)
			if err == nil && (quorum || vantages != 1) {
				t.Errorf("legacy failures became quorum: quorum=%v vantages=%d", quorum, vantages)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	// Mixed endpoints cannot prove complete coverage using a single legacy
	// transport verdict; a success on an actual TCP endpoint is still useful.
	n.Inbounds = append(n.Inbounds, panel.Inbound{Type: "vless", Network: "tcp", Port: 443})
	if err := s.saveObject("nodes", n.ID, n); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		verified string
		success  bool
		want     string
	}{
		{"xhttp-options", false, "unverified"},
		{"xhttp-h3-options", false, "unverified"},
		{"tcp", true, "tcp"},
	} {
		token, _ := enrollTest(t, s, session, "demo-node-1")
		result := wire.HealthResult{NodeID: n.ID, Timestamp: time.Now(), Verified: tc.verified, Success: tc.success}
		if w := perform(s, "PUT", "/api/v1/agents/health-results", wire.HealthReport{Results: []wire.HealthResult{result}}, nil, token, false); w.Code != 200 {
			t.Fatal(w.Code)
		}
		var raw string
		if err := s.db.QueryRow("SELECT data FROM sg_objects WHERE kind='health-vote' AND id=?", hash("demo-node-1\x00"+n.ID)).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var vote healthVote
		if err := json.Unmarshal([]byte(raw), &vote); err != nil || vote.Result.Verified != tc.want {
			t.Fatal("mixed coverage guessed", vote, err)
		}
	}
}
