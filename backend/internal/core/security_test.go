package core

import (
	"strings"
	"testing"
	"time"
)

func TestSecurity(t *testing.T) {
	secret := strings.Repeat("x", 32)
	if !validSignature(secret, "user", SignUser(secret, "user")) || validSignature(secret, "other", SignUser(secret, "user")) || validSignature("", "user", SignUser("", "user")) {
		t.Fatal("signature validation")
	}
	v, e := Encrypt(secret, "password and cookies")
	if e != nil {
		t.Fatal(e)
	}
	plain, e := Decrypt(secret, v)
	if e != nil || plain != "password and cookies" {
		t.Fatal("round trip")
	}
	if _, e = Decrypt(strings.Repeat("y", 32), v); e == nil {
		t.Fatal("wrong key accepted")
	}
	if _, e = Encrypt("short", "secret"); e == nil {
		t.Fatal("weak configuration accepted")
	}
	if has(nil, "send") || has([]string{}, "send") || has([]string{"read"}, "write") || !has([]string{"read"}, "read") {
		t.Fatal("scope validation")
	}
}
func TestRampOverrides(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	settings := map[string]any{"ramp_days": []any{float64(0), float64(10), float64(30)}, "ramp_limits": []any{float64(20), float64(40), float64(80)}}
	if validateRamp(settings) != nil || rampLimit(nil, settings, start, start.AddDate(0, 0, 12)) != 40 {
		t.Fatal("custom ramp")
	}
	if rampLimit(map[string]any{"daily_limit": float64(17)}, settings, start, start.AddDate(0, 0, 90)) != 17 {
		t.Fatal("explicit cap")
	}
	settings["ramp_days"] = []any{float64(0), float64(10), float64(5)}
	if validateRamp(settings) == nil {
		t.Fatal("unordered ramp accepted")
	}
}
