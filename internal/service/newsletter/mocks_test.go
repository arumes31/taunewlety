package newsletter

import (
	"taunewlety/internal/platform/clients"
)

type mockTautulliClient struct {
	GetRecentlyAddedFn     func(count int) ([]map[string]interface{}, error)
	GetTopGenresFn         func(count int) ([]string, error)
	GetWatchHistoryBatchFn func(ratingKeys []string) (map[string]clients.WatchInfo, error)
	GetTopWatchedFn        func(count int) ([]map[string]interface{}, error)
}

func (m *mockTautulliClient) GetRecentlyAdded(count int) ([]map[string]interface{}, error) {
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

func (m *mockTautulliClient) GetTopWatched(count int) ([]map[string]interface{}, error) {
	if m.GetTopWatchedFn != nil {
		return m.GetTopWatchedFn(count)
	}
	return nil, nil
}

type mockOllamaClient struct {
	GenerateFn func(prompt string) (string, int, int, error)
}

func (m *mockOllamaClient) Generate(prompt string) (string, int, int, error) {
	if m.GenerateFn != nil {
		return m.GenerateFn(prompt)
	}
	return "", 0, 0, nil
}
