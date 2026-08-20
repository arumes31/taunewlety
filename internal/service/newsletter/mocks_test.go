package newsletter

import (
	"context"

	"taunewlety/internal/platform/clients"
)

type mockTautulliClient struct {
	GetRecentlyAddedFn func(count int) ([]clients.RecentlyAddedItem, error)
	GetTopGenresFn     func(count int) ([]string, error)
	GetWatchHistoryBatchFn func(ratingKeys []string) (map[string]clients.WatchInfo, error)
	GetTopWatchedFn    func(count int) ([]clients.HomeStatsItem, error)
	GetHomeStatsAllFn  func(count int) (*clients.HomeStatsResult, error)
}

func (m *mockTautulliClient) GetRecentlyAdded(count int) ([]clients.RecentlyAddedItem, error) {
	if m.GetRecentlyAddedFn != nil {
		return m.GetRecentlyAddedFn(count)
	}
	return nil, nil
}

func (m *mockTautulliClient) GetTopGenres(count int) ([]string, error) {
	if m.GetTopGenresFn != nil {
		return m.GetTopGenresFn(count)
	}
	return nil, nil
}

func (m *mockTautulliClient) GetWatchHistoryBatch(ratingKeys []string) (map[string]clients.WatchInfo, error) {
	if m.GetWatchHistoryBatchFn != nil {
		return m.GetWatchHistoryBatchFn(ratingKeys)
	}
	return nil, nil
}

func (m *mockTautulliClient) GetTopWatched(count int) ([]clients.HomeStatsItem, error) {
	if m.GetTopWatchedFn != nil {
		return m.GetTopWatchedFn(count)
	}
	return nil, nil
}

func (m *mockTautulliClient) GetHomeStatsAll(count int) (*clients.HomeStatsResult, error) {
	if m.GetHomeStatsAllFn != nil {
		return m.GetHomeStatsAllFn(count)
	}
	return &clients.HomeStatsResult{}, nil
}

type mockOllamaClient struct {
	GenerateFn func(ctx context.Context, prompt string) (string, int, int, error)
}

func (m *mockOllamaClient) Generate(ctx context.Context, prompt string) (string, int, int, error) {
	if m.GenerateFn != nil {
		return m.GenerateFn(ctx, prompt)
	}
	return "", 0, 0, nil
}
