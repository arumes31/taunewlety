package newsletter

import (
	"context"
	"errors"
	"fmt"
	"taunewlety/internal/domain/models"
	"taunewlety/internal/platform/clients"
	"time"

	"gorm.io/gorm"
)

var ErrNoRecommendations = errors.New("no recommendations available")

type TautulliClient interface {
	GetRecentlyAdded(count int) ([]clients.RecentlyAddedItem, error)
	GetTopGenres(count int) ([]string, error)
	GetWatchHistoryBatch(ratingKeys []string) (map[string]clients.WatchInfo, error)
	GetTopWatched(count int) ([]clients.HomeStatsItem, error)
	GetHomeStatsAll(count int) (*clients.HomeStatsResult, error)
}

type OllamaClient interface {
	Generate(ctx context.Context, prompt string) (string, int, int, error)
}

// NewsletterService holds injected dependencies for newsletter operations.
type NewsletterService struct {
	DB       *gorm.DB
	Tautulli TautulliClient
	Ollama   OllamaClient
	Config   *models.Config
}

// NewNewsletterService creates a service with the injected DB and config.
func NewNewsletterService(db *gorm.DB, config *models.Config) *NewsletterService {
	return &NewsletterService{
		DB:       db,
		Tautulli: clients.NewTautulliClient(config.TautulliURL, config.TautulliAPIKey),
		Ollama:   clients.NewOllamaClient(config.OllamaURL, config.OllamaModel),
		Config:   config,
	}
}

func (s *NewsletterService) GenerateNewsletter() (string, string, error) {
	return s.GenerateNewsletterWithContext(context.Background())
}

// GenerateNewsletterWithContext is the context-aware version of GenerateNewsletter.
func (s *NewsletterService) GenerateNewsletterWithContext(ctx context.Context) (string, string, error) {
	// 1. Get mixed pool of candidates
	selection, err := s.MixRecommendations()
	if err != nil {
		return "", "", err
	}

	if len(selection) == 0 {
		return "", "", ErrNoRecommendations
	}

	// 2. Generate content with AI
	content, err := s.GenerateAIContent(ctx, selection)
	if err != nil {
		return "", "", err
	}

	// 3. Post-generation: track stats and blacklist
	for _, c := range selection {
		ratingKey := fmt.Sprintf("%d", c.Item.RatingKey)
		mediaType := c.Item.MediaType
		title := c.Item.Title

		s.DB.Create(&models.Blacklist{
			MediaID:   ratingKey,
			MediaType: mediaType,
			ExpiresAt: time.Now().AddDate(0, 0, 7).Unix(),
		})

		s.DB.Create(&models.RecommendationStat{
			MediaID: ratingKey,
			Title:   title,
			SentAt:  time.Now().Unix(),
		})
	}

	return content.Subject, content.Body, nil
}
