package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestLoginRateLimiter_check(t *testing.T) {
	limiter := &loginRateLimiter{
		attempts:    make(map[string]*loginAttempt),
		maxAttempts: 3,
		window:      5 * time.Minute,
	}

	t.Run("allows first attempt", func(t *testing.T) {
		if !limiter.check("1.2.3.4") {
			t.Error("expected first attempt to be allowed")
		}
	})

	t.Run("blocks after max attempts", func(t *testing.T) {
		ip := "5.6.7.8"
		for i := 0; i < 3; i++ {
			limiter.recordFailure(ip)
		}
		if limiter.check(ip) {
			t.Error("expected attempt to be blocked after max failures")
		}
	})

	t.Run("allows after window expires", func(t *testing.T) {
		ip := "9.10.11.12"
		limiter.attempts[ip] = &loginAttempt{
			count:    5,
			lastTime: time.Now().Add(-10 * time.Minute), // expired
		}
		if !limiter.check(ip) {
			t.Error("expected attempt to be allowed after window expires")
		}
	})

	t.Run("allows different IP", func(t *testing.T) {
		// Block one IP
		blockedIP := "13.14.15.16"
		for i := 0; i < 3; i++ {
			limiter.recordFailure(blockedIP)
		}
		// Different IP should still be allowed
		if !limiter.check("17.18.19.20") {
			t.Error("expected different IP to be allowed")
		}
	})
}

func TestLoginRateLimiter_recordFailure(t *testing.T) {
	limiter := &loginRateLimiter{
		attempts:    make(map[string]*loginAttempt),
		maxAttempts: 5,
		window:      15 * time.Minute,
	}

	t.Run("creates entry on first failure", func(t *testing.T) {
		limiter.recordFailure("1.1.1.1")
		attempt, exists := limiter.attempts["1.1.1.1"]
		if !exists {
			t.Fatal("expected attempt entry to be created")
		}
		if attempt.count != 1 {
			t.Errorf("expected count 1, got %d", attempt.count)
		}
	})

	t.Run("increments count on subsequent failures", func(t *testing.T) {
		limiter.recordFailure("1.1.1.1")
		limiter.recordFailure("1.1.1.1")
		if limiter.attempts["1.1.1.1"].count != 3 {
			t.Errorf("expected count 3, got %d", limiter.attempts["1.1.1.1"].count)
		}
	})
}

func TestLoginRateLimiter_reset(t *testing.T) {
	limiter := &loginRateLimiter{
		attempts:    make(map[string]*loginAttempt),
		maxAttempts: 5,
		window:      15 * time.Minute,
	}

	t.Run("removes entry on reset", func(t *testing.T) {
		ip := "2.2.2.2"
		limiter.recordFailure(ip)
		limiter.recordFailure(ip)
		limiter.reset(ip)
		if _, exists := limiter.attempts[ip]; exists {
			t.Error("expected entry to be removed after reset")
		}
	})

	t.Run("allows after reset", func(t *testing.T) {
		ip := "3.3.3.3"
		for i := 0; i < 5; i++ {
			limiter.recordFailure(ip)
		}
		limiter.reset(ip)
		if !limiter.check(ip) {
			t.Error("expected IP to be allowed after reset")
		}
	})
}

func TestLoginRateLimit_Middleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Use a fresh limiter for this test
	oldLimiter := loginLimiter
	loginLimiter = &loginRateLimiter{
		attempts:    make(map[string]*loginAttempt),
		maxAttempts: 2,
		window:      15 * time.Minute,
	}
	defer func() { loginLimiter = oldLimiter }()

	t.Run("allows requests under limit", func(t *testing.T) {
		r := gin.New()
		r.POST("/login", LoginRateLimit(), func(c *gin.Context) {
			c.String(http.StatusOK, "ok")
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/login", nil)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}
	})

	t.Run("blocks requests over limit", func(t *testing.T) {
		// Reset the limiter for this subtest
		loginLimiter = &loginRateLimiter{
			attempts:    make(map[string]*loginAttempt),
			maxAttempts: 2,
			window:      15 * time.Minute,
		}

		r := gin.New()
		r.POST("/login", LoginRateLimit(), func(c *gin.Context) {
			c.String(http.StatusOK, "ok")
		})

		// Record failures to hit the limit
		loginLimiter.recordFailure("192.168.0.1")
		loginLimiter.recordFailure("192.168.0.1")

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/login", nil)
		req.RemoteAddr = "192.168.0.1:12345"
		r.ServeHTTP(w, req)

		if w.Code != http.StatusTooManyRequests {
			t.Errorf("expected status 429, got %d", w.Code)
		}
		if w.Body.String() != "Too many login attempts. Try again later." {
			t.Errorf("expected rate limit message, got %q", w.Body.String())
		}
	})
}
