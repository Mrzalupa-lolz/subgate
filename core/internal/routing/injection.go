package routing

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Injection is a validated, operator-authored subscription mutation.
type Injection struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	RuleType string `json:"rule_type"` // append, prepend, patch, remove
	Format   string `json:"format,omitempty"`
	Content  string `json:"content"`
	Path     string `json:"path,omitempty"` // JSON pointer for patch/remove
	Enabled  bool   `json:"enabled"`
}

const (
	maxInjectionRules   = 100
	maxInjectionContent = 256 << 10
	maxSubscriptionSize = 8 << 20
)

// ApplyInjections applies rules in their supplied order. Unknown formats are
// returned unchanged so a custom client never receives a corrupt response.
func ApplyInjections(raw []byte, rules []Injection) ([]byte, error) {
	if len(raw) > maxSubscriptionSize {
		return nil, fmt.Errorf("subscription exceeds %d bytes", maxSubscriptionSize)
	}
	if len(rules) > maxInjectionRules {
		return nil, fmt.Errorf("too many injection rules")
	}
	out := append([]byte(nil), raw...)
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		if len(rule.Content) > maxInjectionContent {
			return nil, fmt.Errorf("injection %q exceeds size limit", rule.ID)
		}
		var err error
		switch detectFormat(out, rule.Format) {
		case "uri-list":
			out, err = applyURI(out, rule)
		case "json":
			out, err = applyJSON(out, rule)
		case "yaml":
			out, err = applyYAML(out, rule)
		default:
			// Preserve opaque subscriptions. A rule explicitly tied to an
			// unknown format is ignored; it cannot be safely interpreted.
			continue
		}
		if err != nil {
			return nil, err
		}
		if len(out) > maxSubscriptionSize {
			return nil, fmt.Errorf("injection result exceeds size limit")
		}
	}
	return out, nil
}

func detectFormat(raw []byte, requested string) string {
	requested = strings.ToLower(strings.TrimSpace(requested))
	if requested != "" {
		switch requested {
		case "uri", "uri-list", "base64", "vless", "vmess", "trojan":
			return "uri-list"
		case "json", "xray-json", "sing-box":
			return "json"
		case "yaml", "yml", "mihomo":
			return "yaml"
		default:
			return "unknown"
		}
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		return "json"
	}
	if bytes.Contains(trimmed, []byte("proxies:")) || bytes.Contains(trimmed, []byte("proxy-groups:")) {
		return "yaml"
	}
	if bytes.Contains(trimmed, []byte("://")) {
		return "uri-list"
	}
	if decoded, err := decodeSubscriptionBase64(trimmed); err == nil && bytes.Contains(decoded, []byte("://")) {
		return "uri-list"
	}
	return "unknown"
}

func applyURI(raw []byte, rule Injection) ([]byte, error) {
	encoded := false
	data := bytes.TrimSpace(raw)
	if !bytes.Contains(data, []byte("://")) {
		decoded, err := decodeSubscriptionBase64(data)
		if err != nil {
			return raw, nil
		}
		data, encoded = decoded, true
	}
	lines := splitNonEmptyLines(string(data))
	content := splitNonEmptyLines(rule.Content)
	if len(content) == 0 {
		return raw, nil
	}
	valid := make([]string, 0, len(content))
	for _, line := range content {
		u, err := url.Parse(line)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return nil, fmt.Errorf("invalid URI injection")
		}
		valid = append(valid, line)
	}
	switch rule.RuleType {
	case "append":
		lines = append(lines, valid...)
	case "prepend":
		lines = append(valid, lines...)
	case "remove":
		remove := make(map[string]bool, len(valid))
		for _, line := range valid {
			remove[line] = true
		}
		kept := lines[:0]
		for _, line := range lines {
			if !remove[line] {
				kept = append(kept, line)
			}
		}
		lines = kept
	case "patch":
		return raw, nil
	default:
		return nil, fmt.Errorf("unsupported injection rule %q", rule.RuleType)
	}
	if len(lines) == 0 {
		return nil, fmt.Errorf("injection would empty URI subscription")
	}
	out := []byte(strings.Join(lines, "\n") + "\n")
	if encoded {
		out = []byte(base64.StdEncoding.EncodeToString(out))
	}
	return out, nil
}

func applyJSON(raw []byte, rule Injection) ([]byte, error) {
	var root any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		return nil, err
	}
	content, err := parseJSONContent(rule.Content)
	if err != nil {
		return nil, err
	}
	if rule.Path != "" && (rule.RuleType == "patch" || rule.RuleType == "remove") {
		if err := mutateJSONPointer(&root, rule.Path, content, rule.RuleType == "remove"); err != nil {
			return nil, err
		}
	} else {
		if err := mutateEndpointList(&root, content, rule.RuleType); err != nil {
			return nil, err
		}
	}
	return json.Marshal(root)
}

func applyYAML(raw []byte, rule Injection) ([]byte, error) {
	var root map[string]any
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return nil, err
	}
	content, err := parseYAMLContent(rule.Content)
	if err != nil {
		return nil, err
	}
	var anyRoot any = root
	if rule.Path != "" && (rule.RuleType == "patch" || rule.RuleType == "remove") {
		if err := mutateJSONPointer(&anyRoot, rule.Path, content, rule.RuleType == "remove"); err != nil {
			return nil, err
		}
	} else if err := mutateEndpointList(&anyRoot, content, rule.RuleType); err != nil {
		return nil, err
	}
	if converted, ok := anyRoot.(map[string]any); ok {
		root = converted
	}
	return yaml.Marshal(root)
}

func parseJSONContent(raw string) (any, error) {
	var v any
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&v); err != nil {
		return nil, fmt.Errorf("invalid JSON injection content: %w", err)
	}
	return v, nil
}
func parseYAMLContent(raw string) (any, error) {
	var v any
	if err := yaml.Unmarshal([]byte(raw), &v); err != nil {
		return nil, err
	}
	return normalizeYAML(v), nil
}
func normalizeYAML(value any) any {
	switch v := value.(type) {
	case map[string]interface{}:
		out := map[string]any{}
		for key, item := range v {
			out[key] = normalizeYAML(item)
		}
		return out
	case []interface{}:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = normalizeYAML(item)
		}
		return out
	default:
		return value
	}
}
func mutateEndpointList(root *any, content any, operation string) error {
	obj, isObj := (*root).(map[string]any)
	if !isObj {
		list, ok := (*root).([]any)
		if !ok {
			return fmt.Errorf("subscription root is not an object or array")
		}
		return mutateList(&list, content, operation)
	}
	for _, key := range []string{"outbounds", "proxies"} {
		if list, ok := obj[key].([]any); ok {
			if err := mutateList(&list, content, operation); err != nil {
				return err
			}
			obj[key] = list
			return nil
		}
	}
	return fmt.Errorf("subscription has no endpoint list")
}
func mutateList(list *[]any, content any, operation string) error {
	var additions []any
	switch v := content.(type) {
	case []any:
		additions = v
	default:
		additions = []any{v}
	}
	switch operation {
	case "append":
		*list = append(*list, additions...)
	case "prepend":
		*list = append(append([]any{}, additions...), (*list)...)
	case "remove":
		kept := (*list)[:0]
		for _, item := range *list {
			if !jsonEquivalentAny(item, additions) {
				kept = append(kept, item)
			}
		}
		*list = kept
	case "patch":
		for i, item := range *list {
			if obj, ok := item.(map[string]any); ok {
				if patch, ok := content.(map[string]any); ok {
					for key, value := range patch {
						obj[key] = value
					}
					(*list)[i] = obj
				}
			}
		}
	default:
		return fmt.Errorf("unsupported injection rule %q", operation)
	}
	if len(*list) == 0 {
		return fmt.Errorf("injection would empty endpoint list")
	}
	return nil
}
func jsonEquivalentAny(item any, candidates []any) bool {
	for _, candidate := range candidates {
		a, _ := json.Marshal(item)
		b, _ := json.Marshal(candidate)
		if bytes.Equal(a, b) {
			return true
		}
	}
	return false
}

func mutateJSONPointer(root *any, pointer string, value any, remove bool) error {
	if pointer == "" {
		if remove {
			return fmt.Errorf("cannot remove document root")
		}
		*root = value
		return nil
	}
	parts := strings.Split(strings.TrimPrefix(pointer, "/"), "/")
	for i := range parts {
		parts[i] = strings.ReplaceAll(strings.ReplaceAll(parts[i], "~1", "/"), "~0", "~")
	}
	updated, err := mutatePointerNode(*root, parts, value, remove)
	if err != nil {
		return err
	}
	*root = updated
	return nil
}

func mutatePointerNode(node any, parts []string, value any, remove bool) (any, error) {
	if len(parts) == 0 {
		if remove {
			return nil, fmt.Errorf("cannot remove document root")
		}
		return value, nil
	}
	part := parts[0]
	if len(parts) == 1 {
		switch current := node.(type) {
		case map[string]any:
			if remove {
				delete(current, part)
			} else {
				current[part] = value
			}
			return current, nil
		case []any:
			if part == "-" && !remove {
				return append(current, value), nil
			}
			index, err := pointerIndex(part, len(current))
			if err != nil {
				return nil, err
			}
			if remove {
				return append(current[:index], current[index+1:]...), nil
			}
			current[index] = value
			return current, nil
		default:
			return nil, fmt.Errorf("JSON pointer target is not an object or array")
		}
	}
	switch current := node.(type) {
	case map[string]any:
		next, ok := current[part]
		if !ok {
			return nil, fmt.Errorf("JSON pointer path does not exist")
		}
		updated, err := mutatePointerNode(next, parts[1:], value, remove)
		if err != nil {
			return nil, err
		}
		current[part] = updated
		return current, nil
	case []any:
		index, err := pointerIndex(part, len(current))
		if err != nil {
			return nil, err
		}
		updated, err := mutatePointerNode(current[index], parts[1:], value, remove)
		if err != nil {
			return nil, err
		}
		current[index] = updated
		return current, nil
	default:
		return nil, fmt.Errorf("JSON pointer parent is not an object or array")
	}
}

func pointerIndex(raw string, length int) (int, error) {
	if raw == "" {
		return 0, fmt.Errorf("JSON pointer array index is empty")
	}
	index, err := strconv.Atoi(raw)
	if err != nil || index < 0 || index >= length {
		return 0, fmt.Errorf("JSON pointer array index is invalid")
	}
	return index, nil
}

func splitNonEmptyLines(raw string) []string {
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}
func decodeSubscriptionBase64(raw []byte) ([]byte, error) {
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if decoded, err := enc.DecodeString(string(raw)); err == nil {
			return decoded, nil
		}
	}
	return nil, fmt.Errorf("invalid base64")
}
