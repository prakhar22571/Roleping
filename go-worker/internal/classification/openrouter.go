package classification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/syumai/workers/cloudflare/fetch"
)

const openRouterURL = "https://openrouter.ai/api/v1/chat/completions"

var retryDelays = []time.Duration{500 * time.Millisecond, 1500 * time.Millisecond}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type ChatCompletionResult struct {
	RawResponse string
	Content     string
}

func CallOpenRouter(ctx context.Context, client *fetch.Client, apiKey, model, systemPrompt, userPrompt string) (*ChatCompletionResult, error) {
	var lastErr error

	for attempt := 0; attempt <= len(retryDelays); attempt++ {
		result, retryable, err := doOpenRouterRequest(ctx, client, apiKey, model, systemPrompt, userPrompt)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !retryable || attempt == len(retryDelays) {
			break
		}
		time.Sleep(retryDelays[attempt])
	}

	return nil, lastErr
}

func doOpenRouterRequest(ctx context.Context, client *fetch.Client, apiKey, model, systemPrompt, userPrompt string) (*ChatCompletionResult, bool, error) {
	body, err := json.Marshal(chatCompletionRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0,
	})
	if err != nil {
		return nil, false, err
	}

	req, err := fetch.NewRequest(ctx, http.MethodPost, openRouterURL, bytes.NewReader(body))
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req, nil)
	if err != nil {
		return nil, true, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		return nil, retryable, fmt.Errorf("openrouter request failed: %s", resp.Status)
	}

	var parsed chatCompletionResponse
	rawBytes := new(bytes.Buffer)
	if _, err := rawBytes.ReadFrom(resp.Body); err != nil {
		return nil, true, err
	}
	raw := rawBytes.String()

	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, false, fmt.Errorf("failed to decode openrouter response: %w", err)
	}
	if len(parsed.Choices) == 0 || parsed.Choices[0].Message.Content == "" {
		return nil, false, fmt.Errorf("openrouter response missing choices[0].message.content")
	}

	return &ChatCompletionResult{RawResponse: raw, Content: parsed.Choices[0].Message.Content}, false, nil
}
