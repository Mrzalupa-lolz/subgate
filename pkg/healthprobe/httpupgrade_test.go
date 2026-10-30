package healthprobe

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPUpgradeProbeRequiresUpgrade(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		success        bool
	}{
		{"accepted", "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n", true},
		{"plain-http", "HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n", false},
		{"redirect", "HTTP/1.1 302 Found\r\nLocation: http://127.0.0.1:1/\r\nContent-Length: 0\r\n\r\n", false},
		{"bare101", "HTTP/1.1 101 Switching Protocols\r\n\r\n", false},
		{"wrong-upgrade", "HTTP/1.1 101 Switching Protocols\r\nUpgrade: h2c\r\nConnection: Upgrade\r\n\r\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					return
				}
				defer conn.Close()
				fmt.Fprint(conn, tc.response)
			}))
			defer srv.Close()
			target := httpTarget(t, srv.URL)
			target.Inbounds[0].Network = "httpupgrade"
			r := Run(context.Background(), target, time.Second)
			if r.Verified != "httpupgrade" || r.Success != tc.success {
				t.Fatal(r)
			}
		})
	}
}
