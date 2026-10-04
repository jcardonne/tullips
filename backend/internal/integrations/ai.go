package integrations

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

type AI struct {
	BaseURL, Key, Model                                           string
	InputPricePerMillion, OutputPricePerMillion, CreditsPerDollar float64
}
type Completion struct {
	Text         string  `json:"text"`
	InputTokens  int64   `json:"inputTokens"`
	OutputTokens int64   `json:"outputTokens"`
	CostUSD      float64 `json:"costUSD"`
	Credits      int64   `json:"credits"`
}

func (a AI) Complete(ctx context.Context, system, prompt string) (Completion, error) {
	var out Completion
	if a.BaseURL == "" || a.Key == "" || a.Model == "" {
		return out, errors.New("AI gateway is not configured")
	}
	body, _ := json.Marshal(map[string]any{"model": a.Model, "max_tokens": 2048, "messages": []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": prompt}}})
	req, e := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(a.BaseURL, "/")+"/chat/completions", bytes.NewReader(body))
	if e != nil {
		return out, e
	}
	req.Header.Set("Authorization", "Bearer "+a.Key)
	req.Header.Set("Content-Type", "application/json")
	resp, e := (&http.Client{Timeout: 90 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(req)
	if e != nil {
		return out, e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return out, fmt.Errorf("AI gateway returned HTTP %d", resp.StatusCode)
	}
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage *struct {
			Prompt     int64 `json:"prompt_tokens"`
			Completion int64 `json:"completion_tokens"`
		} `json:"usage"`
	}
	if e = json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&result); e != nil {
		return out, e
	}
	if len(result.Choices) == 0 {
		return out, errors.New("AI returned no completion")
	}
	if result.Usage == nil || result.Usage.Prompt < 0 || result.Usage.Completion < 0 {
		return out, errors.New("AI gateway returned no valid token usage")
	}
	out.Text = result.Choices[0].Message.Content
	out.InputTokens = result.Usage.Prompt
	out.OutputTokens = result.Usage.Completion
	out.CostUSD = (float64(out.InputTokens)*a.InputPricePerMillion + float64(out.OutputTokens)*a.OutputPricePerMillion) / 1e6
	out.Credits = int64(math.Ceil(out.CostUSD * a.CreditsPerDollar))
	return out, nil
}

// EstimateMaxCredits conservatively treats every UTF-8 byte as an input token.
// Callers include system instructions and all context in prompt before reserving.
func (a AI) EstimateMaxCredits(prompt string) int64 {
	return int64(math.Ceil(((float64(len(prompt)+128) * a.InputPricePerMillion) + (2048 * a.OutputPricePerMillion)) / 1e6 * a.CreditsPerDollar))
}
