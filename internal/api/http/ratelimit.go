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

// accountBackoff slows down password guessing against the single admin
// account regardless of client IP, so rotating source addresses does not
// bypass the per-IP limit. Instead of a lockout it imposes a short,
// exponentially growing delay, which keeps an attacker from locking the
// admin out for long.
type accountBackoff struct {
	mu        sync.Mutex
	failures  int
	lastFail  time.Time
	until     time.Time
	threshold int
	maxDelay  time.Duration
	window    time.Duration
}

var loginBackoff = &accountBackoff{
	threshold: 5,
	maxDelay:  30 * time.Second,
	window:    15 * time.Minute,
}

// allowed reports whether a login attempt may proceed now.
func (b *accountBackoff) allowed() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return !time.Now().Before(b.until)
}

// recordFailure counts a failed login. Past the threshold, each further
// failure doubles the wait before the next attempt, up to maxDelay.
func (b *accountBackoff) recordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	if now.Sub(b.lastFail) > b.window {
		b.failures = 0
	}
	b.failures++
	b.lastFail = now

	if over := b.failures - b.threshold; over > 0 {
		delay := b.maxDelay
		if over < 16 {
			delay = min(time.Second<<(over-1), b.maxDelay)
		}
		b.until = now.Add(delay)
	}
}

func (b *accountBackoff) reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures = 0
	b.until = time.Time{}
}
