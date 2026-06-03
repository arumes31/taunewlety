package clients

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	Response           string `json:"response"`
	PromptEvalCount    int    `json:"prompt_eval_count"`
	EvalCount          int    `json:"eval_count"`
}

func (c *OllamaClient) Generate(prompt string) (string, int, int, error) {
	reqBody := OllamaRequest{
		Model:  c.Model,
		Prompt: prompt,
		Stream: false,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", 0, 0, err
	}

	resp, err := c.HTTP.Post(fmt.Sprintf("%s/api/generate", c.BaseURL), "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", 0, 0, err
	}
	defer resp.Body.Close()

	var ollamaResp OllamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return "", 0, 0, err
	}

	return ollamaResp.Response, ollamaResp.PromptEvalCount, ollamaResp.EvalCount, nil
}
