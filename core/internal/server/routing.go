package server

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/subgate/subgate/core/internal/panel"
	"github.com/subgate/subgate/core/internal/routing"
)

type routingTier struct {
	ID         string   `json:"id"`
	Strategy   string   `json:"strategy"`
	Nodes      []string `json:"node_ids"`
	MinHealthy int      `json:"min_healthy"`
	Fallback   string   `json:"fallback_tier_id"`
	Enabled    bool     `json:"enabled"`
}

func (s *Server) transformSubscription(r *http.Request, data []byte, headers http.Header) ([]byte, string, error) {
	// Cache stores upstream bytes. Policy is evaluated for every request so geo,
	// pin and health changes take effect even on a subscription cache hit.
	var raw string
	err := s.db.QueryRow("SELECT data FROM sg_objects WHERE kind='users' AND json_extract(data,'$.short_uuid')=?", r.PathValue("shortUuid")).Scan(&raw)
	if err != nil {
		return data, "passthrough", nil
	}
	var user panel.User
	if json.Unmarshal([]byte(raw), &user) != nil {
		return data, "passthrough", nil
	}
	nodeData, err := s.objects("nodes")
	if err != nil {
		return data, "bypass", err
	}
	nodes := map[string]panel.Node{}
	addresses := map[string]string{}
	for _, entry := range nodeData {
		var n panel.Node
		if json.Unmarshal(entry, &n) == nil {
			nodes[n.ID] = n
			if n.IPAddress != "" {
				addresses[strings.ToLower(n.IPAddress)] = n.ID
			}
		}
	}
	// Shared addresses cannot identify a node unambiguously. Leave them untouched.
	for _, entry := range nodeData {
		var n panel.Node
		_ = json.Unmarshal(entry, &n)
		for _, other := range nodes {
			if n.ID != other.ID && strings.EqualFold(n.IPAddress, other.IPAddress) {
				delete(addresses, strings.ToLower(n.IPAddress))
			}
		}
	}
	hostData, _ := s.objects("hosts")
	ambiguous := map[string]bool{}
	for _, entry := range hostData {
		var h panel.Host
		if json.Unmarshal(entry, &h) != nil || h.Disabled || h.Address == "" {
			continue
		}
		key := strings.ToLower(strings.TrimSuffix(h.Address, "."))
		if len(h.Nodes) != 1 {
			ambiguous[key] = true
			continue
		}
		if old, ok := addresses[key]; ok && old != h.Nodes[0] {
			ambiguous[key] = true
		} else {
			addresses[key] = h.Nodes[0]
		}
	}
	for key := range ambiguous {
		delete(addresses, key)
	}
	selected, policy := "", ""
	excluded := s.excludedHealthNodes()
	pins, err := s.objects("pins")
	if err != nil {
		return data, "bypass", err
	}
	for _, entry := range pins {
		var pin struct {
			User    string `json:"user_id"`
			Node    string `json:"node_id"`
			Enabled bool   `json:"enabled"`
		}
		if json.Unmarshal(entry, &pin) == nil && pin.Enabled && pin.User == user.ID && nodes[pin.Node].Status == "online" && !excluded[pin.Node] {
			selected = pin.Node
			policy = "pin"
			break
		}
	}
	if selected == "" {
		policies, e := s.objects("policies")
		if e != nil {
			return data, "bypass", e
		}
		type rule struct {
			ID         string `json:"id"`
			Tier       string `json:"tier_id"`
			Priority   int    `json:"priority"`
			Enabled    bool   `json:"enabled"`
			Conditions struct {
				Countries []string `json:"countries"`
				ASNs      []string `json:"asns"`
				UserAgent string   `json:"user_agent"`
			} `json:"conditions"`
		}
		rules := []rule{}
		for _, entry := range policies {
			var p rule
			if json.Unmarshal(entry, &p) == nil && p.Enabled {
				rules = append(rules, p)
			}
		}
		sort.SliceStable(rules, func(i, j int) bool { return rules[i].Priority < rules[j].Priority })
		intel, _ := s.intel.Lookup(s.clientIP(r))
		for _, rule := range rules {
			c := rule.Conditions
			if c.UserAgent != "" && !strings.Contains(strings.ToLower(r.UserAgent()), strings.ToLower(c.UserAgent)) {
				continue
			}
			if len(c.Countries) > 0 && (intel == nil || !inStrings(c.Countries, intel.Country)) {
				continue
			}
			if len(c.ASNs) > 0 && (intel == nil || !inStrings(c.ASNs, strconv.Itoa(intel.ASN))) {
				continue
			}
			id, e := s.selectTierEligible(rule.Tier, nodes, map[string]bool{}, excluded)
			if e == nil {
				selected = id
				policy = "tier"
			} else {
				// All fallback tiers exhausted or unverifiable: preserve the panel
				// layout in panic mode, including opaque/unmapped endpoints.
				return data, "tier-unavailable", nil
			}
			break
		}
	}
	if selected == "" && len(excluded) == 0 {
		return data, "passthrough", nil
	}
	if selected != "" && s.quota != nil && s.quota.Remaining(selected, user.ID) == 0 {
		return data, "quota-exhausted", nil
	}
	transformed, err := routing.Transform(data, routing.Selection{Selected: selected, Addresses: addresses, Excluded: excluded})
	if err != nil {
		if len(excluded) == 0 {
			return transformed, policy, err
		}
		// No safe eligible layout (including all-quarantined and unsupported
		// references/formats): preserve the exact panel subscription.
		return data, "routing-bypass", nil
	}
	if selected == "" {
		return transformed, "health", nil
	}
	if rules := s.injectionRules(user.ID); len(rules) > 0 {
		transformed, err = routing.ApplyInjections(transformed, rules)
		if err != nil {
			return data, policy, err
		}
		policy += "+injection"
	}
	return transformed, policy, nil
}
func inStrings(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(value, want) {
			return true
		}
	}
	return false
}
func (s *Server) selectTier(id string, nodes map[string]panel.Node, visited map[string]bool) (string, error) {
	excluded := s.excludedHealthNodes()
	return s.selectTierEligible(id, nodes, visited, excluded)
}

func (s *Server) selectTierEligible(id string, nodes map[string]panel.Node, visited map[string]bool, excluded map[string]bool) (string, error) {
	if visited[id] {
		return "", fmt.Errorf("tier fallback cycle")
	}
	visited[id] = true
	var tier routingTier
	if err := s.object("tiers", id, &tier); err != nil || !tier.Enabled {
		return "", fmt.Errorf("tier unavailable")
	}
	healthy := []panel.Node{}
	for _, id := range tier.Nodes {
		if n, ok := nodes[id]; ok && n.Status == "online" && !excluded[id] {
			healthy = append(healthy, n)
		}
	}
	minimum := tier.MinHealthy
	if minimum < 1 {
		minimum = 1
	}
	if len(healthy) < minimum {
		if tier.Fallback != "" {
			return s.selectTierEligible(tier.Fallback, nodes, visited, excluded)
		}
		return "", fmt.Errorf("tier has too few healthy nodes")
	}
	switch tier.Strategy {
	case "random":
		return healthy[rand.IntN(len(healthy))].ID, nil
	case "roundRobin":
		s.roundMu.Lock()
		i := s.roundRobin[id]
		s.roundRobin[id] = i + 1
		s.roundMu.Unlock()
		return healthy[i%uint64(len(healthy))].ID, nil
	case "leastLoad":
		sort.SliceStable(healthy, func(i, j int) bool { return healthy[i].UsersOnline < healthy[j].UsersOnline })
		return healthy[0].ID, nil
	case "leastPing":
		// Ping is only available after an operator triggers a real probe.
		best := ""
		var latency int64
		for _, n := range healthy {
			var probe nodeProbe
			if s.object("health", n.ID, &probe) == nil {
				age := time.Since(probe.Timestamp)
				if probe.Success && (probe.Verified == "tcp" || probe.Verified == "tls" || probe.Verified == "ws" || probe.Verified == "httpupgrade" || probe.Verified == "grpc" || probe.Verified == "xhttp-options" || probe.Verified == "xhttp-h3-options") && age >= -time.Minute && age <= 10*time.Minute && probe.Duration >= 0 && (best == "" || probe.Duration < latency) {
					best = n.ID
					latency = probe.Duration
				}
			}
		}
		if best == "" {
			if tier.Fallback != "" {
				return s.selectTierEligible(tier.Fallback, nodes, visited, excluded)
			}
			return "", fmt.Errorf("no verified ping samples within 10 minutes")
		}
		return best, nil
	}
	return "", fmt.Errorf("unsupported tier strategy")
}
