package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// signPayload builds a valid HMAC signature on an already-encoded payload string.
func signPayload(encoded, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(encoded))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("%s.%s", encoded, sig)
}

func TestSignVerifyToken_RoundTrip(t *testing.T) {
	secret := "test-secret-key"
	userID := "user-abc-123"
	token := SignToken(userID, secret)
	got, err := VerifyToken(token, secret)
	if err != nil {
		t.Fatalf("VerifyToken returned error: %v", err)
	}
	if got != userID {
		t.Errorf("expected userID %q, got %q", userID, got)
	}
}

func TestVerifyToken_WrongSecret(t *testing.T) {
	token := SignToken("user-123", "secret-a")
	_, err := VerifyToken(token, "secret-b")
	if err == nil {
		t.Error("expected error for wrong secret, got nil")
	}
}

func TestVerifyToken_Expired(t *testing.T) {
	secret := "test-secret"
	token := MakeTokenWithExpiry("user-123", secret, time.Now().Add(-time.Hour).Unix())
	_, err := VerifyToken(token, secret)
	if err == nil {
		t.Error("expected error for expired token, got nil")
	}
}

func TestVerifyToken_Malformed_NoSeparator(t *testing.T) {
	_, err := VerifyToken("thisisnotavalidtoken", "secret")
	if err == nil {
		t.Error("expected error for token with no '.' separator")
	}
}

func TestVerifyToken_Malformed_BadBase64(t *testing.T) {
	_, err := VerifyToken("!!!.###", "secret")
	if err == nil {
		t.Error("expected error for invalid base64 payload")
	}
}

func TestVerifyToken_Malformed_BadPayload(t *testing.T) {
	// Valid HMAC on a base64 payload that is not JSON
	secret := "secret"
	bad := base64.RawURLEncoding.EncodeToString([]byte("not-json"))
	token := signPayload(bad, secret)
	_, err := VerifyToken(token, secret)
	if err == nil {
		t.Error("expected error for non-JSON payload")
	}
}

func TestVerifyToken_TamperedSignature(t *testing.T) {
	secret := "secret"
	token := SignToken("user-123", secret)
	tampered := token[:len(token)-1] + "X"
	_, err := VerifyToken(tampered, secret)
	if err == nil {
		t.Error("expected error for tampered signature")
	}
}

func TestVerifyToken_TamperedPayload(t *testing.T) {
	secret := "secret"
	token := SignToken("user-123", secret)
	altPayload, _ := json.Marshal(map[string]interface{}{
		"sub": "attacker",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	altEncoded := base64.RawURLEncoding.EncodeToString(altPayload)
	dotIdx := -1
	for i := len(token) - 1; i >= 0; i-- {
		if token[i] == '.' {
			dotIdx = i
			break
		}
	}
	if dotIdx < 0 {
		t.Fatal("test token has no dot separator")
	}
	tampered := altEncoded + token[dotIdx:]
	_, err := VerifyToken(tampered, secret)
	if err == nil {
		t.Error("expected error for tampered payload, got nil")
	}
}
