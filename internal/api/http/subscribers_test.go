package http

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"taunewlety/internal/domain/models"
	"taunewlety/internal/platform/database"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func TestSubscriberAdd(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		email          string
		setupMock      func()
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "Empty Email",
			email:          "",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Email address is required",
		},
		{
			name:           "Invalid Email Format",
			email:          "invalid-email",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Invalid email address format",
		},
		{
			name:  "Database Error Checking Existence",
			email: "test@example.com",
			setupMock: func() {
				database.DB.Callback().Query().Before("gorm:query").Register("fail_existence", func(d *gorm.DB) {
					d.AddError(errors.New("simulated existence error"))
				})
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   "Database error checking subscriber existence",
		},
		{
			name:  "Subscriber Already Exists",
			email: "existing@example.com",
			setupMock: func() {
				database.DB.Create(&models.Subscriber{Email: "existing@example.com"})
			},
			expectedStatus: http.StatusConflict,
			expectedBody:   "Subscriber with this email already exists",
		},
		{
			name:  "Database Error on Create",
			email: "new@example.com",
			setupMock: func() {
				database.DB.Callback().Create().Before("gorm:create").Register("fail_create", func(d *gorm.DB) {
					d.AddError(errors.New("simulated create error"))
				})
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   "Failed to add subscriber",
		},
		{
			name:           "Success",
			email:          "success@example.com",
			expectedStatus: http.StatusFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			database.InitDB(":memory:")
			if tt.setupMock != nil {
				tt.setupMock()
			}

			w := httptest.NewRecorder()
			c, r := gin.CreateTestContext(w)
			h := &Handler{}
			r.POST("/subscribers", h.SubscriberAdd)

			form := url.Values{}
			form.Set("email", tt.email)
			c.Request, _ = http.NewRequest(http.MethodPost, "/subscribers", strings.NewReader(form.Encode()))
			c.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			r.ServeHTTP(w, c.Request)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
			if tt.expectedStatus == http.StatusFound {
				if loc := w.Header().Get("Location"); loc != "/" {
					t.Errorf("expected redirect to /, got %q", loc)
				}
			}
			if tt.expectedBody != "" && !strings.Contains(w.Body.String(), tt.expectedBody) {
				t.Errorf("expected body to contain %q, got %q", tt.expectedBody, w.Body.String())
			}
		})
	}
}

func TestSubscriberDelete(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		id             string
		setupMock      func()
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "Empty ID",
			id:             "",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Subscriber ID is required",
		},
		{
			name:           "Invalid ID Format",
			id:             "abc",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Invalid subscriber ID format",
		},
		{
			name: "Database Error on Delete",
			id:   "1",
			setupMock: func() {
				database.DB.Callback().Delete().Before("gorm:delete").Register("fail_delete", func(d *gorm.DB) {
					d.AddError(errors.New("simulated delete error"))
				})
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   "Failed to delete subscriber",
		},
		{
			name:           "Subscriber Not Found",
			id:             "999",
			expectedStatus: http.StatusNotFound,
			expectedBody:   "Subscriber not found",
		},
		{
			name: "Success",
			id:   "1",
			setupMock: func() {
				database.DB.Create(&models.Subscriber{Email: "to-delete@example.com"})
			},
			expectedStatus: http.StatusFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			database.InitDB(":memory:")
			if tt.setupMock != nil {
				tt.setupMock()
			}

			w := httptest.NewRecorder()
			c, r := gin.CreateTestContext(w)
			h := &Handler{}
			r.POST("/subscribers/delete", h.SubscriberDelete)

			form := url.Values{}
			form.Set("id", tt.id)
			c.Request, _ = http.NewRequest(http.MethodPost, "/subscribers/delete", strings.NewReader(form.Encode()))
			c.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			r.ServeHTTP(w, c.Request)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
			if tt.expectedStatus == http.StatusFound {
				if loc := w.Header().Get("Location"); loc != "/" {
					t.Errorf("expected redirect to /, got %q", loc)
				}
			}
			if tt.expectedBody != "" && !strings.Contains(w.Body.String(), tt.expectedBody) {
				t.Errorf("expected body to contain %q, got %q", tt.expectedBody, w.Body.String())
			}
		})
	}
}
