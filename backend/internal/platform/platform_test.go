package platform

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"testing"
	"time"
)

func TestSecurityBoundaries(t *testing.T) {
	for _, u := range []string{"https://agent.example/callback", "http://127.0.0.1:9312/callback"} {
		if !validRedirect(u) {
			t.Fatalf("valid redirect rejected: %s", u)
		}
	}
	for _, u := range []string{"http://evil.example/callback", "javascript:alert(1)", "https://user:pass@evil.example", "https://agent.example/#fragment"} {
		if validRedirect(u) {
			t.Fatalf("unsafe redirect accepted: %s", u)
		}
	}
	now := time.Unix(1800000000, 0)
	payload := []byte(`{"id":"evt_test"}`)
	secret := "webhook-test-secret"
	stamp := strconv.FormatInt(now.Unix(), 10)
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(stamp + "."))
	m.Write(payload)
	header := "t=" + stamp + ",v1=" + hex.EncodeToString(m.Sum(nil))
	if !verifyStripe(payload, header, secret, now) {
		t.Fatal("valid webhook rejected")
	}
	if verifyStripe([]byte(`{}`), header, secret, now) || verifyStripe(payload, header, secret, now.Add(6*time.Minute)) || verifyStripe(payload, header, "", now) {
		t.Fatal("invalid webhook accepted")
	}
	for _, tool := range tools() {
		if tool.Scope == "" || tool.Input == nil {
			t.Fatal("tool missing scope or schema")
		}
	}
}
