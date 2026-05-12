package models

import (
	"testing"
)

// ── JSONB.Value ───────────────────────────────────────────────────────────────

func TestJSONBValue_Nil(t *testing.T) {
	var j JSONB
	val, err := j.Value()
	if err != nil {
		t.Fatalf("Value() error on nil JSONB: %v", err)
	}
	if val != nil {
		t.Errorf("Value() on nil JSONB: expected nil, got %v", val)
	}
}

func TestJSONBValue_Empty(t *testing.T) {
	j := JSONB{}
	val, err := j.Value()
	if err != nil {
		t.Fatalf("Value() error on empty JSONB: %v", err)
	}
	if val == nil {
		t.Error("Value() on empty JSONB: expected non-nil JSON string, got nil")
	}
}

func TestJSONBValue_NonNil(t *testing.T) {
	j := JSONB{"key": "value", "num": float64(42)}
	val, err := j.Value()
	if err != nil {
		t.Fatalf("Value() error: %v", err)
	}
	str, ok := val.(string)
	if !ok {
		t.Fatalf("Value() expected string, got %T", val)
	}
	if str == "" {
		t.Error("Value() returned empty string")
	}
}

// ── JSONB.Scan ────────────────────────────────────────────────────────────────

func TestJSONBScan_Nil(t *testing.T) {
	var j JSONB
	if err := j.Scan(nil); err != nil {
		t.Fatalf("Scan(nil) error: %v", err)
	}
	if j != nil {
		t.Errorf("Scan(nil) expected nil JSONB, got %v", j)
	}
}

func TestJSONBScan_FromBytes(t *testing.T) {
	var j JSONB
	if err := j.Scan([]byte(`{"key":"value"}`)); err != nil {
		t.Fatalf("Scan([]byte) error: %v", err)
	}
	if j["key"] != "value" {
		t.Errorf("expected key=value, got %v", j["key"])
	}
}

func TestJSONBScan_FromString(t *testing.T) {
	var j JSONB
	if err := j.Scan(`{"num":123}`); err != nil {
		t.Fatalf("Scan(string) error: %v", err)
	}
	if j["num"] != float64(123) {
		t.Errorf("expected num=123 (float64), got %v (%T)", j["num"], j["num"])
	}
}

func TestJSONBScan_InvalidType(t *testing.T) {
	var j JSONB
	if err := j.Scan(12345); err == nil {
		t.Error("Scan(int) expected error, got nil")
	}
}

func TestJSONBScan_InvalidJSON(t *testing.T) {
	var j JSONB
	if err := j.Scan([]byte("not-json")); err == nil {
		t.Error("Scan(invalid JSON) expected error, got nil")
	}
}

// ── Round-trip ────────────────────────────────────────────────────────────────

func TestJSONBRoundTrip(t *testing.T) {
	original := JSONB{
		"str":    "hello",
		"num":    float64(42),
		"bool":   true,
		"nested": map[string]interface{}{"x": float64(1)},
	}
	val, err := original.Value()
	if err != nil {
		t.Fatalf("Value() error: %v", err)
	}
	var got JSONB
	if err := got.Scan(val); err != nil {
		t.Fatalf("Scan() error: %v", err)
	}
	if got["str"] != "hello" {
		t.Errorf("round-trip str: expected %q, got %v", "hello", got["str"])
	}
	if got["num"] != float64(42) {
		t.Errorf("round-trip num: expected 42, got %v", got["num"])
	}
	if got["bool"] != true {
		t.Errorf("round-trip bool: expected true, got %v", got["bool"])
	}
}
