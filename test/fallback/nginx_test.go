package fallback

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const nginxImage = "nginx@sha256:30f1c0d78e0ad60901648be663a710bdadf19e4c10ac6782c235200619158284"

func TestKilledCoreSubscriptionFallback(t *testing.T) {
	if os.Getenv("SUBGATE_RUN_NGINX_FALLBACK") != "1" {
		t.Skip("opt-in Docker nginx/process integration")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := os.MkdirTemp(filepath.Join(root, "tmp"), "r07-fallback-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspace)
	payload := []byte("synthetic working subscription\n")
	received := make(chan string, 64)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/system/stats/recap" {
			io.WriteString(w, `{"response":{"version":"3.4.5"}}`)
			return
		}
		if r.URL.Path == "/api/nodes" || r.URL.Path == "/api/hosts" {
			io.WriteString(w, `{"response":[]}`)
			return
		}
		if r.URL.Path == "/api/users" {
			io.WriteString(w, `{"response":{"users":[],"total":0}}`)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/sub/") {
			t.Error("fallback escaped subscription endpoint")
			w.WriteHeader(404)
			return
		}
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("Proxy-Authorization") != "" || r.Header.Get("Proxy-Connection") != "" {
			t.Error("fallback forwarded client credentials")
		}
		received <- r.Method + " " + r.URL.RequestURI()
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Announce", "synthetic")
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		if r.Method != "HEAD" {
			w.Write(payload)
		}
	}))
	defer origin.Close()
	coreAddress, proxyAddress := freeAddress(t), freeAddress(t)
	cfg := map[string]any{
		"http":     map[string]any{"listen": coreAddress},
		"database": map[string]any{"url": "file:" + filepath.Join(workspace, "synthetic.db"), "backup_path": filepath.Join(workspace, "backups")},
		"panel":    map[string]any{"url": origin.URL, "api_token": "synthetic-test-token", "subscription_url": origin.URL + "/api/sub"},
		"auth":     map[string]any{"admin_password": "synthetic-strong-password"},
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(workspace, "core.json")
	if err = os.WriteFile(configPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	core := exec.Command(filepath.Join(root, "bin/subgate-core"), "-config", configPath)
	core.Dir = workspace
	core.Stdout = io.Discard
	core.Stderr = io.Discard
	if err = core.Start(); err != nil {
		t.Fatal("build Core before running integration")
	}
	coreStopped := false
	defer func() {
		if !coreStopped {
			core.Process.Kill()
			core.Wait()
		}
	}()
	client := &http.Client{Timeout: 2 * time.Second}
	waitHTTP(t, client, "http://"+coreAddress+"/health")
	template, err := os.ReadFile(filepath.Join(root, "deploy/nginx.conf.example"))
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, line := range strings.Split(string(template), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "ssl_certificate") || strings.HasPrefix(trimmed, "listen [::]") {
			continue
		}
		line = strings.ReplaceAll(line, "listen 443 ssl;", "listen "+proxyAddress+";")
		line = strings.ReplaceAll(line, "127.0.0.1:8080", coreAddress)
		line = strings.ReplaceAll(line, "https://subscriptions.example.com", origin.URL)
		lines = append(lines, line)
	}
	nginxConfig := "pid /tmp/nginx.pid;\nerror_log /dev/stderr crit;\nevents {}\nhttp { access_log off; proxy_temp_path /tmp/proxy; client_body_temp_path /tmp/client; fastcgi_temp_path /tmp/fastcgi; uwsgi_temp_path /tmp/uwsgi; scgi_temp_path /tmp/scgi;\n" + strings.Join(lines, "\n") + "\n}\n"
	nginxPath := filepath.Join(workspace, "nginx.conf")
	if err = os.WriteFile(nginxPath, []byte(nginxConfig), 0644); err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("subgate-r07-%d-%d", os.Getpid(), time.Now().UnixNano())
	args := []string{"run", "--rm", "--name", name, "--network", "host", "--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()), "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--pids-limit", "64", "--tmpfs", "/tmp:rw,nosuid,size=16m", "--mount", "type=bind,source=" + nginxPath + ",target=/etc/nginx/nginx.conf,readonly", "--entrypoint", "nginx", nginxImage, "-g", "daemon off;"}
	proxy := exec.Command("docker", args...)
	proxy.Stdout = io.Discard
	proxy.Stderr = io.Discard
	if err = proxy.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		exec.CommandContext(ctx, "docker", "rm", "-f", name).Run()
		proxy.Wait()
	}()
	waitHTTP(t, client, "http://"+proxyAddress+"/health")
	request := func(method, path string, credentials bool) *http.Response {
		t.Helper()
		req, err := http.NewRequest(method, "http://"+proxyAddress+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("User-Agent", "SyntheticClient/1")
		req.Header.Set("Accept-Language", "ru")
		req.Header.Set("X-Device-Test", "synthetic")
		if credentials {
			req.Header.Set("Authorization", "Bearer synthetic-client-token")
			req.Header.Set("Cookie", "synthetic=value")
			req.Header.Set("Proxy-Authorization", "Basic synthetic-proxy-credential")
			req.Header.Set("Proxy-Connection", "keep-alive")
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	assertSubscription := func(method string) {
		t.Helper()
		resp := request(method, "/sub/synthetic123/mihomo?flag=1&flag=2", true)
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		expected := payload
		if method == "HEAD" {
			expected = nil
		}
		if err != nil || resp.StatusCode != 200 || !bytes.Equal(data, expected) || resp.Header.Get("Announce") != "synthetic" {
			t.Fatalf("%s subscription failed status=%d", method, resp.StatusCode)
		}
		select {
		case got := <-received:
			if got != method+" /api/sub/synthetic123/mihomo?flag=1&flag=2" {
				t.Fatalf("method/path/query changed: %s", got)
			}
		case <-time.After(time.Second):
			t.Fatal("origin not reached")
		}
	}
	assertSubscription("GET")
	if err = core.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	core.Wait()
	coreStopped = true
	assertSubscription("GET")
	assertSubscription("HEAD")
	resp := request("POST", "/sub/synthetic123/mihomo", true)
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("non-read subscription method accepted: %d", resp.StatusCode)
	}
	select {
	case <-received:
		t.Fatal("non-read method reached subscription origin")
	default:
	}
	resp = request("GET", "/api/v1/system/status", false)
	resp.Body.Close()
	if resp.StatusCode != 502 {
		t.Fatalf("console unexpectedly uses subscription fallback: %d", resp.StatusCode)
	}
}

func freeAddress(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := l.Addr().String()
	l.Close()
	return address
}
func waitHTTP(t *testing.T, c *http.Client, address string) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		r, err := c.Get(address)
		if err == nil {
			r.Body.Close()
			if r.StatusCode == 200 {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("isolated service did not become ready")
}
