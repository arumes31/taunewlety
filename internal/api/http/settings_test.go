package http

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"taunewlety/internal/domain/models"
	"taunewlety/internal/platform/database"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func TestSettingsHandlers_DashboardGet(t *testing.T) {
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
				database.InitDB(":memory:")
				database.DB.Create(&models.Subscriber{Email: "test@example.com"})
				database.DB.Create(&models.TokenUsage{TotalTokens: 100})
			},
			expectedStatus: http.StatusOK,
			expectedBody:   "test@example.com",
		},
		{
			name: "ScanError",
			setup: func() {
				database.InitDB(":memory:")
				_ = database.DB.Migrator().DropTable(&models.TokenUsage{})
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "ConfigNil",
			setup: func() {
				database.InitDB(":memory:")
				database.DB.Exec("DELETE FROM configs")
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			w := httptest.NewRecorder()
			_, r := gin.CreateTestContext(w)
			r.LoadHTMLGlob(filepath.Join(resolveWebDir(), "template", "*"))
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
			name: "SaveError",
			setup: func() {
				database.InitDB(":memory:")
				_ = database.DB.Callback().Create().Before("gorm:create").Register("fail_save", func(d *gorm.DB) {
					_ = d.AddError(errors.New("simulated save error"))
				})
				_ = database.DB.Callback().Update().Before("gorm:update").Register("fail_save", func(d *gorm.DB) {
					_ = d.AddError(errors.New("simulated save error"))
				})

			},
			formData: url.Values{
				"tautulli_url": {"http://localhost:8181"},
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name: "SuccessNewConfig",
			setup: func() {
				database.InitDB(":memory:")
				database.DB.Exec("DELETE FROM configs")
			},
			formData: url.Values{
				"tautulli_url": {"http://new-config:8181"},
				"smtp_port":    {"587"},
			},
			expectedStatus: http.StatusFound,
		},
		{
			name: "SuccessUpdateConfig",
			setup: func() {
				database.InitDB(":memory:")
			},
			formData: url.Values{
				"tautulli_url": {"http://updated-config:8181"},
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
