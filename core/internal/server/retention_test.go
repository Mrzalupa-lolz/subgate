package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPruneStorageCoversEveryGrowingTable(t *testing.T) {
	s := newTestServer(t)
	now := time.Now()
	old, fresh := now.Add(-25*time.Hour).UnixMilli(), now.Add(-time.Minute).UnixMilli()
	for _, at := range []int64{old, fresh} {
		if _, err := s.db.Exec("INSERT INTO sg_speed(node_id,user_id,timestamp,uplink,downlink) VALUES('n','u',?,1,1)", at); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec("INSERT INTO sg_speed_meta(node_id,user_id,timestamp,interval_ms,scope,gap,warming,uplink_delta,downlink_delta,received_at) VALUES('n','u',?,1000,'user',0,0,1,1,?)", at, at); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec("INSERT INTO sg_usage(node_id,user_id,timestamp,uplink,downlink) VALUES('n','u',?,1,1)", at); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.db.Exec("INSERT INTO audit_log(action,timestamp) VALUES('old.action',?),('fresh.action',CURRENT_TIMESTAMP)", now.Add(-200*24*time.Hour).UTC().Format("2006-01-02 15:04:05")); err != nil {
		t.Fatal(err)
	}
	s.pruneStorage(now)
	for _, table := range []string{"sg_speed", "sg_speed_meta", "sg_usage"} {
		var total, stale int
		if err := s.db.QueryRow("SELECT COUNT(*), COALESCE(SUM(timestamp<?),0) FROM "+table, fresh).Scan(&total, &stale); err != nil {
			t.Fatal(err)
		}
		if total != 1 || stale != 0 {
			t.Fatalf("%s retention: total=%d stale=%d", table, total, stale)
		}
	}
	var oldAudit, freshAudit int
	if err := s.db.QueryRow("SELECT COALESCE(SUM(action='old.action'),0), COALESCE(SUM(action='fresh.action'),0) FROM audit_log").Scan(&oldAudit, &freshAudit); err != nil {
		t.Fatal(err)
	}
	if oldAudit != 0 || freshAudit != 1 {
		t.Fatalf("audit retention: old=%d fresh=%d", oldAudit, freshAudit)
	}
}

func TestPruneBackupsKeepsNewest(t *testing.T) {
	dir := t.TempDir()
	names := []string{"subgate-20261101T000000-a.sgb", "subgate-20261102T000000-b.sgb", "subgate-20261103T000000-c.sgb", "other.txt"}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := pruneBackups(dir, 2); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]bool{names[0]: false, names[1]: true, names[2]: true, names[3]: true} {
		if _, err := os.Stat(filepath.Join(dir, name)); (err == nil) != want {
			t.Fatalf("%s present=%v, want %v", name, err == nil, want)
		}
	}
}
