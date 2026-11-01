// Package routing filters recognized subscription endpoints while preserving
// unknown protocols, credentials, transport fields and unmapped entries.
package routing

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"gopkg.in/yaml.v3"
)

type Selection struct {
	Selected  string
	Addresses map[string]string
	Excluded  map[string]bool
}

func (s Selection) allowed(address string) bool {
	node, known := s.Addresses[strings.ToLower(strings.TrimSuffix(address, "."))]
	return !known || (!s.Excluded[node] && (s.Selected == "" || node == s.Selected))
}
func address(entry map[string]any) string {
	for _, key := range []string{"server", "address"} {
		if v, ok := entry[key].(string); ok {
			return v
		}
	}
	settings, _ := entry["settings"].(map[string]any)
	for _, key := range []string{"vnext", "servers"} {
		if list, ok := settings[key].([]any); ok && len(list) > 0 {
			if first, ok := list[0].(map[string]any); ok {
				if v, ok := first["address"].(string); ok {
					return v
				}
			}
		}
	}
	return ""
}
func Transform(raw []byte, selection Selection) ([]byte, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return raw, nil
	}
	if trimmed[0] == '{' || trimmed[0] == '[' {
		var root any
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&root); err != nil {
			return nil, err
		}
		transformed, changed, err := transformJSON(root, selection)
		if err != nil {
			return nil, err
		}
		if !changed {
			return raw, nil
		}
		return json.Marshal(transformed)
	}
	// YAML proxy groups are repaired together with the proxy list.
	if bytes.Contains(trimmed, []byte("proxies:")) || bytes.Contains(trimmed, []byte("proxy-groups:")) {
		var root map[string]any
		if err := yaml.Unmarshal(raw, &root); err != nil {
			return nil, err
		}
		transformed, changed, err := transformJSON(root, selection)
		if err != nil {
			return nil, err
		}
		if !changed {
			return raw, nil
		}
		return yaml.Marshal(transformed)
	}
	encoded := false
	data := trimmed
	if !bytes.Contains(data, []byte("://")) {
		decoded, err := decodeBase64(string(data))
		if err != nil || !bytes.Contains(decoded, []byte("://")) {
			return nil, fmt.Errorf("unrecognized subscription representation")
		}
		data = decoded
		encoded = true
	}
	lines := strings.Split(string(data), "\n")
	kept := []string{}
	removed := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		addr := ""
		if strings.HasPrefix(line, "vmess://") {
			payload, err := decodeBase64(strings.TrimPrefix(line, "vmess://"))
			if err == nil {
				var vmess map[string]any
				if json.Unmarshal(payload, &vmess) == nil {
					addr, _ = vmess["add"].(string)
				}
			}
		} else {
			u, err := url.Parse(line)
			if err == nil {
				addr = u.Hostname()
			}
		}
		if addr != "" && !selection.allowed(addr) {
			removed++
			continue
		}
		kept = append(kept, line)
	}
	if removed == 0 {
		return raw, nil
	}
	if len(kept) == 0 {
		return nil, fmt.Errorf("routing would empty subscription")
	}
	out := []byte(strings.Join(kept, "\n") + "\n")
	if encoded {
		out = []byte(base64.StdEncoding.EncodeToString(out))
	}
	return out, nil
}
func decodeBase64(s string) ([]byte, error) {
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if raw, err := encoding.DecodeString(s); err == nil {
			return raw, nil
		}
	}
	return nil, fmt.Errorf("invalid base64")
}
func transformJSON(root any, s Selection) (any, bool, error) {
	if list, ok := root.([]any); ok {
		kept := []any{}
		changed := false
		for _, item := range list {
			profile, ok := item.(map[string]any)
			if !ok {
				kept = append(kept, item)
				continue
			}
			addr := address(profile)
			if addr == "" {
				if outbounds, ok := profile["outbounds"].([]any); ok {
					for _, out := range outbounds {
						if out, ok := out.(map[string]any); ok {
							addr = address(out)
							if addr != "" {
								break
							}
						}
					}
				}
			}
			if addr != "" && !s.allowed(addr) {
				changed = true
				continue
			}
			kept = append(kept, item)
		}
		if changed && len(kept) == 0 {
			return nil, false, fmt.Errorf("routing would empty subscription")
		}
		return kept, changed, nil
	}
	obj, ok := root.(map[string]any)
	if !ok {
		return nil, false, fmt.Errorf("subscription JSON is not object or array")
	}
	key := "outbounds"
	list, ok := obj[key].([]any)
	if !ok {
		key = "proxies"
		list, ok = obj[key].([]any)
	}
	if !ok {
		return root, false, fmt.Errorf("subscription has no endpoint list")
	}
	kept := []any{}
	removed := map[string]bool{}
	changed := false
	retainedMapped := 0
	for _, item := range list {
		entry, ok := item.(map[string]any)
		if !ok {
			kept = append(kept, item)
			continue
		}
		addr := address(entry)
		if addr != "" && !s.allowed(addr) {
			changed = true
			tag, _ := entry["tag"].(string)
			if tag == "" {
				tag, _ = entry["name"].(string)
			}
			if tag != "" {
				removed[tag] = true
			}
			continue
		}
		if addr != "" && s.allowed(addr) {
			retainedMapped++
		}
		kept = append(kept, item)
	}
	if !changed {
		return root, false, nil
	}
	if len(kept) == 0 || retainedMapped == 0 {
		return nil, false, fmt.Errorf("routing would remove all proxy endpoints")
	}
	if key == "proxies" {
		groups, _ := obj["proxy-groups"].([]any)
		for _, group := range groups {
			g, ok := group.(map[string]any)
			if !ok {
				continue
			}
			if refs, ok := g["proxies"].([]any); ok {
				filtered := filterRefs(refs, removed)
				if len(filtered) == 0 {
					return nil, false, fmt.Errorf("routing would empty proxy group")
				}
				g["proxies"] = filtered
			}
		}
	} else {
		// Named routing rules cannot be rewritten safely without changing intent.
		// Refuse the transform when they target removed outbounds. Selectors can be
		// filtered because their purpose is to balance among interchangeable nodes.
		if route, ok := obj["routing"].(map[string]any); ok {
			if rules, ok := route["rules"].([]any); ok {
				for _, rule := range rules {
					if r, ok := rule.(map[string]any); ok {
						if tag, ok := r["outboundTag"].(string); ok && removed[tag] {
							return nil, false, fmt.Errorf("routing rule references excluded outbound")
						}
					}
				}
			}
			if balancers, ok := route["balancers"].([]any); ok {
				for _, balancer := range balancers {
					if b, ok := balancer.(map[string]any); ok {
						if refs, ok := b["selector"].([]any); ok {
							filtered := filterRefs(refs, removed)
							if len(filtered) == 0 {
								return nil, false, fmt.Errorf("balancer would have no selector")
							}
							b["selector"] = filtered
						}
					}
				}
			}
		}
		for _, item := range kept {
			if out, ok := item.(map[string]any); ok {
				if typ, _ := out["type"].(string); typ == "selector" || typ == "urltest" {
					if refs, ok := out["outbounds"].([]any); ok {
						filtered := filterRefs(refs, removed)
						if len(filtered) == 0 {
							return nil, false, fmt.Errorf("selector would have no outbounds")
						}
						out["outbounds"] = filtered
					}
					if def, _ := out["default"].(string); removed[def] {
						delete(out, "default")
					}
				}
			}
		}
		if containsRemovedReference(obj, removed) {
			return nil, false, fmt.Errorf("config contains reference to an excluded outbound")
		}
	}
	obj[key] = kept
	return obj, true, nil
}
func filterRefs(refs []any, removed map[string]bool) []any {
	out := []any{}
	for _, ref := range refs {
		tag, _ := ref.(string)
		if !removed[tag] {
			out = append(out, ref)
		}
	}
	return out
}
func containsRemovedReference(value any, removed map[string]bool) bool {
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			if key == "outbounds" {
				if _, list := item.([]any); list {
					continue
				}
			}
			if key == "outbound" || key == "detour" || key == "dialerProxy" || key == "final" {
				if tag, ok := item.(string); ok && removed[tag] {
					return true
				}
			}
			if containsRemovedReference(item, removed) {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if containsRemovedReference(item, removed) {
				return true
			}
		}
	}
	return false
}
