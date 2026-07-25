package newsletter

import (
	"testing"

	"taunewlety/internal/domain/models"
	"taunewlety/internal/platform/clients"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func mixerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	if err := db.AutoMigrate(&models.Blacklist{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// TestMixRecommendations_SurpriseIsNotADuplicate verifies the "Surprise Me!"
// pick cannot repeat an item that another category already selected.
func TestMixRecommendations_SurpriseIsNotADuplicate(t *testing.T) {
	// The single recently-added item is also the only top-watched item, so a
	// naive random pick would always duplicate it.
	item := clients.RecentlyAddedItem{RatingKey: 7, Title: "Only Item", Rating: 9.5, MediaType: "movie"}

	svc := &NewsletterService{
		DB:     mixerTestDB(t),
		Config: &models.Config{RecCount: 5},
		Tautulli: &mockTautulliClient{
			GetRecentlyAddedFn: func(int) ([]clients.RecentlyAddedItem, error) {
				return []clients.RecentlyAddedItem{item}, nil
			},
			GetHomeStatsAllFn: func(int) (*clients.HomeStatsResult, error) {
				return &clients.HomeStatsResult{
					TopWatched: []clients.HomeStatsItem{
						{RatingKey: 7, Title: "Only Item", MediaType: "movie"},
					},
				}, nil
			},
			GetWatchHistoryBatchFn: func([]string) (map[string]clients.WatchInfo, error) {
				return map[string]clients.WatchInfo{}, nil
			},
		},
	}

	// Run repeatedly: the pick is randomized, so a single pass could hide the
	// duplicate.
	for i := 0; i < 25; i++ {
		selection, err := svc.MixRecommendations()
		if err != nil {
			t.Fatalf("MixRecommendations() error = %v", err)
		}

		seen := map[int]int{}
		for _, c := range selection {
			seen[c.Item.RatingKey]++
		}
		for key, count := range seen {
			if count > 1 {
				t.Fatalf("rating key %d appears %d times in the selection", key, count)
			}
		}
	}
}

// TestMixRecommendations_SurprisePrefersAnUnusedItem checks that when the
// random pick collides with an already-selected item, the next candidate is
// used instead of dropping the Surprise entry.
func TestMixRecommendations_SurprisePrefersAnUnusedItem(t *testing.T) {
	svc := &NewsletterService{
		DB:     mixerTestDB(t),
		Config: &models.Config{RecCount: 5},
		Tautulli: &mockTautulliClient{
			GetRecentlyAddedFn: func(int) ([]clients.RecentlyAddedItem, error) {
				return []clients.RecentlyAddedItem{
					{RatingKey: 1, Title: "Acclaimed", Rating: 9.5},
				}, nil
			},
			GetHomeStatsAllFn: func(int) (*clients.HomeStatsResult, error) {
				return &clients.HomeStatsResult{
					TopWatched: []clients.HomeStatsItem{
						{RatingKey: 1, Title: "Acclaimed", MediaType: "movie"},
						{RatingKey: 2, Title: "A Series", MediaType: "show"},
					},
				}, nil
			},
			GetWatchHistoryBatchFn: func([]string) (map[string]clients.WatchInfo, error) {
				return map[string]clients.WatchInfo{}, nil
			},
		},
	}

	for i := 0; i < 25; i++ {
		selection, err := svc.MixRecommendations()
		if err != nil {
			t.Fatalf("MixRecommendations() error = %v", err)
		}

		var surprise *Candidate
		for idx := range selection {
			for _, tag := range selection[idx].Tags {
				if tag == "Surprise Me!" {
					surprise = &selection[idx]
				}
			}
		}
		if surprise == nil {
			t.Fatal("expected a Surprise Me! entry when an unused candidate exists")
		}
		if surprise.Item.RatingKey != 2 {
			t.Fatalf("expected the surprise to fall through to the unused item 2, got %d",
				surprise.Item.RatingKey)
		}
		// The media type must come from the source stat, not be hardcoded.
		if surprise.Item.MediaType != "show" {
			t.Fatalf("expected media type %q from top_tv, got %q", "show", surprise.Item.MediaType)
		}
	}
}

// TestMixRecommendations_SurpriseKeepsMovieType confirms a top_movies entry
// still maps to "movie".
func TestMixRecommendations_SurpriseKeepsMovieType(t *testing.T) {
	svc := &NewsletterService{
		DB:     mixerTestDB(t),
		Config: &models.Config{RecCount: 5},
		Tautulli: &mockTautulliClient{
			GetRecentlyAddedFn: func(int) ([]clients.RecentlyAddedItem, error) {
				return nil, nil
			},
			GetHomeStatsAllFn: func(int) (*clients.HomeStatsResult, error) {
				return &clients.HomeStatsResult{
					TopWatched: []clients.HomeStatsItem{
						{RatingKey: 5, Title: "A Movie", MediaType: "movie"},
					},
				}, nil
			},
			GetWatchHistoryBatchFn: func([]string) (map[string]clients.WatchInfo, error) {
				return map[string]clients.WatchInfo{}, nil
			},
		},
	}

	selection, err := svc.MixRecommendations()
	if err != nil {
		t.Fatalf("MixRecommendations() error = %v", err)
	}
	if len(selection) != 1 {
		t.Fatalf("expected exactly the surprise entry, got %d", len(selection))
	}
	if selection[0].Item.MediaType != "movie" {
		t.Errorf("expected media type %q, got %q", "movie", selection[0].Item.MediaType)
	}
}
