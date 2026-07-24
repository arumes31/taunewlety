package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type OllamaClient struct {
	BaseURL string
	Model   string
	HTTP    *http.Client
}

func NewOllamaClient(url, model string) *OllamaClient {
	return &OllamaClient{
		BaseURL: url,
		Model:   model,
		HTTP: &http.Client{
			Timeout: 10 * time.Minute, // High timeout for CPU inference
		},
	}
}

type OllamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type OllamaResponse struct {
	Response        string `json:"response"`
	PromptEvalCount int    `json:"prompt_eval_count"`
	EvalCount       int    `json:"eval_count"`
}

// Generate sends a prompt to the Ollama API and returns the response.
// The ctx parameter allows cancellation when the HTTP handler's context
// is cancelled (e.g., user closes the browser).
func (c *OllamaClient) Generate(ctx context.Context, prompt string) (string, int, int, error) {
	reqBody := OllamaRequest{
		Model:  c.Model,
		Prompt: prompt,
		Stream: false,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", 0, 0, fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/api/generate", c.BaseURL), bytes.NewBuffer(jsonData))
	if err != nil {
		return "", 0, 0, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", 0, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", 0, 0, fmt.Errorf("ollama API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var ollamaResp OllamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return "", 0, 0, err
	}

	return ollamaResp.Response, ollamaResp.PromptEvalCount, ollamaResp.EvalCount, nil
}
