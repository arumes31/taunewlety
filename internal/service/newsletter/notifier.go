package newsletter

import (
	"fmt"
	"net/smtp"
	"strings"
)

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

// SendNotifications can be expanded to Discord/Telegram in the future
func (s *NewsletterService) SendNotifications(subject string, body string) error {
	// For now, we mainly use SendEmail in the orchestrator
	return nil
}
