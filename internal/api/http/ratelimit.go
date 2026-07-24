package http

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type loginAttempt struct {
	count    int
	lastTime time.Time
}

type loginRateLimiter struct {
	mu          sync.Mutex
	attempts    map[string]*loginAttempt
	maxAttempts int
	window      time.Duration
}

var loginLimiter = &loginRateLimiter{
	attempts:    make(map[string]*loginAttempt),
	maxAttempts: 5,
	window:      15 * time.Minute,
}

func (l *loginRateLimiter) check(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	attempt, exists := l.attempts[ip]
	if !exists {
		return true // allowed
	}

	// Reset if window has passed
	if time.Since(attempt.lastTime) > l.window {
		delete(l.attempts, ip)
		return true
	}

	return attempt.count < l.maxAttempts
}

func (l *loginRateLimiter) recordFailure(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	attempt, exists := l.attempts[ip]
	if !exists {
		attempt = &loginAttempt{}
		l.attempts[ip] = attempt
	}
	attempt.count++
	attempt.lastTime = time.Now()
}

func (l *loginRateLimiter) reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.attempts, ip)
}

// LoginRateLimit is a middleware that blocks login attempts from IPs
// that have exceeded the maximum number of failed attempts within
// the configured time window.
func LoginRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if !loginLimiter.check(ip) {
			c.String(http.StatusTooManyRequests, "Too many login attempts. Try again later.")
			c.Abort()
			return
		}
		c.Next()
	}
}
