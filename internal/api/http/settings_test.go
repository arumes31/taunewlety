package http

import (
	"errors"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"taunewlety/internal/domain/models"
	"taunewlety/internal/platform/database"
	"taunewlety/internal/platform/sanitize"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func TestSettingsHandlers_DashboardGet(t *testing.T) {
	os.Setenv("DB_PATH", ":memory:")
	defer os.Unsetenv("DB_PATH")

	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		setup          func()
		expectedStatus int
		expectedBody   string
	}{
		{
			name: "Success",
			setup: func() {
				database.InitDB()
				database.GetDB().Create(&models.Subscriber{Email: "test@example.com"})
				database.GetDB().Create(&models.TokenUsage{TotalTokens: 100})
			},
			expectedStatus: http.StatusOK,
			expectedBody:   "test@example.com",
		},
		{
			name: "ScanError",
			setup: func() {
				database.InitDB()
				_ = database.GetDB().Migrator().DropTable(&models.TokenUsage{})
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "ConfigNil",
			setup: func() {
				database.InitDB()
				database.GetDB().Exec("DELETE FROM configs")
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			w := httptest.NewRecorder()
			_, r := gin.CreateTestContext(w)
			r.SetHTMLTemplate(template.Must(
				template.New("").Funcs(template.FuncMap{
					"sanitizeHTML": func(input string) template.HTML {
						return template.HTML(sanitize.HTML(input))
					},
				}).ParseGlob(filepath.Join(resolveWebDir(), "template", "*")),
			))
			h := &Handler{}
			r.GET("/", h.DashboardGet)

			req, _ := http.NewRequest(http.MethodGet, "/", nil)
			r.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
			if tt.expectedBody != "" && !strings.Contains(w.Body.String(), tt.expectedBody) {
				t.Errorf("expected body to contain %q", tt.expectedBody)
			}
		})
	}
}

func TestSettingsHandlers_SettingsPost(t *testing.T) {
	os.Setenv("DB_PATH", ":memory:")
	defer os.Unsetenv("DB_PATH")

	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		setup          func()
		formData       url.Values
		expectedStatus int
	}{
		{
			name: "BindError",
			formData: url.Values{
				"smtp_port": {"not-a-number"},
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "ValidationError_InvalidURL",
			formData: url.Values{
				"tautulli_url":     {"not-a-url"},
				"tautulli_api_key": {"testkey"},
				"ollama_url":       {"http://localhost:11434"},
				"ollama_model":     {"llama3"},
				"smtp_host":        {"smtp.example.com"},
				"smtp_port":        {"587"},
				"smtp_user":        {"user@example.com"},
				"smtp_pass":        {"password"},
				"smtp_sender":      {"sender@example.com"},
				"app_base_url":     {"http://localhost:8080"},
				"language":         {"en_US"},
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "ValidationError_InvalidPort",
			formData: url.Values{
				"tautulli_url":     {"http://localhost:8181"},
				"tautulli_api_key": {"testkey"},
				"ollama_url":       {"http://localhost:11434"},
				"ollama_model":     {"llama3"},
				"smtp_host":        {"smtp.example.com"},
				"smtp_port":        {"99999"},
				"smtp_user":        {"user@example.com"},
				"smtp_pass":        {"password"},
				"smtp_sender":      {"sender@example.com"},
				"app_base_url":     {"http://localhost:8080"},
				"language":         {"en_US"},
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "SaveError",
			setup: func() {
				database.InitDB()
				_ = database.GetDB().Callback().Create().Before("gorm:create").Register("fail_save", func(d *gorm.DB) {
					_ = d.AddError(errors.New("simulated save error"))
				})
				_ = database.GetDB().Callback().Update().Before("gorm:update").Register("fail_save", func(d *gorm.DB) {
					_ = d.AddError(errors.New("simulated save error"))
				})

			},
			formData: url.Values{
				"tautulli_url":     {"http://localhost:8181"},
				"tautulli_api_key": {"testkey"},
				"ollama_url":       {"http://localhost:11434"},
				"ollama_model":     {"llama3"},
				"smtp_host":        {"smtp.example.com"},
				"smtp_port":        {"587"},
				"smtp_user":        {"user@example.com"},
				"smtp_pass":        {"password"},
				"smtp_sender":      {"sender@example.com"},
				"app_base_url":     {"http://localhost:8080"},
				"language":         {"en_US"},
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name: "SuccessNewConfig",
			setup: func() {
				database.InitDB()
				database.GetDB().Exec("DELETE FROM configs")
			},
			formData: url.Values{
				"tautulli_url":     {"http://new-config:8181"},
				"tautulli_api_key": {"testkey"},
				"ollama_url":       {"http://localhost:11434"},
				"ollama_model":     {"llama3"},
				"smtp_host":        {"smtp.example.com"},
				"smtp_port":        {"587"},
				"smtp_user":        {"user@example.com"},
				"smtp_pass":        {"password"},
				"smtp_sender":      {"sender@example.com"},
				"app_base_url":     {"http://localhost:8080"},
				"language":         {"en_US"},
			},
			expectedStatus: http.StatusFound,
		},
		{
			name: "SuccessUpdateConfig",
			setup: func() {
				database.InitDB()
			},
			formData: url.Values{
				"tautulli_url":     {"http://updated-config:8181"},
				"tautulli_api_key": {"testkey"},
				"ollama_url":       {"http://localhost:11434"},
				"ollama_model":     {"llama3"},
				"smtp_host":        {"smtp.example.com"},
				"smtp_port":        {"587"},
				"smtp_user":        {"user@example.com"},
				"smtp_pass":        {"password"},
				"smtp_sender":      {"sender@example.com"},
				"app_base_url":     {"http://localhost:8080"},
				"language":         {"en_US"},
			},
			expectedStatus: http.StatusFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup()
			}
			w := httptest.NewRecorder()
			_, r := gin.CreateTestContext(w)
			h := &Handler{}
			r.POST("/settings", h.SettingsPost)

			req, _ := http.NewRequest(http.MethodPost, "/settings", strings.NewReader(tt.formData.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			r.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedStatus == http.StatusFound && tt.name == "SuccessUpdateConfig" {
				config, _ := database.GetConfig()
				if config.TautulliURL != "http://updated-config:8181" {
					t.Errorf("expected TautulliURL to be updated, got %s", config.TautulliURL)
				}
			}
		})
	}
}
