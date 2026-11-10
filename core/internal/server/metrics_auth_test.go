package server

import (
	"strings"
	"testing"
)

func TestMetricsRequireOwnerOrScrapeToken(t *testing.T) {
	s := newTestServer(t)
	if w := perform(s, "GET", "/metrics", nil, nil, "", false); w.Code != 401 {
		t.Fatalf("anonymous metrics: %d", w.Code)
	}
	if w := perform(s, "GET", "/metrics", nil, loginTest(t, s), "", false); w.Code != 200 {
		t.Fatalf("owner metrics: %d", w.Code)
	}
	s.config.HTTP.MetricsToken = "scrape-token-for-tests"
	if w := perform(s, "GET", "/metrics", nil, nil, "scrape-token-for-tests", false); w.Code != 200 {
		t.Fatalf("token metrics: %d", w.Code)
	}
	if w := perform(s, "GET", "/metrics", nil, nil, "wrong-token", false); w.Code != 401 {
		t.Fatalf("wrong token metrics: %d", w.Code)
	}
}

func TestEnrollmentIsRateLimitedPerClient(t *testing.T) {
	s := newTestServer(t)
	body := map[string]string{"node_id": "demo-node-1", "token": "invalid-enrollment-token"}
	for i := 0; i < enrollAttemptsPerMinute; i++ {
		if w := perform(s, "POST", "/api/v1/agents/enroll", body, nil, "", false); w.Code == 429 {
			t.Fatalf("limited too early at attempt %d", i+1)
		}
	}
	if w := perform(s, "POST", "/api/v1/agents/enroll", body, nil, "", false); w.Code != 429 {
		t.Fatalf("enrollment flood not limited: %d", w.Code)
	}
}

func TestCoreServesAgentInstaller(t *testing.T) {
	s := newTestServer(t)
	w := perform(s, "GET", "/install-agent.sh", nil, nil, "", false)
	if w.Code != 200 || !strings.HasPrefix(w.Body.String(), "#!") || !strings.Contains(w.Body.String(), "--enrollment-token") {
		t.Fatalf("installer not served: %d %.80q", w.Code, w.Body.String())
	}
}
