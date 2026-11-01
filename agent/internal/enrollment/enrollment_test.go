package enrollment

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestEnrollToFile(t *testing.T) {
	for _, success := range []bool{false, true} {
		t.Run(map[bool]string{false: "failure preserves credential", true: "atomic replacement"}[success], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !success {
					w.WriteHeader(401)
					return
				}
				w.WriteHeader(201)
				w.Write([]byte("{\"token\":\"new-secret\",\"node_id\":\"test-node\"}"))
			}))
			defer server.Close()
			path := filepath.Join(t.TempDir(), "token")
			if err := os.WriteFile(path, []byte("old-secret"), 0600); err != nil {
				t.Fatal(err)
			}
			err := EnrollToFile(server.URL, "one-time", "test-node", path)
			if (err == nil) != success {
				t.Fatalf("success=%v: %v", success, err)
			}
			raw, _ := os.ReadFile(path)
			if string(raw) != map[bool]string{true: "new-secret", false: "old-secret"}[success] {
				t.Fatal("credential corrupted")
			}
			info, _ := os.Stat(path)
			if info.Mode().Perm() != 0600 {
				t.Fatal("credential permissions changed")
			}
			files, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".enroll-*"))
			if len(files) != 0 {
				t.Fatal("temporary credential leaked")
			}
		})
	}
}

func TestUnwritableDestinationDoesNotConsumeToken(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer server.Close()
	parent := filepath.Join(t.TempDir(), "not-a-directory")
	os.WriteFile(parent, nil, 0600)
	if EnrollToFile(server.URL, "one-time", "node", filepath.Join(parent, "token")) == nil || called {
		t.Fatal("contacted Core before checking credential storage")
	}
}
