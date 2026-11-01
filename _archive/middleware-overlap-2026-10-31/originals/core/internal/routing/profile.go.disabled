package routing

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// ValidateXrayProfile recognizes the Xray shape without restricting extension
// fields or protocols. sing-box uses type/route rather than protocol/routing.
// RawMessage keeps number tokens and every nested value intact.
func ValidateXrayProfile(raw []byte) error {
	if len(raw) == 0 || len(raw) > maxInjectionContent || !json.Valid(raw) {
		return fmt.Errorf("profile must be one valid JSON object, at most 256 KiB")
	}
	if !isXrayProfile(raw) {
		return fmt.Errorf("expected Xray profile: nonempty outbounds with protocol; sing-box type/route is unsupported")
	}
	return nil
}

func isXrayProfile(raw []byte) bool {
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil || obj == nil {
		return false
	}
	var outbounds []map[string]json.RawMessage
	if json.Unmarshal(obj["outbounds"], &outbounds) != nil || len(outbounds) == 0 {
		return false
	}
	for _, outbound := range outbounds {
		var protocol string
		if json.Unmarshal(outbound["protocol"], &protocol) != nil || protocol == "" || outbound["type"] != nil {
			return false
		}
	}
	return true
}

// ApplyXrayProfiles requires an explicit upstream Xray JSON request as well as
// structural recognition. It never merges profiles or deduplicates their tags.
func ApplyXrayProfiles(raw []byte, rules []Injection, explicitXray bool) ([]byte, error) {
	if !explicitXray || len(rules) == 0 || !json.Valid(raw) {
		return raw, nil
	}
	if len(raw) > maxSubscriptionSize || len(rules) > maxInjectionRules {
		return nil, fmt.Errorf("profile injection limit exceeded")
	}
	var profiles []json.RawMessage
	trimmed := bytes.TrimSpace(raw)
	if trimmed[0] == '{' {
		profiles = []json.RawMessage{json.RawMessage(trimmed)}
	} else if json.Unmarshal(trimmed, &profiles) != nil || len(profiles) == 0 {
		return raw, nil
	}
	for _, profile := range profiles {
		if !isXrayProfile(profile) {
			return raw, nil
		}
	}
	added := false
	for _, rule := range rules {
		if !rule.Enabled || rule.RuleType != "xray-profile" {
			continue
		}
		// A damaged persisted rule is ignored without exposing its contents.
		if rule.Format != "xray-json" || rule.Path != "" || ValidateXrayProfile([]byte(rule.Content)) != nil {
			continue
		}
		profiles = append(profiles, json.RawMessage(rule.Content))
		added = true
	}
	if !added {
		return raw, nil
	}
	out, err := json.Marshal(profiles)
	if len(out) > maxSubscriptionSize {
		return nil, fmt.Errorf("profile injection result exceeds size limit")
	}
	return out, err
}
