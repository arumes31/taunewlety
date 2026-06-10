package newsletter

import (
	"encoding/json"
	"fmt"
	"strings"
	"taunewlety/internal/domain/models"
	"taunewlety/internal/platform/database"
)

type GeneratedContent struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func (s *NewsletterService) GenerateAIContent(selection []Candidate) (*GeneratedContent, error) {
	var itemsList []string
	for _, c := range selection {
		title := fmt.Sprintf("%v", c.Item["title"])
		year := fmt.Sprintf("%v", c.Item["year"])
		if year == "<nil>" || year == "0" {
			year = "N/A"
		}

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

	resp, promptTokens, completionTokens, err := s.Ollama.Generate(prompt)
	if err != nil {
		return nil, err
	}

	database.GetDB().Create(&models.TokenUsage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      promptTokens + completionTokens,
		LLMModel:         s.Config.OllamaModel,
	})

	// Robust JSON extraction
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

	var result GeneratedContent
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		return nil, fmt.Errorf("failed to parse AI response as JSON: %w", err)
	}

	return &result, nil
}
