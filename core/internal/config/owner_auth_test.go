package config

import "testing"

func TestOwnerPasswordOnlyConfiguration(t *testing.T) {
	for _, factor := range []string{"totp", "webauthn"} {
		cfg := Defaults()
		cfg.Demo = true
		if factor == "totp" {
			cfg.Auth.TOTPRequired = true
		} else {
			cfg.Auth.WebAuthnEnabled = true
		}
		if cfg.Validate() == nil {
			t.Errorf("unsupported factor %s silently accepted", factor)
		}
	}
}
