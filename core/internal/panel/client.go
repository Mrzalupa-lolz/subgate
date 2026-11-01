// Package panel implements the Remnawave v3 REST contract.
package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-faster/jx"
	"github.com/subgate/subgate/core/internal/panel/generated"
	"github.com/subgate/subgate/pkg/wire"
)

type User struct {
	ID                string `json:"id"`
	ShortUUID         string `json:"short_uuid"`
	Username          string `json:"username"`
	Email             string `json:"email"`
	Status            string `json:"status"`
	TrafficLimitBytes int64  `json:"traffic_limit_bytes"`
	UsedTrafficBytes  int64  `json:"used_traffic_bytes"`
	ExpireAt          string `json:"expire_at"`
}
type Inbound struct {
	Tag      string           `json:"tag"`
	Type     string           `json:"type"`
	Network  string           `json:"network"`
	Security string           `json:"security"`
	Port     int              `json:"port"`
	HTTP     *wire.HealthHTTP `json:"http,omitempty"`
}
type Host struct {
	ID       string   `json:"uuid"`
	Address  string   `json:"address"`
	Port     int      `json:"port"`
	Nodes    []string `json:"nodes"`
	Disabled bool     `json:"isDisabled"`
	Remark   string   `json:"remark"`
}

func (c *Client) ListHostsContext(ctx context.Context) ([]Host, error) {
	raw, err := c.Request(ctx, "GET", "/api/hosts", nil)
	if err != nil {
		return nil, err
	}
	var hosts []Host
	if err = json.Unmarshal(raw, &hosts); err != nil {
		return nil, err
	}
	if hosts == nil {
		return nil, fmt.Errorf("panel hosts response is missing array")
	}
	return hosts, nil
}

type Node struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	IPAddress        string    `json:"ip_address"`
	Status           string    `json:"status"`
	Country          string    `json:"country"`
	Tags             []string  `json:"tags"`
	UsersOnline      int       `json:"users_online"`
	TrafficUsedBytes int64     `json:"traffic_used_bytes"`
	Inbounds         []Inbound `json:"inbounds"`
}
type Client struct {
	baseURL, apiToken string
	httpClient        *http.Client
	logger            *slog.Logger
	allowWrites       bool
}

var ErrReadOnly = errors.New("panel client is read-only: only GET is allowed")

// Option configures a client before it is used by concurrent callers.
type Option func(*Client)

// WithWriteAccess must be explicitly requested for a writable panel. Never use
// this option with the production read-only token during development.
func WithWriteAccess() Option { return func(c *Client) { c.allowWrites = true } }

type APIError struct{ Status int }

func (e *APIError) Error() string { return fmt.Sprintf("panel returned HTTP %d", e.Status) }
func NewClient(base, token string, logger *slog.Logger, options ...Option) *Client {
	c := &Client{baseURL: strings.TrimRight(base, "/"), apiToken: token, logger: logger, httpClient: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}
	for _, option := range options {
		option(c)
	}
	return c
}
func (c *Client) SetTimeout(d time.Duration) { c.httpClient.Timeout = d }
func (c *Client) Request(ctx context.Context, method, path string, body any) ([]byte, error) {
	if !c.allowWrites && method != http.MethodGet {
		return nil, ErrReadOnly
	}
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "SubGate/0.2")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("panel request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{Status: resp.StatusCode}
	}
	data, err = io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if len(data) >= 16<<20 {
		return nil, fmt.Errorf("panel response exceeds limit")
	}
	var envelope struct {
		Response json.RawMessage `json:"response"`
	}
	if err = json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("invalid panel JSON: %w", err)
	}
	if len(envelope.Response) == 0 || string(envelope.Response) == "null" {
		return nil, fmt.Errorf("missing panel response envelope")
	}
	return envelope.Response, nil
}
func (c *Client) ListUsersContext(ctx context.Context, offset, limit int) ([]User, int, error) {
	b, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/users?start=%d&size=%d", offset, limit), nil)
	if err != nil {
		return nil, 0, err
	}
	var result struct {
		Users []json.RawMessage `json:"users"`
		Total int               `json:"total"`
	}
	if err = json.Unmarshal(b, &result); err != nil {
		return nil, 0, err
	}
	if result.Users == nil {
		return nil, 0, fmt.Errorf("panel users response is missing users array")
	}
	users := make([]User, 0, len(result.Users))
	for _, raw := range result.Users {
		u, err := decodeGeneratedUser(raw)
		if err != nil {
			return nil, 0, err
		}
		users = append(users, *u)
	}
	return users, result.Total, nil
}

func decodeGeneratedUser(raw []byte) (*User, error) {
	var item generated.GetUsersResponseDtoResponseUsersItem
	if err := item.Decode(jx.DecodeBytes(raw)); err != nil {
		return nil, errors.New("invalid panel user response fields")
	}
	// Decode tolerates unknown enum strings; the strict writer validator must
	// not reject future response statuses. Normalize from original bytes to
	// retain exact int64 counters and stable legacy UUID identity precedence.
	user, err := decodeUser(raw)
	if err != nil {
		return nil, errors.New("invalid panel user identity or traffic")
	}
	return user, nil
}
func (c *Client) ListUsers(offset, limit int) ([]User, int, error) {
	return c.ListUsersContext(context.Background(), offset, limit)
}
func decodeUser(raw []byte) (*User, error) {
	var w struct {
		ID        json.RawMessage `json:"id"`
		UUID      string          `json:"uuid"`
		ShortUUID string          `json:"shortUuid"`
		Username  string          `json:"username"`
		Email     string          `json:"email"`
		Status    string          `json:"status"`
		Limit     int64           `json:"trafficLimitBytes"`
		ExpireAt  string          `json:"expireAt"`
		Traffic   struct {
			Used int64 `json:"usedTrafficBytes"`
		} `json:"userTraffic"`
	}
	if err := json.Unmarshal(raw, &w); err != nil {
		return nil, err
	}
	if len(w.ID) > 0 && string(w.ID) != "null" {
		var numericID json.Number
		if err := json.Unmarshal(w.ID, &numericID); err != nil {
			return nil, errors.New("panel user id must be numeric")
		}
	}
	id := w.UUID
	if id == "" {
		id = strings.Trim(string(w.ID), "\"")
	}
	if id == "" || id == "null" || w.ShortUUID == "" {
		return nil, fmt.Errorf("panel user is missing identity")
	}
	return &User{ID: id, ShortUUID: w.ShortUUID, Username: w.Username, Email: w.Email, Status: strings.ToLower(w.Status), TrafficLimitBytes: w.Limit, UsedTrafficBytes: w.Traffic.Used, ExpireAt: w.ExpireAt}, nil
}

// DecodeWebhookUser uses the native Remnawave 3.4.5 ExtendedUsersSchema
// projection, shared with REST reads. Unknown response fields/statuses survive
// as tolerant-reader input; malformed identities or counters are rejected.
// Source: remnawave/backend@3.4.5 libs/contract/models/webhook/webhook.schema.ts.
func DecodeWebhookUser(raw []byte) (*User, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	for _, name := range []string{"shortUuid", "username", "status", "trafficLimitBytes", "expireAt", "userTraffic", "email"} {
		value, exists := fields[name]
		if !exists || (string(value) == "null" && name != "email") {
			return nil, errors.New("missing webhook user projection fields")
		}
	}
	var traffic map[string]json.RawMessage
	if err := json.Unmarshal(fields["userTraffic"], &traffic); err != nil {
		return nil, err
	}
	if value, exists := traffic["usedTrafficBytes"]; !exists || string(value) == "null" {
		return nil, errors.New("missing webhook traffic counter")
	}
	user, err := decodeUser(raw)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(user.ID) == "" || strings.TrimSpace(user.ShortUUID) == "" || strings.TrimSpace(user.Status) == "" || user.Username == "" || user.TrafficLimitBytes < 0 || user.UsedTrafficBytes < 0 {
		return nil, errors.New("invalid webhook user fields")
	}
	if _, err := time.Parse(time.RFC3339Nano, user.ExpireAt); err != nil {
		return nil, errors.New("invalid webhook expiry")
	}
	return user, nil
}

func (c *Client) GetUser(id string) (*User, error) {
	b, e := c.Request(context.Background(), "GET", "/api/users/"+url.PathEscape(id), nil)
	if e != nil {
		return nil, e
	}
	return decodeUser(b)
}
func (c *Client) GetUserByShortUUID(id string) (*User, error) {
	b, e := c.Request(context.Background(), "GET", "/api/users/by-short-uuid/"+url.PathEscape(id), nil)
	if e != nil {
		return nil, e
	}
	return decodeUser(b)
}
func (c *Client) ListNodesContext(ctx context.Context) ([]Node, error) {
	b, err := c.Request(ctx, "GET", "/api/nodes", nil)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		UUID        string          `json:"uuid"`
		ID          json.RawMessage `json:"id"`
		Name        string          `json:"name"`
		Address     string          `json:"address"`
		Connected   bool            `json:"isConnected"`
		Disabled    bool            `json:"isDisabled"`
		Country     string          `json:"countryCode"`
		Tags        []string        `json:"tags"`
		UsersOnline int             `json:"usersOnline"`
		Traffic     int64           `json:"trafficUsedBytes"`
		Profile     struct {
			Inbounds []Inbound `json:"activeInbounds"`
		} `json:"configProfile"`
	}
	if err = json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, fmt.Errorf("panel nodes response is missing array")
	}
	nodes := make([]Node, 0, len(raw))
	for _, w := range raw {
		id := w.UUID
		if id == "" {
			id = strings.Trim(string(w.ID), "\"")
		}
		if id == "" {
			return nil, fmt.Errorf("panel node missing identity")
		}
		status := "offline"
		if w.Connected {
			status = "online"
		}
		if w.Disabled {
			status = "disabled"
		}
		nodes = append(nodes, Node{ID: id, Name: w.Name, IPAddress: w.Address, Status: status, Country: w.Country, Tags: w.Tags, UsersOnline: w.UsersOnline, TrafficUsedBytes: w.Traffic, Inbounds: w.Profile.Inbounds})
	}
	return nodes, nil
}
func (c *Client) ListNodes() ([]Node, error) { return c.ListNodesContext(context.Background()) }
func (c *Client) Action(ctx context.Context, kind, id, action string) error {
	// User actions are delegated to remnawave-subscription-middleware.
	if kind != "nodes" {
		return fmt.Errorf("unsupported resource")
	}
	_, err := c.Request(ctx, "POST", "/api/"+kind+"/"+url.PathEscape(id)+"/actions/"+url.PathEscape(action), nil)
	return err
}
func (c *Client) Health() error { _, e := c.ListNodes(); return e }
func (c *Client) Close() error  { c.httpClient.CloseIdleConnections(); return nil }
