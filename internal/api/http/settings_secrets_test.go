package http

import (
	"html/template"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"taunewlety/internal/domain/models"
	"taunewlety/internal/platform/database"

	"github.com/gin-gonic/gin"
)

// loadTestTemplates parses the real templates so handler tests exercise the
// same rendering path as production.
func loadTestTemplates(t *testing.T) *template.Template {
	t.Helper()
	return template.Must(
		template.New("").Funcs(templateFuncMap()).ParseGlob(filepath.Join(resolveWebDir(), "template", "*")),
	)
}

// settingsForm builds a valid settings submission, letting the caller
// override individual fields.
func settingsForm(overrides map[string]string) url.Values {
	form := url.Values{}
	form.Set("tautulli_url", "http://tautulli:8181")
	form.Set("tautulli_api_key", "")
	form.Set("plex_url", "")
	form.Set("plex_token", "")
	form.Set("ollama_url", "http://ollama:11434")
	form.Set("ollama_model", "llama3.2:3b")
	form.Set("smtp_host", "smtp.example.com")
	form.Set("smtp_port", "587")
	form.Set("smtp_user", "mailer")
	form.Set("smtp_pass", "")
	form.Set("smtp_sender", "news@example.com")
	form.Set("smtp_encryption", "starttls")
	form.Set("app_base_url", "http://localhost:8080")
	form.Set("discord_webhook", "")
	form.Set("telegram_bot_token", "")
	form.Set("telegram_chat_id", "")
	form.Set("newsletter_time", "09:00")
	form.Set("rec_count", "10")
	form.Set("language", "en_US")

	for k, v := range overrides {
		form.Set(k, v)
	}
	return form
}

func postSettings(t *testing.T, h *Handler, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	r := gin.New()
	r.POST("/settings", h.SettingsPost)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/settings", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.ServeHTTP(w, req)
	return w
}

// TestSettingsPost_PreservesSecretsOnBlank covers the flow behind the blanked
// secret inputs: an empty submission must keep the stored value rather than
// wiping it, and a non-empty one must replace it.
func TestSettingsPost_PreservesSecretsOnBlank(t *testing.T) {
	os.Setenv("DB_PATH", ":memory:")
	defer os.Unsetenv("DB_PATH")
	gin.SetMode(gin.TestMode)

	db, err := database.InitDB()
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	h := NewHandler(db)

	// Seed the stored secrets.
	config, err := database.GetConfig()
	if err != nil {
		t.Fatalf("GetConfig failed: %v", err)
	}
	config.TautulliAPIKey = "stored-tautulli-key"
	config.PlexToken = "stored-plex-token"
	config.SMTPPass = "stored-smtp-pass"
	config.TelegramBotTok = "stored-telegram-token"
	if err := database.SaveConfig(config); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	t.Run("Blank secrets are kept", func(t *testing.T) {
		w := postSettings(t, h, settingsForm(nil))
		if w.Code != http.StatusFound {
			t.Fatalf("expected redirect after save, got %d: %s", w.Code, w.Body.String())
		}

		saved, err := database.GetConfig()
		if err != nil {
			t.Fatalf("GetConfig failed: %v", err)
		}
		checks := map[string]string{
			"TautulliAPIKey": saved.TautulliAPIKey,
			"PlexToken":      saved.PlexToken,
			"SMTPPass":       saved.SMTPPass,
			"TelegramBotTok": saved.TelegramBotTok,
		}
		wants := map[string]string{
			"TautulliAPIKey": "stored-tautulli-key",
			"PlexToken":      "stored-plex-token",
			"SMTPPass":       "stored-smtp-pass",
			"TelegramBotTok": "stored-telegram-token",
		}
		for field, got := range checks {
			if got != wants[field] {
				t.Errorf("%s = %q after a blank submission, want %q", field, got, wants[field])
			}
		}
		// Non-secret fields still update.
		if saved.SMTPUser != "mailer" {
			t.Errorf("SMTPUser = %q, want %q", saved.SMTPUser, "mailer")
		}
	})

	t.Run("Supplied secrets replace stored values", func(t *testing.T) {
		w := postSettings(t, h, settingsForm(map[string]string{
			"smtp_pass":        "new-smtp-pass",
			"tautulli_api_key": "new-tautulli-key",
		}))
		if w.Code != http.StatusFound {
			t.Fatalf("expected redirect after save, got %d: %s", w.Code, w.Body.String())
		}

		saved, err := database.GetConfig()
		if err != nil {
			t.Fatalf("GetConfig failed: %v", err)
		}
		if saved.SMTPPass != "new-smtp-pass" {
			t.Errorf("SMTPPass = %q, want %q", saved.SMTPPass, "new-smtp-pass")
		}
		if saved.TautulliAPIKey != "new-tautulli-key" {
			t.Errorf("TautulliAPIKey = %q, want %q", saved.TautulliAPIKey, "new-tautulli-key")
		}
		// Untouched secrets survive.
		if saved.PlexToken != "stored-plex-token" {
			t.Errorf("PlexToken = %q, want it unchanged", saved.PlexToken)
		}
	})
}

// TestDashboardGet_SecretsNotRendered verifies the dashboard never echoes a
// stored secret back into the HTML.
func TestDashboardGet_SecretsNotRendered(t *testing.T) {
	os.Setenv("DB_PATH", ":memory:")
	defer os.Unsetenv("DB_PATH")
	t.Setenv("SESSION_SECRET", "dashboard-secret-test")
	gin.SetMode(gin.TestMode)

	db, err := database.InitDB()
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}

	config, err := database.GetConfig()
	if err != nil {
		t.Fatalf("GetConfig failed: %v", err)
	}
	config.TautulliAPIKey = "SECRET-TAUTULLI-KEY"
	config.PlexToken = "SECRET-PLEX-TOKEN"
	config.SMTPPass = "SECRET-SMTP-PASS"
	config.TelegramBotTok = "SECRET-TELEGRAM-TOKEN"
	if err := database.SaveConfig(config); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	h := NewHandler(db)
	r := gin.New()
	r.SetHTMLTemplate(loadTestTemplates(t))
	r.GET("/", h.DashboardGet)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, secret := range []string{
		"SECRET-TAUTULLI-KEY", "SECRET-PLEX-TOKEN", "SECRET-SMTP-PASS", "SECRET-TELEGRAM-TOKEN",
	} {
		if strings.Contains(body, secret) {
			t.Errorf("dashboard rendered the stored secret %q", secret)
		}
	}
}

// TestDashboardGet_PaginationClamped checks that hostile pagination values are
// clamped rather than overflowing the offset or triggering an unbounded scan.
func TestDashboardGet_PaginationClamped(t *testing.T) {
	os.Setenv("DB_PATH", ":memory:")
	defer os.Unsetenv("DB_PATH")
	t.Setenv("SESSION_SECRET", "pagination-test")
	gin.SetMode(gin.TestMode)

	db, err := database.InitDB()
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	db.Create(&models.Subscriber{Email: "a@example.com", Active: true})

	h := NewHandler(db)
	r := gin.New()
	r.SetHTMLTemplate(loadTestTemplates(t))
	r.GET("/", h.DashboardGet)

	queries := []string{
		"/?page=2147483647&per_page=2147483647",
		"/?page=9223372036854775807&per_page=9223372036854775807",
		"/?page=-1&per_page=-1",
		"/?page=abc&per_page=abc",
		"/?page=0&per_page=0",
		"/?per_page=100000",
	}

	for _, q := range queries {
		t.Run(q, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, q, nil)
			r.ServeHTTP(w, req) // must not panic or overflow

			if w.Code != http.StatusOK {
				t.Errorf("expected 200 for %s, got %d: %s", q, w.Code, w.Body.String())
			}
		})
	}
}

// TestSubscribersImport_MalformedRows verifies a row with the wrong column
// count is skipped instead of aborting the whole import.
func TestSubscribersImport_MalformedRows(t *testing.T) {
	os.Setenv("DB_PATH", ":memory:")
	defer os.Unsetenv("DB_PATH")
	gin.SetMode(gin.TestMode)

	db, err := database.InitDB()
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	db.Exec("DELETE FROM subscribers")

	csvBody := strings.Join([]string{
		"email,name,active",
		"good1@example.com,,true",
		"broken@example.com,extra,columns,here,too",
		"not-an-email,,true",
		"good2@example.com,,true",
	}, "\n")

	var buf strings.Builder
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", "subs.csv")
	if err != nil {
		t.Fatalf("CreateFormFile failed: %v", err)
	}
	if _, err := part.Write([]byte(csvBody)); err != nil {
		t.Fatalf("writing CSV body failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("closing multipart writer failed: %v", err)
	}

	h := NewHandler(db)
	r := gin.New()
	r.POST("/subscribers/import", h.SubscribersImport)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/subscribers/import", strings.NewReader(buf.String()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	r.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected a redirect after import, got %d: %s", w.Code, w.Body.String())
	}
	location := w.Header().Get("Location")
	if !strings.Contains(location, "imported=2") {
		t.Errorf("expected both valid rows to import, redirect was %q", location)
	}
	if !strings.Contains(location, "skipped=2") {
		t.Errorf("expected the malformed and invalid rows to be skipped, redirect was %q", location)
	}

	var count int64
	db.Model(&models.Subscriber{}).Count(&count)
	if count != 2 {
		t.Errorf("expected 2 imported subscribers, got %d", count)
	}
}
