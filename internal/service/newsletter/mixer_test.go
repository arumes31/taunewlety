package newsletter

import (
	"errors"
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
	_ = db.AutoMigrate(&models.Blacklist{})
	database.SetDB(db)

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
				m.GetRecentlyAddedFn = func(count int) ([]clients.RecentlyAddedItem, error) {
					return nil, errors.New("tautulli error")
				}
			},
			wantErr: true,
		},
		{
			name: "Full Mix Success",
			setupMock: func(m *mockTautulliClient) {
				m.GetRecentlyAddedFn = func(count int) ([]clients.RecentlyAddedItem, error) {
					return []clients.RecentlyAddedItem{
						{RatingKey: 1, Rating: 8.5, Genres: "Action", Title: "High Rated"},
						{RatingKey: 2, Rating: 5.0, Genres: "Drama", Title: "Trending"},
						{RatingKey: 3, Rating: 6.0, Genres: "Comedy", Title: "Genre Match"},
						{RatingKey: 4, Rating: 4.0, Genres: "Horror", Title: "Fresh"},
						{RatingKey: 5, Rating: 7.0, Genres: "Sci-Fi", Title: "Blacklisted"},
						{RatingKey: 6, Rating: 7.0, Genres: "Sci-Fi", Title: "Expired Blacklist"},
					}, nil
				}
				m.GetHomeStatsAllFn = func(count int) (*clients.HomeStatsResult, error) {
					return &clients.HomeStatsResult{
						TopGenres: []string{"Comedy"},
						TopWatched: []clients.HomeStatsItem{
							{RatingKey: 99, Title: "Surprise Me"},
						},
					}, nil
				}
				m.GetWatchHistoryBatchFn = func(ratingKeys []string) (map[string]clients.WatchInfo, error) {
					return map[string]clients.WatchInfo{
						"2": {WatchCount: 5},
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
				foundKeys := make(map[int]bool)
				for _, c := range res {
					foundKeys[c.Item.RatingKey] = true
				}

				if !foundKeys[1] {
					t.Error("High Rated item missing")
				}
				if !foundKeys[2] {
					t.Error("Trending item missing")
				}
				if !foundKeys[3] {
					t.Error("Genre Match item missing")
				}
				if !foundKeys[4] {
					t.Error("Fresh item missing")
				}
				if foundKeys[5] {
					t.Error("Blacklisted item present")
				}
				if !foundKeys[6] {
					t.Error("Expired blacklist item missing")
				}
				if !foundKeys[99] {
					t.Error("Surprise item missing")
				}
			},
		},
		{
			name: "Deduplication and Limits",
			setupMock: func(m *mockTautulliClient) {
				m.GetRecentlyAddedFn = func(count int) ([]clients.RecentlyAddedItem, error) {
					return []clients.RecentlyAddedItem{
						{RatingKey: 1, Rating: 9.0, Genres: "Action", Title: "Multiple Tags"},
						{RatingKey: 2, Rating: 8.5, Genres: "Drama"},
						{RatingKey: 3, Rating: 8.5, Genres: "Drama"},
						{RatingKey: 4, Rating: 8.5, Genres: "Drama"},
						{RatingKey: 5, Rating: 8.5, Genres: "Drama"},
					}, nil
				}
				m.GetHomeStatsAllFn = func(count int) (*clients.HomeStatsResult, error) {
					return &clients.HomeStatsResult{}, nil
				}
				m.GetWatchHistoryBatchFn = func(ratingKeys []string) (map[string]clients.WatchInfo, error) {
					return map[string]clients.WatchInfo{
						"1": {WatchCount: 10},
					}, nil
				}
			},
			checkResults: func(t *testing.T, res []Candidate) {
				counts := make(map[int]int)
				for _, c := range res {
					counts[c.Item.RatingKey]++
				}
				for key, count := range counts {
					if count > 1 {
						t.Errorf("Item %d appeared %d times", key, count)
					}
				}
			},
		},
		{
			name: "Invalid Rating Parsing",
			setupMock: func(m *mockTautulliClient) {
				m.GetRecentlyAddedFn = func(count int) ([]clients.RecentlyAddedItem, error) {
					return []clients.RecentlyAddedItem{
						{RatingKey: 1, Rating: 0, Genres: "Action"},
					}, nil
				}
				m.GetHomeStatsAllFn = func(count int) (*clients.HomeStatsResult, error) {
					return &clients.HomeStatsResult{}, nil
				}
				m.GetWatchHistoryBatchFn = func(ratingKeys []string) (map[string]clients.WatchInfo, error) { return nil, nil }
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
				m.GetRecentlyAddedFn = func(count int) ([]clients.RecentlyAddedItem, error) {
					return []clients.RecentlyAddedItem{
						{RatingKey: 1, Rating: 5.0, Genres: "ACTION, DRAMA"},
					}, nil
				}
				m.GetHomeStatsAllFn = func(count int) (*clients.HomeStatsResult, error) {
					return &clients.HomeStatsResult{
						TopGenres: []string{"action"},
					}, nil
				}
				m.GetWatchHistoryBatchFn = func(ratingKeys []string) (map[string]clients.WatchInfo, error) { return nil, nil }
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
				m.GetRecentlyAddedFn = func(count int) ([]clients.RecentlyAddedItem, error) {
					return []clients.RecentlyAddedItem{}, nil
				}
				m.GetHomeStatsAllFn = func(count int) (*clients.HomeStatsResult, error) {
					return &clients.HomeStatsResult{}, nil
				}
				m.GetWatchHistoryBatchFn = func(ratingKeys []string) (map[string]clients.WatchInfo, error) { return nil, nil }
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
				tt.setupDB(database.GetDB())
			} else {
				database.GetDB().Exec("DELETE FROM blacklists")
			}

			s := &NewsletterService{
				DB:       db,
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
