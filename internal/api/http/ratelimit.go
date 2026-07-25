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

func init() {
	loginLimiter.startSweeper(loginLimiter.window)
}

// sweep drops attempt records whose window has already elapsed. Without it
// the map grows without bound, since check/reset only remove the entry for
// an IP that comes back.
func (l *loginRateLimiter) sweep() {
	l.mu.Lock()
	defer l.mu.Unlock()

	for ip, attempt := range l.attempts {
		if time.Since(attempt.lastTime) > l.window {
			delete(l.attempts, ip)
		}
	}
}

// startSweeper runs sweep on a ticker for the lifetime of the process.
func (l *loginRateLimiter) startSweeper(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			l.sweep()
		}
	}()
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
