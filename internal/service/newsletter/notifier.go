package newsletter

import (
	"fmt"
	"mime"
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
	return smtp.SendMail(addr, auth, s.Config.SMTPSender, []string{toSanitized}, msg)
}

// SendNotifications can be expanded to Discord/Telegram in the future
func (s *NewsletterService) SendNotifications(subject string, body string) error {
	// For now, we mainly use SendEmail in the orchestrator
	return nil
}
