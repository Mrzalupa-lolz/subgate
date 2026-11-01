//go:build linux

package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"syscall"
	"time"

	"github.com/subgate/subgate/pkg/wire"
)

const ownedHandle = "7a11:"

type Runner func(context.Context, string, ...string) ([]byte, error)

func RunCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

type Lease struct {
	Action     wire.ShaperAction `json:"action"`
	Interfaces []string          `json:"interfaces"`
}

type Shaper struct {
	Path string
	Run  Runner
	Now  func() time.Time
}

func (s Shaper) owned(ctx context.Context, iface string) (bool, error) {
	raw, err := s.Run(ctx, "tc", "-j", "qdisc", "show", "dev", iface)
	if err != nil {
		return false, fmt.Errorf("qdisc inspection failed")
	}
	var list []struct {
		Kind   string `json:"kind"`
		Handle string `json:"handle"`
		Root   bool   `json:"root"`
	}
	if json.Unmarshal(raw, &list) != nil {
		return false, fmt.Errorf("invalid qdisc response")
	}
	owned := false
	for _, q := range list {
		if !q.Root {
			continue
		}
		if q.Handle == ownedHandle && q.Kind == "tbf" {
			owned = true
			continue
		}
		if q.Kind != "noqueue" {
			return false, fmt.Errorf("interface has a foreign root qdisc")
		}
	}
	return owned, nil
}

func (s Shaper) load() ([]Lease, error) {
	raw, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return []Lease{}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(raw) > 2<<20 {
		return nil, fmt.Errorf("lease journal too large")
	}
	var leases []Lease
	if json.Unmarshal(raw, &leases) != nil || len(leases) > 2000 {
		return nil, fmt.Errorf("invalid lease journal")
	}
	return leases, nil
}

func (s Shaper) save(leases []Lease) error {
	raw, err := json.Marshal(leases)
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.Path)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".lease-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	defer f.Close()
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(raw)
	}
	if err == nil {
		err = f.Sync()
	}
	if err == nil {
		err = f.Close()
	}
	if err == nil {
		err = os.Rename(name, s.Path)
	}
	if err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func (s Shaper) lock() (func(), error) {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(s.Path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, err
	}
	return func() { syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, nil
}

func (s Shaper) remove(ctx context.Context, l Lease) error {
	for _, iface := range l.Interfaces {
		if !ValidInterface(iface) {
			return fmt.Errorf("invalid journal interface")
		}
		owned, err := s.owned(ctx, iface)
		if err != nil {
			return err
		}
		if owned {
			if _, err = s.Run(ctx, "tc", "qdisc", "del", "dev", iface, "root", "handle", ownedHandle); err != nil {
				return fmt.Errorf("qdisc removal failed")
			}
		}
	}
	return nil
}

// Reconcile journals intent before touching TC, retries partial removals, and
// never extends a command's absolute expiry. nil desired means Core unavailable:
// keep existing leases until TTL, while still honoring local binding/allowlist.
func (s Shaper) Reconcile(ctx context.Context, desired []wire.ShaperAction, bindings []wire.Binding, allowlist []string, force bool) ([]wire.ShaperResult, error) {
	unlock, err := s.lock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	leases, err := s.load()
	if err != nil {
		return nil, err
	}
	results := []wire.ShaperResult{}
	now := s.Now()
	valid := func(a wire.ShaperAction) *wire.Binding {
		if force || slices.Contains(allowlist, a.UserID) || !a.ExpiresAt.After(now) || a.CreatedAt.After(now.Add(5*time.Second)) || a.ExpiresAt.Sub(a.CreatedAt) > 24*time.Hour || a.Mbps < 1 || a.Mbps > 100000 {
			return nil
		}
		var found *wire.Binding
		for i := range bindings {
			b := &bindings[i]
			if b.UserID == a.UserID {
				if found != nil {
					return nil
				}
				found = b
			}
		}
		if found == nil || !found.Exclusive || found.ValidFrom.IsZero() || found.ValidFrom.After(a.CreatedAt) || found.ExpiresAt.Before(a.ExpiresAt) || len(found.ShapeInterfaces) == 0 || len(found.ShapeInterfaces) > 2 {
			return nil
		}
		for _, iface := range found.ShapeInterfaces {
			if !ValidInterface(iface) {
				return nil
			}
			for _, b := range bindings {
				if b.UserID != a.UserID && slices.Contains(b.ShapeInterfaces, iface) {
					return nil
				}
			}
		}
		return found
	}
	kept := []Lease{}
	for _, l := range leases {
		b := valid(l.Action)
		wanted := desired == nil || slices.ContainsFunc(desired, func(a wire.ShaperAction) bool { return a == l.Action })
		if b == nil || !wanted || !slices.Equal(l.Interfaces, b.ShapeInterfaces) {
			if err = s.remove(ctx, l); err != nil {
				kept = append(kept, l)
				results = append(results, wire.ShaperResult{ID: l.Action.ID, Status: "failed", Error: err.Error()})
			} else {
				results = append(results, wire.ShaperResult{ID: l.Action.ID, Status: "removed"})
			}
		} else {
			kept = append(kept, l)
		}
	}
	if len(kept) != len(leases) {
		if err = s.save(kept); err != nil {
			return results, err
		}
	}
	for _, a := range desired {
		b := valid(a)
		if b == nil {
			results = append(results, wire.ShaperResult{ID: a.ID, Status: "failed", Error: "binding, allowlist or TTL prevents application"})
			continue
		}
		index := slices.IndexFunc(kept, func(l Lease) bool { return l.Action.ID == a.ID })
		if index < 0 {
			conflict := false
			for _, l := range kept {
				for _, iface := range l.Interfaces {
					if slices.Contains(b.ShapeInterfaces, iface) {
						conflict = true
					}
				}
			}
			if conflict {
				results = append(results, wire.ShaperResult{ID: a.ID, Status: "failed", Error: "interface already leased"})
				continue
			}
			kept = append(kept, Lease{Action: a, Interfaces: append([]string{}, b.ShapeInterfaces...)})
			if err = s.save(kept); err != nil {
				return results, err
			}
			index = len(kept) - 1
		}
		applyErr := error(nil)
		for _, iface := range kept[index].Interfaces {
			owned, e := s.owned(ctx, iface)
			if e != nil {
				applyErr = e
				break
			}
			if owned {
				continue
			}
			_, e = s.Run(ctx, "tc", "qdisc", "add", "dev", iface, "root", "handle", ownedHandle, "tbf", "rate", strconv.Itoa(a.Mbps)+"mbit", "burst", "128kb", "latency", "50ms")
			if e != nil {
				applyErr = fmt.Errorf("qdisc application failed")
				break
			}
		}
		if applyErr != nil {
			// Best-effort rollback of every already-applied direction. The durable
			// lease is retained on cleanup failure for the independent TTL timer.
			cleanupErr := s.remove(ctx, kept[index])
			if cleanupErr == nil {
				kept = append(kept[:index], kept[index+1:]...)
				if err = s.save(kept); err != nil {
					return results, err
				}
			}
			results = append(results, wire.ShaperResult{ID: a.ID, Status: "failed", Error: applyErr.Error()})
		} else {
			results = append(results, wire.ShaperResult{ID: a.ID, Status: "applied"})
		}
	}
	return results, nil
}
