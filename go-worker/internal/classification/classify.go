package classification

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/syumai/workers/cloudflare/fetch"

	"roleping-worker/internal/adapters"
)

type Verdict struct {
	ModelID     string
	Match       bool
	Confidence  *float64
	Reasoning   string
	RawResponse *string
}

type parsedVerdict struct {
	Match      bool     `json:"match"`
	Confidence *float64 `json:"confidence"`
	Reasoning  string   `json:"reasoning"`
}

func stripCodeFences(text string) string {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```JSON")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	return strings.TrimSpace(text)
}

func parseVerdict(content string) (*parsedVerdict, error) {
	cleaned := stripCodeFences(content)
	var v parsedVerdict
	if err := json.Unmarshal([]byte(cleaned), &v); err != nil {
		return nil, fmt.Errorf("verdict JSON parse failed: %w", err)
	}
	if v.Reasoning == "" {
		return nil, fmt.Errorf("verdict JSON missing required fields")
	}
	return &v, nil
}

func classifyWithModel(ctx context.Context, client *fetch.Client, apiKey, model string, job adapters.NormalizedJob) Verdict {
	result, err := CallOpenRouter(ctx, client, apiKey, model, SystemPrompt, BuildUserPrompt(job))
	if err != nil {
		return Verdict{
			ModelID:   model,
			Match:     true,
			Reasoning: fmt.Sprintf("classification_failed: %s", err.Error()),
		}
	}

	parsed, err := parseVerdict(result.Content)
	if err != nil {
		return Verdict{
			ModelID:   model,
			Match:     true,
			Reasoning: fmt.Sprintf("classification_failed: %s", err.Error()),
		}
	}

	rawResponse := result.RawResponse
	return Verdict{
		ModelID:     model,
		Match:       parsed.Match,
		Confidence:  parsed.Confidence,
		Reasoning:   parsed.Reasoning,
		RawResponse: &rawResponse,
	}
}

// RunClassification classifies a job with both configured models in parallel.
func RunClassification(ctx context.Context, client *fetch.Client, apiKey, modelA, modelB string, job adapters.NormalizedJob) (Verdict, Verdict) {
	var verdictA, verdictB Verdict
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		verdictA = classifyWithModel(ctx, client, apiKey, modelA, job)
	}()
	go func() {
		defer wg.Done()
		verdictB = classifyWithModel(ctx, client, apiKey, modelB, job)
	}()

	wg.Wait()
	return verdictA, verdictB
}
