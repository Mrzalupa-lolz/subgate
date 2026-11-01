package routing

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestFullXrayProfilePreservesNestedValues(t *testing.T) {
	profile, err := os.ReadFile("testdata/warp-profile.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateXrayProfile(profile); err != nil {
		t.Fatal(err)
	}
	rule := Injection{ID: "warp", RuleType: "xray-profile", Format: "xray-json", Content: string(profile), Enabled: true}
	original := `{"remarks":"existing","outbounds":[{"protocol":"future-protocol","settings":{"value":900719925474099312345}}]}`
	for _, raw := range []string{original, "[" + original + "]"} {
		out, err := ApplyXrayProfiles([]byte(raw), []Injection{rule}, true)
		if err != nil {
			t.Fatal(err)
		}
		var profiles []json.RawMessage
		if json.Unmarshal(out, &profiles) != nil || len(profiles) != 2 {
			t.Fatal("expected two profiles")
		}
		// RawMessage compacting may escape HTML but must not change any value,
		// numeric token, or order of array elements (including related outbounds).
		compact := func(raw []byte) []byte {
			var b bytes.Buffer
			if err := json.Compact(&b, raw); err != nil {
				t.Fatal(err)
			}
			return b.Bytes()
		}
		var expected json.RawMessage = profile
		encoded, _ := json.Marshal(expected)
		if !bytes.Equal(compact(profiles[1]), encoded) || !bytes.Equal(profiles[0], []byte(original)) {
			t.Fatal("profile values or numeric tokens changed")
		}
	}
	for _, raw := range []string{original, "[" + original + "]", `vless://test@example.com:443`, `dmxlc3M6Ly90ZXN0QGV4YW1wbGUuY29tOjQ0Mw==`, "proxies: []", `{"outbounds":[{"type":"direct"}],"route":{}}`, `{"outbounds":[]}`, `[{},` + original + `]`, "<html>error</html>", "[]", "null", "{}"} {
		for _, explicit := range []bool{false, true} {
			if explicit && (raw == original || raw == "["+original+"]") {
				continue
			}
			out, err := ApplyXrayProfiles([]byte(raw), []Injection{rule}, explicit)
			if err != nil || string(out) != raw {
				t.Fatalf("unsupported format changed: explicit=%v", explicit)
			}
		}
	}
	for _, rules := range [][]Injection{nil, {{RuleType: "xray-profile", Enabled: false, Content: string(profile)}}, {{RuleType: "xray-profile", Format: "xray-json", Enabled: true, Content: "broken"}}} {
		out, err := ApplyXrayProfiles([]byte(original), rules, true)
		if err != nil || string(out) != original {
			t.Fatal("disabled/deleted/corrupt rule changed source")
		}
	}
	// Same remarks and tags are independent in different full profiles.
	out, err := ApplyXrayProfiles([]byte(original), []Injection{rule, rule}, true)
	var profiles []json.RawMessage
	if err != nil || json.Unmarshal(out, &profiles) != nil || len(profiles) != 3 {
		t.Fatal("same-name profiles were deduplicated")
	}
}

func TestProfileValidationAndLimits(t *testing.T) {
	for _, raw := range []string{"", " ", "{}", "[]", `{"outbounds":[{"type":"direct"}]}`, `{"outbounds":[{"protocol":"freedom"}]} {}`, strings.Repeat(" ", 256<<10) + "{}"} {
		if ValidateXrayProfile([]byte(raw)) == nil {
			t.Fatal("invalid profile accepted")
		}
	}
	if _, err := ApplyXrayProfiles([]byte(`{"outbounds":[{"protocol":"freedom"}]}`), make([]Injection, 101), true); err == nil {
		t.Fatal("rule limit ignored")
	}
	base := `{"outbounds":[{"protocol":"freedom"}],"padding":"` + strings.Repeat("a", (8<<20)-100) + `"}`
	rule := Injection{RuleType: "xray-profile", Format: "xray-json", Enabled: true, Content: `{"outbounds":[{"protocol":"freedom"}],"padding":"` + strings.Repeat("b", 200) + `"}`}
	if _, err := ApplyXrayProfiles([]byte(base), []Injection{rule}, true); err == nil {
		t.Fatal("result size limit ignored")
	}
}

func TestLegacyRootArrayLengthMutation(t *testing.T) {
	for _, tt := range []struct{ operation, content, want string }{
		{"append", `{"tag":"b"}`, `[{"tag":"a"},{"tag":"b"}]`},
		{"prepend", `{"tag":"b"}`, `[{"tag":"b"},{"tag":"a"}]`},
		{"remove", `{"tag":"b"}`, `[{"tag":"a"}]`},
	} {
		raw := `[{"tag":"a"}]`
		if tt.operation == "remove" {
			raw = `[{"tag":"a"},{"tag":"b"}]`
		}
		out, err := ApplyInjections([]byte(raw), []Injection{{RuleType: tt.operation, Content: tt.content, Enabled: true}})
		if err != nil || string(out) != tt.want {
			t.Fatalf("%s: %s %v", tt.operation, out, err)
		}
	}
}
