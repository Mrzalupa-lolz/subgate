package panel

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// Fixtures are synthetic examples of the required response properties in the
// checked-in 3.4.5 specification, including nullable inbound fields.
func TestPanelSpecReadContract(t *testing.T) {
	specBytes, err := os.ReadFile("../../../remnawave-openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Info  struct{ Version string }
		Paths map[string]map[string]json.RawMessage
	}
	if err = json.Unmarshal(specBytes, &spec); err != nil {
		t.Fatal(err)
	}
	if spec.Info.Version != "3.4.5" {
		t.Fatal("fixtures require panel 3.4.5")
	}
	u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || spec.Paths[r.URL.Path]["get"] == nil {
			t.Error("read endpoint absent from specification")
			w.WriteHeader(404)
			return
		}
		var file string
		switch r.URL.Path {
		case "/api/users":
			file = "users"
		case "/api/nodes":
			file = "nodes"
		case "/api/hosts":
			file = "hosts"
		default:
			t.Error("unexpected endpoint")
			w.WriteHeader(404)
			return
		}
		b, err := os.ReadFile("testdata/" + file + "-3.4.5.json")
		if err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		w.Write(b)
	}))
	defer u.Close()
	c := NewClient(u.URL, "synthetic-token", slog.Default())
	defer c.Close()
	users, total, err := c.ListUsers(0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(users) != 1 || users[0].ID != "42" || users[0].Email != "" || users[0].Status != "future_status" || users[0].UsedTrafficBytes != 512 || users[0].TrafficLimitBytes != 4096 {
		t.Fatalf("numeric identity/null/unknown-enum user decode failed: %+v total=%d", users, total)
	}
	nodes, err := c.ListNodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || nodes[0].Status != "online" || nodes[0].UsersOnline != 1 || nodes[0].TrafficUsedBytes != 0 || len(nodes[0].Inbounds) != 1 {
		t.Fatalf("null node fields failed: %+v", nodes)
	}
	ib := nodes[0].Inbounds[0]
	if ib.Type != "future-protocol" || ib.Tag != "synthetic-inbound" || ib.Network != "" || ib.Security != "" || ib.Port != 0 {
		t.Fatalf("nullable object activeInbounds failed: %+v", ib)
	}
	hosts, err := c.ListHostsContext(context.Background())
	if err != nil || len(hosts) != 1 || hosts[0].Address != "192.0.2.10" || hosts[0].Port != 443 || hosts[0].Disabled || len(hosts[0].Nodes) != 0 {
		t.Fatalf("host read contract failed: %+v %v", hosts, err)
	}
}
