package routing

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func selection() Selection {
	return Selection{Selected: "fi", Addresses: map[string]string{"fi.example": "fi", "nl.example": "nl"}}
}
func TestURIFormatsPreserveUnknown(t *testing.T) {
	raw := []byte("vless://secret@fi.example:443?security=reality&future=1#one\ntrojan://secret@nl.example:443#two\nfuture://secret@unknown.example:8443?future=true#three\n")
	for _, encoded := range []bool{false, true} {
		input := raw
		if encoded {
			input = []byte(base64.StdEncoding.EncodeToString(raw))
		}
		out, err := Transform(input, selection())
		if err != nil {
			t.Fatal(err)
		}
		if encoded {
			out, err = base64.StdEncoding.DecodeString(string(out))
			if err != nil {
				t.Fatal(err)
			}
		}
		if strings.Contains(string(out), "nl.example") || !strings.Contains(string(out), "future://") || !strings.Contains(string(out), "future=1") {
			t.Fatalf("unexpected transform %s", out)
		}
	}
}
func TestJSONAndYAMLRepairSelectors(t *testing.T) {
	cases := []string{
		`{"outbounds":[{"type":"vless","tag":"fi","server":"fi.example","custom":{"future":42}},{"type":"trojan","tag":"nl","server":"nl.example"},{"type":"selector","tag":"select","outbounds":["fi","nl"],"default":"nl"}]}`,
		"proxies:\n  - {name: fi, type: future, server: fi.example, future: 42}\n  - {name: nl, type: vless, server: nl.example}\nproxy-groups:\n  - {name: select, type: select, proxies: [fi, nl]}\n",
	}
	for _, raw := range cases {
		out, err := Transform([]byte(raw), selection())
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(out), "nl.example") || !strings.Contains(string(out), "future") {
			t.Fatalf("unexpected result: %s", out)
		}
	}
}
func TestReferencedExcludedOutboundBypasses(t *testing.T) {
	raw := []byte(`{"outbounds":[{"tag":"fi","settings":{"vnext":[{"address":"fi.example"}]}},{"tag":"nl","settings":{"vnext":[{"address":"nl.example"}]}}],"routing":{"rules":[{"outboundTag":"nl"}]}}`)
	if _, err := Transform(raw, selection()); err == nil {
		t.Fatal("expected unsafe named route to reject transform")
	}
}
func TestProfilesAndNumbers(t *testing.T) {
	raw := []byte(`[{"outbounds":[{"protocol":"vless","settings":{"vnext":[{"address":"fi.example","port":443,"future":9007199254740993}]}}]},{"outbounds":[{"protocol":"future","settings":{"vnext":[{"address":"nl.example"}]}}]}]`)
	out, err := Transform(raw, selection())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "9007199254740993") {
		t.Fatal("precision lost")
	}
	var profiles []json.RawMessage
	if json.Unmarshal(out, &profiles) != nil || len(profiles) != 1 {
		t.Fatal(string(out))
	}
}
func TestOpaqueAndNoEmpty(t *testing.T) {
	opaque := []byte(`{"future":"payload"}`)
	if _, err := Transform(opaque, selection()); err == nil {
		t.Fatal("unrecognized representation should signal bypass")
	}
	if _, err := Transform([]byte("vless://secret@nl.example:443"), selection()); err == nil {
		t.Fatal("empty subscription should signal bypass")
	}
}
func FuzzTransform(f *testing.F) {
	for _, seed := range []string{"", "vless://x@fi.example:443", `{"outbounds":[]}`, "proxies: []", "%%%"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, value string) { _, _ = Transform([]byte(value), selection()) })
}
