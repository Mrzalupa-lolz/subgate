package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// fixtureServer serves synthetic responses shaped like Remnawave 3.4.8.
func fixtureServer(t *testing.T, users []byte) *httptest.Server {
	t.Helper()
	nodes, err := os.ReadFile("testdata/nodes-3.4.8.json")
	if err != nil {
		t.Fatal(err)
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method %s", r.Method)
		}
		switch r.URL.Path {
		case "/api/users":
			w.Write(users)
		case "/api/nodes":
			w.Write(nodes)
		case "/api/system/stats/recap":
			io.WriteString(w, `{"response":{"version":"3.4.8","thisMonth":{"users":4,"traffic":"1"},"total":{"users":1,"nodes":1,"traffic":"1"}}}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
}

func fixtureUsers(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/users-3.4.8.json")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestPanelFixtureReadContract(t *testing.T) {
	u := fixtureServer(t, fixtureUsers(t))
	defer u.Close()
	c := NewClient(u.URL, "synthetic-token", slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer c.Close()
	version, err := c.Version(context.Background())
	if err != nil || version != "3.4.8" || !VersionCompatible(version) {
		t.Fatalf("version contract: %q %v", version, err)
	}
	users, total, err := c.ListUsers(0, 10)
	if err != nil || total != 1 || len(users) != 1 {
		t.Fatalf("users contract: %+v total=%d err=%v", users, total, err)
	}
	if got := users[0]; got.ID != "42" || got.ShortUUID != "SyntheticShort42" || got.Username != "synthetic-user" || got.Status != "active" || got.Email != "" || got.TrafficLimitBytes != 4096 || got.UsedTrafficBytes != 512 {
		t.Fatalf("user projection: %+v", got)
	}
	nodes, err := c.ListNodes()
	if err != nil || len(nodes) != 2 {
		t.Fatalf("nodes contract: %+v %v", nodes, err)
	}
	if n := nodes[0]; n.ID != "00000000-0000-4000-8000-000000000001" || n.Status != "online" || n.Country != "NL" || n.UsersOnline != 19 || len(n.Inbounds) != 3 || n.Inbounds[0].Network != "raw" {
		t.Fatalf("node projection: %+v", n)
	}
	if nodes[1].Status != "disabled" || len(nodes[1].Inbounds) != 0 {
		t.Fatalf("disabled node projection: %+v", nodes[1])
	}
	stored, _ := json.Marshal(nodes)
	if bytes.Contains(stored, []byte("synthetic-private-key")) || bytes.Contains(stored, []byte("rawInbound")) {
		t.Fatal("inbound secrets must not be retained")
	}
}

func TestPanelUserValidation(t *testing.T) {
	var envelope struct {
		Response struct {
			Users []json.RawMessage `json:"users"`
		} `json:"response"`
	}
	if err := json.Unmarshal(fixtureUsers(t), &envelope); err != nil {
		t.Fatal(err)
	}
	original := envelope.Response.Users[0]
	for _, tc := range []struct {
		name                string
		change              func(map[string]json.RawMessage)
		wantError           bool
		wantID, wantStatus  string
		wantLimit, wantUsed int64
	}{
		{"numeric id", func(map[string]json.RawMessage) {}, false, "42", "active", 4096, 512},
		{"unknown status stays readable", func(m map[string]json.RawMessage) { m["status"] = json.RawMessage(`"FUTURE_STATUS"`) }, false, "42", "future_status", 4096, 512},
		{"legacy uuid identity preferred", func(m map[string]json.RawMessage) {
			m["uuid"] = json.RawMessage(`"00000000-0000-4000-8000-0000000000ff"`)
		}, false, "00000000-0000-4000-8000-0000000000ff", "active", 4096, 512},
		{"legacy uuid without numeric id", func(m map[string]json.RawMessage) {
			m["uuid"] = json.RawMessage(`"00000000-0000-4000-8000-0000000000ff"`)
			delete(m, "id")
		}, false, "00000000-0000-4000-8000-0000000000ff", "active", 4096, 512},
		{"exact large counters", func(m map[string]json.RawMessage) {
			m["trafficLimitBytes"] = json.RawMessage(`9007199254740993`)
			m["userTraffic"] = json.RawMessage(`{"usedTrafficBytes":9007199254740993}`)
		}, false, "42", "active", 9007199254740993, 9007199254740993},
		{"object id", func(m map[string]json.RawMessage) { m["id"] = json.RawMessage(`{"opaque":true}`) }, true, "", "", 0, 0},
		{"fractional id", func(m map[string]json.RawMessage) { m["id"] = json.RawMessage(`4.2`) }, true, "", "", 0, 0},
		{"missing identity", func(m map[string]json.RawMessage) { delete(m, "id") }, true, "", "", 0, 0},
		{"malformed expiry", func(m map[string]json.RawMessage) { m["expireAt"] = json.RawMessage(`"invalid"`) }, true, "", "", 0, 0},
		{"counter overflow", func(m map[string]json.RawMessage) { m["trafficLimitBytes"] = json.RawMessage(`1e20`) }, true, "", "", 0, 0},
		{"fractional counter", func(m map[string]json.RawMessage) { m["userTraffic"] = json.RawMessage(`{"usedTrafficBytes":1.5}`) }, true, "", "", 0, 0},
		{"negative counter", func(m map[string]json.RawMessage) { m["trafficLimitBytes"] = json.RawMessage(`-1`) }, true, "", "", 0, 0},
		{"null traffic limit", func(m map[string]json.RawMessage) { m["trafficLimitBytes"] = json.RawMessage(`null`) }, true, "", "", 0, 0},
		{"missing traffic object", func(m map[string]json.RawMessage) { delete(m, "userTraffic") }, true, "", "", 0, 0},
		{"missing used counter", func(m map[string]json.RawMessage) { m["userTraffic"] = json.RawMessage(`{}`) }, true, "", "", 0, 0},
		{"missing short uuid", func(m map[string]json.RawMessage) { delete(m, "shortUuid") }, true, "", "", 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(original, &fields); err != nil {
				t.Fatal(err)
			}
			tc.change(fields)
			raw, err := json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			u := fixtureServer(t, append(append([]byte(`{"response":{"total":1,"users":[`), raw...), []byte(`]}}`)...))
			defer u.Close()
			c := NewClient(u.URL, "synthetic-token", slog.New(slog.NewTextHandler(io.Discard, nil)))
			defer c.Close()
			users, _, err := c.ListUsers(0, 10)
			if tc.wantError {
				if err == nil {
					t.Fatalf("invalid user accepted: %+v", users)
				}
				return
			}
			if err != nil || len(users) != 1 {
				t.Fatalf("user rejected: %v", err)
			}
			if got := users[0]; got.ID != tc.wantID || got.Status != tc.wantStatus || got.TrafficLimitBytes != tc.wantLimit || got.UsedTrafficBytes != tc.wantUsed {
				t.Fatalf("user projection: %+v", got)
			}
		})
	}
}

func TestPanelVersionSeries(t *testing.T) {
	for version, want := range map[string]bool{"3.4.8": true, "3.4.0": true, "3.4.12-beta.1": true, "3.5.0": false, "4.4.8": false, "": false, "do-not-log": false} {
		if got := VersionCompatible(version); got != want {
			t.Errorf("VersionCompatible(%q)=%v, want %v", version, got, want)
		}
	}
}
