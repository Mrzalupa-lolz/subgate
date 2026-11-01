package server

import (
	"net/http"
	"testing"
)

func TestMiddlewareOwnedAPIsAreInactive(t *testing.T) {
	s := newTestServer(t)
	session := loginTest(t, s)
	for _, endpoint := range []struct{ method, path string }{
		{"GET", "/api/v1/injections"},
		{"GET", "/sub/synthetic"}, {"HEAD", "/sub/synthetic/client"},
		{"GET", "/synthetic"}, {"GET", "/synthetic/client"},
		{"GET", "/api/v1/request-signals"}, {"GET", "/api/v1/tiers"},
		{"GET", "/api/v1/policies"}, {"GET", "/api/v1/pins"},
		{"GET", "/api/v1/injections/legacy"},
		{"PUT", "/api/v1/injections/legacy"},
		{"DELETE", "/api/v1/injections/legacy"},
		{"POST", "/api/v1/users/1/actions/disable"},
		{"POST", "/api/v1/users/1/actions/enable"},
		{"POST", "/api/v1/users/1/actions/reset-traffic"},
	} {
		if w := perform(s, endpoint.method, endpoint.path, nil, session, "", true); w.Code != http.StatusNotFound {
			t.Errorf("archived route %s %s returned %d", endpoint.method, endpoint.path, w.Code)
		}
	}
	for _, path := range []string{"/api/v1/users", "/api/v1/users/1", "/api/v1/quotas"} {
		if w := perform(s, "GET", path, nil, session, "", false); w.Code != http.StatusOK {
			t.Errorf("retained route %s returned %d", path, w.Code)
		}
	}
}
