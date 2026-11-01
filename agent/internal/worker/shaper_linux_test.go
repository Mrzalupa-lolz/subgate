//go:build linux

package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/subgate/subgate/pkg/wire"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeTC struct {
	devices        map[string]string
	adds           int
	fail           string
	removalFailure bool
}

func TestShaperTCNamespace(t *testing.T) {
	if os.Getenv("SUBGATE_TEST_TC_NETNS") != "1" {
		t.Skip("opt-in isolated Linux network namespace")
	}
	namespace := fmt.Sprintf("sg-test-%d", os.Getpid())
	if raw, err := exec.Command("ip", "netns", "add", namespace).CombinedOutput(); err != nil {
		t.Fatalf("isolated namespace unavailable: %s %v", raw, err)
	}
	t.Cleanup(func() {
		if raw, err := exec.Command("ip", "netns", "del", namespace).CombinedOutput(); err != nil {
			t.Errorf("namespace cleanup failed: %s %v", raw, err)
		}
	})
	run := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		prefix := []string{"netns", "exec", namespace, name}
		return exec.CommandContext(ctx, "ip", append(prefix, args...)...).CombinedOutput()
	}
	if raw, err := run(context.Background(), "ip", "link", "add", "sg-one", "type", "veth", "peer", "name", "sg-peer"); err != nil {
		t.Fatalf("veth creation failed: %s %v", raw, err)
	}
	now := time.Now().UTC()
	s := Shaper{Path: filepath.Join(t.TempDir(), "leases.json"), Run: run, Now: func() time.Time { return now }}
	a := wire.ShaperAction{ID: "real-tc", UserID: "account", Mbps: 10, CreatedAt: now, ExpiresAt: now.Add(time.Minute)}
	b := wire.Binding{UserID: "account", Interface: "sg-one", ShapeInterfaces: []string{"sg-one", "sg-peer"}, Exclusive: true, ValidFrom: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour)}
	results, err := s.Reconcile(context.Background(), []wire.ShaperAction{a}, []wire.Binding{b}, nil, false)
	if err != nil || len(results) != 1 || results[0].Status != "applied" {
		t.Fatal(results, err)
	}
	for _, iface := range b.ShapeInterfaces {
		owned, err := s.owned(context.Background(), iface)
		if err != nil || !owned {
			t.Fatal("kernel did not confirm qdisc", iface, err)
		}
	}
	now = now.Add(61 * time.Second)
	results, err = s.Reconcile(context.Background(), nil, []wire.Binding{b}, nil, false)
	if err != nil || len(results) != 1 || results[0].Status != "removed" {
		t.Fatal("kernel TTL cleanup failed", results, err)
	}
	for _, iface := range b.ShapeInterfaces {
		owned, err := s.owned(context.Background(), iface)
		if err != nil || owned {
			t.Fatal("qdisc survived expiry", iface, err)
		}
	}
}

func (f *fakeTC) run(_ context.Context, _ string, args ...string) ([]byte, error) {
	iface := ""
	for i, v := range args {
		if v == "dev" {
			iface = args[i+1]
		}
	}
	if len(args) > 0 && args[0] == "-j" {
		handle := f.devices[iface]
		kind := "noqueue"
		if handle == ownedHandle {
			kind = "tbf"
		} else if handle != "" {
			kind = "fq_codel"
		}
		return json.Marshal([]map[string]any{{"kind": kind, "handle": handle, "root": true}})
	}
	if args[1] == "add" {
		f.adds++
		if iface == f.fail {
			return nil, fmt.Errorf("fixture")
		}
		f.devices[iface] = ownedHandle
		return nil, nil
	}
	if args[1] == "del" {
		if f.removalFailure {
			return nil, fmt.Errorf("fixture")
		}
		delete(f.devices, iface)
		return nil, nil
	}
	return nil, fmt.Errorf("unexpected tc command: %s", strings.Join(args, " "))
}

func TestShaperIdempotencyExpiryRollbackAndForeignQdisc(t *testing.T) {
	now := time.Unix(1800000000, 0).UTC()
	f := &fakeTC{devices: map[string]string{}}
	s := Shaper{Path: filepath.Join(t.TempDir(), "leases.json"), Run: f.run, Now: func() time.Time { return now }}
	a := wire.ShaperAction{ID: "test", NodeID: "node", UserID: "account", Mbps: 10, CreatedAt: now, ExpiresAt: now.Add(time.Minute)}
	b := wire.Binding{UserID: "account", Interface: "sg-one", ShapeInterfaces: []string{"sg-one", "sg-peer"}, Exclusive: true, ValidFrom: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour)}
	apply := func(desired []wire.ShaperAction) []wire.ShaperResult {
		t.Helper()
		results, err := s.Reconcile(context.Background(), desired, []wire.Binding{b}, nil, false)
		if err != nil {
			t.Fatal(err)
		}
		return results
	}
	if got := apply([]wire.ShaperAction{a}); len(got) != 1 || got[0].Status != "applied" || f.adds != 2 {
		t.Fatal(got, f.adds)
	}
	apply([]wire.ShaperAction{a})
	if f.adds != 2 {
		t.Fatal("repeated command mutated tc")
	}
	// Core outage does not remove an unexpired action, and a new process
	// recovers the same absolute TTL from the on-disk journal.
	s = Shaper{Path: s.Path, Run: f.run, Now: func() time.Time { return now }}
	apply(nil)
	if len(f.devices) != 2 {
		t.Fatal("outage dropped active lease")
	}
	now = now.Add(61 * time.Second)
	got := apply(nil)
	if len(f.devices) != 0 || len(got) != 1 || got[0].Status != "removed" {
		t.Fatal("offline TTL failed", got, f.devices)
	}
	now = a.CreatedAt
	f.fail = "sg-peer"
	got = apply([]wire.ShaperAction{a})
	if len(f.devices) != 0 || got[0].Status != "failed" {
		t.Fatal("partial application not rolled back", got, f.devices)
	}
	f.fail = ""
	f.devices["sg-one"] = "1234:"
	got = apply([]wire.ShaperAction{a})
	if f.devices["sg-one"] != "1234:" || got[0].Status != "failed" {
		t.Fatal("foreign qdisc overwritten", got)
	}
	delete(f.devices, "sg-one")
	apply([]wire.ShaperAction{a})
	if got = apply([]wire.ShaperAction{}); got[0].Status != "removed" || len(f.devices) != 0 {
		t.Fatal("cancel snapshot not reconciled", got)
	}
}

func TestShaperAllowlistUncertainBindingAndCleanupRetry(t *testing.T) {
	now := time.Unix(1800000000, 0).UTC()
	f := &fakeTC{devices: map[string]string{}}
	s := Shaper{Path: filepath.Join(t.TempDir(), "leases.json"), Run: f.run, Now: func() time.Time { return now }}
	a := wire.ShaperAction{ID: "test", UserID: "account", Mbps: 10, CreatedAt: now, ExpiresAt: now.Add(time.Minute)}
	b := wire.Binding{UserID: "account", Interface: "sg-one", ShapeInterfaces: []string{"sg-one"}, ValidFrom: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour)}
	for _, exclusive := range []bool{false, true} {
		b.Exclusive = exclusive
		_, err := s.Reconcile(context.Background(), []wire.ShaperAction{a}, []wire.Binding{b}, []string{"account"}, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(f.devices) != 0 {
			t.Fatal("allowlisted account shaped")
		}
	}
	b.Exclusive = false
	s.Reconcile(context.Background(), []wire.ShaperAction{a}, []wire.Binding{b}, nil, false)
	if len(f.devices) != 0 {
		t.Fatal("ambiguous account shaped")
	}
	b.Exclusive = true
	s.Reconcile(context.Background(), []wire.ShaperAction{a}, []wire.Binding{b}, nil, false)
	f.removalFailure = true
	now = now.Add(2 * time.Minute)
	results, err := s.Reconcile(context.Background(), nil, []wire.Binding{b}, nil, false)
	if err != nil || results[0].Status != "failed" {
		t.Fatal(results, err)
	}
	leases, err := s.load()
	if err != nil || len(leases) != 1 {
		t.Fatal("cleanup failure lost durable recovery record")
	}
	f.removalFailure = false
	s.Reconcile(context.Background(), nil, []wire.Binding{b}, nil, false)
	leases, err = s.load()
	if err != nil || len(leases) != 0 || len(f.devices) != 0 {
		t.Fatal("cleanup did not retry")
	}
}
