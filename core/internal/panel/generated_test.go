package panel

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestGeneratedPanelUsersContract(t *testing.T) {
	fixture, err := os.ReadFile("testdata/users-3.4.5.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		body      []byte
		wantError bool
	}{
		{"schema numeric ID", fixture, false},
		{"malformed ID must reject", bytes.Replace(fixture, []byte(`"id": 42`), []byte(`"id": {"opaque":true}`), 1), true},
		{"counter must not wrap", bytes.Replace(fixture, []byte(`"trafficLimitBytes": 4096`), []byte(`"trafficLimitBytes": 1e20`), 1), true},
		{"fractional bytes must reject", bytes.Replace(fixture, []byte(`"usedTrafficBytes": 512`), []byte(`"usedTrafficBytes": 1.5`), 1), true},
		{"unknown status stays readable", fixture, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.Path != "/api/users" || r.URL.Query().Get("size") != "10" {
					t.Error("incorrect generated read request")
				}
				w.Write(tc.body)
			}))
			defer u.Close()
			c := NewClient(u.URL, "synthetic-token", slog.New(slog.NewTextHandler(io.Discard, nil)))
			defer c.Close()
			users, total, err := c.ListUsers(0, 10)
			if tc.wantError {
				if err == nil {
					t.Fatal("schema-invalid identity accepted")
				}
				return
			}
			if err != nil || total != 1 || len(users) != 1 || users[0].ID != "42" || users[0].Status != "future_status" {
				t.Fatalf("generated contract result: %+v total=%d err=%v", users, total, err)
			}
		})
	}
}

func TestGeneratedPanelUsersIntegrity(t *testing.T) {
	fixture, err := os.ReadFile("testdata/users-3.4.5.json")
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Response struct {
			Users []json.RawMessage `json:"users"`
		} `json:"response"`
	}
	if err := json.Unmarshal(fixture, &envelope); err != nil {
		t.Fatal(err)
	}
	original := envelope.Response.Users[0]
	for _, tc := range []struct {
		name                string
		change              func(map[string]json.RawMessage)
		wantError           bool
		wantID              string
		wantLimit, wantUsed int64
	}{
		{"complete UUID identity", func(m map[string]json.RawMessage) { m["uuid"] = json.RawMessage(`"legacy-id"`) }, false, "legacy-id", 4096, 512},
		{"sparse UUID identity", func(m map[string]json.RawMessage) { m["uuid"] = json.RawMessage(`"legacy-id"`); delete(m, "createdAt") }, false, "legacy-id", 4096, 512},
		{"exact complete large counters", func(m map[string]json.RawMessage) {
			m["trafficLimitBytes"] = json.RawMessage(`9007199254740993`)
			m["userTraffic"] = bytes.Replace(m["userTraffic"], []byte(`512`), []byte(`9007199254740993`), 1)
		}, false, "42", 9007199254740993, 9007199254740993},
		{"exact large counters", func(m map[string]json.RawMessage) {
			m["trafficLimitBytes"] = json.RawMessage(`9007199254740993`)
			m["userTraffic"] = json.RawMessage(`{"usedTrafficBytes":9007199254740993}`)
		}, false, "42", 9007199254740993, 9007199254740993},
		{"malformed date after sparse traffic", func(m map[string]json.RawMessage) {
			m["userTraffic"] = json.RawMessage(`{"usedTrafficBytes":512}`)
			m["expireAt"] = json.RawMessage(`"invalid"`)
		}, true, "", 0, 0},
		{"malformed UUID after sparse traffic", func(m map[string]json.RawMessage) {
			m["userTraffic"] = json.RawMessage(`{"usedTrafficBytes":512}`)
			m["vlessUuid"] = json.RawMessage(`"invalid"`)
		}, true, "", 0, 0},
		{"missing canonical identity", func(m map[string]json.RawMessage) { delete(m, "id") }, true, "", 0, 0},
		{"null traffic limit", func(m map[string]json.RawMessage) { m["trafficLimitBytes"] = json.RawMessage(`null`) }, true, "", 0, 0},
		{"missing traffic limit", func(m map[string]json.RawMessage) { delete(m, "trafficLimitBytes") }, true, "", 0, 0},
		{"missing traffic object", func(m map[string]json.RawMessage) { delete(m, "userTraffic") }, true, "", 0, 0},
		{"missing used counter", func(m map[string]json.RawMessage) { m["userTraffic"] = json.RawMessage(`{}`) }, true, "", 0, 0},
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
			// Put the sparse nested object first to reproduce early decoder errors.
			if traffic, ok := fields["userTraffic"]; ok {
				delete(fields, "userTraffic")
				rest, err := json.Marshal(fields)
				if err != nil {
					t.Fatal(err)
				}
				raw = append(append([]byte(`{"userTraffic":`), traffic...), append([]byte(","), rest[1:]...)...)
			}
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write(append(append([]byte(`{"response":{"total":1,"users":[`), raw...), []byte(`]}}`)...))
			}))
			defer upstream.Close()
			c := NewClient(upstream.URL, "synthetic-token", slog.New(slog.NewTextHandler(io.Discard, nil)))
			defer c.Close()
			users, _, err := c.ListUsers(0, 10)
			if tc.wantError {
				if err == nil {
					t.Fatal("invalid response accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(users) != 1 || users[0].ID != tc.wantID || users[0].TrafficLimitBytes != tc.wantLimit || users[0].UsedTrafficBytes != tc.wantUsed {
				t.Fatalf("identity or exact counters changed: %+v", users)
			}
		})
	}
}
