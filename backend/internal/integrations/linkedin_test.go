package integrations

import (
	"context"
	"encoding/json"
	http "github.com/bogdanfinn/fhttp"
	tlsclient "github.com/bogdanfinn/tls-client"
	"io"
	"strings"
	"testing"
)

type linkedinFixture struct {
	tlsclient.HttpClient
	do func(*http.Request) (*http.Response, error)
}

func (f linkedinFixture) Do(r *http.Request) (*http.Response, error) { return f.do(r) }
func fixtureResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}
func TestInviteMatchesTarget(t *testing.T) {
	target := "https://www.linkedin.com/in/target/"
	makeAction := func(profile string) map[string]any {
		return map[string]any{"requestId": "com.linkedin.sdui.requests.mynetwork.addaAddConnection", "requestedArguments": map[string]any{"payload": map[string]any{"profileCanonicalUrl": profile}}}
	}
	raw, _ := json.Marshal([]any{makeAction("https://www.linkedin.com/in/recommendation/"), makeAction(target)})
	client, _ := NewLinkedIn()
	calls := 0
	client.client = linkedinFixture{client.client, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "www.linkedin.com" {
			t.Fatal("foreign host")
		}
		if r.Method == "GET" {
			return fixtureResponse(200, "0:"+string(raw)), nil
		}
		var req map[string]any
		if e := json.NewDecoder(r.Body).Decode(&req); e != nil {
			t.Fatal(e)
		}
		args := req["requestedArguments"].(map[string]any)
		if args["payload"].(map[string]any)["profileCanonicalUrl"] != target {
			t.Fatal("wrong prospect invited")
		}
		return fixtureResponse(200, "1:[]\n0:{\"response\":{\"errors\":\"$1\"}}"), nil
	}}
	if e := client.Invite(context.Background(), target); e != nil {
		t.Fatal(e)
	}
	if calls != 2 {
		t.Fatal(calls)
	}
}
func TestForeignRedirectRejected(t *testing.T) {
	client, _ := NewLinkedIn()
	calls := 0
	client.client = linkedinFixture{client.client, func(r *http.Request) (*http.Response, error) {
		calls++
		resp := fixtureResponse(302, "")
		resp.Header.Set("Location", "https://evil.example/collect")
		return resp, nil
	}}
	if _, e := client.page(context.Background(), "/login"); e == nil {
		t.Fatal("foreign redirect allowed")
	}
	if calls != 1 {
		t.Fatal("followed redirect")
	}
}
func TestSessionRoundTrip(t *testing.T) {
	client, _ := NewLinkedIn()
	raw := []byte(`{"linkedin.com":[{"Name":"JSESSIONID","Value":"test-only","Domain":".linkedin.com","Path":"/","Secure":true}]}`)
	if e := client.Restore(raw); e != nil {
		t.Fatal(e)
	}
	saved, e := client.Session()
	if e != nil {
		t.Fatal(e)
	}
	other, _ := NewLinkedIn()
	if e = other.Restore(saved); e != nil {
		t.Fatal(e)
	}
	next, _ := other.Session()
	if string(saved) != string(next) {
		t.Fatal("session lost cookie attributes")
	}
	if e = client.Restore([]byte(`{"linkedin.com":[],"evil.example":[]}`)); e == nil {
		t.Fatal("foreign cookies accepted")
	}
}
func TestIdentityRequiresReferencedProfile(t *testing.T) {
	raw := []byte(`{"data":{"*miniProfile":"urn:li:fs_miniProfile:self"},"included":[{"entityUrn":"urn:li:fs_miniProfile:other","dashEntityUrn":"urn:li:fsd_profile:other"},{"entityUrn":"urn:li:fs_miniProfile:self","dashEntityUrn":"urn:li:fsd_profile:self"}]}`)
	urn, e := ParseIdentity(raw)
	if e != nil || urn != "urn:li:fsd_profile:self" {
		t.Fatal(urn, e)
	}
}
func TestRelationshipIsScoped(t *testing.T) {
	raw := []byte(`{"profiles":[{"profileUrl":"https://www.linkedin.com/in/other","distance":"DISTANCE_1"},{"profileUrl":"https://www.linkedin.com/in/target","distance":"DISTANCE_2"}]}`)
	ok, e := ParseRelationship(raw, "https://www.linkedin.com/in/target/")
	if ok || e != nil {
		t.Fatal(ok, e)
	}
	if _, e = ParseRelationship(raw, "https://www.linkedin.com/in/missing"); e == nil {
		t.Fatal("missing relation inferred")
	}
}
