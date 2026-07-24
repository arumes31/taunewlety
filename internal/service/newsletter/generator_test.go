package newsletter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"taunewlety/internal/domain/models"
	"taunewlety/internal/platform/clients"
	"taunewlety/internal/platform/database"
	"testing"
)

func TestNewsletterService_GenerateAIContent(t *testing.T) {
	os.Setenv("DB_PATH", ":memory:")
	defer os.Unsetenv("DB_PATH")

	db, err := database.InitDB()
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}

	tests := []struct {
		name           string
		config         *models.Config
		selection      []Candidate
		ollamaResponse string
		ollamaError    bool
		wantSubject    string
		wantBody       string
		wantErr        bool
	}{
		{
			name: "Success with JSON fences",
			config: &models.Config{
				Language:    "de_DE",
				OllamaModel: "test-model",
			},
			selection: []Candidate{
				{
					Item: clients.RecentlyAddedItem{
						Title:  "Test Movie",
						Year:   2026,
						Genres: "Action",
						Rating: 7.5,
					},
					Tags: []string{"Critically Acclaimed"},
				},
			},
			ollamaResponse: "Greeting... ```json\n{\"subject\": \"Success Subject\", \"body\": \"Success Body\"}\n```",
			wantSubject:    "Success Subject",
			wantBody:       "Success Body",
		},
		{
			name:           "Success with raw fences",
			ollamaResponse: "```\n{\"subject\": \"Raw Fence Subject\", \"body\": \"Raw Fence Body\"}\n```",
			wantSubject:    "Raw Fence Subject",
			wantBody:       "Raw Fence Body",
		},
		{
			name:           "Success with no fences",
			ollamaResponse: "{\"subject\": \"No Fence Subject\", \"body\": \"No Fence Body\"}",
			wantSubject:    "No Fence Subject",
			wantBody:       "No Fence Body",
		},
		{
			name:           "Error when JSON is invalid",
			ollamaResponse: "This is not JSON",
			wantErr:        true,
		},
		{
			name:        "Ollama client error",
			ollamaError: true,
			wantErr:     true,
		},
		{
			name: "Metadata edge cases (nil/0 values)",
			selection: []Candidate{
				{
					Item: clients.RecentlyAddedItem{
						Title:  "Edge Movie",
						Year:   0,
						Genres: "",
						Rating: 0,
					},
				},
				{
					Item: clients.RecentlyAddedItem{
						Title: "Missing Fields Movie",
					},
				},
			},
			ollamaResponse: "{\"subject\": \"Edge Case Subject\", \"body\": \"Edge Case Body\"}",
			wantSubject:    "Edge Case Subject",
			wantBody:       "Edge Case Body",
		},
		{
			name: "Language fallback to en_US",
			config: &models.Config{
				Language: "",
			},
			ollamaResponse: "{\"subject\": \"Lang Subject\", \"body\": \"Lang Body\"}",
			wantSubject:    "Lang Subject",
			wantBody:       "Lang Body",
		},
		{
			name:           "Broken fences (missing closing ```)",
			ollamaResponse: "```json\n{\"subject\": \"Broken Subject\", \"body\": \"Broken Body\"}",
			wantSubject:    "Broken Subject",
			wantBody:       "Broken Body",
		},
		{
			name:           "Broken plain fences (missing closing ```)",
			ollamaResponse: "```\n{\"subject\": \"Broken Subject\", \"body\": \"Broken Body\"}",
			wantSubject:    "Broken Subject",
			wantBody:       "Broken Body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.ollamaError {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				resp := clients.OllamaResponse{
					Response:         tt.ollamaResponse,
					PromptEvalCount:  10,
					EvalCount:        20,
				}
				_ = json.NewEncoder(w).Encode(resp)
			}))
			defer ts.Close()

			conf := tt.config
			if conf == nil {
				conf = &models.Config{}
			}
			conf.OllamaURL = ts.URL

			svc := NewNewsletterService(db, conf)
			content, err := svc.GenerateAIContent(context.Background(), tt.selection)

			if (err != nil) != tt.wantErr {
				t.Fatalf("wantErr = %v, got err = %v", tt.wantErr, err)
			}

			if !tt.wantErr {
				if content.Subject != tt.wantSubject {
					t.Errorf("wantSubject = %v, got = %v", tt.wantSubject, content.Subject)
				}
				if content.Body != tt.wantBody {
					t.Errorf("wantBody = %v, got = %v", tt.wantBody, content.Body)
				}
			}
		})
	}
}
