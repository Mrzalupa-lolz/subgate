package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/subgate/subgate/core/internal/panel"
)

// Xray's email label may contain a username, email address or short UUID.
// Keep the original telemetry identity for authenticated encryption, and expand
// journal filters using only aliases owned by one synchronized panel user.
func (s *Server) activityIdentities(selector string) ([]string, error) {
	identities := []string{selector}
	var raw string
	err := s.db.QueryRow("SELECT data FROM sg_objects WHERE kind='users' AND id=?", selector).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		rows, e := s.db.Query(`SELECT data FROM sg_objects WHERE kind='users' AND
		 (json_extract(data,'$.username')=? OR json_extract(data,'$.email')=? OR json_extract(data,'$.short_uuid')=?) LIMIT 2`, selector, selector, selector)
		if e != nil {
			return nil, e
		}
		matches := 0
		for rows.Next() {
			if e = rows.Scan(&raw); e != nil {
				rows.Close()
				return nil, e
			}
			matches++
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, e
		}
		if matches != 1 {
			return identities, nil
		}
	} else if err != nil {
		return nil, err
	}
	var user panel.User
	if err := json.Unmarshal([]byte(raw), &user); err != nil {
		return nil, err
	}
	identities = []string{user.ID}
	for _, alias := range []string{user.Username, user.Email, user.ShortUUID} {
		if alias == "" || slices.Contains(identities, alias) {
			continue
		}
		var conflicts int
		err := s.db.QueryRow(`SELECT COUNT(*) FROM sg_objects WHERE kind='users' AND id!=? AND
		 (id=? OR json_extract(data,'$.username')=? OR json_extract(data,'$.email')=? OR json_extract(data,'$.short_uuid')=?)`, user.ID, alias, alias, alias, alias).Scan(&conflicts)
		if err != nil {
			return nil, err
		}
		if conflicts == 0 {
			identities = append(identities, alias)
		}
	}
	return identities, nil
}

func identityPredicate(identities []string) (string, []any) {
	args := make([]any, len(identities))
	for i, id := range identities {
		args[i] = id
	}
	return "user_id IN (" + strings.TrimRight(strings.Repeat("?,", len(identities)), ",") + ")", args
}
