package newsletter

import (
	"testing"
	"time"

	"taunewlety/internal/domain/models"
	"taunewlety/internal/platform/clients"
)

// TestMixRecommendations_SurpriseSkipsBlacklisted verifies that a suppressed
// title cannot come back through the "Surprise Me!" pick. Top-watched items
// never pass through the main candidate loop, so they need their own
// blacklist check.
func TestMixRecommendations_SurpriseSkipsBlacklisted(t *testing.T) {
	db := mixerTestDB(t)

	// Item 1 is blacklisted for another hour; item 2 is eligible.
	if err := db.Create(&models.Blacklist{
		MediaID:   "1",
		MediaType: "movie",
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}).Error; err != nil {
		t.Fatalf("failed to seed blacklist: %v", err)
	}

	svc := &NewsletterService{
		DB:     db,
		Config: &models.Config{RecCount: 5},
		Tautulli: &mockTautulliClient{
			GetRecentlyAddedFn: func(int) ([]clients.RecentlyAddedItem, error) {
				return nil, nil
			},
			GetHomeStatsAllFn: func(int) (*clients.HomeStatsResult, error) {
				return &clients.HomeStatsResult{
					TopWatched: []clients.HomeStatsItem{
						{RatingKey: 1, Title: "Blacklisted", MediaType: "movie"},
						{RatingKey: 2, Title: "Eligible", MediaType: "show"},
					},
				}, nil
			},
			GetWatchHistoryBatchFn: func([]string) (map[string]clients.WatchInfo, error) {
				return map[string]clients.WatchInfo{}, nil
			},
		},
	}

	// The pick starts at a random offset, so run it enough times to land on
	// the blacklisted entry first.
	for i := 0; i < 30; i++ {
		selection, err := svc.MixRecommendations()
		if err != nil {
			t.Fatalf("MixRecommendations() error = %v", err)
		}
		if len(selection) != 1 {
			t.Fatalf("expected exactly the surprise entry, got %d", len(selection))
		}
		if selection[0].Item.RatingKey == 1 {
			t.Fatal("blacklisted item was offered as the Surprise Me! pick")
		}
		if selection[0].Item.RatingKey != 2 {
			t.Fatalf("expected the eligible item 2, got %d", selection[0].Item.RatingKey)
		}
	}
}

// TestMixRecommendations_SurpriseAllBlacklisted verifies the surprise is
// simply omitted when every top-watched item is suppressed, rather than
// falling back to a blacklisted title.
func TestMixRecommendations_SurpriseAllBlacklisted(t *testing.T) {
	db := mixerTestDB(t)

	expires := time.Now().Add(time.Hour).Unix()
	for _, key := range []string{"1", "2"} {
		if err := db.Create(&models.Blacklist{
			MediaID: key, MediaType: "movie", ExpiresAt: expires,
		}).Error; err != nil {
			t.Fatalf("failed to seed blacklist: %v", err)
		}
	}

	svc := &NewsletterService{
		DB:     db,
		Config: &models.Config{RecCount: 5},
		Tautulli: &mockTautulliClient{
			GetRecentlyAddedFn: func(int) ([]clients.RecentlyAddedItem, error) {
				return nil, nil
			},
			GetHomeStatsAllFn: func(int) (*clients.HomeStatsResult, error) {
				return &clients.HomeStatsResult{
					TopWatched: []clients.HomeStatsItem{
						{RatingKey: 1, Title: "Blocked A", MediaType: "movie"},
						{RatingKey: 2, Title: "Blocked B", MediaType: "show"},
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
	if len(selection) != 0 {
		t.Errorf("expected no recommendations when everything is blacklisted, got %d: %+v",
			len(selection), selection)
	}
}

// TestMixRecommendations_SurpriseIgnoresExpiredBlacklist confirms an entry
// whose suppression window has passed becomes eligible again.
func TestMixRecommendations_SurpriseIgnoresExpiredBlacklist(t *testing.T) {
	db := mixerTestDB(t)

	if err := db.Create(&models.Blacklist{
		MediaID:   "1",
		MediaType: "movie",
		ExpiresAt: time.Now().Add(-time.Hour).Unix(), // already expired
	}).Error; err != nil {
		t.Fatalf("failed to seed blacklist: %v", err)
	}

	svc := &NewsletterService{
		DB:     db,
		Config: &models.Config{RecCount: 5},
		Tautulli: &mockTautulliClient{
			GetRecentlyAddedFn: func(int) ([]clients.RecentlyAddedItem, error) {
				return nil, nil
			},
			GetHomeStatsAllFn: func(int) (*clients.HomeStatsResult, error) {
				return &clients.HomeStatsResult{
					TopWatched: []clients.HomeStatsItem{
						{RatingKey: 1, Title: "Previously Blocked", MediaType: "movie"},
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
	if len(selection) != 1 || selection[0].Item.RatingKey != 1 {
		t.Errorf("expected the expired blacklist entry to be eligible again, got %+v", selection)
	}
}
