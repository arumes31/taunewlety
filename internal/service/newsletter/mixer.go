package newsletter

import (
	"fmt"
	"math/rand"
	"strings"
	"taunewlety/internal/domain/models"
	"taunewlety/internal/platform/clients"
	"time"
)

// Candidate represents a recommended item with its reason tags.
type Candidate struct {
	Item clients.RecentlyAddedItem
	Tags []string
}

// isBlacklisted reports whether a media item is currently suppressed. A
// lookup error is treated as "not blacklisted" so a database hiccup cannot
// empty the newsletter.
func (s *NewsletterService) isBlacklisted(ratingKey string) bool {
	var bl models.Blacklist
	result := s.DB.Where("media_id = ? AND expires_at > ?", ratingKey, time.Now().Unix()).First(&bl)
	return result.Error == nil
}

func (s *NewsletterService) MixRecommendations() ([]Candidate, error) {
	candidates, err := s.Tautulli.GetRecentlyAdded(100)
	if err != nil {
		return nil, err
	}

	// Single API call to get both top genres and top watched (B-11 fix)
	homeStats, _ := s.Tautulli.GetHomeStatsAll(20)
	var topGenres []string
	var topWatched []clients.HomeStatsItem
	if homeStats != nil {
		topGenres = homeStats.TopGenres
		topWatched = homeStats.TopWatched
	}

	// Collect rating keys to batch query watch history
	ratingKeys := make([]string, len(candidates))
	for i, item := range candidates {
		ratingKeys[i] = fmt.Sprintf("%d", item.RatingKey)
	}

	watchCounts, _ := s.Tautulli.GetWatchHistoryBatch(ratingKeys)

	var highRated []Candidate
	var trending []Candidate
	var genreMatch []Candidate
	var fresh []Candidate

	for _, item := range candidates {
		ratingKey := fmt.Sprintf("%d", item.RatingKey)

		if s.isBlacklisted(ratingKey) {
			continue
		}

		var allTags []string

		if item.Rating >= 8.0 {
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

		genresStr := item.Genres
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
				if s.Item.RatingKey == c.Item.RatingKey {
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

	// Add "Surprise Me" — use topWatched from the single GetHomeStatsAll call.
	// Start at a random offset and hand the whole rotated list to
	// appendLimited, so a pick that is already in the selection falls through
	// to the next candidate instead of producing a duplicate.
	if len(topWatched) > 0 {
		start := rand.Intn(len(topWatched))
		surprises := make([]Candidate, 0, len(topWatched))
		for i := 0; i < len(topWatched); i++ {
			item := topWatched[(start+i)%len(topWatched)]
			// Top-watched items skip the loop above, so they need the same
			// blacklist check — otherwise a suppressed title can reappear
			// here as the surprise.
			if s.isBlacklisted(fmt.Sprintf("%d", item.RatingKey)) {
				continue
			}
			surprises = append(surprises, Candidate{
				clients.RecentlyAddedItem{
					RatingKey: item.RatingKey,
					Title:     item.Title,
					MediaType: item.MediaType,
				},
				[]string{"Surprise Me!"},
			})
		}
		appendLimited(surprises, 1)
	}

	return finalSelection, nil
}
