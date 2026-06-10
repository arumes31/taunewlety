package newsletter

import (
	"errors"
	"fmt"
	"taunewlety/internal/domain/models"
	"taunewlety/internal/platform/clients"
	"taunewlety/internal/platform/database"
	"time"
)

var ErrNoRecommendations = errors.New("no recommendations available")

type TautulliClient interface {
	GetRecentlyAdded(count int) ([]map[string]interface{}, error)
	GetTopGenres(count int) ([]string, error)
	GetWatchHistoryBatch(ratingKeys []string) (map[string]clients.WatchInfo, error)
	GetTopWatched(count int) ([]map[string]interface{}, error)
}

type OllamaClient interface {
	Generate(prompt string) (string, int, int, error)
}

type NewsletterService struct {
	Tautulli TautulliClient
	Ollama   OllamaClient
	Config   *models.Config
}

func NewNewsletterService(config *models.Config) *NewsletterService {
	return &NewsletterService{
		Tautulli: clients.NewTautulliClient(config.TautulliURL, config.TautulliAPIKey),
		Ollama:   clients.NewOllamaClient(config.OllamaURL, config.OllamaModel),
		Config:   config,
	}
}

func (s *NewsletterService) GenerateNewsletter() (string, string, error) {
	// 1. Get mixed pool of candidates
	selection, err := s.MixRecommendations()
	if err != nil {
		return "", "", err
	}

	if len(selection) == 0 {
		return "", "", ErrNoRecommendations
	}

	// 2. Generate content with AI
	content, err := s.GenerateAIContent(selection)
	if err != nil {
		return "", "", err
	}

	// 3. Post-generation: track stats and blacklist
	for _, c := range selection {
		ratingKey := fmt.Sprintf("%v", c.Item["rating_key"])
		mediaType := fmt.Sprintf("%v", c.Item["media_type"])
		title := fmt.Sprintf("%v", c.Item["title"])
		
		database.GetDB().Create(&models.Blacklist{
			MediaID: ratingKey,
			MediaType: mediaType,
			ExpiresAt: time.Now().AddDate(0, 0, 7).Unix(),
		})

		database.GetDB().Create(&models.RecommendationStat{
			MediaID: ratingKey,
			Title:   title,
			SentAt:  time.Now().Unix(),
		})
	}

	return content.Subject, content.Body, nil
}
