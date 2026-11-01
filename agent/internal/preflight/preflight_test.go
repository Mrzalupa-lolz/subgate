package preflight

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestPreflight(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	stats := grpc.NewServer()
	stats.RegisterService(&grpc.ServiceDesc{
		ServiceName: "xray.app.stats.command.StatsService",
		HandlerType: (*interface{})(nil),
		Methods: []grpc.MethodDesc{{MethodName: "QueryStats", Handler: func(_ any, _ context.Context, decode func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
			request := new(emptypb.Empty)
			if err := decode(request); err != nil {
				return nil, err
			}
			return new(emptypb.Empty), nil
		}}},
	}, struct{}{})
	go stats.Serve(listener)
	defer stats.Stop()
	dir := t.TempDir()
	log := filepath.Join(dir, "access.log")
	os.WriteFile(log, nil, 0600)
	token := filepath.Join(dir, "token")
	for _, tc := range []struct {
		name, body, log, token, want string
		status                       int
	}{
		{"ready", "{\"database\":true}", log, token, "", 200},
		{"database down", "{\"database\":false}", log, token, "Core /health", 200},
		{"proxy error", "<html>not Core</html>", log, token, "Core /health", 200},
		{"redirect", "", log, token, "Core /health", 302},
		{"unreadable log", "{\"database\":true}", log + ".missing", token, "access.log", 200},
		{"log directory", "{\"database\":true}", dir, token, "regular file", 200},
		{"unwritable volume", "{\"database\":true}", log, filepath.Join(log, "token"), "not writable", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/health" || r.Method != "GET" {
					t.Error("unexpected request")
				}
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}))
			defer server.Close()
			err := Check(server.URL, listener.Addr().String(), tc.log, tc.token, slog.Default())
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %s", err, tc.want)
			}
			if _, err := os.Stat(token); !os.IsNotExist(err) {
				t.Fatal("preflight wrote a durable token")
			}
		})
	}
}
