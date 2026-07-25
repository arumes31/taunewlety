package http

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"taunewlety/internal/platform/database"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// TestLogsRequireAuth verifies that the in-memory log buffer is not readable
// without a dashboard session.
func TestLogsRequireAuth(t *testing.T) {
	os.Setenv("DB_PATH", ":memory:")
	defer os.Unsetenv("DB_PATH")
	t.Setenv("SESSION_SECRET", "logs-test-secret")

	db, err := database.InitDB()
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}

	gin.SetMode(gin.TestMode)
	r, err := SetupRouter(db, zap.NewNop())
	if err != nil {
		t.Fatalf("SetupRouter failed: %v", err)
	}

	GlobalLogBuffer.Add("info", "a secret operational detail")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/logs", nil)
	r.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Errorf("expected /api/logs to reject an unauthenticated request, got 200: %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "a secret operational detail") {
		t.Error("log contents leaked to an unauthenticated caller")
	}
}

// TestLogRingBufferNonPositiveSize guards the modulo in Add against a zero or
// negative capacity.
func TestLogRingBufferNonPositiveSize(t *testing.T) {
	for _, size := range []int{0, -1, -100} {
		b := NewLogRingBuffer(size)
		// Must not panic with a division by zero.
		b.Add("info", "first")
		b.Add("info", "second")

		entries := b.Entries()
		if len(entries) == 0 {
			t.Errorf("size %d: expected the buffer to retain at least one entry", size)
			continue
		}
		if entries[len(entries)-1].Message != "second" {
			t.Errorf("size %d: expected the newest entry to be retained, got %q",
				size, entries[len(entries)-1].Message)
		}
	}
}

// TestLoginRateLimiterSweep verifies that stale attempt records are evicted so
// the map cannot grow without bound.
func TestLoginRateLimiterSweep(t *testing.T) {
	l := &loginRateLimiter{
		attempts:    make(map[string]*loginAttempt),
		maxAttempts: 5,
		window:      50 * time.Millisecond,
	}

	l.recordFailure("10.0.0.1")
	l.recordFailure("10.0.0.2")
	if len(l.attempts) != 2 {
		t.Fatalf("expected 2 tracked IPs, got %d", len(l.attempts))
	}

	// Not yet expired.
	l.sweep()
	if len(l.attempts) != 2 {
		t.Errorf("expected entries inside the window to survive, got %d", len(l.attempts))
	}

	time.Sleep(80 * time.Millisecond)
	l.sweep()
	if len(l.attempts) != 0 {
		t.Errorf("expected expired entries to be swept, got %d", len(l.attempts))
	}

	// A fresh failure after the sweep still counts.
	l.recordFailure("10.0.0.3")
	if !l.check("10.0.0.3") {
		t.Error("a single failure should not block the IP")
	}
}

// TestCSRFNonStringSessionValue ensures a malformed session cookie cannot
// panic the middleware.
func TestCSRFNonStringSessionValue(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	store := cookie.NewStore([]byte("csrf-type-test"))
	r.Use(sessions.Sessions("mysession", store))
	r.Use(func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("csrf_token", 12345) // not a string
		_ = session.Save()
		c.Next()
	})
	r.Use(CSRFProtection())
	r.GET("/", func(c *gin.Context) {
		token, _ := c.Get("csrf_token")
		s, ok := token.(string)
		if !ok || s == "" {
			c.String(http.StatusInternalServerError, "no token")
			return
		}
		c.String(http.StatusOK, s)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(w, req) // must not panic

	if w.Code != http.StatusOK {
		t.Errorf("expected a regenerated token, got %d: %s", w.Code, w.Body.String())
	}
}

// TestCSRFRejectsUnverifiedPatch confirms PATCH is treated as a mutating
// method.
func TestCSRFRejectsUnverifiedPatch(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	store := cookie.NewStore([]byte("csrf-patch-test"))
	r.Use(sessions.Sessions("mysession", store))
	r.Use(CSRFProtection())
	r.Handle(http.MethodPatch, "/thing", func(c *gin.Context) {
		c.String(http.StatusOK, "mutated")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPatch, "/thing", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected PATCH without a CSRF token to be rejected, got %d: %s", w.Code, w.Body.String())
	}
}
