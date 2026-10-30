// Package backup contains remote backup destinations and retention helpers.
package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"
)

type Artifact struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
	ETag      string    `json:"etag,omitempty"`
}

type Store interface {
	Put(context.Context, Artifact, io.Reader) error
	Get(context.Context, string) (io.ReadCloser, Artifact, error)
	List(context.Context) ([]Artifact, error)
	Delete(context.Context, string) error
}

type HTTPStore struct {
	Endpoint  string
	Bucket    string
	Prefix    string
	AccessKey string
	SecretKey string
	Client    *http.Client
}

func (s *HTTPStore) objectURL(name string) (string, error) {
	base, err := url.Parse(strings.TrimRight(s.Endpoint, "/"))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return "", fmt.Errorf("invalid backup endpoint")
	}
	if s.Bucket == "" || strings.ContainsAny(s.Bucket, "/\\") {
		return "", fmt.Errorf("invalid backup bucket")
	}
	base.Path = path.Join(base.Path, s.Bucket, s.Prefix, name)
	return base.String(), nil
}
func (s *HTTPStore) request(ctx context.Context, method, name string, body io.Reader) (*http.Response, error) {
	u, err := s.objectURL(name)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}
	if s.AccessKey != "" {
		req.SetBasicAuth(s.AccessKey, s.SecretKey)
	}
	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}
	return client.Do(req)
}
func (s *HTTPStore) Put(ctx context.Context, artifact Artifact, body io.Reader) error {
	response, err := s.request(ctx, http.MethodPut, artifact.Name, body)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("remote backup PUT returned %s: %s", response.Status, strings.TrimSpace(string(raw)))
	}
	return nil
}
func (s *HTTPStore) Get(ctx context.Context, name string) (io.ReadCloser, Artifact, error) {
	response, err := s.request(ctx, http.MethodGet, name, nil)
	if err != nil {
		return nil, Artifact{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		response.Body.Close()
		return nil, Artifact{}, fmt.Errorf("remote backup GET returned %s", response.Status)
	}
	return response.Body, Artifact{Name: name, Size: response.ContentLength, ETag: response.Header.Get("ETag")}, nil
}
func (s *HTTPStore) List(ctx context.Context) ([]Artifact, error) {
	u, err := s.objectURL("__list__")
	if err != nil {
		return nil, err
	}
	u = strings.TrimSuffix(u, "/__list__") + "?list-type=2"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	if s.AccessKey != "" {
		req.SetBasicAuth(s.AccessKey, s.SecretKey)
	}
	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("remote backup LIST returned %s", response.Status)
	}
	var result struct {
		Objects []struct {
			Key          string    `json:"key"`
			Size         int64     `json:"size"`
			LastModified time.Time `json:"last_modified"`
			ETag         string    `json:"etag"`
		} `json:"objects"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&result); err != nil {
		return nil, err
	}
	out := make([]Artifact, 0, len(result.Objects))
	for _, item := range result.Objects {
		out = append(out, Artifact{Name: item.Key, Size: item.Size, CreatedAt: item.LastModified, ETag: item.ETag})
	}
	return out, nil
}
func (s *HTTPStore) Delete(ctx context.Context, name string) error {
	response, err := s.request(ctx, http.MethodDelete, name, nil)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("remote backup DELETE returned %s", response.Status)
	}
	return nil
}

type TelegramStore struct {
	Token  string
	ChatID int64
	Client *http.Client
}

func (s *TelegramStore) endpoint(method string) string {
	return "https://api.telegram.org/bot" + url.PathEscape(s.Token) + "/" + method
}
func (s *TelegramStore) Put(ctx context.Context, artifact Artifact, body io.Reader) error {
	if s.Token == "" || s.ChatID == 0 {
		return fmt.Errorf("telegram backup destination is not configured")
	}
	data, err := io.ReadAll(io.LimitReader(body, 50<<20))
	if err != nil {
		return err
	}
	var payload bytes.Buffer
	form := multipart.NewWriter(&payload)
	_ = form.WriteField("chat_id", fmt.Sprint(s.ChatID))
	part, err := form.CreateFormFile("document", artifact.Name)
	if err != nil {
		return err
	}
	if _, err = part.Write(data); err != nil {
		return err
	}
	if err = form.Close(); err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint("sendDocument"), &payload)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", form.FormDataContentType())
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("telegram backup returned %s", response.Status)
	}
	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if json.Unmarshal(raw, &result) != nil || !result.OK {
		return fmt.Errorf("telegram backup rejected: %s", result.Description)
	}
	return nil
}
func (s *TelegramStore) Get(context.Context, string) (io.ReadCloser, Artifact, error) {
	return nil, Artifact{}, fmt.Errorf("telegram destination does not support direct download")
}
func (s *TelegramStore) List(context.Context) ([]Artifact, error) {
	return nil, fmt.Errorf("telegram destination does not support listing")
}
func (s *TelegramStore) Delete(context.Context, string) error {
	return fmt.Errorf("telegram destination does not support deletion")
}

func Prune(ctx context.Context, store Store, keep int) (int, error) {
	if keep < 0 {
		return 0, fmt.Errorf("keep must not be negative")
	}
	items, err := store.List(ctx)
	if err != nil {
		return 0, err
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	removed := 0
	for _, item := range items[keep:] {
		if err := store.Delete(ctx, item.Name); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}
