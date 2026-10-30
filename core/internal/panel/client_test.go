package panel

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRemnawaveV3Contract(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer panel-secret" {
			t.Error("missing panel authorization")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/users":
			if r.URL.Query().Get("start") != "100" || r.URL.Query().Get("size") != "50" {
				t.Errorf("incorrect pagination: %s", r.URL.RawQuery)
			}
			io.WriteString(w, `{"response":{"total":101,"users":[{"uuid":"user-uuid","id":42,"shortUuid":"short123","username":"alice","email":"alice@example.com","status":"ACTIVE","trafficLimitBytes":9999,"expireAt":"2027-01-01T00:00:00Z","userTraffic":{"usedTrafficBytes":1234}}]}}`)
		case "/api/users/by-short-uuid/short123", "/api/users/user-uuid":
			io.WriteString(w, `{"response":{"uuid":"user-uuid","shortUuid":"short123","username":"alice","status":"DISABLED","userTraffic":{"usedTrafficBytes":25}}}`)
		case "/api/nodes":
			io.WriteString(w, `{"response":[{"uuid":"node-uuid","name":"edge","address":"192.0.2.10","isConnected":true,"isDisabled":false,"countryCode":"FI","tags":["edge"],"usersOnline":7,"trafficUsedBytes":888,"configProfile":{"activeInbounds":[{"tag":"reality","type":"vless","port":443}]}},{"id":17,"name":"disabled","isConnected":true,"isDisabled":true}]}`)
		case "/api/nodes/node-uuid/actions/restart":
			if r.Method != http.MethodPost {
				t.Error("action must be POST")
			}
			io.WriteString(w, `{"response":{"ok":true}}`)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer upstream.Close()
	c := NewClient(upstream.URL, "panel-secret", slog.New(slog.NewTextHandler(io.Discard, nil)), WithWriteAccess())
	defer c.Close()
	users, total, err := c.ListUsersContext(context.Background(), 100, 50)
	if err != nil {
		t.Fatal(err)
	}
	if total != 101 || len(users) != 1 {
		t.Fatalf("pagination result: total=%d users=%v", total, users)
	}
	u := users[0]
	if u.ID != "user-uuid" || u.ShortUUID != "short123" || u.Status != "active" || u.TrafficLimitBytes != 9999 || u.UsedTrafficBytes != 1234 {
		t.Fatalf("camelCase user decode: %+v", u)
	}
	for _, fetch := range []func() (*User, error){func() (*User, error) { return c.GetUser("user-uuid") }, func() (*User, error) { return c.GetUserByShortUUID("short123") }} {
		u, err := fetch()
		if err != nil || u.Status != "disabled" || u.UsedTrafficBytes != 25 {
			t.Fatalf("single user: %+v %v", u, err)
		}
	}
	nodes, err := c.ListNodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 || nodes[0].ID != "node-uuid" || nodes[0].IPAddress != "192.0.2.10" || nodes[0].Status != "online" || nodes[0].Country != "FI" || nodes[0].UsersOnline != 7 || len(nodes[0].Inbounds) != 1 || nodes[1].ID != "17" || nodes[1].Status != "disabled" {
		t.Fatalf("camelCase nodes decode: %+v", nodes)
	}
	if err := c.Action(context.Background(), "nodes", "node-uuid", "restart"); err != nil {
		t.Fatal(err)
	}
}

func TestPanelRejectsInvalidContractAndStatus(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"unenveloped", `{"users":[],"total":0}`, 200},
		{"null envelope", `{"response":null}`, 200},
		{"missing users", `{"response":{"total":3}}`, 200},
		{"missing identity", `{"response":{"users":[{"uuid":"id"}],"total":1}}`, 200},
		{"invalid JSON", `{"response":`, 200},
		{"forbidden", `secret upstream diagnostics`, 403},
		{"redirect", ``, 302},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); io.WriteString(w, tc.body) }))
			defer u.Close()
			c := NewClient(u.URL, "", slog.Default())
			defer c.Close()
			_, _, err := c.ListUsers(0, 10)
			if err == nil {
				t.Fatal("invalid response accepted")
			}
			if tc.status != 200 {
				var api *APIError
				if !errors.As(err, &api) || api.Status != tc.status {
					t.Fatalf("status lost: %v", err)
				}
			}
		})
	}
}
