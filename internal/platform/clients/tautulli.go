package clients

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type TautulliClient struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
}

func NewTautulliClient(url, apiKey string) *TautulliClient {
	return &TautulliClient{
		BaseURL: url,
		APIKey:  apiKey,
		HTTP: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// --- Typed response structs for the Tautulli API ---

// TautulliResponse is the top-level envelope for every Tautulli API reply.
type TautulliResponse struct {
	Response struct {
		Data    json.RawMessage `json:"data"`
		Result  string          `json:"result"`
		Message string          `json:"message"`
	} `json:"response"`
}

// RecentlyAddedItem represents a single item from get_recently_added.
type RecentlyAddedItem struct {
	RatingKey        int     `json:"rating_key"`
	Title            string  `json:"title"`
	Type             string  `json:"type"`
	Thumb            string  `json:"thumb"`
	Year             int     `json:"year"`
	ParentTitle      string  `json:"parent_title"`      // for episodes
	GrandparentTitle string  `json:"grandparent_title"` // for episodes
	MediaType        string  `json:"media_type"`
	Genres           string  `json:"genres"`
	Rating           float64 `json:"rating"`
}

// recentlyAddedData is the inner data structure for get_recently_added.
type recentlyAddedData struct {
	RecentlyAdded []RecentlyAddedItem `json:"recently_added"`
}

// HomeStatsItem represents a single row from get_home_stats (top_movies, top_tv).
type HomeStatsItem struct {
	Title     string `json:"title"`
	Total     int    `json:"total"`
	RatingKey int    `json:"rating_key"`
	SectionID int    `json:"section_id"`
}

// homeStatsData is the inner data structure for get_home_stats.
type homeStatsData struct {
	StatID string          `json:"stat_id"`
	Rows   json.RawMessage `json:"rows"`
}

// homeStatsRow is a generic row used for both top watched and genre rows.
type homeStatsRow struct {
	Title     string `json:"title"`
	Total     int    `json:"total"`
	RatingKey int    `json:"rating_key"`
	SectionID int    `json:"section_id"`
	Genre     string `json:"genre"`
}

// watchHistoryData is the inner data structure for get_history.
type watchHistoryData struct {
	RecordsFiltered int `json:"recordsFiltered"`
}

// HomeStatsResult contains both top watched items and top genres from a
// single get_home_stats API call, avoiding duplicate HTTP requests.
type HomeStatsResult struct {
	TopWatched []HomeStatsItem
	TopGenres  []string
}

// WatchInfo holds the watch count for a media item.
type WatchInfo struct {
	WatchCount int
}

// --- Helper for decoding Tautulli responses ---

func (c *TautulliClient) doRequest(params url.Values) (*TautulliResponse, error) {
	params.Set("apikey", c.APIKey)
	fullURL := c.BaseURL + "/api/v2?" + params.Encode()
	resp, err := c.HTTP.Get(fullURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tautulli API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var tautulliResp TautulliResponse
	if err := json.NewDecoder(resp.Body).Decode(&tautulliResp); err != nil {
		return nil, err
	}

	if len(tautulliResp.Response.Data) == 0 || string(tautulliResp.Response.Data) == "null" {
		return nil, fmt.Errorf("missing data field in Tautulli API response")
	}

	return &tautulliResp, nil
}

// unmarshalHomeStats decodes the raw data field into individual homeStatsData
// entries, gracefully skipping any array elements that are not JSON objects
// (e.g. stray strings or numbers that the Tautulli API may include).
func unmarshalHomeStats(raw json.RawMessage) ([]homeStatsData, error) {
	// First try direct unmarshal into []homeStatsData (fast path for clean data).
	var stats []homeStatsData
	if err := json.Unmarshal(raw, &stats); err == nil {
		return stats, nil
	}

	// Slow path: decode as []json.RawMessage and try each element individually.
	// Reset stats since the fast path may have partially populated it before failing.
	stats = nil

	var rawItems []json.RawMessage
	if err := json.Unmarshal(raw, &rawItems); err != nil {
		return nil, fmt.Errorf("failed to unmarshal home_stats data: %w", err)
	}

	for _, item := range rawItems {
		var stat homeStatsData
		if err := json.Unmarshal(item, &stat); err != nil {
			continue // skip non-object entries
		}
		stats = append(stats, stat)
	}
	return stats, nil
}

// unmarshalHomeStatsRows decodes the Rows field of a homeStatsData entry.
// Null or empty rows are acceptable (returns empty slice, no error).
// Malformed rows (wrong type or bad elements) return an error.
func unmarshalHomeStatsRows(raw json.RawMessage) ([]homeStatsRow, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil // null/missing rows are acceptable
	}
	var rows []homeStatsRow
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("failed to unmarshal home_stats rows: %w", err)
	}
	return rows, nil
}

// --- Public API methods ---

func (c *TautulliClient) GetWatchHistoryBatch(ratingKeys []string) (map[string]WatchInfo, error) {
	results := make(map[string]WatchInfo)
	if len(ratingKeys) == 0 {
		return results, nil
	}

	type batchResult struct {
		key   string
		count int
		err   error
	}

	ch := make(chan batchResult, len(ratingKeys))
	for _, key := range ratingKeys {
		go func(k string) {
			count, err := c.GetWatchHistory(k)
			ch <- batchResult{key: k, count: count, err: err}
		}(key)
	}

	var errCount int
	var errMu sync.Mutex

	for i := 0; i < len(ratingKeys); i++ {
		res := <-ch
		if res.err == nil {
			results[res.key] = WatchInfo{WatchCount: res.count}
		} else {
			errMu.Lock()
			errCount++
			errMu.Unlock()
			log.Printf("Warning: failed to get watch history for rating key %v: %v", res.key, res.err)
		}
	}

	if errCount > 0 && errCount > len(ratingKeys)/2 {
		return results, fmt.Errorf("%d out of %d watch history queries failed", errCount, len(ratingKeys))
	}

	return results, nil
}

func (c *TautulliClient) GetRecentlyAdded(count int) ([]RecentlyAddedItem, error) {
	params := url.Values{}
	params.Set("cmd", "get_recently_added")
	params.Set("count", fmt.Sprintf("%d", count))

	tautulliResp, err := c.doRequest(params)
	if err != nil {
		return nil, err
	}

	var data recentlyAddedData
	if err := json.Unmarshal(tautulliResp.Response.Data, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal recently_added data: %w", err)
	}

	if data.RecentlyAdded == nil {
		return nil, fmt.Errorf("missing recently_added field in Tautulli API response")
	}

	return data.RecentlyAdded, nil
}

func (c *TautulliClient) GetWatchHistory(ratingKey string) (int, error) {
	params := url.Values{}
	params.Set("cmd", "get_history")
	params.Set("rating_key", ratingKey)

	tautulliResp, err := c.doRequest(params)
	if err != nil {
		return 0, err
	}

	// The data field must be a JSON object for watch history.
	if tautulliResp.Response.Data[0] != '{' {
		return 0, fmt.Errorf("data field is not a map in watch history response")
	}

	var data watchHistoryData
	if err := json.Unmarshal(tautulliResp.Response.Data, &data); err != nil {
		return 0, fmt.Errorf("failed to unmarshal watch history data: %w", err)
	}

	return data.RecordsFiltered, nil
}

func (c *TautulliClient) GetTopWatched(count int) ([]HomeStatsItem, error) {
	params := url.Values{}
	params.Set("cmd", "get_home_stats")
	params.Set("count", fmt.Sprintf("%d", count))

	tautulliResp, err := c.doRequest(params)
	if err != nil {
		return nil, err
	}

	stats, err := unmarshalHomeStats(tautulliResp.Response.Data)
	if err != nil {
		return nil, err
	}

	var items []HomeStatsItem
	for _, stat := range stats {
		if stat.StatID == "top_movies" || stat.StatID == "top_tv" {
			rows, err := unmarshalHomeStatsRows(stat.Rows)
			if err != nil {
				return nil, err
			}
			for _, row := range rows {
				items = append(items, HomeStatsItem{
					Title:     row.Title,
					Total:     row.Total,
					RatingKey: row.RatingKey,
					SectionID: row.SectionID,
				})
			}
		}
	}

	return items, nil
}

func (c *TautulliClient) GetTopGenres(count int) ([]string, error) {
	params := url.Values{}
	params.Set("cmd", "get_home_stats")
	params.Set("count", fmt.Sprintf("%d", count))

	tautulliResp, err := c.doRequest(params)
	if err != nil {
		return nil, err
	}

	stats, err := unmarshalHomeStats(tautulliResp.Response.Data)
	if err != nil {
		return nil, err
	}

	var genres []string
	for _, stat := range stats {
		if stat.StatID == "top_genres" {
			rows, err := unmarshalHomeStatsRows(stat.Rows)
			if err != nil {
				return nil, err
			}
			for _, row := range rows {
				if row.Genre != "" {
					genres = append(genres, row.Genre)
				}
			}
		}
	}

	return genres, nil
}

// GetHomeStatsAll makes a single API call to get_home_stats and extracts
// both top watched items and top genres from the response.
func (c *TautulliClient) GetHomeStatsAll(count int) (*HomeStatsResult, error) {
	params := url.Values{}
	params.Set("cmd", "get_home_stats")
	params.Set("count", fmt.Sprintf("%d", count))

	tautulliResp, err := c.doRequest(params)
	if err != nil {
		return nil, err
	}

	stats, err := unmarshalHomeStats(tautulliResp.Response.Data)
	if err != nil {
		return nil, err
	}

	homeStats := &HomeStatsResult{}

	for _, stat := range stats {
		// Extract top watched (top_movies, top_tv)
		if stat.StatID == "top_movies" || stat.StatID == "top_tv" {
			rows, err := unmarshalHomeStatsRows(stat.Rows)
			if err != nil {
				return nil, err
			}
			for _, row := range rows {
				homeStats.TopWatched = append(homeStats.TopWatched, HomeStatsItem{
					Title:     row.Title,
					Total:     row.Total,
					RatingKey: row.RatingKey,
					SectionID: row.SectionID,
				})
			}
		}

		// Extract top genres
		if stat.StatID == "top_genres" {
			rows, err := unmarshalHomeStatsRows(stat.Rows)
			if err != nil {
				return nil, err
			}
			for _, row := range rows {
				if row.Genre != "" {
					homeStats.TopGenres = append(homeStats.TopGenres, row.Genre)
				}
			}
		}
	}

	return homeStats, nil
}
