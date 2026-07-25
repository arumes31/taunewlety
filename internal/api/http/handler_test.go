package http

import (
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func TestNewHandler(t *testing.T) {
	h := NewHandler(nil)
	if h == nil {
		t.Fatal("NewHandler returned nil")
	}
}

func TestRegisterHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	// We need to provide a secret for sessions middleware used in AuthRequired
	t.Setenv("SESSION_SECRET", "test-secret")

	var db *gorm.DB // nil is fine for route registration tests
	RegisterHandlers(r, db)

	// Verify some routes are registered
	routes := r.Routes()
	expectedRoutes := []struct {
		method string
		path   string
	}{
		{"GET", "/login"},
		{"POST", "/login"},
		{"GET", "/unsubscribe"},
		{"POST", "/unsubscribe"},
		{"GET", "/"},
		{"POST", "/logout"},
		{"GET", "/api/logs"},
		{"POST", "/settings"},
		{"GET", "/subscribers/export"},
		{"POST", "/subscribers/import"},
	}

	for _, tt := range expectedRoutes {
		found := false
		for _, route := range routes {
			if route.Method == tt.method && route.Path == tt.path {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected route %s %s not found", tt.method, tt.path)
		}
	}
}
