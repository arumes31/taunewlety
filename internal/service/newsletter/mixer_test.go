package newsletter

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"taunewlety/internal/domain/models"
	"taunewlety/internal/platform/clients"
	"taunewlety/internal/platform/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestMixRecommendations(t *testing.T) {
	// Setup DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect database: %v", err)
	}
	db.AutoMigrate(&models.Blacklist{})
	database.DB = db

	tests := []struct {
		name         string
		setupMock    func(m *mockTautulliClient)
		setupDB      func(db *gorm.DB)
		wantErr      bool
		checkResults func(t *testing.T, res []Candidate)
	}{
		{
			name: "GetRecentlyAdded Error",
			setupMock: func(m *mockTautulliClient) {
				m.GetRecentlyAddedFn = func(count int) ([]map[string]interface{}, error) {
					return nil, errors.New("tautulli error")
				}
			},
			wantErr: true,
		},
		{
			name: "Full Mix Success",
			setupMock: func(m *mockTautulliClient) {
				m.GetRecentlyAddedFn = func(count int) ([]map[string]interface{}, error) {
					return []map[string]interface{}{
						{"rating_key": "1", "rating": "8.5", "genres": "Action", "title": "High Rated"},
						{"rating_key": "2", "rating": "5.0", "genres": "Drama", "title": "Trending"},
						{"rating_key": "3", "rating": "6.0", "genres": "Comedy", "title": "Genre Match"},
						{"rating_key": "4", "rating": "4.0", "genres": "Horror", "title": "Fresh"},
						{"rating_key": "5", "rating": "7.0", "genres": "Sci-Fi", "title": "Blacklisted"},
						{"rating_key": "6", "rating": "7.0", "genres": "Sci-Fi", "title": "Expired Blacklist"},
					}, nil
				}
				m.GetTopGenresFn = func(count int) ([]string, error) {
					return []string{"Comedy"}, nil
				}
				m.GetWatchHistoryBatchFn = func(ratingKeys []string) (map[string]clients.WatchInfo, error) {
					return map[string]clients.WatchInfo{
						"2": {WatchCount: 5},
					}, nil
				}
				m.GetTopWatchedFn = func(count int) ([]map[string]interface{}, error) {
					return []map[string]interface{}{
						{"rating_key": "99", "title": "Surprise Me"},
					}, nil
				}
			},
			setupDB: func(db *gorm.DB) {
				db.Exec("DELETE FROM blacklists")
				db.Create(&models.Blacklist{
					MediaID:   "5",
					ExpiresAt: time.Now().Add(time.Hour).Unix(),
				})
				db.Create(&models.Blacklist{
					MediaID:   "6",
					ExpiresAt: time.Now().Add(-time.Hour).Unix(),
				})
			},
			checkResults: func(t *testing.T, res []Candidate) {
				foundKeys := make(map[string]bool)
				for _, c := range res {
					foundKeys[fmt.Sprintf("%v", c.Item["rating_key"])] = true
				}

				if !foundKeys["1"] { t.Error("High Rated item missing") }
				if !foundKeys["2"] { t.Error("Trending item missing") }
				if !foundKeys["3"] { t.Error("Genre Match item missing") }
				if !foundKeys["4"] { t.Error("Fresh item missing") }
				if foundKeys["5"] { t.Error("Blacklisted item present") }
				if !foundKeys["6"] { t.Error("Expired blacklist item missing") }
				if !foundKeys["99"] { t.Error("Surprise item missing") }
			},
		},
		{
			name: "Deduplication and Limits",
			setupMock: func(m *mockTautulliClient) {
				m.GetRecentlyAddedFn = func(count int) ([]map[string]interface{}, error) {
					return []map[string]interface{}{
						{"rating_key": "1", "rating": "9.0", "genres": "Action", "title": "Multiple Tags"},
						{"rating_key": "2", "rating": "8.5", "genres": "Drama"},
						{"rating_key": "3", "rating": "8.5", "genres": "Drama"},
						{"rating_key": "4", "rating": "8.5", "genres": "Drama"},
						{"rating_key": "5", "rating": "8.5", "genres": "Drama"},
					}, nil
				}
				m.GetTopGenresFn = func(count int) ([]string, error) { return nil, nil }
				m.GetWatchHistoryBatchFn = func(ratingKeys []string) (map[string]clients.WatchInfo, error) {
					return map[string]clients.WatchInfo{
						"1": {WatchCount: 10},
					}, nil
				}
				m.GetTopWatchedFn = func(count int) ([]map[string]interface{}, error) { return nil, nil }
			},
			checkResults: func(t *testing.T, res []Candidate) {
				counts := make(map[string]int)
				for _, c := range res {
					counts[fmt.Sprintf("%v", c.Item["rating_key"])]++
				}
				for key, count := range counts {
					if count > 1 {
						t.Errorf("Item %s appeared %d times", key, count)
					}
				}
			},
		},
		{
			name: "Invalid Rating Parsing",
			setupMock: func(m *mockTautulliClient) {
				m.GetRecentlyAddedFn = func(count int) ([]map[string]interface{}, error) {
					return []map[string]interface{}{
						{"rating_key": "1", "rating": "invalid", "genres": "Action"},
					}, nil
				}
				m.GetTopGenresFn = func(count int) ([]string, error) { return nil, nil }
				m.GetWatchHistoryBatchFn = func(ratingKeys []string) (map[string]clients.WatchInfo, error) { return nil, nil }
				m.GetTopWatchedFn = func(count int) ([]map[string]interface{}, error) { return nil, nil }
			},
			checkResults: func(t *testing.T, res []Candidate) {
				if len(res) != 1 {
					t.Errorf("expected 1 candidate, got %d", len(res))
				}
				if res[0].Tags[0] != "Freshly Added" {
					t.Errorf("expected Freshly Added tag for invalid rating, got %v", res[0].Tags)
				}
			},
		},
		{
			name: "Case Insensitive Genre Match",
			setupMock: func(m *mockTautulliClient) {
				m.GetRecentlyAddedFn = func(count int) ([]map[string]interface{}, error) {
					return []map[string]interface{}{
						{"rating_key": "1", "rating": "5.0", "genres": "ACTION, DRAMA"},
					}, nil
				}
				m.GetTopGenresFn = func(count int) ([]string, error) {
					return []string{"action"}, nil
				}
				m.GetWatchHistoryBatchFn = func(ratingKeys []string) (map[string]clients.WatchInfo, error) { return nil, nil }
				m.GetTopWatchedFn = func(count int) ([]map[string]interface{}, error) { return nil, nil }
			},
			checkResults: func(t *testing.T, res []Candidate) {
				if len(res) != 1 {
					t.Errorf("expected 1 candidate, got %d", len(res))
				}
				if res[0].Tags[0] != "Based on your library tastes" {
					t.Errorf("expected Genre Match tag, got %v", res[0].Tags)
				}
			},
		},
		{
			name: "Empty Tautulli Results",
			setupMock: func(m *mockTautulliClient) {
				m.GetRecentlyAddedFn = func(count int) ([]map[string]interface{}, error) { return []map[string]interface{}{}, nil }
				m.GetTopGenresFn = func(count int) ([]string, error) { return nil, nil }
				m.GetWatchHistoryBatchFn = func(ratingKeys []string) (map[string]clients.WatchInfo, error) { return nil, nil }
				m.GetTopWatchedFn = func(count int) ([]map[string]interface{}, error) { return nil, nil }
			},
			checkResults: func(t *testing.T, res []Candidate) {
				if len(res) != 0 {
					t.Errorf("expected 0 candidates, got %d", len(res))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &mockTautulliClient{}
			if tt.setupMock != nil {
				tt.setupMock(m)
			}
			
			if tt.setupDB != nil {
				tt.setupDB(database.DB)
			} else {
				database.DB.Exec("DELETE FROM blacklists")
			}

			s := &NewsletterService{
				Tautulli: m,
			}

			res, err := s.MixRecommendations()
			if (err != nil) != tt.wantErr {
				t.Errorf("MixRecommendations() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.checkResults != nil {
				tt.checkResults(t, res)
			}
		})
	}
}
