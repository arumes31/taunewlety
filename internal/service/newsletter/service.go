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

type NewsletterService struct {
	Tautulli *clients.TautulliClient
	Ollama   *clients.OllamaClient
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
		
		database.DB.Create(&models.Blacklist{
			MediaID:   ratingKey,
			MediaType: mediaType,
			ExpiresAt: time.Now().AddDate(0, 0, 7).Unix(),
		})

		database.DB.Create(&models.RecommendationStat{
			MediaID: ratingKey,
			Title:   title,
			SentAt:  time.Now().Unix(),
		})
	}

	return content.Subject, content.Body, nil
}
