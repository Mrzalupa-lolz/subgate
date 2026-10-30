package panel

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
)

var versionSyntax = regexp.MustCompile(`^[0-9]{1,4}\.[0-9]{1,4}\.[0-9]{1,4}(?:[-+][0-9A-Za-z.-]{1,48})?$`)

// Version reads the version field from the 3.4.5 recap contract. Invalid or
// missing values are not returned to diagnostics or logs.
func (c *Client) Version(ctx context.Context) (string, error) {
	raw, err := c.Request(ctx, "GET", "/api/system/stats/recap", nil)
	if err != nil {
		return "", errors.New("panel version unavailable")
	}
	var recap struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(raw, &recap) != nil || !versionSyntax.MatchString(recap.Version) {
		return "", errors.New("invalid panel version response")
	}
	return recap.Version, nil
}
