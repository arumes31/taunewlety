package newsletter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"mime"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/url"
	"strings"
)

func (s *NewsletterService) SendEmail(to string, subject string, body string) error {
	// Sanitize to prevent header injection
	toSanitized := strings.NewReplacer("\r", "", "\n", "").Replace(to)
	subjectSanitized := strings.NewReplacer("\r", "", "\n", "").Replace(subject)

	// Validate recipient address
	_, err := mail.ParseAddress(toSanitized)
	if err != nil {
		return fmt.Errorf("invalid recipient email: %w", err)
	}

	unsubURL := fmt.Sprintf("%s/unsubscribe?email=%s", s.Config.AppBaseURL, url.QueryEscape(toSanitized))
	body = strings.ReplaceAll(body, "{{.UnsubscribeURL}}", unsubURL)

	auth := smtp.PlainAuth("", s.Config.SMTPUser, s.Config.SMTPPass, s.Config.SMTPHost)
	encodedSubject := mime.QEncoding.Encode("utf-8", subjectSanitized)

	msg := []byte("To: " + toSanitized + "\r\n" +
		"Subject: " + encodedSubject + "\r\n" +
		"Content-Type: text/html; charset=UTF-8\r\n" +
		"\r\n" +
		body + "\r\n")

	addr := fmt.Sprintf("%s:%d", s.Config.SMTPHost, s.Config.SMTPPort)
	return smtpSendMail(addr, auth, s.Config.SMTPSender, []string{toSanitized}, msg)
}

var smtpSendMail = smtp.SendMail

// discordHTTPPost is a variable to allow mocking in tests.
var discordHTTPPost = func(webhookURL string, contentType string, body *bytes.Reader) (*http.Response, error) {
	return http.Post(webhookURL, contentType, body)
}

// telegramHTTPPost is a variable to allow mocking in tests.
var telegramHTTPPost = func(apiURL string, contentType string, body *bytes.Reader) (*http.Response, error) {
	return http.Post(apiURL, contentType, body)
}

// DiscordPayload represents the JSON body sent to a Discord webhook.
type DiscordPayload struct {
	Content string `json:"content"`
}

// TelegramPayload represents the JSON body sent to the Telegram Bot API.
type TelegramPayload struct {
	ChatID string `json:"chat_id"`
	Text   string `json:"text"`
}

// SendNotifications sends notifications via Discord webhook and Telegram bot
// when a new newsletter is published. Errors are logged but do not fail the
// overall operation — email delivery is the primary channel.
func (s *NewsletterService) SendNotifications(subject string, body string) error {
	if s.Config == nil {
		return nil
	}

	// Discord Webhook
	if s.Config.DiscordWebhook != "" {
		payload := DiscordPayload{Content: fmt.Sprintf("📰 New newsletter: %s", subject)}
		jsonData, err := json.Marshal(payload)
		if err != nil {
			log.Printf("Failed to marshal Discord payload: %v", err)
		} else {
			resp, err := discordHTTPPost(s.Config.DiscordWebhook, "application/json", bytes.NewReader(jsonData))
			if err != nil {
				log.Printf("Failed to send Discord notification: %v", err)
			} else {
				resp.Body.Close()
			}
		}
	}

	// Telegram Bot
	if s.Config.TelegramBotTok != "" && s.Config.TelegramChatID != "" {
		payload := TelegramPayload{
			ChatID: s.Config.TelegramChatID,
			Text:   fmt.Sprintf("📰 New newsletter: %s", subject),
		}
		jsonData, err := json.Marshal(payload)
		if err != nil {
			log.Printf("Failed to marshal Telegram payload: %v", err)
		} else {
			apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", s.Config.TelegramBotTok)
			resp, err := telegramHTTPPost(apiURL, "application/json", bytes.NewReader(jsonData))
			if err != nil {
				log.Printf("Failed to send Telegram notification: %v", err)
			} else {
				resp.Body.Close()
			}
		}
	}

	return nil
}
