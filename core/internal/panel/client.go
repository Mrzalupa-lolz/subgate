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

// Inbound is a display-only projection of configProfile.activeInbounds.
// rawInbound (client settings, REALITY keys, certificates) is never decoded.
type Inbound struct {
	Tag      string `json:"tag"`
	Type     string `json:"type"`
	Network  string `json:"network"`
	Security string `json:"security"`
	Port     int    `json:"port"`
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
		u, err := decodeUser(raw)
		if err != nil {
			return nil, 0, err
		}
		users = append(users, *u)
	}
	return users, result.Total, nil
}

func (c *Client) ListUsers(offset, limit int) ([]User, int, error) {
	return c.ListUsersContext(context.Background(), offset, limit)
}

// decodeUser reads the Remnawave 3.4 user projection. Only the fields SubGate
// relies on are decoded and validated; credentials, squads and unknown fields
// are ignored. Remnawave 3.x identifies users by numeric id; a legacy uuid is
// still preferred when present so existing identities stay stable.
func decodeUser(raw []byte) (*User, error) {
	var w struct {
		ID        json.RawMessage `json:"id"`
		UUID      *string         `json:"uuid"`
		ShortUUID *string         `json:"shortUuid"`
		Username  *string         `json:"username"`
		Email     *string         `json:"email"`
		Status    *string         `json:"status"`
		Limit     *int64          `json:"trafficLimitBytes"`
		ExpireAt  *string         `json:"expireAt"`
		Traffic   *struct {
			Used *int64 `json:"usedTrafficBytes"`
		} `json:"userTraffic"`
	}
	if err := json.Unmarshal(raw, &w); err != nil {
		return nil, errors.New("invalid panel user fields")
	}
	if w.ShortUUID == nil || w.Username == nil || w.Status == nil || w.Limit == nil || w.ExpireAt == nil || w.Traffic == nil || w.Traffic.Used == nil {
		return nil, errors.New("panel user is missing required fields")
	}
	if *w.Limit < 0 || *w.Traffic.Used < 0 {
		return nil, errors.New("panel user traffic counters must not be negative")
	}
	if _, err := time.Parse(time.RFC3339Nano, *w.ExpireAt); err != nil {
		return nil, errors.New("invalid panel user expiry")
	}
	id := ""
	if w.UUID != nil {
		id = strings.TrimSpace(*w.UUID)
	}
	if len(w.ID) > 0 && string(w.ID) != "null" {
		var numericID json.Number
		if err := json.Unmarshal(w.ID, &numericID); err != nil || strings.ContainsAny(numericID.String(), ".eE-") {
			return nil, errors.New("panel user id must be a positive integer")
		}
		if id == "" {
			id = numericID.String()
		}
	}
	if id == "" || *w.ShortUUID == "" || *w.Username == "" || *w.Status == "" {
		return nil, fmt.Errorf("panel user is missing identity")
	}
	email := ""
	if w.Email != nil {
		email = *w.Email
	}
	return &User{ID: id, ShortUUID: *w.ShortUUID, Username: *w.Username, Email: email, Status: strings.ToLower(*w.Status), TrafficLimitBytes: *w.Limit, UsedTrafficBytes: *w.Traffic.Used, ExpireAt: *w.ExpireAt}, nil
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
