package server

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// A single owner shares one bounded read/export budget, independent of sessions
// and client IPs. The fixed window starts at the first accepted request.
type activityReadBudget struct {
	Start time.Time
	Count int
}

func (s *Server) allowActivityRead(now time.Time) (bool, int) {
	s.activityReadMu.Lock()
	defer s.activityReadMu.Unlock()
	b := &s.activityReadBudget
	if b.Start.IsZero() || !now.Before(b.Start.Add(time.Minute)) {
		b.Start = now
		b.Count = 0
	}
	if b.Count >= 60 {
		return false, max(1, int(b.Start.Add(time.Minute).Sub(now).Seconds())+1)
	}
	b.Count++
	return true, 0
}

type activityFilters struct {
	User     string `json:"user"`
	Node     string `json:"node"`
	Domain   string `json:"domain"`
	From     string `json:"from"`
	To       string `json:"to"`
	Limit    int    `json:"limit"`
	Format   string `json:"format"`
	Returned int    `json:"returned"`
}

func parseActivityFilters(r *http.Request, maxLimit int) (activityFilters, error) {
	f := activityFilters{Limit: 100, Format: "json"}
	if len(r.URL.RawQuery) > 8192 {
		return f, fmt.Errorf("invalid activity filters")
	}
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return f, fmt.Errorf("invalid activity filters")
	}
	for key, values := range q {
		switch key {
		case "user", "node", "domain", "from", "to", "limit", "format":
		default:
			return f, fmt.Errorf("unsupported activity filter")
		}
		if len(values) != 1 || len(values[0]) > 512 {
			return f, fmt.Errorf("invalid activity filters")
		}
	}
	f.User = q.Get("user")
	f.Node = q.Get("node")
	f.Domain = strings.ToLower(q.Get("domain"))
	f.Limit = limit(r, 100, maxLimit)
	if v := q.Get("format"); v != "" {
		if v != "csv" && v != "json" {
			return f, fmt.Errorf("invalid activity format")
		}
		f.Format = v
	}
	var from, to time.Time
	for _, bound := range []struct {
		name   string
		dst    *string
		parsed *time.Time
	}{{"from", &f.From, &from}, {"to", &f.To, &to}} {
		if v := q.Get(bound.name); v != "" {
			t, err := time.Parse(time.RFC3339Nano, v)
			if err != nil {
				return f, fmt.Errorf("invalid activity timestamp")
			}
			*bound.dst = t.UTC().Format(time.RFC3339Nano)
			*bound.parsed = t
		}
	}
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		return f, fmt.Errorf("invalid activity range")
	}
	return f, nil
}

const activityAuditPrefix = "sg-activity-audit-v1:"

func activityAuditAAD(action string, ownerID int) string {
	return "activity-audit-v1:" + action + ":" + strconv.Itoa(ownerID)
}
func (s *Server) recordActivityAccess(r *http.Request, action, status string, f activityFilters) error {
	p := admin(r)
	plain, err := json.Marshal(f)
	if err != nil {
		return err
	}
	encrypted, err := s.encrypt(plain, activityAuditAAD(action, p.ID))
	if err != nil {
		return err
	}
	details := activityAuditPrefix + base64.StdEncoding.EncodeToString(encrypted)
	_, err = s.db.Exec("INSERT INTO audit_log(admin_id,action,resource,resource_id,details,status,ip_address) VALUES(?,?,?,?,?,?,?)", p.ID, action, "activity", f.User, details, status, s.clientIP(r))
	return err
}
func (s *Server) decodeActivityAudit(details, action string, ownerID int) (any, error) {
	if !strings.HasPrefix(details, activityAuditPrefix) {
		return nil, nil
	} // legacy rows have no filter details
	encrypted, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(details, activityAuditPrefix))
	if err != nil {
		return nil, err
	}
	plain, err := s.decrypt(encrypted, activityAuditAAD(action, ownerID))
	if err != nil {
		return nil, err
	}
	var f activityFilters
	if err = json.Unmarshal(plain, &f); err != nil {
		return nil, err
	}
	return f, nil
}

// Query first, commit a final access outcome, then release data. Audit failure
// always denies the response; query failures never become successful reads.
func (s *Server) readActivity(w http.ResponseWriter, r *http.Request, maxLimit int, action string) ([]activityRow, activityFilters, bool) {
	if ok, retry := s.allowActivityRead(time.Now()); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(retry))
		fail(w, 429, "activity read rate limit exceeded")
		return nil, activityFilters{}, false
	}
	f, err := parseActivityFilters(r, maxLimit)
	if err == nil && action == "activity.read" && f.Format != "json" {
		err = fmt.Errorf("CSV format requires the export endpoint")
	}
	if err != nil {
		if auditErr := s.recordActivityAccess(r, action, "failure", activityFilters{}); auditErr != nil {
			fail(w, 500, "audit unavailable")
		} else {
			fail(w, 400, err.Error())
		}
		return nil, f, false
	}
	data, err := s.queryActivity(f)
	status := "success"
	if err != nil {
		status = "failure"
	}
	f.Returned = len(data)
	if auditErr := s.recordActivityAccess(r, action, status, f); auditErr != nil {
		fail(w, 500, "audit unavailable")
		return nil, f, false
	}
	if err != nil {
		fail(w, 500, "activity query failed")
		return nil, f, false
	}
	return data, f, true
}
