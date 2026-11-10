package server

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/subgate/subgate/pkg/wire"
)

// A domain search must find matches older than the newest Limit rows.
func TestActivityDomainSearchLooksPastLimit(t *testing.T) {
	s := newTestServer(t)
	session := loginTest(t, s)
	now := time.Now().UTC()
	insert := func(host string, at time.Time) {
		raw, _ := json.Marshal(wire.Activity{UserID: "1", Timestamp: at, DstHost: host, DstPort: 443})
		payload, err := s.encrypt(raw, "demo-node-1:1")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.db.Exec("INSERT INTO sg_activity(node_id,user_id,timestamp,payload) VALUES('demo-node-1','1',?,?)", at.UnixMilli(), payload); err != nil {
			t.Fatal(err)
		}
	}
	insert("rare-tracker.example.invalid", now.Add(-time.Hour))
	for i := 0; i < 30; i++ {
		insert(fmt.Sprintf("noise-%d.example.invalid", i), now.Add(-time.Duration(i)*time.Second))
	}
	w := perform(s, "GET", "/api/v1/activity?domain=rare-tracker&limit=10", nil, session, "", false)
	if w.Code != 200 {
		t.Fatalf("activity status %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Data []struct {
			DstHost string `json:"dst_host"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 1 || body.Data[0].DstHost != "rare-tracker.example.invalid" {
		t.Fatalf("older domain match not found: %s", w.Body.String())
	}
}
