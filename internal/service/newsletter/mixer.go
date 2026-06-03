package newsletter

import (
	"fmt"
	"strings"
	"taunewlety/internal/domain/models"
	"taunewlety/internal/platform/clients"
	"taunewlety/internal/platform/database"
	"time"
)

type Candidate struct {
	Item map[string]interface{}
	Tags []string
}

func (s *NewsletterService) MixRecommendations() ([]Candidate, error) {
	candidates, err := s.Tautulli.GetRecentlyAdded(100)
	if err != nil {
		return nil, err
	}

	topGenres, _ := s.Tautulli.GetTopGenres(5)
	
	// Collect rating keys to batch query watch history
	ratingKeys := make([]string, len(candidates))
	for i, item := range candidates {
		ratingKeys[i] = fmt.Sprintf("%v", item["rating_key"])
	}
	
	watchCounts, err := s.Tautulli.GetWatchHistoryBatch(ratingKeys)
	if err != nil {
		// Log or handle batch query failure, fallback to empty map
		watchCounts = make(map[string]clients.WatchInfo)
	}
	
	var highRated []Candidate
	var trending []Candidate
	var genreMatch []Candidate
	var fresh []Candidate

	for _, item := range candidates {
		ratingKey := fmt.Sprintf("%v", item["rating_key"])
		
		var bl models.Blacklist
		result := database.DB.Where("media_id = ? AND expires_at > ?", ratingKey, time.Now().Unix()).First(&bl)
		if result.Error == nil {
			continue
		}

		var allTags []string
		
		ratingStr := fmt.Sprintf("%v", item["rating"])
		var rating float64
		_, _ = fmt.Sscanf(ratingStr, "%f", &rating)
		if rating >= 8.0 {
			allTags = append(allTags, "Critically Acclaimed")
			tagsCopy := make([]string, len(allTags))
			copy(tagsCopy, allTags)
			highRated = append(highRated, Candidate{item, tagsCopy})
		}

		watchInfo, ok := watchCounts[ratingKey]
		watchCount := 0
		if ok {
			watchCount = watchInfo.WatchCount
		}
		if watchCount > 2 {
			allTags = append(allTags, "Trending on Server")
			tagsCopy := make([]string, len(allTags))
			copy(tagsCopy, allTags)
			trending = append(trending, Candidate{item, tagsCopy})
		}

		genresStr := fmt.Sprintf("%v", item["genres"])
		isGenreMatch := false
		for _, tg := range topGenres {
			if strings.Contains(strings.ToLower(genresStr), strings.ToLower(tg)) {
				isGenreMatch = true
				break
			}
		}
		if isGenreMatch {
			allTags = append(allTags, "Based on your library tastes")
			tagsCopy := make([]string, len(allTags))
			copy(tagsCopy, allTags)
			genreMatch = append(genreMatch, Candidate{item, tagsCopy})
		}

		if len(allTags) == 0 {
			allTags = append(allTags, "Freshly Added")
			tagsCopy := make([]string, len(allTags))
			copy(tagsCopy, allTags)
			fresh = append(fresh, Candidate{item, tagsCopy})
		}
	}

	var finalSelection []Candidate
	limit := 3
	
	appendLimited := func(list []Candidate, n int) {
		count := 0
		for _, c := range list {
			alreadySelected := false
			for _, s := range finalSelection {
				if s.Item["rating_key"] == c.Item["rating_key"] {
					alreadySelected = true
					break
				}
			}
			if !alreadySelected {
				finalSelection = append(finalSelection, c)
				count++
				if count >= n {
					break
				}
			}
		}
	}

	appendLimited(highRated, limit)
	appendLimited(trending, limit)
	appendLimited(genreMatch, limit)
	appendLimited(fresh, limit)

	// Add "Surprise Me"
	top, _ := s.Tautulli.GetTopWatched(20)
	if len(top) > 0 {
		randItem := top[time.Now().Unix()%int64(len(top))]
		finalSelection = append(finalSelection, Candidate{randItem, []string{"Surprise Me!"}})
	}

	return finalSelection, nil
}
