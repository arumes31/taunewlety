package newsletter

import (
	"errors"
	"os"
	"strings"
	"taunewlety/internal/domain/models"
	"taunewlety/internal/platform/database"
	"testing"
)

func TestNewsletterService_GenerateNewsletter(t *testing.T) {
	os.Setenv("DB_PATH", ":memory:")
	defer os.Unsetenv("DB_PATH")

	database.InitDB()

	tests := []struct {
		name           string
		mockTautulli   *mockTautulliClient
		mockOllama     *mockOllamaClient
		wantErr        bool
		errContains    string
		wantSubject    string
		wantBody       string
		checkBlacklist bool
	}{
		{
			name: "Successful newsletter generation",
			mockTautulli: &mockTautulliClient{
				GetRecentlyAddedFn: func(count int) ([]map[string]interface{}, error) {
					return []map[string]interface{}{
						{
							"rating_key": "mov_1",
							"title":      "Test Movie",
							"media_type": "movie",
						},
					}, nil
				},
				GetTopGenresFn: func(count int) ([]string, error) {
					return []string{"Action"}, nil
				},
			},
			mockOllama: &mockOllamaClient{
				GenerateFn: func(prompt string) (string, int, int, error) {
					return `{"subject": "Plex Subject", "body": "Plex Body"}`, 100, 100, nil
				},
			},
			wantErr:        false,
			wantSubject:    "Plex Subject",
			wantBody:       "Plex Body",
			checkBlacklist: true,
		},
		{
			name: "No recommendations available",
			mockTautulli: &mockTautulliClient{
				GetRecentlyAddedFn: func(count int) ([]map[string]interface{}, error) {
					return []map[string]interface{}{}, nil
				},
				GetTopGenresFn: func(count int) ([]string, error) {
					return []string{}, nil
				},
			},
			wantErr:     true,
			errContains: ErrNoRecommendations.Error(),
		},
		{
			name: "Tautulli error during mix",
			mockTautulli: &mockTautulliClient{
				GetRecentlyAddedFn: func(count int) ([]map[string]interface{}, error) {
					return nil, errors.New("tautulli error")
				},
			},
			wantErr:     true,
			errContains: "tautulli error",
		},
		{
			name: "Ollama error during generation",
			mockTautulli: &mockTautulliClient{
				GetRecentlyAddedFn: func(count int) ([]map[string]interface{}, error) {
					return []map[string]interface{}{
						{
							"rating_key": "mov_1",
							"title":      "Test Movie",
							"media_type": "movie",
						},
					}, nil
				},
			},
			mockOllama: &mockOllamaClient{
				GenerateFn: func(prompt string) (string, int, int, error) {
					return "", 0, 0, errors.New("ollama error")
				},
			},
			wantErr:     true,
			errContains: "ollama error",
		},
		{
			name: "Malformed AI response",
			mockTautulli: &mockTautulliClient{
				GetRecentlyAddedFn: func(count int) ([]map[string]interface{}, error) {
					return []map[string]interface{}{
						{
							"rating_key": "mov_1",
							"title":      "Test Movie",
							"media_type": "movie",
						},
					}, nil
				},
			},
			mockOllama: &mockOllamaClient{
				GenerateFn: func(prompt string) (string, int, int, error) {
					return `invalid json`, 100, 100, nil
				},
			},
			wantErr:     true,
			errContains: "failed to parse AI response as JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &NewsletterService{
				Tautulli: tt.mockTautulli,
				Ollama:   tt.mockOllama,
				Config:   &models.Config{},
			}

			subject, body, err := svc.GenerateNewsletter()
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateNewsletter() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil {
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("GenerateNewsletter() error = %v, wantErrContains %v", err, tt.errContains)
				}
				return
			}

			if subject != tt.wantSubject {
				t.Errorf("GenerateNewsletter() subject = %v, want %v", subject, tt.wantSubject)
			}
			if body != tt.wantBody {
				t.Errorf("GenerateNewsletter() body = %v, want %v", body, tt.wantBody)
			}

			if tt.checkBlacklist {
				var bls []models.Blacklist
				database.GetDB().Find(&bls)
				if len(bls) == 0 {
					t.Errorf("expected blacklist items to be created")
				}
				// Clean up for next test case
				database.GetDB().Exec("DELETE FROM blacklists")
				database.GetDB().Exec("DELETE FROM recommendation_stats")
			}
		})
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

// Re-using TestNewsletterService_SendEmail and TestNewsletterService_SendNotifications from notifier_test.go
// but those are already in another file in the same package.
