package routing

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestApplyInjectionsURIAppendPrependRemove(t *testing.T) {
	raw := []byte("vless://one@example.com:443#one\ntrojan://two@example.com:443#two\n")

	out, err := ApplyInjections(raw, []Injection{{RuleType: "prepend", Content: "ss://zero@example.com:443#zero", Enabled: true}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(out), "ss://zero@example.com:443#zero\nvless://one") {
		t.Fatalf("prepend order was not preserved: %q", out)
	}

	out, err = ApplyInjections(out, []Injection{{RuleType: "append", Content: "vless://three@example.com:443#three", Enabled: true}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(out), "vless://three@example.com:443#three\n") {
		t.Fatalf("append result missing: %q", out)
	}

	out, err = ApplyInjections(out, []Injection{{RuleType: "remove", Content: "trojan://two@example.com:443#two", Enabled: true}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "two@example.com") {
		t.Fatalf("remove result still contains target: %q", out)
	}
}

func TestApplyInjectionsBase64AndDisabled(t *testing.T) {
	raw := []byte("vless://one@example.com:443#one\n")
	encoded := []byte(base64.StdEncoding.EncodeToString(raw))
	out, err := ApplyInjections(encoded, []Injection{
		{ID: "disabled", RuleType: "append", Content: "vless://ignored@example.com:443", Enabled: false},
		{RuleType: "append", Format: "base64", Content: "trojan://two@example.com:443#two", Enabled: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(string(out))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(decoded), "ignored@example.com") || !strings.Contains(string(decoded), "two@example.com") {
		t.Fatalf("base64 injection result: %q", decoded)
	}
}

func TestApplyInjectionsJSONPointerAndEndpointPatch(t *testing.T) {
	raw := []byte(`{"outbounds":[{"tag":"one","server":"one.example"},{"tag":"two","server":"two.example"}],"routing":{}}`)
	out, err := ApplyInjections(raw, []Injection{
		{RuleType: "patch", Path: "/routing/domainStrategy", Content: `"AsIs"`, Enabled: true},
		{RuleType: "patch", Content: `{"security":"tls"}`, Enabled: true},
		{RuleType: "remove", Path: "/outbounds/1", Content: `null`, Enabled: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	var value struct {
		Outbounds []map[string]any `json:"outbounds"`
		Routing   map[string]any   `json:"routing"`
	}
	if err := json.Unmarshal(out, &value); err != nil {
		t.Fatal(err)
	}
	if value.Routing["domainStrategy"] != "AsIs" || len(value.Outbounds) != 1 {
		t.Fatalf("unexpected JSON patch result: %s", out)
	}
	for _, endpoint := range value.Outbounds {
		if endpoint["security"] != "tls" {
			t.Fatalf("endpoint patch missing from %#v", endpoint)
		}
	}
}

func TestApplyInjectionsPreservesUnknownAndEnforcesLimits(t *testing.T) {
	raw := []byte("opaque subscription")
	out, err := ApplyInjections(raw, []Injection{{RuleType: "append", Format: "future-format", Content: "ignored", Enabled: true}})
	if err != nil || string(out) != string(raw) {
		t.Fatalf("unknown format should be unchanged: %q, %v", out, err)
	}

	rules := make([]Injection, maxInjectionRules+1)
	if _, err := ApplyInjections([]byte("vless://one@example.com:443"), rules); err == nil {
		t.Fatal("too many injection rules were accepted")
	}
	if _, err := ApplyInjections([]byte("vless://one@example.com:443"), []Injection{{RuleType: "append", Content: strings.Repeat("x", maxInjectionContent+1), Enabled: true}}); err == nil {
		t.Fatal("oversized injection was accepted")
	}
}
