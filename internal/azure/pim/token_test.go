package pim

import (
	"encoding/base64"
	"testing"
)

func TestOIDFromToken(t *testing.T) {
	enc := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	tok := "header." + enc(`{"oid":"1234-abcd"}`) + ".sig"
	if got, err := OIDFromToken(tok); err != nil || got != "1234-abcd" {
		t.Errorf("OIDFromToken = %q, %v", got, err)
	}
	for _, bad := range []string{"header." + enc(`{"sub":"x"}`) + ".sig", "not-a-jwt", ""} {
		if got, err := OIDFromToken(bad); err == nil {
			t.Errorf("OIDFromToken(%q) = %q, nil; want error", bad, got)
		}
	}
}
