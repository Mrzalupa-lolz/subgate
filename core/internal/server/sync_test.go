package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/subgate/subgate/core/internal/panel"
)

func TestPanelSyncCommitsCompleteSnapshotAndPreservesLastGood(t *testing.T) {
	s := newTestServer(t)
	var failPage atomic.Bool
	var requests atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/nodes":
			io.WriteString(w, `{"response":[{"uuid":"real-node","name":"Real","address":"192.0.2.1","isConnected":true}]}`)
		case "/api/hosts":
			io.WriteString(w, `{"response":[]}`)
		case "/api/users":
			requests.Add(1)
			offset, err := strconv.Atoi(r.URL.Query().Get("start"))
			if err != nil {
				t.Error(err)
			}
			if offset == 0 {
				io.WriteString(w, `{"response":{"total":2,"users":[{"uuid":"real-user-1","shortUuid":"short1","username":"one","status":"ACTIVE","trafficLimitBytes":0,"expireAt":"2027-01-01T00:00:00Z","userTraffic":{"usedTrafficBytes":0}}]}}`)
			} else if offset == 1 && !failPage.Load() {
				io.WriteString(w, `{"response":{"total":2,"users":[{"uuid":"real-user-2","shortUuid":"short2","username":"two","status":"ACTIVE","trafficLimitBytes":0,"expireAt":"2027-01-01T00:00:00Z","userTraffic":{"usedTrafficBytes":0}}]}}`)
			} else {
				w.WriteHeader(503)
			}
		default:
			t.Errorf("unexpected sync request %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer upstream.Close()
	s.config.Demo = false
	s.panel.Close()
	s.panel = panel.NewClient(upstream.URL, "panel-key", s.logger)
	if err := s.syncPanel(context.Background()); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 {
		t.Fatalf("did not paginate actual returned count: %d", requests.Load())
	}
	users, err := s.objects("users")
	if err != nil || len(users) != 2 {
		t.Fatalf("snapshot users: %v %v", users, err)
	}
	nodes, err := s.objects("nodes")
	if err != nil || len(nodes) != 1 {
		t.Fatalf("snapshot nodes: %v %v", nodes, err)
	}
	before := s.lastSync
	failPage.Store(true)
	if err := s.syncPanel(context.Background()); err == nil {
		t.Fatal("failed panel page accepted")
	}
	users, err = s.objects("users")
	if err != nil || len(users) != 2 {
		t.Fatalf("failed pagination erased last good: %v %v", users, err)
	}
	if !s.lastSync.Equal(before) || s.syncError == "" {
		t.Fatalf("sync state wrong: before=%v after=%v error=%s", before, s.lastSync, s.syncError)
	}
}

func TestPanelSyncRejectsOversizedTotalWithoutErasingSnapshot(t *testing.T) {
	s := newTestServer(t)
	before, err := s.objects("users")
	if err != nil || len(before) == 0 {
		t.Fatal("demo snapshot missing", err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/nodes" {
			io.WriteString(w, `{"response":[]}`)
			return
		}
		io.WriteString(w, `{"response":{"total":1000001,"users":[]}}`)
	}))
	defer upstream.Close()
	s.config.Demo = false
	s.panel.Close()
	s.panel = panel.NewClient(upstream.URL, "test", s.logger)
	if err := s.syncPanel(context.Background()); err == nil {
		t.Fatal("oversized incomplete snapshot accepted")
	}
	after, err := s.objects("users")
	if err != nil || len(after) != len(before) {
		t.Fatal("invalid pagination erased snapshot", err)
	}
}
