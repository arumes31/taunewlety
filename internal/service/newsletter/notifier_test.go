package newsletter

import (
	"errors"
	"net/smtp"
	"strings"
	"taunewlety/internal/domain/models"
	"testing"
)

func TestNewsletterService_SendEmail(t *testing.T) {
	// Save the original smtpSendMail and restore it after tests
	originalSmtpSendMail := smtpSendMail
	defer func() { smtpSendMail = originalSmtpSendMail }()

	tests := []struct {
		name          string
		to            string
		subject       string
		body          string
		config        *models.Config
		mockSendMail  func(addr string, a smtp.Auth, from string, to []string, msg []byte) error
		wantErr       bool
		errContains   string
	}{
		{
			name:    "Successful email sending",
			to:      "recipient@example.com",
			subject: "Test Subject",
			body:    "Hello {{.UnsubscribeURL}}",
			config: &models.Config{
				AppBaseURL: "http://localhost:8080",
				SMTPHost:   "smtp.example.com",
				SMTPPort:   587,
				SMTPUser:   "user",
				SMTPPass:   "pass",
				SMTPSender: "sender@example.com",
			},
			mockSendMail: func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
				if addr != "smtp.example.com:587" {
					t.Errorf("expected addr smtp.example.com:587, got %s", addr)
				}
				if from != "sender@example.com" {
					t.Errorf("expected from sender@example.com, got %s", from)
				}
				if len(to) != 1 || to[0] != "recipient@example.com" {
					t.Errorf("expected to [recipient@example.com], got %v", to)
				}
				msgStr := string(msg)
				if !strings.Contains(msgStr, "To: recipient@example.com") {
					t.Errorf("message should contain recipient")
				}
				if !strings.Contains(msgStr, "Subject: Test Subject") {
					t.Errorf("message should contain subject")
				}
				if !strings.Contains(msgStr, "http://localhost:8080/unsubscribe?email=recipient%40example.com") {
					t.Errorf("message should contain unsubscribe URL")
				}
				return nil
			},
			wantErr: false,
		},
		{
			name:    "Invalid recipient email",
			to:      "invalid-email",
			subject: "Test",
			body:    "Body",
			config: &models.Config{
				AppBaseURL: "http://localhost:8080",
			},
			wantErr:     true,
			errContains: "invalid recipient email",
		},
		{
			name:    "SMTP send error",
			to:      "recipient@example.com",
			subject: "Test",
			body:    "Body",
			config: &models.Config{
				AppBaseURL: "http://localhost:8080",
				SMTPHost:   "smtp.example.com",
				SMTPPort:   587,
			},
			mockSendMail: func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
				return errors.New("smtp error")
			},
			wantErr:     true,
			errContains: "smtp error",
		},
		{
			name:    "Sanitization check",
			to:      "recipient\r\n@example.com",
			subject: "Subject\r\nLine",
			body:    "Body",
			config: &models.Config{
				AppBaseURL: "http://localhost:8080",
				SMTPHost:   "smtp.example.com",
				SMTPPort:   587,
			},
			mockSendMail: func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
				if to[0] != "recipient@example.com" {
					t.Errorf("expected sanitized email, got %s", to[0])
				}
				msgStr := string(msg)
				if strings.Contains(msgStr, "\r\nSubject: Subject\r\nLine") {
					t.Errorf("message should not contain injected newlines in subject")
				}
				if !strings.Contains(msgStr, "Subject: SubjectLine") {
					t.Errorf("subject should be sanitized")
				}
				return nil
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			smtpSendMail = tt.mockSendMail
			if tt.mockSendMail == nil {
				smtpSendMail = func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
					return nil
				}
			}

			svc := &NewsletterService{
				Config: tt.config,
			}

			err := svc.SendEmail(tt.to, tt.subject, tt.body)
			if (err != nil) != tt.wantErr {
				t.Errorf("SendEmail() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("SendEmail() error = %v, errContains %v", err, tt.errContains)
			}
		})
	}
}

func TestNewsletterService_SendNotifications(t *testing.T) {
	svc := &NewsletterService{}
	err := svc.SendNotifications("subject", "body")
	if err != nil {
		t.Errorf("SendNotifications() error = %v, want nil", err)
	}
}
