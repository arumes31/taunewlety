package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSecurityHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newRouter := func() *gin.Engine {
		r := gin.New()
		r.Use(SecurityHeaders())
		r.GET("/", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
		return r
	}

	t.Run("CSP forbids inline scripts", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/", nil)
		newRouter().ServeHTTP(w, req)

		csp := w.Header().Get("Content-Security-Policy")
		if csp == "" {
			t.Fatal("expected a Content-Security-Policy header")
		}
		scriptSrc := ""
		for _, directive := range strings.Split(csp, ";") {
			if strings.HasPrefix(strings.TrimSpace(directive), "script-src") {
				scriptSrc = strings.TrimSpace(directive)
			}
		}
		if scriptSrc == "" {
			t.Fatalf("expected a script-src directive, got: %s", csp)
		}
		if strings.Contains(scriptSrc, "unsafe-inline") {
			t.Errorf("script-src must not allow unsafe-inline, got: %s", scriptSrc)
		}
		if strings.Contains(scriptSrc, "unsafe-eval") {
			t.Errorf("script-src must not allow unsafe-eval, got: %s", scriptSrc)
		}
	})

	t.Run("Baseline headers are set", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/", nil)
		newRouter().ServeHTTP(w, req)

		want := map[string]string{
			"X-Content-Type-Options": "nosniff",
			"X-Frame-Options":        "DENY",
			"X-XSS-Protection":       "1; mode=block",
			"Referrer-Policy":        "strict-origin-when-cross-origin",
		}
		for header, value := range want {
			if got := w.Header().Get(header); got != value {
				t.Errorf("%s = %q, want %q", header, got, value)
			}
		}
	})

	t.Run("HSTS set behind an HTTPS proxy", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Forwarded-Proto", "https")
		newRouter().ServeHTTP(w, req)

		hsts := w.Header().Get("Strict-Transport-Security")
		if hsts == "" {
			t.Fatal("expected Strict-Transport-Security over HTTPS")
		}
		if !strings.Contains(hsts, "max-age=") {
			t.Errorf("HSTS should carry a max-age, got: %q", hsts)
		}
	})

	t.Run("HSTS omitted over plain HTTP", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/", nil)
		newRouter().ServeHTTP(w, req)

		if hsts := w.Header().Get("Strict-Transport-Security"); hsts != "" {
			t.Errorf("HSTS is meaningless over plain HTTP, got: %q", hsts)
		}
	})
}
