package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// VerifyToken validates an HMAC-signed token and returns the subject (user ID).
func VerifyToken(token, secret string) (string, error) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid token format")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(parts[0]))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(parts[1]), []byte(expected)) {
		return "", fmt.Errorf("invalid signature")
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", fmt.Errorf("invalid payload encoding")
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return "", fmt.Errorf("invalid payload json")
	}
	exp, ok := payload["exp"].(float64)
	if !ok || time.Now().Unix() > int64(exp) {
		return "", fmt.Errorf("token expired")
	}
	sub, ok := payload["sub"].(string)
	if !ok {
		return "", fmt.Errorf("missing subject")
	}
	return sub, nil
}

// SignToken creates an HMAC-SHA256 signed token: base64url(payload).base64url(sig).
func SignToken(userID, secret string) string {
	payload, _ := json.Marshal(map[string]interface{}{
		"sub": userID,
		"exp": time.Now().Add(24 * time.Hour).Unix(),
	})
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(encoded))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("%s.%s", encoded, sig)
}

// MakeTokenWithExpiry creates a signed token with a specific expiry — for testing.
func MakeTokenWithExpiry(userID, secret string, exp int64) string {
	payload, _ := json.Marshal(map[string]interface{}{
		"sub": userID,
		"exp": exp,
	})
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(encoded))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("%s.%s", encoded, sig)
}
