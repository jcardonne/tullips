package integrations

import (
	"context"
	"errors"
	"os"
	"strconv"
)

func envFloat(k string) float64 { v, _ := strconv.ParseFloat(os.Getenv(k), 64); return v }
func ConfiguredAI() AI {
	return AI{BaseURL: os.Getenv("LLM_BASE_URL"), Key: os.Getenv("LLM_API_KEY"), Model: os.Getenv("LLM_MODEL"), InputPricePerMillion: envFloat("LLM_INPUT_PRICE_PER_MILLION"), OutputPricePerMillion: envFloat("LLM_OUTPUT_PRICE_PER_MILLION"), CreditsPerDollar: envFloat("AI_CREDITS_PER_DOLLAR")}
}
func Dispatch(ctx context.Context, kind string, payload map[string]any) (map[string]any, error) {
	switch kind {
	case "website.analyze":
		raw, _ := payload["url"].(string)
		pages, e := Crawl(ctx, raw, 5)
		if e != nil {
			return nil, e
		}
		return map[string]any{"pages": pages}, nil
	default:
		return nil, errors.New("provider action not enabled; connect and validate provider before execution")
	}
}
