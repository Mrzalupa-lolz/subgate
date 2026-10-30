package server

import (
	"encoding/json"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/subgate/subgate/core/internal/panel"
	"github.com/subgate/subgate/pkg/wire"
)

func TestActivityPanelLinksResolveXrayIdentitiesAndPurge(t *testing.T) {
	s := newTestServer(t)
	session := loginTest(t, s)
	credential, _ := enrollTest(t, s, session, "demo-node-1")
	batch := wire.Telemetry{BatchID: "identity-batch", NodeID: "demo-node-1"}
	for _, id := range []string{"1", "alex", "alex@example.invalid", "demo-alex", "maria"} {
		batch.Events = append(batch.Events, wire.Activity{UserID: id, Timestamp: time.Now(), DstHost: "identity.example.invalid", DstPort: 443})
	}
	if w := perform(s, "POST", "/api/v1/agents/telemetry", batch, nil, credential, false); w.Code != 200 {
		t.Fatalf("ingest: %d %s", w.Code, w.Body)
	}
	for _, selector := range []string{"1", "alex", "alex@example.invalid", "demo-alex"} {
		w := perform(s, "GET", "/api/v1/activity?user="+url.QueryEscape(selector), nil, session, "", false)
		var result struct {
			Data []activityRow `json:"data"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || len(result.Data) != 4 {
			t.Fatalf("selector %q: %d %s", selector, w.Code, w.Body)
		}
		for _, event := range result.Data {
			if event.UserID == "maria" {
				t.Fatal("another user's activity leaked into filter")
			}
		}
	}
	w := perform(s, "GET", "/api/v1/activity/export?user=1", nil, session, "", false)
	var exported []activityRow
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &exported) != nil || len(exported) != 4 {
		t.Fatalf("export: %d %s", w.Code, w.Body)
	}
	if w := perform(s, "DELETE", "/api/v1/activity/1", nil, session, "", true); w.Code != 200 {
		t.Fatalf("purge: %d %s", w.Code, w.Body)
	}
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM sg_activity WHERE user_id='maria'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("purge removed another user: %d %v", count, err)
	}
	if err := s.db.QueryRow("SELECT COUNT(*) FROM sg_activity").Scan(&count); err != nil || count != 1 {
		t.Fatalf("purge left aliases: %d %v", count, err)
	}
}

func TestActivityIdentitiesDoNotExpandAmbiguousAliases(t *testing.T) {
	s := newTestServer(t)
	if err := s.saveObject("users", "other", panel.User{ID: "other", Username: "alex"}); err != nil {
		t.Fatal(err)
	}
	ids, err := s.activityIdentities("1")
	if err != nil || slices.Contains(ids, "alex") || !slices.Contains(ids, "alex@example.invalid") {
		t.Fatalf("ambiguous alias included: %v %v", ids, err)
	}
	ids, err = s.activityIdentities("alex")
	if err != nil || !slices.Equal(ids, []string{"alex"}) {
		t.Fatalf("ambiguous selector expanded: %v %v", ids, err)
	}
	ids, err = s.activityIdentities("unsynchronized-xray-email")
	if err != nil || !slices.Equal(ids, []string{"unsynchronized-xray-email"}) {
		t.Fatalf("raw unknown identity lost: %v %v", ids, err)
	}
}
