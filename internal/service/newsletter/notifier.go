package newsletter

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"mime"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/url"
	"strings"
	"time"
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

	encryption := s.Config.SMTPEncryption
	if encryption == "" {
		encryption = "starttls"
	}

	switch encryption {
	case "tls":
		return smtpSendMailTLS(addr, auth, s.Config.SMTPSender, []string{toSanitized}, msg)
	case "none":
		log.Printf("WARNING: SMTP encryption is disabled. Emails will be sent in plain text.")
		return smtpSendMailNoAuth(addr, s.Config.SMTPSender, []string{toSanitized}, msg)
	default: // "starttls"
		return smtpSendMail(addr, auth, s.Config.SMTPSender, []string{toSanitized}, msg)
	}
}

var smtpSendMail = smtp.SendMail

// smtpSendMailTLS sends email using implicit TLS (port 465).
var smtpSendMailTLS = func(addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
	host, _, _ := net.SplitHostPort(addr)
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host})
	if err != nil {
		return fmt.Errorf("tls dial failed: %w", err)
	}
	defer func() { _ = conn.Close() }()

	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("smtp client creation failed: %w", err)
	}
	defer func() { _ = c.Close() }()

	if err = c.Auth(auth); err != nil {
		return fmt.Errorf("smtp auth failed: %w", err)
	}
	if err = c.Mail(from); err != nil {
		return fmt.Errorf("smtp mail from failed: %w", err)
	}
	for _, rcpt := range to {
		if err = c.Rcpt(rcpt); err != nil {
			return fmt.Errorf("smtp rcpt to failed: %w", err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp data failed: %w", err)
	}
	if _, err = w.Write(msg); err != nil {
		return fmt.Errorf("smtp write failed: %w", err)
	}
	if err = w.Close(); err != nil {
		return fmt.Errorf("smtp close data failed: %w", err)
	}
	return c.Quit()
}

// smtpSendMailNoAuth sends email without authentication or encryption.
// This should only be used for local testing or trusted internal networks.
var smtpSendMailNoAuth = func(addr string, from string, to []string, msg []byte) error {
	c, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("smtp dial failed: %w", err)
	}
	defer func() { _ = c.Close() }()

	if err = c.Mail(from); err != nil {
		return fmt.Errorf("smtp mail from failed: %w", err)
	}
	for _, rcpt := range to {
		if err = c.Rcpt(rcpt); err != nil {
			return fmt.Errorf("smtp rcpt to failed: %w", err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp data failed: %w", err)
	}
	if _, err = w.Write(msg); err != nil {
		return fmt.Errorf("smtp write failed: %w", err)
	}
	if err = w.Close(); err != nil {
		return fmt.Errorf("smtp close data failed: %w", err)
	}
	return c.Quit()
}

// notifyHTTPClient posts webhook notifications. http.DefaultClient has no
// timeout, so an unresponsive webhook host would otherwise block a newsletter
// run indefinitely.
var notifyHTTPClient = &http.Client{Timeout: 10 * time.Second}

// discordHTTPPost is a variable to allow mocking in tests.
// #nosec G107
var discordHTTPPost = func(webhookURL string, contentType string, body *bytes.Reader) (*http.Response, error) {
	return notifyHTTPClient.Post(webhookURL, contentType, body)
}

// telegramHTTPPost is a variable to allow mocking in tests.
// #nosec G107
var telegramHTTPPost = func(apiURL string, contentType string, body *bytes.Reader) (*http.Response, error) {
	return notifyHTTPClient.Post(apiURL, contentType, body)
}

// redactSecret removes a secret (e.g. a Telegram bot token) from an error
// message. Transport errors wrap the request URL, which for Telegram embeds
// the bot token in its path.
func redactSecret(err error, secret string) error {
	if err == nil || secret == "" {
		return err
	}
	msg := strings.ReplaceAll(err.Error(), secret, "[REDACTED]")
	if msg == err.Error() {
		return err
	}
	return errors.New(msg)
}

// isSuccessStatus reports whether an HTTP status indicates the notification
// was accepted. A 4xx/5xx reply means the message was not delivered, even
// though the request itself succeeded.
func isSuccessStatus(code int) bool {
	return code >= 200 && code < 300
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
				if !isSuccessStatus(resp.StatusCode) {
					log.Printf("Failed to send Discord notification: webhook returned status %d", resp.StatusCode)
				}
				_ = resp.Body.Close()
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
				// The error wraps apiURL, which contains the bot token.
				log.Printf("Failed to send Telegram notification: %v", redactSecret(err, s.Config.TelegramBotTok))
			} else {
				if !isSuccessStatus(resp.StatusCode) {
					log.Printf("Failed to send Telegram notification: API returned status %d", resp.StatusCode)
				}
				_ = resp.Body.Close()
			}
		}
	}

	return nil
}
