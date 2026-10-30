package ipintel

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestPrefixLongestMatchAndLastGoodReload(t *testing.T) {
	engine := NewEngine(slog.Default())
	path := filepath.Join(t.TempDir(), "prefixes.json")
	raw := `[{"cidr":"203.0.113.0/24","asn":64501,"country":"RU","confidence":"medium","sources":["operator"]},{"cidr":"203.0.113.64/26","asn":64502,"country":"FI","confidence":"high","network_type":"mobile","sources":["operator-reviewed"]},{"cidr":"2001:db8::/32","asn":64503,"country":"NL","confidence":"medium","sources":["operator"]}]`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if err := engine.LoadPrefixes(path); err != nil {
		t.Fatal(err)
	}
	result, err := engine.Lookup("::ffff:203.0.113.70")
	if err != nil || result.ASN != 64502 || result.Country != "FI" {
		t.Fatal(result, err)
	}
	result, err = engine.Lookup("2001:db8::1234")
	if err != nil || result.ASN != 64503 {
		t.Fatal(result, err)
	}
	_ = os.WriteFile(path, []byte(`[{"cidr":"invalid"}]`), 0600)
	if err := engine.LoadPrefixes(path); err == nil {
		t.Fatal("invalid reload accepted")
	}
	result, _ = engine.Lookup("203.0.113.70")
	if result.ASN != 64502 {
		t.Fatal("lost last good dataset")
	}
	result, _ = engine.Lookup("198.51.100.1")
	if result.Country != "UNKNOWN" || result.Confidence != "low" || len(result.Sources) != 0 {
		t.Fatal("fabricated unknown location")
	}
	if engine.Health()["ip_cache_size"].(int) != 0 {
		t.Fatal("unknown IP lookups must not accumulate unbounded cache")
	}
}

func TestMMDBLookupReloadAndSourceSwitch(t *testing.T) {
	engine := NewEngine(slog.Default())
	t.Cleanup(func() { engine.Close() })
	if err := engine.LoadPrefixes("testdata"); err != nil {
		t.Fatal(err)
	}
	city, err := engine.Lookup("2001:218::1234")
	if err != nil || city.Country != "JP" || city.Timezone != "Asia/Tokyo" || city.Latitude == 0 || len(city.Sources) == 0 || city.BuildDate.IsZero() {
		t.Fatalf("city: %+v %v", city, err)
	}
	asn, err := engine.Lookup("::ffff:1.0.0.1")
	if err != nil || asn.ASN != 15169 || asn.ASOrg != "Google Inc." || len(asn.Sources) == 0 {
		t.Fatalf("asn: %+v %v", asn, err)
	}
	unknown, err := engine.Lookup("192.0.2.1")
	if err != nil || unknown.Country != "UNKNOWN" || unknown.ASN != 0 || len(unknown.Sources) != 0 || !unknown.BuildDate.IsZero() {
		t.Fatalf("unknown: %+v %v", unknown, err)
	}
	broken := filepath.Join(t.TempDir(), "broken.mmdb")
	if err := os.WriteFile(broken, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := engine.LoadPrefixes(broken); err == nil {
		t.Fatal("broken MMDB accepted")
	}
	lastGood, _ := engine.Lookup("1.0.0.1")
	if lastGood.ASN != 15169 {
		t.Fatal("failed reload erased last-good MMDB")
	}
	prefix := filepath.Join(t.TempDir(), "prefix.json")
	if err := os.WriteFile(prefix, []byte(`[{"cidr":"203.0.113.0/24","asn":64501}]`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := engine.LoadPrefixes(prefix); err != nil {
		t.Fatal(err)
	}
	cleared, _ := engine.Lookup("1.0.0.1")
	if cleared.ASN != 0 || engine.Health()["mmdb_sources"].(int) != 0 {
		t.Fatal("source switch retained old MMDB")
	}
	if err := engine.LoadPrefixes("testdata"); err != nil {
		t.Fatal(err)
	}
	cleared, _ = engine.Lookup("203.0.113.1")
	if cleared.ASN != 0 || engine.Health()["prefix_records"].(int) != 0 {
		t.Fatal("source switch retained old prefix dataset")
	}
}
