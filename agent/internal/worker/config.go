//go:build linux

package worker

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"syscall"

	"github.com/subgate/subgate/pkg/wire"
)

type Config struct {
	CoreURL           string         `json:"core_url"`
	NodeID            string         `json:"node_id"`
	TokenFile         string         `json:"token_file"`
	Collector         string         `json:"collector"`
	ResourceInterface string         `json:"resource_interface"`
	CapacityMbps      int            `json:"capacity_mbps"`
	Bindings          []wire.Binding `json:"bindings"`
	Allowlist         []string       `json:"allowlist"`
}

func ValidInterface(v string) bool {
	if len(v) < 1 || len(v) > 15 || v == "lo" {
		return false
	}
	for _, c := range v {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-' || c == '.') {
			return false
		}
	}
	return true
}

func LoadConfig(path string) (Config, error) {
	var cfg Config
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return cfg, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 1<<20 {
		return cfg, fmt.Errorf("worker config must be a private regular file <=1MiB")
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok && os.Geteuid() == 0 && stat.Uid != 0 {
		return cfg, fmt.Errorf("worker config must be root-owned")
	}
	d := json.NewDecoder(f)
	d.DisallowUnknownFields()
	if err = d.Decode(&cfg); err != nil {
		return cfg, err
	}
	if d.Decode(new(any)) != io.EOF {
		return cfg, fmt.Errorf("worker config must contain one JSON object")
	}
	if cfg.NodeID == "" || len(cfg.NodeID) > 256 || !ValidInterface(cfg.ResourceInterface) || cfg.CapacityMbps < 1 || cfg.CapacityMbps > 1000000 || len(cfg.Bindings) > 64 {
		return cfg, fmt.Errorf("invalid worker configuration")
	}
	users, interfaces := []string{}, []string{}
	for _, b := range cfg.Bindings {
		if b.UserID == "" || len(b.UserID) > 256 || !ValidInterface(b.Interface) || b.ExpiresAt.IsZero() || b.Exclusive && (b.ValidFrom.IsZero() || !b.ExpiresAt.After(b.ValidFrom)) || len(b.ShapeInterfaces) > 2 || slices.Contains(users, b.UserID) || slices.Contains(interfaces, b.Interface) {
			return cfg, fmt.Errorf("duplicate or invalid account binding")
		}
		users = append(users, b.UserID)
		interfaces = append(interfaces, b.Interface)
		for _, iface := range b.ShapeInterfaces {
			if !ValidInterface(iface) || iface == cfg.ResourceInterface {
				return cfg, fmt.Errorf("shaper interfaces must be dedicated account devices")
			}
		}
	}
	for i, b := range cfg.Bindings {
		for j, c := range cfg.Bindings {
			if i == j {
				continue
			}
			for _, iface := range b.ShapeInterfaces {
				if slices.Contains(c.ShapeInterfaces, iface) {
					return cfg, fmt.Errorf("shared shaper interface")
				}
			}
		}
	}
	return cfg, nil
}
