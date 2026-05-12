package api

import (
	"strings"
	"testing"
)

// ── generateRandomString ──────────────────────────────────────────────────────

func TestGenerateRandomString_Length(t *testing.T) {
	for _, n := range []int{1, 8, 16, 32, 64} {
		s := generateRandomString(n)
		if len(s) != n {
			t.Errorf("generateRandomString(%d): got length %d", n, len(s))
		}
	}
}

func TestGenerateRandomString_ValidCharset(t *testing.T) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	s := generateRandomString(200)
	for i, ch := range s {
		if !strings.ContainsRune(charset, ch) {
			t.Errorf("generateRandomString returned invalid character %q at index %d", ch, i)
		}
	}
}

func TestGenerateRandomString_Unique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 10; i++ {
		s := generateRandomString(32)
		if seen[s] {
			t.Errorf("generateRandomString produced a duplicate on iteration %d: %q", i, s)
		}
		seen[s] = true
	}
}

// The original bug: time-based impl produced the same character repeated.
// Verify that the result is NOT all the same character.
func TestGenerateRandomString_NotAllSameChar(t *testing.T) {
	s := generateRandomString(32)
	first := s[0]
	allSame := true
	for i := 1; i < len(s); i++ {
		if s[i] != first {
			allSame = false
			break
		}
	}
	if allSame {
		t.Errorf("generateRandomString returned all identical characters: %q", s)
	}
}

// ── generateAPIKey ────────────────────────────────────────────────────────────

func TestGenerateAPIKey_Prefix(t *testing.T) {
	key := generateAPIKey()
	if !strings.HasPrefix(key, "ct_") {
		t.Errorf("generateAPIKey() missing 'ct_' prefix: %q", key)
	}
}

func TestGenerateAPIKey_Length(t *testing.T) {
	key := generateAPIKey()
	// "ct_" (3) + 32 random chars = 35
	if len(key) != 35 {
		t.Errorf("generateAPIKey() expected length 35, got %d: %q", len(key), key)
	}
}

func TestGenerateAPIKey_Unique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 20; i++ {
		k := generateAPIKey()
		if seen[k] {
			t.Errorf("generateAPIKey produced duplicate on iteration %d: %q", i, k)
		}
		seen[k] = true
	}
}
