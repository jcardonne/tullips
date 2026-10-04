package core

import (
	"encoding/json"
	"errors"
	"strings"
)

type Qualification struct {
	URL    string `json:"url"`
	Match  *bool  `json:"match"`
	Score  int    `json:"score"`
	Reason string `json:"reason"`
}

func parseQualifications(text string, allowed map[string]bool) (map[string]Qualification, error) {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	var all []Qualification
	if e := json.Unmarshal([]byte(strings.TrimSpace(text)), &all); e != nil {
		return nil, e
	}
	out := map[string]Qualification{}
	for _, q := range all {
		q.URL = strings.TrimRight(q.URL, "/")
		if !allowed[q.URL] {
			return nil, errors.New("qualification refers to an unknown profile")
		}
		if _, exists := out[q.URL]; exists {
			return nil, errors.New("duplicate qualification")
		}
		if q.Score < 0 || q.Score > 100 || q.Reason == "" {
			return nil, errors.New("invalid qualification evidence")
		}
		out[q.URL] = q
	}
	return out, nil
}
