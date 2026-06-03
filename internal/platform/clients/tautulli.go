package clients

import (
	"encoding/json"
	"fmt"
	"net/http"
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

type WatchInfo struct {
	WatchCount int
}

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

	for i := 0; i < len(ratingKeys); i++ {
		res := <-ch
		if res.err == nil {
			results[res.key] = WatchInfo{WatchCount: res.count}
		}
	}

	return results, nil
}

func (c *TautulliClient) GetRecentlyAdded(count int) ([]map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/v2?apikey=%s&cmd=get_recently_added&count=%d", c.BaseURL, c.APIKey, count)
	resp, err := c.HTTP.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("tautulli API returned status %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	resVal, ok := result["response"]
	if !ok || resVal == nil {
		return nil, fmt.Errorf("missing response field in Tautulli API payload")
	}
	response, ok := resVal.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("response field is not a map[string]interface{}")
	}

	dataVal, ok := response["data"]
	if !ok || dataVal == nil {
		return nil, fmt.Errorf("missing data field in Tautulli API response")
	}
	data, ok := dataVal.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("data field is not a map[string]interface{}")
	}

	recentlyAddedVal, ok := data["recently_added"]
	if !ok || recentlyAddedVal == nil {
		return nil, fmt.Errorf("missing recently_added field in Tautulli API response")
	}
	recentlyAdded, ok := recentlyAddedVal.([]interface{})
	if !ok {
		return nil, fmt.Errorf("recently_added field is not a slice")
	}

	items := make([]map[string]interface{}, len(recentlyAdded))
	for i, v := range recentlyAdded {
		itemMap, ok := v.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("recently_added item at index %d is not a map[string]interface{}", i)
		}
		items[i] = itemMap
	}

	return items, nil
}

func (c *TautulliClient) GetWatchHistory(ratingKey string) (int, error) {
	url := fmt.Sprintf("%s/api/v2?apikey=%s&cmd=get_history&rating_key=%s", c.BaseURL, c.APIKey, ratingKey)
	resp, err := c.HTTP.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("tautulli API returned status %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, err
	}

	resVal, ok := result["response"]
	if !ok || resVal == nil {
		return 0, fmt.Errorf("missing response field in Tautulli API payload")
	}
	response, ok := resVal.(map[string]interface{})
	if !ok {
		return 0, fmt.Errorf("response field is not a map[string]interface{}")
	}

	dataVal, ok := response["data"]
	if !ok || dataVal == nil {
		return 0, fmt.Errorf("missing data field in Tautulli API response")
	}
	data, ok := dataVal.(map[string]interface{})
	if !ok {
		return 0, fmt.Errorf("data field is not a map[string]interface{}")
	}

	recordsVal, ok := data["recordsFiltered"]
	if !ok {
		return 0, nil
	}

	records, ok := recordsVal.(float64)
	if !ok {
		return 0, fmt.Errorf("recordsFiltered is not a numeric type")
	}

	return int(records), nil
}

func (c *TautulliClient) GetTopWatched(count int) ([]map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/v2?apikey=%s&cmd=get_home_stats&count=%d", c.BaseURL, c.APIKey, count)
	resp, err := c.HTTP.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("tautulli API returned status %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	resVal, ok := result["response"]
	if !ok || resVal == nil {
		return nil, fmt.Errorf("missing response field in Tautulli API payload")
	}
	response, ok := resVal.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("response field is not a map[string]interface{}")
	}

	dataVal, ok := response["data"]
	if !ok || dataVal == nil {
		return nil, fmt.Errorf("missing data field in Tautulli API response")
	}
	data, ok := dataVal.([]interface{})
	if !ok {
		return nil, fmt.Errorf("data field is not a slice")
	}
	
	items := []map[string]interface{}{}
	for _, stat := range data {
		statMap, ok := stat.(map[string]interface{})
		if !ok {
			continue
		}
		statId := fmt.Sprintf("%v", statMap["stat_id"])
		if statId == "top_movies" || statId == "top_tv" {
			rowsVal, ok := statMap["rows"]
			if !ok || rowsVal == nil {
				continue
			}
			rows, ok := rowsVal.([]interface{})
			if !ok {
				return nil, fmt.Errorf("rows field is not a slice")
			}
			for _, row := range rows {
				rowMap, ok := row.(map[string]interface{})
				if !ok {
					return nil, fmt.Errorf("row item is not a map[string]interface{}")
				}
				items = append(items, rowMap)
			}
		}
	}

	return items, nil
}

func (c *TautulliClient) GetTopGenres(count int) ([]string, error) {
	url := fmt.Sprintf("%s/api/v2?apikey=%s&cmd=get_home_stats&count=%d", c.BaseURL, c.APIKey, count)
	resp, err := c.HTTP.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("tautulli API returned status %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	resVal, ok := result["response"]
	if !ok || resVal == nil {
		return nil, fmt.Errorf("missing response field in Tautulli API payload")
	}
	response, ok := resVal.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("response field is not a map[string]interface{}")
	}

	dataVal, ok := response["data"]
	if !ok || dataVal == nil {
		return nil, fmt.Errorf("missing data field in Tautulli API response")
	}
	data, ok := dataVal.([]interface{})
	if !ok {
		return nil, fmt.Errorf("data field is not a slice")
	}

	genres := []string{}
	for _, stat := range data {
		statMap, ok := stat.(map[string]interface{})
		if !ok {
			continue
		}
		statId := fmt.Sprintf("%v", statMap["stat_id"])
		if statId == "top_genres" {
			rowsVal, ok := statMap["rows"]
			if !ok || rowsVal == nil {
				continue
			}
			rows, ok := rowsVal.([]interface{})
			if !ok {
				return nil, fmt.Errorf("rows field is not a slice")
			}
			for _, row := range rows {
				rowMap, ok := row.(map[string]interface{})
				if !ok {
					return nil, fmt.Errorf("row item is not a map")
				}
				genreVal, exists := rowMap["genre"]
				if exists && genreVal != nil {
					genres = append(genres, fmt.Sprintf("%v", genreVal))
				}
			}
		}
	}

	return genres, nil
}
