package pim

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// OIDFromToken returns the "oid" claim of a JWT. The signature is not
// verified: the token is only read for the caller's own object ID, and ARM
// validates it on use.
func OIDFromToken(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", errors.New("token is not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("decode token payload: %w", err)
	}
	var claims struct {
		OID string `json:"oid"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("parse token payload: %w", err)
	}
	if claims.OID == "" {
		return "", errors.New("token has no oid claim")
	}
	return claims.OID, nil
}
