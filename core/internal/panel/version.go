package panel

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

// SupportedVersion is the Remnawave release the read contract and fixtures in
// testdata were last verified against. Any 3.4.x panel is treated as
// compatible; other minor or major releases are reported as a mismatch.
const SupportedVersion = "3.4.8"

// VersionCompatible reports whether actual shares SupportedVersion's
// major.minor series.
func VersionCompatible(actual string) bool {
	return versionSeries(actual) != "" && versionSeries(actual) == versionSeries(SupportedVersion)
}

func versionSeries(v string) string {
	if !versionSyntax.MatchString(v) {
		return ""
	}
	parts := strings.SplitN(v, ".", 3)
	return parts[0] + "." + parts[1]
}

var versionSyntax = regexp.MustCompile(`^[0-9]{1,4}\.[0-9]{1,4}\.[0-9]{1,4}(?:[-+][0-9A-Za-z.-]{1,48})?$`)

// Version reads the version field from the recap contract. Invalid or
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
