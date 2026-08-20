package clients

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewOllamaClient(t *testing.T) {
	url := "http://localhost:11434"
	model := "llama3"
	client := NewOllamaClient(url, model)

	if client.BaseURL != url {
		t.Errorf("expected BaseURL %s, got %s", url, client.BaseURL)
	}
	if client.Model != model {
		t.Errorf("expected Model %s, got %s", model, client.Model)
	}
	if client.HTTP == nil {
		t.Error("expected HTTP client to be initialized")
	} else if client.HTTP.Timeout != 10*time.Minute {
		t.Errorf("expected timeout 10m, got %v", client.HTTP.Timeout)
	}
}

func TestOllamaClient_Generate(t *testing.T) {
	tests := []struct {
		name               string
		serverResponse     interface{}
		serverStatusCode   int
		serverRawResponse  string
		shortTimeout       bool
		serverDelay        time.Duration
		expectedResponse   string
		expectedPromptEval int
		expectedEval       int
		expectError        bool
		errorContains      string
	}{
		{
			name: "Success",
			serverResponse: OllamaResponse{
				Response:        "hello world",
				PromptEvalCount: 10,
				EvalCount:       20,
			},
			serverStatusCode:   http.StatusOK,
			expectedResponse:   "hello world",
			expectedPromptEval: 10,
			expectedEval:       20,
			expectError:        false,
		},
		{
			name:              "Non-200 Status Code",
			serverRawResponse: "not found",
			serverStatusCode:  http.StatusNotFound,
			expectError:       true,
			errorContains:     "ollama API returned status 404: not found",
		},
		{
			name:              "Invalid JSON Response",
			serverRawResponse: "{invalid-json}",
			serverStatusCode:  http.StatusOK,
			expectError:       true,
		},
		{
			name:        "Network Error",
			expectError: true,
			// We'll trigger this by using an invalid URL
		},
		{
			name:             "Timeout",
			serverStatusCode: http.StatusOK,
			shortTimeout:     true,
			serverDelay:      100 * time.Millisecond,
			expectError:      true,
			errorContains:    "context deadline exceeded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ts *httptest.Server
			if tt.name != "Network Error" {
				ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if tt.serverDelay > 0 {
						time.Sleep(tt.serverDelay)
					}
					w.WriteHeader(tt.serverStatusCode)
					if tt.serverRawResponse != "" {
						_, _ = w.Write([]byte(tt.serverRawResponse))
					} else if tt.serverResponse != nil {
						_ = json.NewEncoder(w).Encode(tt.serverResponse)
					}
				}))
				defer ts.Close()
			}

			var client *OllamaClient
			if tt.name == "Network Error" {
				// Use an invalid address to trigger network failure
				client = NewOllamaClient("http://localhost:99999", "test-model")
			} else {
				client = NewOllamaClient(ts.URL, "test-model")
			}

			if tt.shortTimeout {
				client.HTTP.Timeout = 10 * time.Millisecond
			}

			resp, promptEval, eval, err := client.Generate(context.Background(), "test prompt")

			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error containing %q, got %q", tt.errorContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if resp != tt.expectedResponse {
				t.Errorf("expected response %s, got %s", tt.expectedResponse, resp)
			}
			if promptEval != tt.expectedPromptEval {
				t.Errorf("expected prompt eval %d, got %d", tt.expectedPromptEval, promptEval)
			}
			if eval != tt.expectedEval {
				t.Errorf("expected eval %d, got %d", tt.expectedEval, eval)
			}
		})
	}
}
