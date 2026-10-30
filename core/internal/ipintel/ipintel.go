// Package ipintel implements IP intelligence lookup (ASN, country, network type)
package ipintel

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/oschwald/maxminddb-golang"
)

// IPIntel represents IP intelligence data for an IP address
type IPIntel struct {
	IP           string    `json:"ip"`
	ASN          int       `json:"asn"`
	ASOrg        string    `json:"as_org"`
	Country      string    `json:"country"`
	Region       string    `json:"region"`
	City         string    `json:"city"`
	Latitude     float64   `json:"latitude"`
	Longitude    float64   `json:"longitude"`
	Timezone     string    `json:"timezone"`
	NetworkType  string    `json:"network_type"` // 'mobile', 'home_isp', 'hosting', 'vpn', 'cdn', 'edu', 'gov'
	Confidence   string    `json:"confidence"`   // 'high', 'medium', 'low'
	Sources      []string  `json:"sources"`      // Which data sources contributed
	BuildDate    time.Time `json:"build_date"`
	StaleWarning string    `json:"stale_warning,omitempty"`
}

// Engine performs IP intelligence lookups
type Engine struct {
	prefixes  []PrefixRecord
	databases []*maxminddb.Reader
	data      map[string]*IPIntel // In-memory cache of known IPs
	asnData   map[int]ASNInfo     // ASN metadata
	mu        sync.RWMutex
	logger    *slog.Logger
}

// PrefixRecord is an operator-supplied record with explicit provenance.
type PrefixRecord struct {
	CIDR string `json:"cidr"`
	IPIntel
	prefix netip.Prefix
}

// LoadPrefixes validates the entire dataset before publishing it atomically.
func (e *Engine) LoadPrefixes(path string) error {
	if info, err := os.Stat(path); err == nil && (info.IsDir() || strings.HasSuffix(strings.ToLower(path), ".mmdb")) {
		return e.loadMMDB(path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var records []PrefixRecord
	if err = json.Unmarshal(raw, &records); err != nil {
		return err
	}
	for i := range records {
		p, err := netip.ParsePrefix(records[i].CIDR)
		if err != nil {
			return err
		}
		records[i].prefix = p.Masked()
		if records[i].Country == "" {
			records[i].Country = "UNKNOWN"
		}
		if records[i].Confidence == "" {
			records[i].Confidence = "low"
		}
		if records[i].NetworkType == "" {
			records[i].NetworkType = "unknown"
		}
		if len(records[i].Sources) == 0 {
			records[i].Sources = []string{"operator"}
		}
	}
	sort.SliceStable(records, func(i, j int) bool { return records[i].prefix.Bits() > records[j].prefix.Bits() })
	e.mu.Lock()
	defer e.mu.Unlock()
	e.prefixes = records
	e.data = map[string]*IPIntel{}
	for _, database := range e.databases {
		database.Close()
	}
	e.databases = nil
	return nil
}

// ASNInfo contains ASN metadata
type ASNInfo struct {
	ASN         int
	ASOrg       string
	NetworkType string
	Country     string
}

// NewEngine creates a new IP intelligence engine
func NewEngine(logger *slog.Logger) *Engine {
	return &Engine{
		data:    make(map[string]*IPIntel),
		asnData: make(map[int]ASNInfo),
		logger:  logger,
	}
}

// Lookup performs an IP intelligence lookup
func (e *Engine) Lookup(ipStr string) (*IPIntel, error) {
	// Validate IP
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP address: %s", ipStr)
	}

	ipStr = ip.String()
	addr, _ := netip.ParseAddr(ipStr)
	addr = addr.Unmap()
	// Check the loaded operator dataset, longest prefix first.
	e.mu.RLock()
	for _, record := range e.prefixes {
		if record.prefix.Contains(addr) {
			result := record.IPIntel
			result.IP = ipStr
			result.Sources = append([]string{}, record.Sources...)
			result.StaleWarning = staleWarning(result.BuildDate)
			e.mu.RUnlock()
			return &result, nil
		}
	}
	if len(e.databases) > 0 {
		result := IPIntel{IP: ipStr, Country: "UNKNOWN", ASOrg: "Unknown", Confidence: "low", NetworkType: "unknown", Sources: []string{}}
		for _, database := range e.databases {
			var record mmdbRecord
			if database.Lookup(ip, &record) != nil {
				continue
			}
			if record.ASN > 0 {
				result.ASN = int(record.ASN)
				result.ASOrg = record.Organization
			}
			country := record.Country.ISOCode
			if country == "" {
				country = record.RegisteredCountry.ISOCode
			}
			if country != "" {
				result.Country = country
				result.Confidence = "medium"
			}
			if record.City.Names["en"] != "" {
				result.City = record.City.Names["en"]
			}
			if len(record.Subdivisions) > 0 {
				result.Region = record.Subdivisions[0].Names["en"]
			}
			if record.Location.Latitude != 0 || record.Location.Longitude != 0 {
				result.Latitude = record.Location.Latitude
				result.Longitude = record.Location.Longitude
			}
			if record.Location.TimeZone != "" {
				result.Timezone = record.Location.TimeZone
			}
			if record.ASN > 0 || country != "" {
				result.Sources = append(result.Sources, database.Metadata.DatabaseType)
				built := time.Unix(int64(database.Metadata.BuildEpoch), 0)
				if result.BuildDate.IsZero() || built.Before(result.BuildDate) {
					result.BuildDate = built
				}
			}
		}
		result.StaleWarning = staleWarning(result.BuildDate)
		if result.ASN != 0 {
			if metadata, ok := e.asnData[result.ASN]; ok {
				if metadata.NetworkType != "" {
					result.NetworkType = metadata.NetworkType
				}
			}
		}
		e.mu.RUnlock()
		return &result, nil
	}
	if result, exists := e.data[ipStr]; exists {
		copy := *result
		copy.Sources = append([]string{}, result.Sources...)
		e.mu.RUnlock()
		return &copy, nil
	}
	e.mu.RUnlock()

	// Unknown addresses stay explicitly unknown; no location is fabricated.
	result := &IPIntel{
		IP:          ipStr,
		ASN:         0,
		ASOrg:       "Unknown",
		Country:     "UNKNOWN",
		Confidence:  "low",
		Sources:     []string{},
		NetworkType: "unknown",
	}

	return result, nil
}

type mmdbRecord struct {
	ASN          uint   `maxminddb:"autonomous_system_number"`
	Organization string `maxminddb:"autonomous_system_organization"`
	Country      struct {
		ISOCode string `maxminddb:"iso_code"`
	} `maxminddb:"country"`
	RegisteredCountry struct {
		ISOCode string `maxminddb:"iso_code"`
	} `maxminddb:"registered_country"`
	City struct {
		Names map[string]string `maxminddb:"names"`
	} `maxminddb:"city"`
	Subdivisions []struct {
		Names map[string]string `maxminddb:"names"`
	} `maxminddb:"subdivisions"`
	Location struct {
		Latitude  float64 `maxminddb:"latitude"`
		Longitude float64 `maxminddb:"longitude"`
		TimeZone  string  `maxminddb:"time_zone"`
	} `maxminddb:"location"`
}

func (e *Engine) loadMMDB(path string) error {
	paths := []string{path}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		matches, err := filepath.Glob(filepath.Join(path, "*.mmdb"))
		if err != nil {
			return err
		}
		paths = matches
	}
	if len(paths) == 0 {
		return fmt.Errorf("no MMDB databases found")
	}
	loaded := []*maxminddb.Reader{}
	for _, path := range paths {
		database, err := maxminddb.Open(path)
		if err != nil {
			for _, db := range loaded {
				db.Close()
			}
			return err
		}
		loaded = append(loaded, database)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	old := e.databases
	e.databases = loaded
	e.prefixes = nil
	e.data = map[string]*IPIntel{}
	for _, db := range old {
		db.Close()
	}
	return nil
}
func (e *Engine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	var err error
	for _, db := range e.databases {
		if e := db.Close(); e != nil {
			err = e
		}
	}
	e.databases = nil
	return err
}

// BulkLookup performs lookups for multiple IPs
func (e *Engine) BulkLookup(ips []string) (map[string]*IPIntel, error) {
	results := make(map[string]*IPIntel)

	for _, ipStr := range ips {
		result, err := e.Lookup(ipStr)
		if err != nil {
			e.logger.Warn("bulk lookup failed for IP", slog.String("ip", ipStr), slog.String("error", err.Error()))
			continue
		}
		results[ipStr] = result
	}

	return results, nil
}

// UpdateASNMetadata updates ASN metadata (called when loading datasets)
func (e *Engine) UpdateASNMetadata(asn int, info ASNInfo) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.asnData[asn] = info
}

// Health returns engine health status
func (e *Engine) Health() map[string]interface{} {
	e.mu.RLock()
	defer e.mu.RUnlock()

	sources := []map[string]any{}
	for _, database := range e.databases {
		built := time.Unix(int64(database.Metadata.BuildEpoch), 0).UTC()
		sources = append(sources, map[string]any{"database_type": database.Metadata.DatabaseType, "build_date": built, "stale_warning": staleWarning(built)})
	}
	return map[string]interface{}{
		"prefix_records": len(e.prefixes),
		"mmdb_sources":   len(e.databases),
		"ip_cache_size":  len(e.data),
		"asn_count":      len(e.asnData),
		"status":         "ok",
		"sources":        sources,
	}
}

func staleWarning(built time.Time) string {
	if !built.IsZero() && time.Since(built) > 45*24*time.Hour {
		return "dataset is older than 45 days; replace and reload it"
	}
	return ""
}

// GetASNInfo returns metadata for an ASN
func (e *Engine) GetASNInfo(asn int) (ASNInfo, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	info, exists := e.asnData[asn]
	return info, exists
}

// LoadTestData loads mock test data
func (e *Engine) LoadTestData() {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Google DNS
	e.data["8.8.8.8"] = &IPIntel{
		IP:          "8.8.8.8",
		ASN:         15169,
		ASOrg:       "Google LLC",
		Country:     "US",
		Region:      "California",
		City:        "Mountain View",
		Latitude:    37.4192,
		Longitude:   -122.0574,
		Timezone:    "America/Los_Angeles",
		NetworkType: "hosting",
		Confidence:  "high",
		Sources:     []string{"L1", "L3"},
		BuildDate:   time.Now(),
	}

	// Cloudflare DNS
	e.data["1.1.1.1"] = &IPIntel{
		IP:          "1.1.1.1",
		ASN:         13335,
		ASOrg:       "Cloudflare Inc.",
		Country:     "US",
		Confidence:  "high",
		NetworkType: "cdn",
		Sources:     []string{"L1", "L3"},
		BuildDate:   time.Now(),
	}

	// ASN metadata
	e.asnData[15169] = ASNInfo{
		ASN:         15169,
		ASOrg:       "Google LLC",
		NetworkType: "hosting",
		Country:     "US",
	}

	e.asnData[13335] = ASNInfo{
		ASN:         13335,
		ASOrg:       "Cloudflare Inc.",
		NetworkType: "cdn",
		Country:     "US",
	}

	e.logger.Info("loaded test IP intelligence data")
}

// PerformanceMetrics returns performance statistics
func (e *Engine) PerformanceMetrics() map[string]interface{} {
	e.mu.RLock()
	cacheSize := len(e.data)
	asnCount := len(e.asnData)
	e.mu.RUnlock()

	return map[string]interface{}{
		"ip_cache_entries": cacheSize,
		"asn_entries":      asnCount,
		"timestamp":        time.Now().Unix(),
	}
}
