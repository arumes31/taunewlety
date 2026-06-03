package clients

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type TautulliClient struct {
	BaseURL string
	APIKey  string
}

func NewTautulliClient(url, apiKey string) *TautulliClient {
	return &TautulliClient{BaseURL: url, APIKey: apiKey}
}

func (c *TautulliClient) GetRecentlyAdded(count int) ([]map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/v2?apikey=%s&cmd=get_recently_added&count=%d", c.BaseURL, c.APIKey, count)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	response := result["response"].(map[string]interface{})
	data := response["data"].(map[string]interface{})
	recentlyAdded := data["recently_added"].([]interface{})

	items := make([]map[string]interface{}, len(recentlyAdded))
	for i, v := range recentlyAdded {
		items[i] = v.(map[string]interface{})
	}

	return items, nil
}

func (c *TautulliClient) GetWatchHistory(ratingKey string) (int, error) {
	url := fmt.Sprintf("%s/api/v2?apikey=%s&cmd=get_history&rating_key=%s", c.BaseURL, c.APIKey, ratingKey)
	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, err
	}

	response := result["response"].(map[string]interface{})
	data := response["data"].(map[string]interface{})
	
	records, ok := data["recordsFiltered"].(float64)
	if !ok {
		return 0, nil
	}
	return int(records), nil
}

func (c *TautulliClient) GetTopWatched(count int) ([]map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/v2?apikey=%s&cmd=get_home_stats&count=%d", c.BaseURL, c.APIKey, count)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	response := result["response"].(map[string]interface{})
	data := response["data"].([]interface{})
	
	items := []map[string]interface{}{}
	for _, stat := range data {
		statMap := stat.(map[string]interface{})
		statId := fmt.Sprintf("%v", statMap["stat_id"])
		if statId == "top_movies" || statId == "top_tv" {
			rows := statMap["rows"].([]interface{})
			for _, row := range rows {
				items = append(items, row.(map[string]interface{}))
			}
		}
	}

	return items, nil
}

func (c *TautulliClient) GetTopGenres(count int) ([]string, error) {
	url := fmt.Sprintf("%s/api/v2?apikey=%s&cmd=get_home_stats&count=%d", c.BaseURL, c.APIKey, count)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	response := result["response"].(map[string]interface{})
	data := response["data"].([]interface{})

	genres := []string{}
	for _, stat := range data {
		statMap := stat.(map[string]interface{})
		statId := fmt.Sprintf("%v", statMap["stat_id"])
		if statId == "top_genres" {
			rows := statMap["rows"].([]interface{})
			for _, row := range rows {
				rowMap := row.(map[string]interface{})
				genres = append(genres, fmt.Sprintf("%v", rowMap["genre"]))
			}
		}
	}

	return genres, nil
}
