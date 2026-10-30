package server

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/subgate/subgate/core/internal/panel"
	"github.com/subgate/subgate/core/internal/tsdb"
	"github.com/subgate/subgate/pkg/wire"
)

// liveAPIItem is deliberately a flat record. A row represents one independent
// (source user, node, scope) series, so simultaneous connections to different
// nodes are never collapsed into one user's total.
type liveAPIItem struct {
	UserID        string `json:"user_id"`
	PanelUserID   string `json:"panel_user_id,omitempty"`
	UserName      string `json:"user_name,omitempty"`
	UserEmail     string `json:"user_email,omitempty"`
	NodeID        string `json:"node_id"`
	NodeName      string `json:"node_name,omitempty"`
	NodeAddress   string `json:"node_address,omitempty"`
	NodeStatus    string `json:"node_status,omitempty"`
	Scope         string `json:"scope"`
	IdentityKind  string `json:"identity_kind"`
	Coarse        bool   `json:"coarse"`
	Timestamp     string `json:"timestamp"`
	UplinkBPS     int64  `json:"uplink_bps"`
	DownlinkBPS   int64  `json:"downlink_bps"`
	UplinkEMA3    int64  `json:"uplink_ema_bps"`
	DownlinkEMA3  int64  `json:"downlink_ema_bps"`
	UplinkEMA10   int64  `json:"uplink_ema10_bps"`
	DownlinkEMA10 int64  `json:"downlink_ema10_bps"`
	UplinkAvg     int64  `json:"uplink_avg_bps"`
	DownlinkAvg   int64  `json:"downlink_avg_bps"`
	UplinkPeak    int64  `json:"uplink_peak_bps"`
	DownlinkPeak  int64  `json:"downlink_peak_bps"`
	UplinkP95     int64  `json:"uplink_p95_bps"`
	DownlinkP95   int64  `json:"downlink_p95_bps"`
	UplinkDelta   int64  `json:"uplink_delta_bytes"`
	DownlinkDelta int64  `json:"downlink_delta_bytes"`
	UplinkTotal   int64  `json:"uplink_total_bytes"`
	DownlinkTotal int64  `json:"downlink_total_bytes"`
	IntervalMS    int64  `json:"interval_ms"`
	AgeMS         int64  `json:"age_ms"`
	Status        string `json:"status"`
	Gap           bool   `json:"gap"`
	Warming       bool   `json:"warming"`
}

type liveIdentity struct {
	UserID, UserName, UserEmail       string
	NodeName, NodeAddress, NodeStatus string
}

func (s *Server) liveIdentities() (map[string]liveIdentity, map[string]liveIdentity) {
	users := map[string]liveIdentity{}
	if rows, err := s.db.Query("SELECT id,data FROM sg_objects WHERE kind='users'"); err == nil {
		defer rows.Close()
		for rows.Next() {
			var id, raw string
			if rows.Scan(&id, &raw) != nil {
				continue
			}
			var u panel.User
			if json.Unmarshal([]byte(raw), &u) != nil {
				continue
			}
			if u.ID == "" {
				u.ID = id
			}
			value := liveIdentity{UserID: u.ID, UserName: u.Username, UserEmail: u.Email}
			for _, key := range []string{u.ID, u.Email, u.Username, u.ShortUUID} {
				if key != "" {
					users[key] = value
				}
			}
		}
	}
	nodes := map[string]liveIdentity{}
	if rows, err := s.db.Query("SELECT id,data FROM sg_objects WHERE kind='nodes'"); err == nil {
		defer rows.Close()
		for rows.Next() {
			var id, raw string
			if rows.Scan(&id, &raw) != nil {
				continue
			}
			var n panel.Node
			if json.Unmarshal([]byte(raw), &n) != nil {
				continue
			}
			if n.ID == "" {
				n.ID = id
			}
			value := liveIdentity{NodeName: n.Name, NodeAddress: n.IPAddress, NodeStatus: n.Status}
			nodes[n.ID] = value
		}
	}
	return users, nodes
}

func (s *Server) currentLiveRows(now time.Time, window time.Duration) ([]liveAPIItem, error) {
	if window <= 0 || window > time.Minute {
		window = 15 * time.Second
	}
	users, nodes := s.liveIdentities()
	latest := map[tsdb.Series]tsdb.LiveSample{}
	for _, sample := range s.liveStore.Snapshot(now) {
		latest[tsdb.Series{User: sample.UserID, Node: sample.NodeID, Scope: sample.Scope}] = sample
	}
	// The in-memory index is intentionally disposable. Read the recent SQLite
	// tail as a restart fallback so a Core restart does not erase live traffic.
	rows, err := s.db.Query(`SELECT p.node_id,p.user_id,p.timestamp,p.uplink,p.downlink,
	 COALESCE(m.interval_ms,1000),COALESCE(m.scope,'user'),COALESCE(m.gap,0),
	 COALESCE(m.warming,0),COALESCE(m.uplink_delta,0),COALESCE(m.downlink_delta,0)
	 FROM sg_speed p LEFT JOIN sg_speed_meta m ON m.node_id=p.node_id AND m.user_id=p.user_id AND m.timestamp=p.timestamp
	 WHERE p.timestamp>=? ORDER BY p.timestamp DESC`, now.Add(-window).UnixMilli())
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var node, user, scope string
		var stamp, up, down, interval, gap, warming, upDelta, downDelta int64
		if err := rows.Scan(&node, &user, &stamp, &up, &down, &interval, &scope, &gap, &warming, &upDelta, &downDelta); err != nil {
			rows.Close()
			return nil, err
		}
		if scope == "" {
			scope = "user"
		}
		key := tsdb.Series{User: user, Node: node, Scope: scope}
		if _, exists := latest[key]; exists {
			continue
		}
		latest[key] = tsdb.LiveSample{Sample: wire.Sample{UserID: user, Timestamp: time.UnixMilli(stamp), UplinkBps: up, DownlinkBps: down, IntervalMS: interval, Scope: scope, Gap: gap != 0, Warming: warming != 0, UplinkDelta: upDelta, DownlinkDelta: downDelta}, NodeID: node}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	totals := map[tsdb.Series][2]int64{}
	usageRows, err := s.db.Query("SELECT node_id,user_id,SUM(uplink),SUM(downlink) FROM sg_usage WHERE timestamp>=? GROUP BY node_id,user_id", now.Add(-24*time.Hour).UnixMilli())
	if err == nil {
		defer usageRows.Close()
		for usageRows.Next() {
			var node, user string
			var up, down int64
			if usageRows.Scan(&node, &user, &up, &down) == nil {
				totals[tsdb.Series{User: user, Node: node, Scope: "user"}] = [2]int64{up, down}
			}
		}
	}

	result := make([]liveAPIItem, 0, len(latest))
	for key, sample := range latest {
		age := now.Sub(sample.Timestamp).Milliseconds()
		if age < 0 {
			age = 0
		}
		if age > window.Milliseconds() {
			continue
		}
		identity := users[key.User]
		nodeIdentity := nodes[key.Node]
		status := sample.Status
		if status == "" {
			status = "live"
			if sample.Gap || age > max(3000, sample.IntervalMS*2+500) {
				status = "gap"
			}
			if sample.Warming {
				status = "warming"
			}
		}
		identityKind := "user"
		if key.Scope == "inbound" {
			identityKind = "inbound"
		}
		total := totals[key]
		result = append(result, liveAPIItem{
			UserID: key.User, PanelUserID: identity.UserID, UserName: identity.UserName, UserEmail: identity.UserEmail,
			NodeID: key.Node, NodeName: nodeIdentity.NodeName, NodeAddress: nodeIdentity.NodeAddress, NodeStatus: nodeIdentity.NodeStatus,
			Scope: key.Scope, IdentityKind: identityKind, Coarse: key.Scope != "user", Timestamp: sample.Timestamp.UTC().Format(time.RFC3339Nano),
			UplinkBPS: sample.UplinkBps, DownlinkBPS: sample.DownlinkBps, UplinkEMA3: sample.EMA3Up, DownlinkEMA3: sample.EMA3Down,
			UplinkEMA10: sample.EMA10Up, DownlinkEMA10: sample.EMA10Down, UplinkAvg: sample.AverageUp, DownlinkAvg: sample.AverageDown,
			UplinkPeak: sample.PeakUp, DownlinkPeak: sample.PeakDown, UplinkP95: sample.P95Up, DownlinkP95: sample.P95Down,
			UplinkDelta: sample.UplinkDelta, DownlinkDelta: sample.DownlinkDelta, UplinkTotal: total[0], DownlinkTotal: total[1],
			IntervalMS: sample.IntervalMS, AgeMS: age, Status: status, Gap: sample.Gap, Warming: sample.Warming,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].NodeID != result[j].NodeID {
			return result[i].NodeID < result[j].NodeID
		}
		return result[i].UserID < result[j].UserID
	})
	return result, nil
}

func liveMatches(item liveAPIItem, r *http.Request) bool {
	user := strings.TrimSpace(r.URL.Query().Get("user"))
	if user != "" && !strings.EqualFold(user, item.UserID) && !strings.EqualFold(user, item.PanelUserID) && !strings.EqualFold(user, item.UserName) && !strings.EqualFold(user, item.UserEmail) {
		return false
	}
	if node := strings.TrimSpace(r.URL.Query().Get("node")); node != "" && node != item.NodeID {
		return false
	}
	if scope := strings.TrimSpace(r.URL.Query().Get("scope")); scope != "" && scope != item.Scope {
		return false
	}
	return true
}

func (s *Server) liveSnapshot(r *http.Request) (map[string]any, error) {
	window := 15 * time.Second
	if raw := r.URL.Query().Get("window_seconds"); raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err != nil || seconds < 1 || seconds > 60 {
			return nil, fmt.Errorf("window_seconds must be between 1 and 60")
		}
		window = time.Duration(seconds) * time.Second
	}
	now := time.Now()
	items, err := s.currentLiveRows(now, window)
	if err != nil {
		return nil, err
	}
	filtered := items[:0]
	for _, item := range items {
		if liveMatches(item, r) {
			filtered = append(filtered, item)
		}
	}
	return map[string]any{"data": filtered, "generated_at": now.UTC(), "sample_interval_ms": 1000, "window_seconds": int(window / time.Second)}, nil
}

func (s *Server) live(w http.ResponseWriter, r *http.Request) {
	response, err := s.liveSnapshot(r)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) liveStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		fail(w, http.StatusInternalServerError, "streaming is unavailable")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Header().Set("Connection", "keep-alive")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	send := func() bool {
		response, err := s.liveSnapshot(r)
		if err != nil {
			_, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", strconv.Quote(err.Error()))
			flusher.Flush()
			return true
		}
		raw, _ := json.Marshal(response)
		if _, err := fmt.Fprintf(w, "event: live\ndata: %s\n\n", raw); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	if !send() {
		return
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if !send() {
				return
			}
		}
	}
}

type trafficBucket struct {
	Key       string
	Bucket    time.Time
	UserID    string
	NodeID    string
	Scope     string
	Up        []int64
	Down      []int64
	UpBytes   int64
	DownBytes int64
}

type trafficAPIItem struct {
	Timestamp       string `json:"timestamp"`
	UserID          string `json:"user_id"`
	NodeID          string `json:"node_id"`
	Scope           string `json:"scope"`
	UplinkBPS       int64  `json:"uplink_bps"`
	DownlinkBPS     int64  `json:"downlink_bps"`
	UplinkAvgBPS    int64  `json:"uplink_avg_bps"`
	DownlinkAvgBPS  int64  `json:"downlink_avg_bps"`
	UplinkPeakBPS   int64  `json:"uplink_peak_bps"`
	DownlinkPeakBPS int64  `json:"downlink_peak_bps"`
	UplinkP95BPS    int64  `json:"uplink_p95_bps"`
	DownlinkP95BPS  int64  `json:"downlink_p95_bps"`
	UplinkBytes     int64  `json:"uplink_bytes"`
	DownlinkBytes   int64  `json:"downlink_bytes"`
	Samples         int    `json:"samples"`
}

func parseTrafficTime(raw string, fallback time.Time) (time.Time, error) {
	if raw == "" {
		return fallback, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, nil
	}
	ms, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid traffic timestamp")
	}
	if ms < 1e12 {
		return time.Unix(ms, 0), nil
	}
	return time.UnixMilli(ms), nil
}

func (s *Server) trafficRows(r *http.Request, forcedUsers []string) (map[string]any, error) {
	now := time.Now()
	minutes := 60
	if raw := r.URL.Query().Get("minutes"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 1440 {
			return nil, fmt.Errorf("minutes must be between 1 and 1440")
		}
		minutes = value
	}
	from, err := parseTrafficTime(r.URL.Query().Get("from"), now.Add(-time.Duration(minutes)*time.Minute))
	if err != nil {
		return nil, err
	}
	to, err := parseTrafficTime(r.URL.Query().Get("to"), now)
	if err != nil {
		return nil, err
	}
	if to.Before(from) || to.Sub(from) > 31*24*time.Hour {
		return nil, fmt.Errorf("traffic range must be positive and at most 31 days")
	}
	args := []any{from.UnixMilli(), to.UnixMilli()}
	query := `SELECT p.node_id,p.user_id,p.timestamp,p.uplink,p.downlink,COALESCE(m.scope,'user'),COALESCE(m.uplink_delta,0),COALESCE(m.downlink_delta,0)
	 FROM sg_speed p LEFT JOIN sg_speed_meta m ON m.node_id=p.node_id AND m.user_id=p.user_id AND m.timestamp=p.timestamp
	 WHERE p.timestamp>=? AND p.timestamp<=?`
	if node := r.URL.Query().Get("node"); node != "" {
		query += " AND p.node_id=?"
		args = append(args, node)
	}
	users := forcedUsers
	if len(users) == 0 {
		if selector := r.URL.Query().Get("user"); selector != "" {
			users, err = s.activityIdentities(selector)
			if err != nil {
				return nil, err
			}
		}
	}
	if len(users) > 0 {
		placeholders := make([]string, len(users))
		for i, user := range users {
			placeholders[i] = "?"
			args = append(args, user)
		}
		query += " AND p.user_id IN (" + strings.Join(placeholders, ",") + ")"
	}
	query += " ORDER BY p.timestamp ASC LIMIT 200000"
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	buckets := map[string]*trafficBucket{}
	for rows.Next() {
		var node, user, scope string
		var stamp, up, down, upBytes, downBytes int64
		if err := rows.Scan(&node, &user, &stamp, &up, &down, &scope, &upBytes, &downBytes); err != nil {
			return nil, err
		}
		if scope == "" {
			scope = "user"
		}
		at := time.UnixMilli(stamp).UTC()
		bucket := at.Truncate(10 * time.Second)
		key := user + "\x00" + node + "\x00" + scope + "\x00" + strconv.FormatInt(bucket.Unix(), 10)
		entry := buckets[key]
		if entry == nil {
			entry = &trafficBucket{Key: key, Bucket: bucket, UserID: user, NodeID: node, Scope: scope}
			buckets[key] = entry
		}
		entry.Up = append(entry.Up, up)
		entry.Down = append(entry.Down, down)
		entry.UpBytes += upBytes
		entry.DownBytes += downBytes
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make([]trafficAPIItem, 0, len(buckets))
	for _, bucket := range buckets {
		upAvg, upPeak, upP95 := tsdb.Distribution(bucket.Up)
		downAvg, downPeak, downP95 := tsdb.Distribution(bucket.Down)
		result = append(result, trafficAPIItem{Timestamp: bucket.Bucket.UTC().Format(time.RFC3339), UserID: bucket.UserID, NodeID: bucket.NodeID, Scope: bucket.Scope, UplinkBPS: upAvg, DownlinkBPS: downAvg, UplinkAvgBPS: upAvg, DownlinkAvgBPS: downAvg, UplinkPeakBPS: upPeak, DownlinkPeakBPS: downPeak, UplinkP95BPS: upP95, DownlinkP95BPS: downP95, UplinkBytes: bucket.UpBytes, DownlinkBytes: bucket.DownBytes, Samples: len(bucket.Up)})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Timestamp != result[j].Timestamp {
			return result[i].Timestamp < result[j].Timestamp
		}
		if result[i].NodeID != result[j].NodeID {
			return result[i].NodeID < result[j].NodeID
		}
		return result[i].UserID < result[j].UserID
	})
	return map[string]any{"data": result, "from": from.UTC(), "to": to.UTC(), "bucket_seconds": 10}, nil
}

func (s *Server) traffic(w http.ResponseWriter, r *http.Request) {
	response, err := s.trafficRows(r, nil)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) userTraffic(w http.ResponseWriter, r *http.Request) {
	selector := chi.URLParam(r, "id")
	identities, err := s.activityIdentities(selector)
	if err != nil {
		fail(w, 400, "invalid user selector")
		return
	}
	response, err := s.trafficRows(r, identities)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) exportLive(w http.ResponseWriter, r *http.Request) {
	response, err := s.liveSnapshot(r)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	items, _ := response["data"].([]liveAPIItem)
	if strings.EqualFold(r.URL.Query().Get("format"), "csv") {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=live-traffic.csv")
		writer := csv.NewWriter(w)
		_ = writer.Write([]string{"timestamp", "user_id", "user_name", "node_id", "node_name", "scope", "uplink_bps", "downlink_bps", "uplink_ema_bps", "downlink_ema_bps", "uplink_total_bytes", "downlink_total_bytes", "age_ms", "status"})
		for _, item := range items {
			_ = writer.Write([]string{item.Timestamp, item.UserID, item.UserName, item.NodeID, item.NodeName, item.Scope, strconv.FormatInt(item.UplinkBPS, 10), strconv.FormatInt(item.DownlinkBPS, 10), strconv.FormatInt(item.UplinkEMA3, 10), strconv.FormatInt(item.DownlinkEMA3, 10), strconv.FormatInt(item.UplinkTotal, 10), strconv.FormatInt(item.DownlinkTotal, 10), strconv.FormatInt(item.AgeMS, 10), item.Status})
		}
		writer.Flush()
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename=live-traffic.json")
	writeJSON(w, http.StatusOK, response)
}
