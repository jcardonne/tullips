package integrations

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPublicDestinations(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "::1", "::ffff:127.0.0.1", "100.100.100.200", "192.168.1.2", "64:ff9b::7f00:1"} {
		if PublicIP(net.ParseIP(s)) {
			t.Fatal("allowed", s)
		}
	}
	if !PublicIP(net.ParseIP("8.8.8.8")) {
		t.Fatal("public IP rejected")
	}
	if _, e := Crawl(context.Background(), "http://127.0.0.1", 1); e == nil {
		t.Fatal("SSRF allowed")
	}
}
func TestAIUsage(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("missing auth")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"Hello"}}],"usage":{"prompt_tokens":1000,"completion_tokens":500}}`))
	}))
	defer s.Close()
	a := AI{s.URL, "secret", "test", 2, 4, 1000}
	r, e := a.Complete(context.Background(), "system", "prompt")
	if e != nil || r.Text != "Hello" || r.Credits != 4 || r.InputTokens != 1000 {
		t.Fatalf("%+v %v", r, e)
	}
}
func TestLinkedInSessionIsolation(t *testing.T) {
	a, e := NewLinkedIn()
	if e != nil {
		t.Fatal(e)
	}
	b, _ := NewLinkedIn()
	if a.jar == b.jar {
		t.Fatal("shared cookies")
	}
	if e = a.Restore([]byte(`{"evil.example":[]}`)); e == nil {
		t.Fatal("foreign cookie domain accepted")
	}
}

func TestRSCLoginAndCheckpoint(t *testing.T) {
	page := []byte(`<html><script>window.__como_rehydration__ = ["0:{\"value\":{\"requestId\":\"com.linkedin.sdui.requests.login.authenticate\",\"requestedArguments\":{\"payload\":{\"identifier\":{\"key\":\"memberIdentifierInput\",\"namespace\":\"MemoryNamespace\"},\"password\":{\"key\":\"passwordInput\",\"namespace\":\"MemoryNamespace\"}}}}}\n"];</script></html>`)
	a, e := FindAction(page, "com.linkedin.sdui.requests.login.authenticate")
	if e != nil {
		t.Fatal(e)
	}
	b, e := BuildAction(a, map[string]string{"identifier": "test@example.com", "password": "not-a-real-password"})
	if e != nil || !strings.Contains(string(b), "memberIdentifierInput") {
		t.Fatalf("binding: %s %v", b, e)
	}
	if _, e = BuildAction(a, map[string]string{"missing": "value"}); e == nil {
		t.Fatal("missing binding accepted")
	}
	form, e := CheckpointForm([]byte(`<form><input name="csrfToken" value="fresh"><input name="challengeId" value="test"></form>`), "123456")
	if e != nil || form.Get("userResponse") != "123456" {
		t.Fatal(form, e)
	}
	if _, e = CheckpointForm([]byte(`<form/>`), "123456"); e == nil {
		t.Fatal("missing challenge tokens accepted")
	}
}
func TestEmailHeadersAndOAuth(t *testing.T) {
	m := Email{From: "me@example.com", To: "you@example.com", Subject: "Hello", Text: "Test", MessageID: "job-1@example.com"}
	b, e := FormatEmail(m)
	if e != nil || !strings.Contains(string(b), "Message-ID: <job-1@example.com>") {
		t.Fatal(e)
	}
	m.Subject = "Hi\r\nBcc: other@example.com"
	if _, e = FormatEmail(m); e == nil {
		t.Fatal("header injection accepted")
	}
	for _, p := range []string{"google", "microsoft"} {
		c, e := MailOAuth(p, "client", "secret", "https://example.com/callback")
		if e != nil || len(c.Scopes) == 0 {
			t.Fatal(p, e)
		}
	}
	if _, e := MailOAuth("evil", "c", "s", "url"); e == nil {
		t.Fatal("unsupported provider")
	}
}
func TestProviderApplicationErrors(t *testing.T) {
	if e := ValidateActionResult([]byte("1:[]\n0:{\"response\":{\"errors\":\"$1\"}}")); e != nil {
		t.Fatal(e)
	}
	if e := ValidateActionResult([]byte("1:[{\"message\":\"rejected\"}]\n0:{\"response\":{\"errors\":\"$1\"}}")); e == nil {
		t.Fatal("provider error considered delivered")
	}
	if e := ValidateActionResult([]byte("garbage")); e == nil {
		t.Fatal("unknown response considered delivered")
	}
}
func TestLinkedInMessageParser(t *testing.T) {
	raw := []byte(`{"data":{"messages":[{"entityUrn":"urn:li:message:1","body":{"text":"hello"},"sender":{"hostIdentityUrn":"urn:li:profile:1","participantType":{"member":{"profileUrl":"https://www.linkedin.com/in/example"}}},"conversation":{"entityUrn":"urn:li:conversation:1"},"deliveredAt":1234}]}}`)
	messages, e := ParseMessages(raw)
	if e != nil || len(messages) != 1 || messages[0].Text != "hello" || messages[0].SenderURN != "urn:li:profile:1" {
		t.Fatal(messages, e)
	}
}
func TestEmailText(t *testing.T) {
	text := EmailText([]byte("From: test@example.com\r\nContent-Type: text/plain\r\nContent-Transfer-Encoding: base64\r\n\r\naGVsbG8="))
	if text != "hello" {
		t.Fatal(text)
	}
	text = EmailText([]byte("Content-Type: text/html\r\n\r\n<p>Hello</p><script>bad()</script>"))
	if text != "Hello" {
		t.Fatal(text)
	}
}
