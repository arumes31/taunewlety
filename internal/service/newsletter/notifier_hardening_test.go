package newsletter

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"taunewlety/internal/domain/models"
)

const testBotToken = "123456:AAH-super-secret-bot-token"

// captureLog collects everything written to the standard logger during fn.
func captureLog(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	originalOut := log.Writer()
	originalFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(originalOut)
		log.SetFlags(originalFlags)
	}()
	fn()
	return buf.String()
}

func stubResponse(status int) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader("{}")),
	}
}

// TestSendNotifications_TelegramTokenNotLogged verifies the bot token never
// reaches the log, including via a *url.Error that wraps the request URL.
func TestSendNotifications_TelegramTokenNotLogged(t *testing.T) {
	original := telegramHTTPPost
	defer func() { telegramHTTPPost = original }()

	tests := []struct {
		name string
		err  func(apiURL string) error
	}{
		{
			name: "url.Error wrapping the request URL",
			err: func(apiURL string) error {
				return &url.Error{Op: "Post", URL: apiURL, Err: errors.New("dial tcp: timeout")}
			},
		},
		{
			name: "plain error containing the URL",
			err: func(apiURL string) error {
				return fmt.Errorf("request to %s failed", apiURL)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			telegramHTTPPost = func(apiURL string, contentType string, body *bytes.Reader) (*http.Response, error) {
				return nil, tt.err(apiURL)
			}

			svc := &NewsletterService{Config: &models.Config{
				TelegramBotTok: testBotToken,
				TelegramChatID: "42",
			}}

			output := captureLog(t, func() {
				if err := svc.SendNotifications("subject", "body"); err != nil {
					t.Errorf("SendNotifications() = %v, want nil", err)
				}
			})

			if strings.Contains(output, testBotToken) {
				t.Errorf("bot token leaked to the log: %s", output)
			}
			if !strings.Contains(output, "Failed to send Telegram notification") {
				t.Errorf("expected the failure to still be logged, got: %s", output)
			}
		})
	}
}

// TestSendNotifications_NonSuccessStatusLogged verifies that a webhook
// answering 4xx/5xx is reported as a failure rather than silently accepted.
func TestSendNotifications_NonSuccessStatusLogged(t *testing.T) {
	originalDiscord := discordHTTPPost
	originalTelegram := telegramHTTPPost
	defer func() {
		discordHTTPPost = originalDiscord
		telegramHTTPPost = originalTelegram
	}()

	tests := []struct {
		name   string
		status int
		want   bool
	}{
		{"200 is a success", http.StatusOK, false},
		{"204 is a success", http.StatusNoContent, false},
		{"400 is a failure", http.StatusBadRequest, true},
		{"401 is a failure", http.StatusUnauthorized, true},
		{"429 is a failure", http.StatusTooManyRequests, true},
		{"500 is a failure", http.StatusInternalServerError, true},
		{"302 is a failure", http.StatusFound, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			discordHTTPPost = func(string, string, *bytes.Reader) (*http.Response, error) {
				return stubResponse(tt.status), nil
			}
			telegramHTTPPost = func(string, string, *bytes.Reader) (*http.Response, error) {
				return stubResponse(tt.status), nil
			}

			svc := &NewsletterService{Config: &models.Config{
				DiscordWebhook: "https://discord.example.com/hook",
				TelegramBotTok: testBotToken,
				TelegramChatID: "42",
			}}

			output := captureLog(t, func() {
				if err := svc.SendNotifications("subject", "body"); err != nil {
					t.Errorf("SendNotifications() = %v, want nil", err)
				}
			})

			discordFailed := strings.Contains(output, "Failed to send Discord notification")
			telegramFailed := strings.Contains(output, "Failed to send Telegram notification")

			if tt.want && (!discordFailed || !telegramFailed) {
				t.Errorf("status %d should be reported as a failure, log was: %s", tt.status, output)
			}
			if !tt.want && (discordFailed || telegramFailed) {
				t.Errorf("status %d should be treated as success, log was: %s", tt.status, output)
			}
			if strings.Contains(output, testBotToken) {
				t.Errorf("bot token leaked to the log: %s", output)
			}
		})
	}
}

// TestNotifyHTTPClientHasTimeout guards against a regression back to
// http.DefaultClient, which never times out.
func TestNotifyHTTPClientHasTimeout(t *testing.T) {
	if notifyHTTPClient.Timeout <= 0 {
		t.Errorf("notification HTTP client must set a finite timeout, got %v", notifyHTTPClient.Timeout)
	}
	if notifyHTTPClient == http.DefaultClient {
		t.Error("notification HTTP client must not be http.DefaultClient")
	}
}

func TestRedactSecret(t *testing.T) {
	t.Run("Nil error passes through", func(t *testing.T) {
		if got := redactSecret(nil, "x"); got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})
	t.Run("Empty secret is a no-op", func(t *testing.T) {
		original := errors.New("boom")
		if got := redactSecret(original, ""); got != original {
			t.Errorf("expected the original error instance, got %v", got)
		}
	})
	t.Run("Unrelated error is unchanged", func(t *testing.T) {
		original := errors.New("boom")
		if got := redactSecret(original, "secret"); got != original {
			t.Errorf("expected the original error instance, got %v", got)
		}
	})
	t.Run("Secret is replaced", func(t *testing.T) {
		got := redactSecret(errors.New("failed for token secret123"), "secret123")
		if strings.Contains(got.Error(), "secret123") {
			t.Errorf("secret survived redaction: %v", got)
		}
	})
}
