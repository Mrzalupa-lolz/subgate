package server

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/subgate/subgate/core/internal/config"
	"github.com/subgate/subgate/core/internal/db"
	"github.com/subgate/subgate/core/internal/gateway"
	"github.com/subgate/subgate/pkg/wire"
)

func TestActivityKeyRotation(t *testing.T) {
	dir := t.TempDir()
	oldKey := bytes.Repeat([]byte{17}, 32)
	newKey := bytes.Repeat([]byte{29}, 32)
	oldFile := filepath.Join(dir, "synthetic-old.key")
	newFile := filepath.Join(dir, "synthetic-new.key")
	for path, key := range map[string][]byte{oldFile: oldKey, newFile: newKey} {
		if err := os.WriteFile(path, key, 0600); err != nil {
			t.Fatal(err)
		}
	}
	cfg := config.Defaults()
	cfg.Demo = true
	cfg.Database.URL = "file:" + filepath.Join(dir, "synthetic.sqlite")
	cfg.Database.BackupPath = filepath.Join(dir, "synthetic-backups")
	cfg.Auth.AdminPassword = "secure-test-password"
	cfg.Storage.EncryptionKeyFile = oldFile
	open := func() *Server {
		t.Helper()
		database, err := db.New(cfg.Database.URL)
		if err != nil {
			t.Fatal(err)
		}
		if err = db.RunMigrations(database); err != nil {
			t.Fatal(err)
		}
		s, err := New(cfg, database, slog.New(slog.NewTextHandler(io.Discard, nil)))
		if err != nil {
			database.Close()
			t.Fatal(err)
		}
		return s
	}
	close := func(s *Server) {
		t.Helper()
		if err := s.Shutdown(context.Background()); err != nil {
			t.Fatal(err)
		}
		if err := s.db.Close(); err != nil {
			t.Fatal(err)
		}
	}
	s := open()
	session := loginTest(t, s)
	// A pre-keyring payload is readable after rotation too.
	block, _ := aes.NewCipher(oldKey)
	legacy, _ := cipher.NewGCM(block)
	nonce := bytes.Repeat([]byte{3}, legacy.NonceSize())
	event := wire.Activity{UserID: "1", Timestamp: time.Now().UTC(), DstHost: "legacy.example.invalid", DstPort: 443}
	plain, _ := json.Marshal(event)
	payload := legacy.Seal(append([]byte(nil), nonce...), nonce, plain, []byte("demo-node-1:1"))
	if _, err := s.db.Exec("INSERT INTO sg_activity(node_id,user_id,timestamp,payload) VALUES(?,?,?,?)", "demo-node-1", "1", event.Timestamp.UnixMilli(), payload); err != nil {
		t.Fatal(err)
	}
	if err := s.Save("rotate-cache", gateway.CacheEntry{Data: []byte("synthetic subscription"), StoredAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if w := perform(s, "GET", "/api/v1/activity?domain=legacy", nil, session, "", false); w.Code != 200 {
		t.Fatal(w.Code)
	}
	before := perform(s, "POST", "/api/v1/backups", nil, session, "", true)
	var backup struct {
		Name string `json:"name"`
	}
	if before.Code != 201 || json.Unmarshal(before.Body.Bytes(), &backup) != nil {
		t.Fatal(before.Code)
	}
	backupData := perform(s, "GET", "/api/v1/backups/"+backup.Name, nil, session, "", false).Body.Bytes()
	close(s)
	cfg.Storage.EncryptionKeyFile = newFile
	cfg.Storage.PreviousEncryptionKeyFiles = []string{oldFile}
	s = open()
	defer close(s)
	session = loginTest(t, s)
	if w := perform(s, "GET", "/api/v1/activity?domain=legacy", nil, session, "", false); w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte("legacy.example.invalid")) {
		t.Fatalf("old activity unavailable: %d", w.Code)
	}
	if entry, err := s.Load("rotate-cache"); err != nil || string(entry.Data) != "synthetic subscription" {
		t.Fatal("old cache unavailable", err)
	}
	if w := perform(s, "GET", "/api/v1/audit", nil, session, "", false); w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte("legacy")) {
		t.Fatal("old audit unavailable", w.Code)
	}
	if raw, err := s.decrypt(backupData[8:], "subgate-backup-v1"); err != nil || !bytes.HasPrefix(raw, []byte("SQLite format")) {
		t.Fatal("old backup unavailable", err)
	}
	encrypted, err := s.encrypt([]byte("new sensitive row"), "rotation-check")
	if err != nil {
		t.Fatal(err)
	}
	// Current-key-only reader can decrypt all new writes; previous key alone cannot.
	cfgOnly := *cfg
	cfgOnly.Database.URL = ":memory:"
	cfgOnly.Storage.PreviousEncryptionKeyFiles = nil
	database, err := db.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err = db.RunMigrations(database); err != nil {
		t.Fatal(err)
	}
	only, err := New(&cfgOnly, database, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer only.Shutdown(context.Background())
	if raw, err := only.decrypt(encrypted, "rotation-check"); err != nil || string(raw) != "new sensitive row" {
		t.Fatal("new writes not current-key encrypted", err)
	}
	if _, err := legacy.Open(nil, encrypted[:legacy.NonceSize()], encrypted[legacy.NonceSize():], []byte("rotation-check")); err == nil {
		t.Fatal("new ciphertext still readable with old key alone")
	}
	credential, _ := enrollTest(t, s, session, "demo-node-1")
	batch := wire.Telemetry{BatchID: "rotated-new", NodeID: "demo-node-1", Events: []wire.Activity{{UserID: "1", Timestamp: time.Now().UTC(), DstHost: "rotated.example.invalid", DstPort: 443}}}
	if w := perform(s, "POST", "/api/v1/agents/telemetry", batch, nil, credential, false); w.Code != 200 {
		t.Fatal("rotated ingest failed", w.Code)
	}
	var persisted []byte
	if err := s.db.QueryRow("SELECT payload FROM sg_activity ORDER BY id DESC LIMIT 1").Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if raw, err := only.decrypt(persisted, "demo-node-1:1"); err != nil || !bytes.Contains(raw, []byte("rotated.example.invalid")) {
		t.Fatal("new activity not current-key encrypted", err)
	}
	if err := s.Save("new-cache", gateway.CacheEntry{Data: []byte("new cache")}); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow("SELECT payload FROM sg_snapshots WHERE key='new-cache'").Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if raw, err := only.decrypt(persisted, "new-cache"); err != nil || !bytes.Contains(raw, []byte("bmV3IGNhY2hl")) {
		t.Fatal("new cache not current-key encrypted", err)
	}
	var details, action string
	var ownerID int
	if err := s.db.QueryRow("SELECT details,action,admin_id FROM audit_log WHERE action='activity.read' ORDER BY id DESC LIMIT 1").Scan(&details, &action, &ownerID); err != nil {
		t.Fatal(err)
	}
	if decoded, err := only.decodeActivityAudit(details, action, ownerID); err != nil || decoded == nil {
		t.Fatal("new audit not current-key encrypted", err)
	}
	newBackup := perform(s, "POST", "/api/v1/backups", nil, session, "", true)
	if newBackup.Code != 201 || json.Unmarshal(newBackup.Body.Bytes(), &backup) != nil {
		t.Fatal("new backup failed", newBackup.Code)
	}
	download := perform(s, "GET", "/api/v1/backups/"+backup.Name, nil, session, "", false)
	if download.Code != 200 || len(download.Body.Bytes()) < 8 {
		t.Fatal("new backup download failed", download.Code)
	}
	if raw, err := only.decrypt(download.Body.Bytes()[8:], "subgate-backup-v1"); err != nil || !bytes.HasPrefix(raw, []byte("SQLite format")) {
		t.Fatal("new backup not current-key encrypted", err)
	}
	if _, err := only.decrypt(payload, "demo-node-1:1"); err == nil {
		t.Fatal("retired key data readable without previous key")
	}
	encrypted[len(encrypted)-1] ^= 1
	if _, err := s.decrypt(encrypted, "rotation-check"); err == nil {
		t.Fatal("tampered rotated payload accepted")
	}
}

func TestStorageKeyringRejectsInvalidFiles(t *testing.T) {
	for _, test := range []string{"missing", "short", "public", "too-many", "symlink", "oversize"} {
		t.Run(test, func(t *testing.T) {
			cfg := config.Defaults()
			cfg.Demo = true
			cfg.Database.URL = ":memory:"
			cfg.Auth.AdminPassword = "secure-test-password"
			path := filepath.Join(t.TempDir(), "synthetic.key")
			cfg.Storage.EncryptionKeyFile = path
			if test != "missing" {
				key := bytes.Repeat([]byte{1}, 32)
				mode := os.FileMode(0600)
				if test == "short" {
					key = key[:16]
				}
				if test == "oversize" {
					key = append(key, 1)
				}
				if test == "public" {
					mode = 0644
				}
				if err := os.WriteFile(path, key, mode); err != nil {
					t.Fatal(err)
				}
				if test == "symlink" {
					link := path + ".link"
					if err := os.Symlink(path, link); err != nil {
						t.Fatal(err)
					}
					cfg.Storage.EncryptionKeyFile = link
				}
			}
			if test == "too-many" {
				for i := 0; i < 9; i++ {
					cfg.Storage.PreviousEncryptionKeyFiles = append(cfg.Storage.PreviousEncryptionKeyFiles, path)
				}
			}
			database, err := db.New(":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			if err = db.RunMigrations(database); err != nil {
				t.Fatal(err)
			}
			s, err := New(cfg, database, slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err == nil {
				s.Shutdown(context.Background())
				t.Error("invalid storage keyring accepted")
			}
		})
	}
}
