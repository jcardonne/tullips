package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/net/html"
	"net/url"
	"strings"
)

// RSCObjects reads JSON-valued React Flight rows and the HTML rehydration array.
// It does not execute scripts. Unknown row encodings are ignored, not evaluated.
func RSCObjects(body []byte) []map[string]any {
	var roots []any
	var consume func(string, int)
	consume = func(s string, depth int) {
		if depth > 8 {
			return
		}
		s = strings.TrimSpace(s)
		var v any
		if json.Unmarshal([]byte(s), &v) == nil {
			roots = append(roots, v)
			if x, ok := v.(string); ok {
				consume(x, depth+1)
			}
			if a, ok := v.([]any); ok {
				for _, x := range a {
					if q, ok := x.(string); ok {
						consume(q, depth+1)
					}
				}
			}
			return
		}
		for _, line := range strings.Split(s, "\n") {
			_, tail, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			if json.Unmarshal([]byte(tail), &v) == nil {
				roots = append(roots, v)
			}
		}
	}
	if strings.Contains(string(body), "<script") {
		doc, e := html.Parse(strings.NewReader(string(body)))
		if e == nil {
			var walk func(*html.Node)
			walk = func(n *html.Node) {
				if n.Type == html.ElementNode && n.Data == "script" {
					for c := n.FirstChild; c != nil; c = c.NextSibling {
						if i := strings.Index(c.Data, "window.__como_rehydration__"); i >= 0 {
							_, s, ok := strings.Cut(c.Data[i:], "=")
							if ok {
								var raw json.RawMessage
								if json.NewDecoder(strings.NewReader(s)).Decode(&raw) == nil {
									consume(string(raw), 0)
								}
							}
						}
					}
				}
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c)
				}
			}
			walk(doc)
		}
	} else {
		consume(string(body), 0)
	}
	var objects []map[string]any
	var walk func(any, int)
	walk = func(v any, depth int) {
		if depth > 100 {
			return
		}
		switch x := v.(type) {
		case map[string]any:
			objects = append(objects, x)
			for _, v := range x {
				walk(v, depth+1)
			}
		case []any:
			for _, v := range x {
				walk(v, depth+1)
			}
		case string:
			if strings.HasPrefix(x, "{") || strings.HasPrefix(x, "[") {
				var y any
				if json.Unmarshal([]byte(x), &y) == nil {
					walk(y, depth+1)
				}
			}
		}
	}
	for _, v := range roots {
		walk(v, 0)
	}
	return objects
}
func FindAction(body []byte, id string) (map[string]any, error) {
	for _, o := range RSCObjects(body) {
		if o["requestId"] == id {
			if args, ok := o["requestedArguments"].(map[string]any); ok {
				if id == "com.linkedin.sdui.requests.login.authenticate" {
					payload, _ := args["payload"].(map[string]any)
					if _, ok := payload["password"]; !ok {
						continue
					}
				}
				return o, nil
			}
		}
	}
	return nil, fmt.Errorf("%w: fresh LinkedIn action %s not present", ErrUnsupported, id)
}
func BuildAction(action map[string]any, values map[string]string) ([]byte, error) {
	raw, e := json.Marshal(action)
	if e != nil {
		return nil, e
	}
	var copy map[string]any
	if e = json.Unmarshal(raw, &copy); e != nil {
		return nil, e
	}
	args, ok := copy["requestedArguments"].(map[string]any)
	if !ok {
		return nil, errors.New("action has no arguments")
	}
	payload, _ := args["payload"].(map[string]any)
	states := []any{}
	for field, value := range values {
		binding, ok := payload[field].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("action missing %s binding", field)
		}
		key, ok := binding["key"].(string)
		if !ok {
			return nil, errors.New("invalid state key")
		}
		states = append(states, map[string]any{"key": key, "namespace": binding["namespace"], "value": value, "originalProtoCase": "stringValue", "protoKey": map[string]any{"$type": "proto.sdui.Key", "value": map[string]any{"$case": "id", "id": key}}})
	}
	outerArgs := map[string]any{}
	for k, v := range args {
		outerArgs[k] = v
	}
	outerArgs["states"] = states
	outerArgs["knownTemplateIds"] = []any{}
	return json.Marshal(map[string]any{"requestId": copy["requestId"], "serverRequest": copy, "states": states, "requestedArguments": outerArgs})
}
func (l *LinkedIn) action(ctx context.Context, page []byte, id string, values map[string]string) ([]byte, error) {
	a, e := FindAction(page, id)
	if e != nil {
		return nil, e
	}
	b, e := BuildAction(a, values)
	if e != nil {
		return nil, e
	}
	return l.request(ctx, "POST", "/flagship-web/rsc-action/actions/server-request", "application/json", b)
}
func (l *LinkedIn) Login(ctx context.Context, email, password string) error {
	if email == "" || password == "" {
		return errors.New("email and password required")
	}
	page, e := l.page(ctx, "/login/fr")
	if e != nil {
		return e
	}
	b, e := l.action(ctx, page, "com.linkedin.sdui.requests.login.authenticate", map[string]string{"identifier": email, "password": password, "rememberMeOptInCheckboxState": "true", "apfc": ""})
	if e != nil {
		return e
	}
	for _, o := range RSCObjects(b) {
		for _, v := range o {
			if s, ok := v.(string); ok && strings.Contains(s, "/checkpoint/") {
				u, e := url.Parse(s)
				if e == nil && (u.Host == "" || u.Host == "www.linkedin.com") {
					l.checkpoint = u.RequestURI()
					return ErrCheckpoint
				}
			}
		}
	}
	_, e = l.Me(ctx)
	return e
}
func (l *LinkedIn) CheckpointURL() string { return l.checkpoint }
func (l *LinkedIn) Verify(ctx context.Context, checkpoint, code string) error {
	u, e := url.Parse(checkpoint)
	if e != nil || (u.Host != "" && u.Host != "www.linkedin.com") || !strings.HasPrefix(u.Path, "/checkpoint/") {
		return errors.New("invalid checkpoint URL")
	}
	page, e := l.page(ctx, u.RequestURI())
	if e != nil {
		return e
	}
	form, e := CheckpointForm(page, code)
	if e != nil {
		return e
	}
	_, e = l.request(ctx, "POST", "/checkpoint/challenge/verifyV2", "application/x-www-form-urlencoded", []byte(form.Encode()))
	if e != nil {
		return e
	}
	_, e = l.Me(ctx)
	return e
}
func CheckpointForm(page []byte, code string) (url.Values, error) {
	doc, e := html.Parse(strings.NewReader(string(page)))
	if e != nil {
		return nil, e
	}
	values := url.Values{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "input" {
			name, value := "", ""
			for _, a := range n.Attr {
				if a.Key == "name" {
					name = a.Val
				}
				if a.Key == "value" {
					value = a.Val
				}
			}
			switch name {
			case "csrfToken", "challengeId", "language", "displayTime", "challengeType", "challengeSource", "recognizedDevice", "requestSubmissionId", "challengeData", "challengeDetails":
				values.Set(name, value)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	if values.Get("csrfToken") == "" || values.Get("challengeId") == "" {
		return nil, fmt.Errorf("%w: checkpoint form missing fresh tokens", ErrUnsupported)
	}
	values.Set("userResponse", code)
	return values, nil
}
func (l *LinkedIn) Invite(ctx context.Context, profileURL string) error {
	u, e := url.Parse(profileURL)
	if e != nil || u.Host != "www.linkedin.com" || !strings.HasPrefix(u.Path, "/in/") {
		return errors.New("expected LinkedIn profile URL")
	}
	page, e := l.page(ctx, u.RequestURI())
	if e != nil {
		return e
	}
	var action map[string]any
	for _, o := range RSCObjects(page) {
		if o["requestId"] != "com.linkedin.sdui.requests.mynetwork.addaAddConnection" {
			continue
		}
		args, _ := o["requestedArguments"].(map[string]any)
		payload, _ := args["payload"].(map[string]any)
		canonical, _ := payload["profileCanonicalUrl"].(string)
		candidate, err := url.Parse(canonical)
		if err == nil && strings.TrimSuffix(candidate.Path, "/") == strings.TrimSuffix(u.Path, "/") {
			action = o
			break
		}
	}
	if action == nil {
		return fmt.Errorf("%w: no invitation action matches requested profile", ErrUnsupported)
	}
	body, e := BuildAction(action, nil)
	if e != nil {
		return e
	}
	result, e := l.request(ctx, "POST", "/flagship-web/rsc-action/actions/server-request", "application/json", body)
	if e != nil {
		return e
	}
	return ValidateActionResult(result)
}

type Prospect struct {
	URL      string `json:"url"`
	Name     string `json:"name"`
	Headline string `json:"headline,omitempty"`
	Company  string `json:"company,omitempty"`
	Location string `json:"location,omitempty"`
}

func (l *LinkedIn) Search(ctx context.Context, keywords string, start int) ([]Prospect, error) {
	if start < 0 {
		return nil, errors.New("invalid page offset")
	}
	q := url.Values{"keywords": {keywords}, "origin": {"GLOBAL_SEARCH_HEADER"}, "start": {fmt.Sprint(start)}}
	b, e := l.page(ctx, "/search/results/people/?"+q.Encode())
	if e != nil {
		return nil, e
	}
	return ParseProspects(b), nil
}
func basicProspects(b []byte) []Prospect {
	out := []Prospect{}
	seen := map[string]bool{}
	for _, o := range RSCObjects(b) {
		for _, v := range o {
			s, ok := v.(string)
			if !ok || !strings.Contains(s, "linkedin.com/in/") {
				continue
			}
			u, e := url.Parse(s)
			if e != nil || u.Host != "www.linkedin.com" || !strings.HasPrefix(u.Path, "/in/") {
				continue
			}
			u.RawQuery = ""
			u.Fragment = ""
			if seen[u.String()] {
				continue
			}
			seen[u.String()] = true
			name, _ := o["name"].(string)
			if name == "" {
				name, _ = o["text"].(string)
			}
			out = append(out, Prospect{URL: u.String(), Name: name})
		}
	}
	return out
}

// A 200 response may still contain application errors. Never mark those as sent.
func ValidateActionResult(body []byte) error {
	for _, o := range RSCObjects(body) {
		response, ok := o["response"].(map[string]any)
		if !ok {
			continue
		}
		es, present := response["errors"]
		if !present {
			return errors.New("unrecognized LinkedIn action result")
		}
		switch value := es.(type) {
		case []any:
			if len(value) > 0 {
				return errors.New("LinkedIn action rejected")
			}
			return nil
		case string:
			if strings.HasPrefix(value, "$") {
				for _, line := range strings.Split(string(body), "\n") {
					id, raw, ok := strings.Cut(line, ":")
					if ok && id == strings.TrimPrefix(value, "$") {
						var errorsList []any
						if json.Unmarshal([]byte(raw), &errorsList) != nil || len(errorsList) > 0 {
							return errors.New("LinkedIn action rejected")
						}
						return nil
					}
				}
			}
		}
		return errors.New("unrecognized LinkedIn action errors")
	}
	return errors.New("unrecognized LinkedIn action response; delivery not confirmed")
}

// IsConnected confirms an explicit relationship scoped to the requested profile.
// It never infers acceptance from missing buttons or unrelated recommended profiles.
func (l *LinkedIn) IsConnected(ctx context.Context, profileURL string) (bool, error) {
	u, e := url.Parse(profileURL)
	if e != nil || u.Host != "www.linkedin.com" || !strings.HasPrefix(u.Path, "/in/") {
		return false, errors.New("expected LinkedIn profile URL")
	}
	body, e := l.page(ctx, u.RequestURI())
	if e != nil {
		return false, e
	}
	return ParseRelationship(body, profileURL)
}
func ParseRelationship(body []byte, profileURL string) (bool, error) {
	target, e := url.Parse(profileURL)
	if e != nil {
		return false, e
	}
	for _, o := range RSCObjects(body) {
		distance, _ := o["distance"].(string)
		if distance == "" {
			continue
		}
		for _, key := range []string{"profileUrl", "profileCanonicalUrl"} {
			raw, _ := o[key].(string)
			u, e := url.Parse(raw)
			if e != nil || u.Host != target.Host || strings.TrimSuffix(u.Path, "/") != strings.TrimSuffix(target.Path, "/") {
				continue
			}
			switch distance {
			case "DISTANCE_1":
				return true, nil
			case "DISTANCE_2", "DISTANCE_3", "OUT_OF_NETWORK":
				return false, nil
			}
		}
	}
	return false, fmt.Errorf("%w: explicit relationship for target profile unavailable", ErrUnsupported)
}
