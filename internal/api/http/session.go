package http

import (
	"crypto/sha256"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-contrib/sessions"
)

// defaultSessionMaxAge bounds how long an admin session stays valid when
// SESSION_MAX_AGE is not set.
const defaultSessionMaxAge = 12 * time.Hour

// sessionMaxAge returns the admin session lifetime from SESSION_MAX_AGE
// (a Go duration such as "12h" or "30m"), falling back to the default for
// unset or invalid values.
func sessionMaxAge() time.Duration {
	raw := os.Getenv("SESSION_MAX_AGE")
	if raw == "" {
		return defaultSessionMaxAge
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < time.Minute {
		log.Printf("WARNING: invalid SESSION_MAX_AGE, using %s", defaultSessionMaxAge)
		return defaultSessionMaxAge
	}
	return d
}

// cookieSecure reports whether the session cookie gets the Secure flag. It
// is on by default; COOKIE_SECURE=false turns it off for local development
// over plain HTTP.
func cookieSecure() bool {
	return !strings.EqualFold(os.Getenv("COOKIE_SECURE"), "false")
}

// sessionEncryptionKey derives the AES-256 key that encrypts the session
// cookie, so its contents (CSRF token, session ID) cannot be read by the
// client. The label keeps it distinct from the signing key.
func sessionEncryptionKey(secret string) []byte {
	key := sha256.Sum256([]byte("taunewlety-session-encryption:" + secret))
	return key[:]
}

// trustedProxies parses TRUSTED_PROXIES, a comma-separated list of IPs or
// CIDRs whose X-Forwarded-For header is honoured when determining the client
// IP. An empty value trusts no proxy.
func trustedProxies() ([]string, error) {
	raw := os.Getenv("TRUSTED_PROXIES")
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var proxies []string
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if net.ParseIP(p) == nil {
			if _, _, err := net.ParseCIDR(p); err != nil {
				return nil, fmt.Errorf("invalid TRUSTED_PROXIES entry %q", p)
			}
		}
		proxies = append(proxies, p)
	}
	return proxies, nil
}

// sessionRegistry tracks the IDs of live admin sessions. The cookie store is
// stateless, so without it a copied cookie would stay valid after logout.
// Sessions live in memory only: restarting the server logs everyone out.
type sessionRegistry struct {
	mu       sync.Mutex
	sessions map[string]time.Time // session ID -> expiry
}

var activeSessions = newSessionRegistry()

func newSessionRegistry() *sessionRegistry {
	r := &sessionRegistry{sessions: make(map[string]time.Time)}
	go func() {
		ticker := time.NewTicker(time.Hour)
		for range ticker.C {
			r.sweep()
		}
	}()
	return r
}

func (r *sessionRegistry) add(id string, ttl time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[id] = time.Now().Add(ttl)
}

func (r *sessionRegistry) valid(id string) bool {
	if id == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	expiry, ok := r.sessions[id]
	if !ok {
		return false
	}
	if time.Now().After(expiry) {
		delete(r.sessions, id)
		return false
	}
	return true
}

func (r *sessionRegistry) revoke(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, id)
}

// sweep drops expired sessions so the map does not grow without bound.
func (r *sessionRegistry) sweep() {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	for id, expiry := range r.sessions {
		if now.After(expiry) {
			delete(r.sessions, id)
		}
	}
}

// startAdminSession marks the session as logged in and registers a fresh
// session ID for server-side revocation.
func startAdminSession(session sessions.Session, user string) error {
	id, err := generateCSRFToken()
	if err != nil {
		return err
	}
	session.Set("user", user)
	session.Set("sid", id)
	activeSessions.add(id, sessionMaxAge())
	return nil
}

// sessionIsActive reports whether the session belongs to a logged-in admin
// whose session has not been revoked or expired.
func sessionIsActive(session sessions.Session) bool {
	if session.Get("user") == nil {
		return false
	}
	id, _ := session.Get("sid").(string)
	return activeSessions.valid(id)
}

// endAdminSession revokes the session ID server-side.
func endAdminSession(session sessions.Session) {
	if id, ok := session.Get("sid").(string); ok {
		activeSessions.revoke(id)
	}
}
