package server

import (
	"encoding/json"
	"net/http"
	"time"
)

// Test adapter for retained encrypted rows; no subscription code runs in Core.
func (s *Server) Load(key string) (legacySnapshot, error) {
	var raw []byte
	var e legacySnapshot
	err := s.db.QueryRow("SELECT payload FROM sg_snapshots WHERE key=?", key).Scan(&raw)
	if err != nil {
		return e, err
	}
	raw, err = s.decrypt(raw, key)
	if err == nil {
		err = json.Unmarshal(raw, &e)
	}
	return e, err
}
func (s *Server) Save(key string, e legacySnapshot) error {
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	raw, err = s.encrypt(raw, key)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("INSERT INTO sg_snapshots(key,payload,stored_at) VALUES(?,?,?) ON CONFLICT(key) DO UPDATE SET payload=excluded.payload,stored_at=excluded.stored_at", key, raw, e.StoredAt.Unix())
	return err
}
func (s *Server) Delete(key string) error {
	_, err := s.db.Exec("DELETE FROM sg_snapshots WHERE key=?", key)
	return err
}
func (s *Server) Clear() error { _, err := s.db.Exec("DELETE FROM sg_snapshots"); return err }

type legacySnapshot struct {
	Data      []byte
	Headers   http.Header
	StoredAt  time.Time
	ExpiresAt time.Time
}
