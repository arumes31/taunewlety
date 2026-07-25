package http

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"taunewlety/internal/platform/database"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// TestDashboardGet_DatabaseFailureRendersErrorPage covers the failure branches
// that previously re-rendered index.html without the fields it needs — the
// template comparison on total_pages would fail mid-render, and the form would
// have been emitted with no CSRF token.
func TestDashboardGet_DatabaseFailureRendersErrorPage(t *testing.T) {
	_ = os.Setenv("DB_PATH", ":memory:")
	defer func() { _ = os.Unsetenv("DB_PATH") }()
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name     string
		callback func(db *gorm.DB)
	}{
		{
			name: "Count fails",
			callback: func(db *gorm.DB) {
				_ = db.Callback().Query().Before("gorm:query").Register("fail_count", func(d *gorm.DB) {
					_ = d.AddError(errors.New("simulated count failure"))
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := database.InitDB()
			if err != nil {
				t.Fatalf("failed to init DB: %v", err)
			}
			tt.callback(db)

			h := NewHandler(db)
			r := gin.New()
			r.SetHTMLTemplate(loadTestTemplates(t))
			r.GET("/", h.DashboardGet)

			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/", nil)
			r.ServeHTTP(w, req)

			if w.Code != http.StatusInternalServerError {
				t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
			}

			body := w.Body.String()
			// A complete error page, not a truncated index.html that blew up
			// partway through rendering.
			if !strings.Contains(body, "</html>") {
				t.Errorf("expected a complete HTML error page, got: %s", body)
			}
			if strings.Contains(body, `name="csrf_token"`) {
				t.Error("the error page must not render forms that need a CSRF token")
			}
			if strings.Contains(strings.ToLower(body), "executing") ||
				strings.Contains(strings.ToLower(body), "incompatible types") {
				t.Errorf("template execution error leaked into the response: %s", body)
			}
		})
	}
}
