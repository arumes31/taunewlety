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

// GenerateNewsletterWithContext builds a newsletter for delivery and records
// the selection (7-day blacklist and recommendation stats). Only call it when
// the newsletter is actually being sent; use PreviewNewsletter otherwise.
func (s *NewsletterService) GenerateNewsletterWithContext(ctx context.Context) (string, string, error) {
	selection, content, err := s.draftNewsletter(ctx)
	if err != nil {
		return "", "", err
	}
	s.recordDelivery(selection)
	return content.Subject, content.Body, nil
}

// PreviewNewsletter builds a newsletter without recording anything about the
// selection, so viewing a preview (a GET that embedded resources can trigger
// with the admin's cookie) never changes which items are recommended next.
// The returned body has no unsubscribe footer; SendEmail adds a signed one
// per recipient.
func (s *NewsletterService) PreviewNewsletter(ctx context.Context) (string, string, error) {
	_, content, err := s.draftNewsletter(ctx)
	if err != nil {
		return "", "", err
	}
	return content.Subject, content.Body, nil
}

// draftNewsletter selects candidates and generates the AI content.
func (s *NewsletterService) draftNewsletter(ctx context.Context) ([]Candidate, *GeneratedContent, error) {
	selection, err := s.MixRecommendations()
	if err != nil {
		return nil, nil, err
	}

	if len(selection) == 0 {
		return nil, nil, ErrNoRecommendations
	}

	content, err := s.GenerateAIContent(ctx, selection)
	if err != nil {
		return nil, nil, err
	}
	return selection, content, nil
}

// recordDelivery blacklists the sent items for 7 days and tracks stats.
func (s *NewsletterService) recordDelivery(selection []Candidate) {
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
}
