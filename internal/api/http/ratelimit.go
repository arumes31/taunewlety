package http

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type loginAttempt struct {
	count    int
	inFlight int // credential checks admitted by reserve but not yet released
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
		if attempt.inFlight == 0 && time.Since(attempt.lastTime) > l.window {
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
	if attempt.inFlight == 0 && time.Since(attempt.lastTime) > l.window {
		delete(l.attempts, ip)
		return true
	}

	return attempt.count+attempt.inFlight < l.maxAttempts
}

// reserve atomically admits one credential check for ip. In-flight checks
// count against the limit, so concurrent requests cannot all pass admission
// before any of their failures is recorded. Every successful reserve must be
// followed by exactly one release.
func (l *loginRateLimiter) reserve(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	attempt, exists := l.attempts[ip]
	if !exists {
		attempt = &loginAttempt{lastTime: time.Now()}
		l.attempts[ip] = attempt
	} else if attempt.inFlight == 0 && time.Since(attempt.lastTime) > l.window {
		attempt.count = 0
	}

	if attempt.count+attempt.inFlight >= l.maxAttempts {
		return false
	}
	attempt.inFlight++
	return true
}

// release completes a reservation, counting it as a failure when failed.
func (l *loginRateLimiter) release(ip string, failed bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	attempt, exists := l.attempts[ip]
	if !exists {
		// A concurrent successful login reset the entry.
		if !failed {
			return
		}
		attempt = &loginAttempt{}
		l.attempts[ip] = attempt
	} else if attempt.inFlight > 0 {
		attempt.inFlight--
	}
	if failed {
		attempt.count++
		attempt.lastTime = time.Now()
	} else if attempt.count == 0 && attempt.inFlight == 0 {
		delete(l.attempts, ip)
	}
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
	inFlight  int // credential checks admitted by reserve but not yet released
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

// reserve atomically admits one credential check. In-flight checks count
// toward the threshold, and once it is reached only one check may be in
// flight at a time, so a burst of concurrent requests cannot all slip past
// the backoff before the first failure sets it. Every successful reserve must
// be followed by exactly one release.
func (b *accountBackoff) reserve() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	if now.Before(b.until) {
		return false
	}
	failures := b.failures
	if now.Sub(b.lastFail) > b.window {
		failures = 0
	}
	if b.inFlight > 0 && failures+b.inFlight >= b.threshold {
		return false
	}
	b.inFlight++
	return true
}

// release completes a reservation, counting it as a failure when failed.
func (b *accountBackoff) release(failed bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.inFlight > 0 {
		b.inFlight--
	}
	if failed {
		b.recordFailureLocked()
	}
}

// recordFailure counts a failed login. Past the threshold, each further
// failure doubles the wait before the next attempt, up to maxDelay.
func (b *accountBackoff) recordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.recordFailureLocked()
}

func (b *accountBackoff) recordFailureLocked() {
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
