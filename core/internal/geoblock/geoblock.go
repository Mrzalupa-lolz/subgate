// Package geoblock implements geo-blocking rules (F7)
package geoblock

import (
	"log/slog"
	"sync"
)

// RuleType represents the type of geo-rule
type RuleType string

const (
	RuleTypeCountry RuleType = "country"
	RuleTypeASN     RuleType = "asn"
	RuleTypeNetType RuleType = "network_type"
	RuleTypeCIDR    RuleType = "cidr"
)

// Action represents the action for a matched rule
type Action string

const (
	ActionAllow Action = "allow"
	ActionDeny  Action = "deny"
)

// UnknownBehavior defines how to handle unknown IPs
type UnknownBehavior string

const (
	UnknownBehaviorAllow UnknownBehavior = "allow"
	UnknownBehaviorDeny  UnknownBehavior = "deny"
	UnknownBehaviorGrace UnknownBehavior = "grace"
)

// BlockResponse defines how to respond to blocked requests
type BlockResponse string

const (
	BlockResponse403     BlockResponse = "403"
	BlockResponse404Mime BlockResponse = "404_mime"
	BlockResponseEmpty   BlockResponse = "empty"
	BlockResponseDecoy   BlockResponse = "decoy"
)

// Rule represents a geo-blocking rule
type Rule struct {
	ID       string
	Type     RuleType
	Action   Action
	Values   []interface{} // Countries, ASNs, CIDR blocks, etc.
	PolicyID string
	Enabled  bool
}

// Engine performs geo-blocking checks
type Engine struct {
	rules           []Rule
	exemptions      map[string]bool // user_id -> exempted
	unknownBehavior UnknownBehavior
	blockResponse   BlockResponse
	mu              sync.RWMutex
	logger          *slog.Logger
}

// NewEngine creates a new geo-blocking engine
func NewEngine(logger *slog.Logger) *Engine {
	return &Engine{
		rules:           []Rule{},
		exemptions:      make(map[string]bool),
		unknownBehavior: UnknownBehaviorAllow,
		blockResponse:   BlockResponse403,
		logger:          logger,
	}
}

// AddRule adds a new geo-blocking rule
func (e *Engine) AddRule(rule Rule) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.rules = append(e.rules, rule)
	e.logger.Info("Geo-block rule added",
		slog.String("id", rule.ID),
		slog.String("type", string(rule.Type)),
		slog.String("action", string(rule.Action)),
	)
}

// RemoveRule removes a geo-blocking rule by ID
func (e *Engine) RemoveRule(ruleID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	for i, rule := range e.rules {
		if rule.ID == ruleID {
			e.rules = append(e.rules[:i], e.rules[i+1:]...)
			e.logger.Info("Geo-block rule removed", slog.String("id", ruleID))
			return
		}
	}
}

// CheckRequest checks if a request should be blocked
func (e *Engine) CheckRequest(clientIP, country string, asn int, networkType, userID string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Check exemptions
	if e.exemptions[userID] {
		return false // Not blocked
	}

	// Check rules in order
	for _, rule := range e.rules {
		if !rule.Enabled {
			continue
		}

		matched := false

		switch rule.Type {
		case RuleTypeCountry:
			for _, val := range rule.Values {
				if countryStr, ok := val.(string); ok && countryStr == country {
					matched = true
					break
				}
			}

		case RuleTypeASN:
			for _, val := range rule.Values {
				if asnInt, ok := val.(int); ok && asnInt == asn {
					matched = true
					break
				}
			}

		case RuleTypeNetType:
			for _, val := range rule.Values {
				if netTypeStr, ok := val.(string); ok && netTypeStr == networkType {
					matched = true
					break
				}
			}
		}

		if matched {
			if rule.Action == ActionDeny {
				return true // Blocked
			} else if rule.Action == ActionAllow {
				return false // Not blocked
			}
		}
	}

	// Handle unknown IP behavior
	if country == "" || country == "UNKNOWN" {
		switch e.unknownBehavior {
		case UnknownBehaviorDeny:
			return true
		case UnknownBehaviorGrace:
			// Log but allow
			e.logger.Warn("Unknown IP allowed (grace)", slog.String("ip", clientIP))
			return false
		default:
			return false
		}
	}

	return false // Not blocked by default
}

// SetUnknownBehavior sets how to handle unknown IPs
func (e *Engine) SetUnknownBehavior(behavior UnknownBehavior) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.unknownBehavior = behavior
	e.logger.Info("Unknown IP behavior set", slog.String("behavior", string(behavior)))
}

// SetBlockResponse sets the block response type
func (e *Engine) SetBlockResponse(response BlockResponse) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.blockResponse = response
	e.logger.Info("Block response set", slog.String("response", string(response)))
}

// ExemptUser adds a user to the exemption list
func (e *Engine) ExemptUser(userID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.exemptions[userID] = true
	e.logger.Info("User exempted from geo-block", slog.String("user_id", userID))
}

// RemoveExemption removes a user from the exemption list
func (e *Engine) RemoveExemption(userID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	delete(e.exemptions, userID)
	e.logger.Info("User exemption removed", slog.String("user_id", userID))
}

// GetExemptions returns the list of exempted users
func (e *Engine) GetExemptions() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var list []string
	for userID := range e.exemptions {
		list = append(list, userID)
	}

	return list
}

// GetRules returns all rules
func (e *Engine) GetRules() []Rule {
	e.mu.RLock()
	defer e.mu.RUnlock()

	rules := make([]Rule, len(e.rules))
	copy(rules, e.rules)

	return rules
}

// Statistics returns geo-block statistics
func (e *Engine) Statistics() map[string]interface{} {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return map[string]interface{}{
		"total_rules":      len(e.rules),
		"exempted_users":   len(e.exemptions),
		"unknown_behavior": string(e.unknownBehavior),
		"block_response":   string(e.blockResponse),
	}
}
