package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestCookieSecure(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{"", true},
		{"true", true},
		{"false", false},
		{"FALSE", false},
	}
	for _, tt := range tests {
		t.Setenv("COOKIE_SECURE", tt.value)
		if got := cookieSecure(); got != tt.want {
			t.Errorf("COOKIE_SECURE=%q: cookieSecure() = %v, want %v", tt.value, got, tt.want)
		}
	}
}

func TestSessionMaxAge(t *testing.T) {
	tests := []struct {
		value string
		want  time.Duration
	}{
		{"", defaultSessionMaxAge},
		{"30m", 30 * time.Minute},
		{"bogus", defaultSessionMaxAge},
		{"1s", defaultSessionMaxAge},
	}
	for _, tt := range tests {
		t.Setenv("SESSION_MAX_AGE", tt.value)
		if got := sessionMaxAge(); got != tt.want {
			t.Errorf("SESSION_MAX_AGE=%q: sessionMaxAge() = %v, want %v", tt.value, got, tt.want)
		}
	}
}

func TestTrustedProxies(t *testing.T) {
	t.Setenv("TRUSTED_PROXIES", "")
	if p, err := trustedProxies(); err != nil || p != nil {
		t.Errorf("empty value: got %v, %v", p, err)
	}

	t.Setenv("TRUSTED_PROXIES", " 10.0.0.1, 172.16.0.0/12 ,")
	p, err := trustedProxies()
	if err != nil || len(p) != 2 || p[0] != "10.0.0.1" || p[1] != "172.16.0.0/12" {
		t.Errorf("valid list: got %v, %v", p, err)
	}

	t.Setenv("TRUSTED_PROXIES", "not-an-ip")
	if _, err := trustedProxies(); err == nil {
		t.Error("expected error for invalid entry")
	}
}

func TestSetupRouter_InvalidTrustedProxies(t *testing.T) {
	t.Setenv("SESSION_SECRET", "test-secret")
	t.Setenv("TRUSTED_PROXIES", "nonsense")
	if _, err := SetupRouter(nil, zap.NewNop()); err == nil {
		t.Error("expected SetupRouter to reject invalid TRUSTED_PROXIES")
	}
}

func TestSetupRouter_SessionCookieFlags(t *testing.T) {
	t.Setenv("SESSION_SECRET", "test-secret")
	t.Setenv("COOKIE_SECURE", "")
	t.Setenv("SESSION_MAX_AGE", "")

	r, err := SetupRouter(nil, zap.NewNop())
	if err != nil {
		t.Fatalf("SetupRouter failed: %v", err)
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/login", nil)
	r.ServeHTTP(w, req)

	setCookie := w.Header().Get("Set-Cookie")
	if !strings.Contains(setCookie, "Secure") {
		t.Errorf("expected Secure cookie by default, got %q", setCookie)
	}
	if !strings.Contains(setCookie, "Max-Age=43200") {
		t.Errorf("expected 12h Max-Age, got %q", setCookie)
	}
	// The cookie is encrypted, so the CSRF token in the page must not be
	// recoverable from it.
	if strings.Contains(setCookie, "csrf_token") {
		t.Errorf("session cookie contents should be encrypted, got %q", setCookie)
	}
}

func TestSessionRegistry(t *testing.T) {
	r := &sessionRegistry{sessions: make(map[string]time.Time)}

	r.add("a", time.Hour)
	if !r.valid("a") {
		t.Error("expected registered session to be valid")
	}
	r.revoke("a")
	if r.valid("a") {
		t.Error("expected revoked session to be invalid")
	}

	r.add("b", -time.Second)
	if r.valid("b") {
		t.Error("expected expired session to be invalid")
	}
	if r.valid("") {
		t.Error("expected empty ID to be invalid")
	}

	r.add("c", -time.Second)
	r.sweep()
	if _, ok := r.sessions["c"]; ok {
		t.Error("expected sweep to drop expired session")
	}
}

func TestAccountBackoff(t *testing.T) {
	b := &accountBackoff{threshold: 2, maxDelay: time.Hour, window: time.Hour}

	b.recordFailure()
	b.recordFailure()
	if !b.allowed() {
		t.Error("expected attempts to be allowed up to the threshold")
	}

	b.recordFailure()
	if b.allowed() {
		t.Error("expected backoff after exceeding the threshold")
	}

	b.reset()
	if !b.allowed() {
		t.Error("expected reset to clear the backoff")
	}

	capped := &accountBackoff{threshold: 0, maxDelay: 2 * time.Second, window: time.Hour}
	for i := 0; i < 40; i++ {
		capped.recordFailure()
	}
	if wait := time.Until(capped.until); wait > 2*time.Second {
		t.Errorf("expected delay capped at maxDelay, got %v", wait)
	}
}
