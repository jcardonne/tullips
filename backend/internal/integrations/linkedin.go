package integrations

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	http "github.com/bogdanfinn/fhttp"
	tlsclient "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
	"io"
	"net/url"
	"strings"
)

var ErrCheckpoint = errors.New("LinkedIn requires user verification; pause this account")
var ErrUnsupported = errors.New("integration operation is not yet validated")

type LinkedIn struct {
	client     tlsclient.HttpClient
	jar        tlsclient.CookieJar
	checkpoint string
}

// Each account must receive a separate instance. Never share jars across accounts.
func NewLinkedIn() (*LinkedIn, error) {
	jar := tlsclient.NewCookieJar()
	c, e := tlsclient.NewHttpClient(tlsclient.NewNoopLogger(), tlsclient.WithTimeoutSeconds(30), tlsclient.WithClientProfile(profiles.Chrome_131), tlsclient.WithNotFollowRedirects(), tlsclient.WithCookieJar(jar))
	if e != nil {
		return nil, e
	}
	return &LinkedIn{client: c, jar: jar}, nil
}

// Session JSON must only be stored using core.Encrypt and returned to no API consumer.
func (l *LinkedIn) Session() ([]byte, error) { return json.Marshal(l.jar.GetAllCookies()) }
func (l *LinkedIn) Restore(data []byte) error {
	var hosts map[string][]*http.Cookie
	if e := json.Unmarshal(data, &hosts); e != nil {
		return e
	}
	for host := range hosts {
		h := strings.TrimPrefix(host, ".")
		if h != "linkedin.com" && !strings.HasSuffix(h, ".linkedin.com") {
			return errors.New("invalid cookie host")
		}
	}
	for host, cookies := range hosts {
		host = strings.TrimPrefix(host, ".")
		if host != "linkedin.com" && !strings.HasSuffix(host, ".linkedin.com") {
			return errors.New("invalid cookie host")
		}
		u := &url.URL{Scheme: "https", Host: host}
		l.client.SetCookies(u, cookies)
	}
	return nil
}
func (l *LinkedIn) request(ctx context.Context, method, path, contentType string, body []byte) ([]byte, error) {
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return nil, errors.New("invalid LinkedIn path")
	}
	u := &url.URL{Scheme: "https", Host: "www.linkedin.com"}
	csrf := ""
	for _, c := range l.client.GetCookies(u) {
		if c.Name == "JSESSIONID" {
			csrf = strings.Trim(c.Value, "\"")
		}
	}
	req, e := http.NewRequestWithContext(ctx, method, u.String()+path, bytes.NewReader(body))
	if e != nil {
		return nil, e
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/vnd.linkedin.normalized+json+2.1")
	if !strings.HasPrefix(path, "/voyager/") {
		req.Header.Set("Accept", "text/html,application/octet-stream,*/*")
	}
	req.Header.Set("Origin", "https://www.linkedin.com")
	req.Header.Set("Referer", "https://www.linkedin.com/")
	req.Header.Set("csrf-token", csrf)
	req.Header.Set("x-restli-protocol-version", "2.0.0")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, e := l.client.Do(req)
	if e != nil {
		return nil, e
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 || strings.Contains(resp.Header.Get("Location"), "checkpoint") {
		l.checkpoint = resp.Header.Get("Location")
		return nil, ErrCheckpoint
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return nil, &linkedinRedirect{resp.Header.Get("Location")}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("LinkedIn returned HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 8<<20))
}
func (l *LinkedIn) Me(ctx context.Context) (json.RawMessage, error) {
	return l.request(ctx, "GET", "/voyager/api/me", "", nil)
}

// SendMessage uses a persisted client token so a retry does not create another message.
// Live execution must be gated by the queue's send permission, quota and reply checks.
func (l *LinkedIn) SendMessage(ctx context.Context, conversation, mailbox, text, token, tracking string) (json.RawMessage, error) {
	if !strings.HasPrefix(conversation, "urn:li:") || !strings.HasPrefix(mailbox, "urn:li:") || strings.TrimSpace(text) == "" || token == "" || tracking == "" {
		return nil, errors.New("message requires conversation, mailbox, text and stable tokens")
	}
	if b, e := base64.StdEncoding.DecodeString(tracking); e != nil || len(b) != 16 {
		sum := sha256.Sum256([]byte(tracking))
		tracking = base64.StdEncoding.EncodeToString(sum[:16])
	}
	b, _ := json.Marshal(map[string]any{"message": map[string]any{"body": map[string]any{"attributes": []any{}, "text": text}, "renderContentUnions": []any{}, "conversationUrn": conversation, "originToken": token}, "mailboxUrn": mailbox, "trackingId": tracking, "dedupeByClientGeneratedToken": true})
	return l.request(ctx, "POST", "/voyager/api/voyagerMessagingDashMessengerMessages?action=createMessage", "application/json", b)
}

func (l *LinkedIn) Conversations(ctx context.Context, mailbox string) (json.RawMessage, error) {
	if !strings.HasPrefix(mailbox, "urn:li:") {
		return nil, errors.New("invalid mailbox URN")
	}
	q := url.Values{"queryId": {"messengerConversations.0d5e6781bbee71c3e51c8843c6519f48"}, "variables": {"(mailboxUrn:" + mailbox + ")"}}
	return l.request(ctx, "GET", "/voyager/api/voyagerMessagingGraphQL/graphql?"+q.Encode(), "", nil)
}
func (l *LinkedIn) Messages(ctx context.Context, conversation string) (json.RawMessage, error) {
	if !strings.HasPrefix(conversation, "urn:li:") {
		return nil, errors.New("invalid conversation URN")
	}
	q := url.Values{"queryId": {"messengerMessages.5846eeb71c981f11e0134cb6626cc314"}, "variables": {"(conversationUrn:" + conversation + ")"}}
	return l.request(ctx, "GET", "/voyager/api/voyagerMessagingGraphQL/graphql?"+q.Encode(), "", nil)
}

type LinkedInMessage struct {
	ID              string `json:"id"`
	ConversationURN string `json:"conversationUrn"`
	SenderURN       string `json:"senderUrn"`
	SenderProfile   string `json:"senderProfile"`
	Text            string `json:"text"`
	DeliveredAt     int64  `json:"deliveredAt"`
}

func ParseMessages(raw []byte) ([]LinkedInMessage, error) {
	var data any
	if e := json.Unmarshal(raw, &data); e != nil {
		return nil, e
	}
	out := []LinkedInMessage{}
	seen := map[string]bool{}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			body, bok := x["body"].(map[string]any)
			sender, sok := x["sender"].(map[string]any)
			if bok && sok {
				id, _ := x["entityUrn"].(string)
				if id != "" && !seen[id] {
					seen[id] = true
					m := LinkedInMessage{ID: id}
					m.Text, _ = body["text"].(string)
					m.SenderURN, _ = sender["hostIdentityUrn"].(string)
					if c, ok := x["conversation"].(map[string]any); ok {
						m.ConversationURN, _ = c["entityUrn"].(string)
					}
					if pt, ok := sender["participantType"].(map[string]any); ok {
						if member, ok := pt["member"].(map[string]any); ok {
							m.SenderProfile, _ = member["profileUrl"].(string)
						}
					}
					if at, ok := x["deliveredAt"].(float64); ok {
						m.DeliveredAt = int64(at)
					}
					out = append(out, m)
				}
			}
			for _, v := range x {
				walk(v)
			}
		case []any:
			for _, v := range x {
				walk(v)
			}
		}
	}
	walk(data)
	return out, nil
}

type linkedinRedirect struct{ location string }

func (e *linkedinRedirect) Error() string { return "LinkedIn redirected request" }
func (l *LinkedIn) page(ctx context.Context, path string) ([]byte, error) {
	for i := 0; i < 5; i++ {
		body, e := l.request(ctx, "GET", path, "", nil)
		var redirect *linkedinRedirect
		if !errors.As(e, &redirect) {
			return body, e
		}
		base := &url.URL{Scheme: "https", Host: "www.linkedin.com", Path: path}
		u, e := base.Parse(redirect.location)
		if e != nil || u.Scheme != "https" || u.Host != "www.linkedin.com" || u.User != nil {
			return nil, errors.New("unexpected LinkedIn redirect destination")
		}
		path = u.RequestURI()
	}
	return nil, errors.New("too many LinkedIn redirects")
}
func ConversationURNs(raw []byte) ([]string, error) {
	var data any
	if e := json.Unmarshal(raw, &data); e != nil {
		return nil, e
	}
	out := []string{}
	seen := map[string]bool{}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if urn, ok := x["entityUrn"].(string); ok && strings.Contains(urn, ":msg_conversation:") && !seen[urn] {
				out = append(out, urn)
				seen[urn] = true
			}
			for _, v := range x {
				walk(v)
			}
		case []any:
			for _, v := range x {
				walk(v)
			}
		}
	}
	walk(data)
	return out, nil
}

func (l *LinkedIn) Identity(ctx context.Context) (string, error) {
	raw, e := l.Me(ctx)
	if e != nil {
		return "", e
	}
	return ParseIdentity(raw)
}
func ParseIdentity(raw []byte) (string, error) {
	var data struct {
		Data struct {
			MiniProfile string `json:"*miniProfile"`
		} `json:"data"`
		Included []struct {
			EntityURN     string `json:"entityUrn"`
			DashEntityURN string `json:"dashEntityUrn"`
		} `json:"included"`
	}
	if e := json.Unmarshal(raw, &data); e != nil {
		return "", e
	}
	for _, profile := range data.Included {
		if profile.EntityURN == data.Data.MiniProfile && strings.HasPrefix(profile.DashEntityURN, "urn:li:fsd_profile:") {
			return profile.DashEntityURN, nil
		}
	}
	return "", errors.New("LinkedIn identity did not include matching mailbox profile")
}

type LinkedInConnection struct {
	ProfileURL      string `json:"profileUrl"`
	ProfileURN      string `json:"profileUrn"`
	ConversationURN string `json:"conversationUrn"`
}

// Connections are only confirmed from explicit DISTANCE_1 participant metadata.
// Missing metadata must not be interpreted as acceptance or rejection.
func ParseConnections(raw []byte) ([]LinkedInConnection, error) {
	var data any
	if e := json.Unmarshal(raw, &data); e != nil {
		return nil, e
	}
	out := []LinkedInConnection{}
	seen := map[string]bool{}
	var walk func(any, string)
	walk = func(v any, conversation string) {
		switch x := v.(type) {
		case map[string]any:
			if urn, ok := x["entityUrn"].(string); ok && strings.Contains(urn, ":msg_conversation:") {
				conversation = urn
			}
			if pt, ok := x["participantType"].(map[string]any); ok {
				if member, ok := pt["member"].(map[string]any); ok && member["distance"] == "DISTANCE_1" {
					profile, _ := member["profileUrl"].(string)
					urn, _ := x["hostIdentityUrn"].(string)
					if profile != "" && !seen[profile] {
						seen[profile] = true
						out = append(out, LinkedInConnection{profile, urn, conversation})
					}
				}
			}
			for _, v := range x {
				walk(v, conversation)
			}
		case []any:
			for _, v := range x {
				walk(v, conversation)
			}
		}
	}
	walk(data, "")
	return out, nil
}
