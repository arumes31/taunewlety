package newsletter

import (
	"fmt"
	"strings"
	"taunewlety/internal/domain/models"
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

		tags := []string{}
		
		ratingStr := fmt.Sprintf("%v", item["rating"])
		var rating float64
		_, _ = fmt.Sscanf(ratingStr, "%f", &rating)
		if rating >= 8.0 {
			tags = append(tags, "Critically Acclaimed")
			highRated = append(highRated, Candidate{item, tags})
		}

		watchCount, _ := s.Tautulli.GetWatchHistory(ratingKey)
		if watchCount > 2 {
			tags = append(tags, "Trending on Server")
			trending = append(trending, Candidate{item, tags})
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
			tags = append(tags, "Based on your library tastes")
			genreMatch = append(genreMatch, Candidate{item, tags})
		}

		if len(tags) == 0 {
			tags = append(tags, "Freshly Added")
			fresh = append(fresh, Candidate{item, tags})
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
