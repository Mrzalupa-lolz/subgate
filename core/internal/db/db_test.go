package db

import (
	"path/filepath"
	"testing"
)

func TestBackupRestoresCommittedWALData(t *testing.T) {
	database, err := New("file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "live.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := RunMigrations(database); err != nil {
		t.Fatal(err)
	}
	if err := RunMigrations(database); err != nil {
		t.Fatalf("migration not idempotent: %v", err)
	}
	if _, err := database.Exec("INSERT INTO settings(key,value) VALUES(?,?)", "restored-key", "committed WAL payload"); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(t.TempDir(), "quoted ' backup.sqlite")
	if err := database.Backup(backup); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec("UPDATE settings SET value='after snapshot' WHERE key='restored-key'"); err != nil {
		t.Fatal(err)
	}
	restored, err := New("file:" + filepath.ToSlash(backup))
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	var value string
	if err := restored.QueryRow("SELECT value FROM settings WHERE key='restored-key'").Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != "committed WAL payload" {
		t.Fatalf("snapshot contains %q", value)
	}
	var integrity string
	if err := restored.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("invalid snapshot: %q %v", integrity, err)
	}
}

func TestFailedTransactionReleasesWriter(t *testing.T) {
	database, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec("CREATE TABLE records(value TEXT)"); err != nil {
		t.Fatal(err)
	}
	tx, err := database.BeginTx()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec("INSERT INTO records VALUES('rollback')"); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	database.TxUnlock()
	if _, err = database.Exec("INSERT INTO records VALUES('commit')"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = database.QueryRow("SELECT COUNT(*) FROM records").Scan(&count); err != nil || count != 1 {
		t.Fatalf("transaction result: %d %v", count, err)
	}
	if err = database.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = database.BeginTx(); err == nil {
		t.Fatal("begin on closed database succeeded")
	}
	if err = database.Close(); err != nil {
		t.Fatal(err)
	}
}
