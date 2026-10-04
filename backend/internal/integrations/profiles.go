package integrations

import (
	"encoding/json"
	"golang.org/x/net/html"
	"net/url"
	"strings"
)

// flightData preserves shared references: text components are commonly separate
// rows referenced as $L<hex-id>, rather than children of the profile link itself.
func flightData(body []byte) ([]any, map[string]any) {
	roots := []any{}
	refs := map[string]any{}
	var consume func(string, int)
	consume = func(s string, depth int) {
		if depth > 8 {
			return
		}
		s = strings.TrimSpace(s)
		var v any
		if json.Unmarshal([]byte(s), &v) == nil {
			roots = append(roots, v)
			if a, ok := v.([]any); ok {
				for _, item := range a {
					if text, ok := item.(string); ok {
						consume(text, depth+1)
					}
				}
			}
			return
		}
		for _, line := range strings.Split(s, "\n") {
			id, tail, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			if json.Unmarshal([]byte(tail), &v) == nil {
				roots = append(roots, v)
				refs[id] = v
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
							_, tail, ok := strings.Cut(c.Data[i:], "=")
							if ok {
								var raw json.RawMessage
								if json.NewDecoder(strings.NewReader(tail)).Decode(&raw) == nil {
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
	return roots, refs
}
func flightRef(v any, refs map[string]any) any {
	for i := 0; i < 20; i++ {
		s, ok := v.(string)
		if !ok || !strings.HasPrefix(s, "$") {
			break
		}
		key := strings.TrimPrefix(s, "$")
		key = strings.TrimPrefix(key, "L")
		next, ok := refs[key]
		if !ok {
			break
		}
		v = next
	}
	return v
}
func profileURL(raw string) string {
	u, e := url.Parse(raw)
	if e != nil || u.Host != "www.linkedin.com" || !strings.HasPrefix(u.Path, "/in/") {
		return ""
	}
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}
func profileTexts(v any, refs map[string]any) []string {
	out := []string{}
	budget := 2000
	var content func(any, int) string
	content = func(v any, d int) string {
		if d > 30 {
			return ""
		}
		v = flightRef(v, refs)
		switch x := v.(type) {
		case string:
			if strings.HasPrefix(x, "$") {
				return ""
			}
			return strings.TrimSpace(x)
		case []any:
			if len(x) >= 4 && x[0] == "$" {
				return content(x[3], d+1)
			}
			for _, v := range x {
				if text := content(v, d+1); text != "" {
					return text
				}
			}
		case map[string]any:
			return content(x["children"], d+1)
		}
		return ""
	}
	var walk func(any, int)
	walk = func(v any, d int) {
		budget--
		if budget < 0 || d > 50 {
			return
		}
		v = flightRef(v, refs)
		switch x := v.(type) {
		case map[string]any:
			if text, ok := x["textProps"].(map[string]any); ok {
				if value := content(text["children"], 0); value != "" {
					out = append(out, value)
				}
				return
			}
			for _, key := range []string{"children", "initialContent"} {
				if child, ok := x[key]; ok {
					walk(child, d+1)
				}
			}
		case []any:
			if len(x) >= 4 && x[0] == "$" {
				walk(x[3], d+1)
			} else {
				for _, child := range x {
					walk(child, d+1)
				}
			}
		}
	}
	walk(v, 0)
	return out
}
func ParseProspects(body []byte) []Prospect {
	out := basicProspects(body)
	index := map[string]int{}
	qualified := map[string]bool{}
	for i, p := range out {
		index[p.URL] = i
	}
	roots, refs := flightData(body)
	budget := 200000
	var walk func(any, []map[string]any, int)
	walk = func(v any, parents []map[string]any, depth int) {
		budget--
		if budget < 0 || depth > 70 {
			return
		}
		v = flightRef(v, refs)
		switch x := v.(type) {
		case map[string]any:
			for _, value := range x {
				raw, ok := value.(string)
				if !ok {
					continue
				}
				link := profileURL(raw)
				if link == "" {
					continue
				}
				for i := len(parents) - 1; i >= 0; i-- {
					children, ok := parents[i]["children"]
					if !ok {
						continue
					}
					if !singleProfile(parents[i], refs, link) {
						continue
					}
					texts := profileTexts(children, refs)
					if len(texts) < 2 {
						continue
					}
					idx, ok := index[link]
					if !ok {
						idx = len(out)
						index[link] = idx
						out = append(out, Prospect{URL: link})
					}
					if !qualified[link] {
						qualified[link] = true
						out[idx].Name = texts[0]
						out[idx].Headline = texts[1]
						if len(texts) > 2 {
							out[idx].Location = texts[2]
						}
					}
					break
				}
			}
			next := append(append([]map[string]any{}, parents...), x)
			for _, child := range x {
				walk(child, next, depth+1)
			}
		case []any:
			for _, child := range x {
				walk(child, parents, depth+1)
			}
		}
	}
	for _, root := range roots {
		walk(root, nil, 0)
	}
	results := []Prospect{}
	for _, p := range out {
		if qualified[p.URL] {
			results = append(results, p)
		}
	}
	return results
}

func singleProfile(v any, refs map[string]any, target string) bool {
	urls := map[string]bool{}
	budget := 4000
	var walk func(any, int)
	walk = func(v any, d int) {
		budget--
		if budget < 0 || d > 50 || len(urls) > 1 {
			return
		}
		v = flightRef(v, refs)
		switch x := v.(type) {
		case string:
			if u := profileURL(x); u != "" {
				urls[u] = true
			}
		case map[string]any:
			for _, v := range x {
				walk(v, d+1)
			}
		case []any:
			for _, v := range x {
				walk(v, d+1)
			}
		}
	}
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	triggers, ok := m["triggers"]
	if !ok {
		return false
	}
	walk(triggers, 0)
	return budget >= 0 && len(urls) == 1 && urls[target]
}
