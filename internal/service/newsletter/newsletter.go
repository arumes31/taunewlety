package newsletter

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/smtp"
	"strings"
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
	// 1. Get pool of candidates
	candidates, err := s.Tautulli.GetRecentlyAdded(100)
	if err != nil {
		return "", "", err
	}

	topGenres, _ := s.Tautulli.GetTopGenres(5)
	
	type Candidate struct {
		Item map[string]interface{}
		Tags []string
	}

	var highRated []Candidate
	var trending []Candidate
	var genreMatch []Candidate
	var fresh []Candidate

	for _, item := range candidates {
		ratingKey := fmt.Sprintf("%v", item["rating_key"])
		
		// Skip blacklisted
		var bl models.Blacklist
		result := database.DB.Where("media_id = ? AND expires_at > ?", ratingKey, time.Now().Unix()).First(&bl)
		if result.Error == nil {
			continue
		}

		tags := []string{}
		
		// Check Rating
		ratingStr := fmt.Sprintf("%v", item["rating"])
		var rating float64
		_, _ = fmt.Sscanf(ratingStr, "%f", &rating)
		if rating >= 8.0 {
			tags = append(tags, "Critically Acclaimed")
			highRated = append(highRated, Candidate{item, tags})
		}

		// Check Popularity
		watchCount, _ := s.Tautulli.GetWatchHistory(ratingKey)
		if watchCount > 2 {
			tags = append(tags, "Trending on Server")
			trending = append(trending, Candidate{item, tags})
		}

		// Check Genre
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

	// Mix them
	var finalSelection []Candidate
	// Try to get 3 of each
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

	if len(finalSelection) == 0 {
		return "", "", ErrNoRecommendations
	}

	// 2. Prepare prompt for Ollama
	var itemsList []string
	for _, c := range finalSelection {
		title := fmt.Sprintf("%v", c.Item["title"])
		year := fmt.Sprintf("%v", c.Item["year"])
		if year == "<nil>" || year == "0" { year = "N/A" }
		
		genres := "Unknown"
		if g, ok := c.Item["genres"]; ok && g != nil {
			genres = fmt.Sprintf("%v", g)
		}
		
		rating := "N/A"
		if r, ok := c.Item["rating"]; ok && r != nil {
			rating = fmt.Sprintf("%v", r)
		}

		tag := strings.Join(c.Tags, ", ")
		itemsList = append(itemsList, fmt.Sprintf("- %s (%s) | Genre: %s | Rating: %s | Recommended because: %s", title, year, genres, rating, tag))
	}

	lang := s.Config.Language
	if lang == "" {
		lang = "en_US"
	}

	prompt := fmt.Sprintf(`Act as a modern newsletter editor for a Plex media server.
Below is a list of recently added movies and series with metadata and reasons for recommendation. 
Select the best 6-10 items and write a catchy, engaging newsletter for the users.
You MUST write the newsletter in the following language/locale: %s.

Format the output as a JSON object with two fields:
1. "subject": A catchy, short subject line for the email.
2. "body": The HTML content of the newsletter. Use a modern, clean style with sections for Movies and Series.

IMPORTANT: You MUST include a small, discreet footer at the bottom of the "body" with a link to unsubscribe. 
The link should look like this: <a href="{{.UnsubscribeURL}}">Unsubscribe</a>.

For each item, use the "Recommended because" metadata to explain to the user why it's recommended.

Items:
%s
`, lang, strings.Join(itemsList, "\n"))

	// 3. Generate with Ollama
	resp, promptTokens, completionTokens, err := s.Ollama.Generate(prompt)
	if err != nil {
		return "", "", err
	}

	// Log Token Usage
	database.DB.Create(&models.TokenUsage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      promptTokens + completionTokens,
		LLMModel:         s.Config.OllamaModel,
	})

	// Simple JSON extraction
	if strings.Contains(resp, "```json") {
		parts := strings.Split(resp, "```json")
		if len(parts) > 1 {
			resp = strings.Split(parts[1], "```")[0]
		}
	} else if strings.Contains(resp, "```") {
		parts := strings.Split(resp, "```")
		if len(parts) > 1 {
			resp = parts[1]
		}
	}
	resp = strings.TrimSpace(resp)

	var result struct {
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		return "Your Daily Plex Update", resp, nil
	}

	// 4. Blacklist selected items and track stats
	for _, c := range finalSelection {
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

	return result.Subject, result.Body, nil
}

func (s *NewsletterService) SendEmail(to string, subject string, body string) error {
	unsubURL := fmt.Sprintf("%s/unsubscribe?email=%s", s.Config.AppBaseURL, to)
	body = strings.ReplaceAll(body, "{{.UnsubscribeURL}}", unsubURL)

	auth := smtp.PlainAuth("", s.Config.SMTPUser, s.Config.SMTPPass, s.Config.SMTPHost)
	msg := []byte("To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"Content-Type: text/html; charset=UTF-8\r\n" +
		"\r\n" +
		body + "\r\n")

	addr := fmt.Sprintf("%s:%d", s.Config.SMTPHost, s.Config.SMTPPort)
	return smtp.SendMail(addr, auth, s.Config.SMTPSender, []string{to}, msg)
}
