package http

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"taunewlety/internal/domain/models"
	"taunewlety/internal/platform/database"
	"taunewlety/pkg"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const unsubTestSecret = "test-secret-123"

// unsubscribeLink returns the signed GET path for email.
func unsubscribeLink(email string) string {
	return "/unsubscribe?email=" + url.QueryEscape(email) +
		"&token=" + url.QueryEscape(pkg.UnsubscribeToken(unsubTestSecret, email))
}

var reCSRF = regexp.MustCompile(`name="csrf_token" value="([^"]+)"`)

// openUnsubscribeForm loads the confirmation page and returns its session
// cookie and CSRF token.
func openUnsubscribeForm(t *testing.T, r http.Handler, email string) (string, string) {
	t.Helper()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, unsubscribeLink(email), nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 for signed link, got: %d", w.Code)
	}
	m := reCSRF.FindStringSubmatch(w.Body.String())
	if len(m) < 2 {
		t.Fatal("could not find CSRF token in unsubscribe form")
	}
	return w.Header().Get("Set-Cookie"), m[1]
}

func postUnsubscribe(r http.Handler, cookieVal string, form url.Values) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/unsubscribe", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if cookieVal != "" {
		req.Header.Set("Cookie", cookieVal)
	}
	r.ServeHTTP(w, req)
	return w
}

func TestUnsubscribeHandlers(t *testing.T) {
	// Override sendUnsubscribeEmail to prevent goroutine from accessing
	// a stale DB when tests run in sequence, and to count confirmations.
	sent := make(chan string, 4)
	originalSendEmail := sendUnsubscribeEmail
	sendUnsubscribeEmail = func(email string) { sent <- email }
	defer func() { sendUnsubscribeEmail = originalSendEmail }()

	_ = os.Setenv("DB_PATH", ":memory:")
	defer func() { _ = os.Unsetenv("DB_PATH") }()

	db, err := database.InitDB()
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	t.Setenv("SESSION_SECRET", unsubTestSecret)
	t.Setenv("UNSUBSCRIBE_SECRET", "")

	database.GetDB().Create(&models.Subscriber{Email: "test@example.com"})

	gin.SetMode(gin.TestMode)
	r, routerErr := SetupRouter(db, zap.NewNop())
	if routerErr != nil {
		t.Fatalf("SetupRouter failed: %v", routerErr)
	}

	t.Run("GET without token is rejected", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/unsubscribe?email=test@example.com", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for unsigned link, got: %d", w.Code)
		}
		if strings.Contains(w.Body.String(), `name="csrf_token"`) {
			t.Error("unsigned link must not render the confirmation form")
		}
	})

	t.Run("GET with forged token is rejected", func(t *testing.T) {
		w := httptest.NewRecorder()
		forged := "/unsubscribe?email=test@example.com&token=" + pkg.UnsubscribeToken("wrong", "test@example.com")
		req, _ := http.NewRequest(http.MethodGet, forged, nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for forged token, got: %d", w.Code)
		}
	})

	cookieVal, csrfToken := openUnsubscribeForm(t, r, "test@example.com")

	t.Run("POST with bad CSRF is rejected", func(t *testing.T) {
		form := url.Values{}
		form.Set("csrf_token", "bad-csrf")
		form.Set("email", "test@example.com")
		form.Set("token", pkg.UnsubscribeToken(unsubTestSecret, "test@example.com"))
		if w := postUnsubscribe(r, cookieVal, form); w.Code != http.StatusForbidden {
			t.Errorf("expected 403 on bad CSRF, got: %d", w.Code)
		}
	})

	t.Run("POST with token for another address is rejected", func(t *testing.T) {
		form := url.Values{}
		form.Set("csrf_token", csrfToken)
		form.Set("email", "test@example.com")
		form.Set("token", pkg.UnsubscribeToken(unsubTestSecret, "attacker@example.com"))
		if w := postUnsubscribe(r, cookieVal, form); w.Code != http.StatusForbidden {
			t.Errorf("expected 403 on mismatched token, got: %d", w.Code)
		}
	})

	t.Run("POST with valid token unsubscribes", func(t *testing.T) {
		form := url.Values{}
		form.Set("csrf_token", csrfToken)
		form.Set("email", "test@example.com")
		form.Set("token", pkg.UnsubscribeToken(unsubTestSecret, "test@example.com"))
		w := postUnsubscribe(r, cookieVal, form)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got: %d, body: %s", w.Code, w.Body.String())
		}
		if w.Body.String() != unsubscribeDoneMessage {
			t.Errorf("unexpected body: %s", w.Body.String())
		}

		var count int64
		database.GetDB().Model(&models.Subscriber{}).Where("email = ?", "test@example.com").Count(&count)
		if count != 0 {
			t.Error("expected subscriber to be deleted from DB")
		}
	})

	t.Run("unknown address gets the same response", func(t *testing.T) {
		cookie2, csrf2 := openUnsubscribeForm(t, r, "notfound@example.com")
		form := url.Values{}
		form.Set("csrf_token", csrf2)
		form.Set("email", "notfound@example.com")
		form.Set("token", pkg.UnsubscribeToken(unsubTestSecret, "notfound@example.com"))
		w := postUnsubscribe(r, cookie2, form)
		if w.Code != http.StatusOK || w.Body.String() != unsubscribeDoneMessage {
			t.Errorf("expected generic 200 response, got: %d %s", w.Code, w.Body.String())
		}
	})

	// Confirmations are sent from a goroutine; exactly one is expected,
	// for the real removal only.
	select {
	case got := <-sent:
		if got != "test@example.com" {
			t.Errorf("confirmation sent to %q, want test@example.com", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected a confirmation email for the real removal")
	}
	select {
	case got := <-sent:
		t.Errorf("unexpected confirmation email to %q", got)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestUnsubscribePost_DBError(t *testing.T) {
	originalSendEmail := sendUnsubscribeEmail
	sendUnsubscribeEmail = func(email string) {}
	defer func() { sendUnsubscribeEmail = originalSendEmail }()

	t.Setenv("SESSION_SECRET", unsubTestSecret)
	t.Setenv("UNSUBSCRIBE_SECRET", "")

	_ = os.Setenv("DB_PATH", ":memory:")
	defer func() { _ = os.Unsetenv("DB_PATH") }()

	db, _ := database.InitDB()
	database.GetDB().Create(&models.Subscriber{Email: "test@example.com"})

	gin.SetMode(gin.TestMode)
	h := NewHandler(db)

	r := gin.New()
	r.Use(sessions.Sessions("mysession", cookie.NewStore([]byte("secret"))))
	r.POST("/unsubscribe", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("csrf_token", "valid-csrf")
		_ = session.Save()
		c.Next()
	}, h.UnsubscribePost)

	_ = database.GetDB().Callback().Delete().Before("gorm:delete").Register("fail_subscriber_delete", func(d *gorm.DB) {
		_ = d.AddError(fmt.Errorf("simulated subscriber delete error"))
	})

	form := url.Values{}
	form.Set("csrf_token", "valid-csrf")
	form.Set("email", "test@example.com")
	form.Set("token", pkg.UnsubscribeToken(unsubTestSecret, "test@example.com"))
	w := postUnsubscribe(r, "", form)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected InternalServerError (500) on database delete error, got: %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "simulated") {
		t.Errorf("database error details must not be exposed, got: %s", w.Body.String())
	}
}

func TestUnsubscribePost_EdgeCases(t *testing.T) {
	originalSendEmail := sendUnsubscribeEmail
	sendUnsubscribeEmail = func(email string) {}
	defer func() { sendUnsubscribeEmail = originalSendEmail }()

	_ = os.Setenv("DB_PATH", ":memory:")
	defer func() { _ = os.Unsetenv("DB_PATH") }()

	db, _ := database.InitDB()
	gin.SetMode(gin.TestMode)
	h := NewHandler(db)

	validToken := pkg.UnsubscribeToken(unsubTestSecret, "test@example.com")

	tests := []struct {
		name     string
		secret   string
		email    string
		token    string
		wantCode int
	}{
		{"missing email", unsubTestSecret, "", validToken, http.StatusForbidden},
		{"missing token", unsubTestSecret, "test@example.com", "", http.StatusForbidden},
		{"no secret configured", "", "test@example.com", validToken, http.StatusForbidden},
		{"valid", unsubTestSecret, "test@example.com", validToken, http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SESSION_SECRET", tt.secret)
			t.Setenv("UNSUBSCRIBE_SECRET", "")

			r := gin.New()
			r.Use(sessions.Sessions("mysession", cookie.NewStore([]byte("secret"))))
			r.POST("/unsubscribe", func(c *gin.Context) {
				session := sessions.Default(c)
				session.Set("csrf_token", "valid-csrf")
				_ = session.Save()
				c.Next()
			}, h.UnsubscribePost)

			form := url.Values{}
			form.Set("csrf_token", "valid-csrf")
			form.Set("email", tt.email)
			form.Set("token", tt.token)
			if w := postUnsubscribe(r, "", form); w.Code != tt.wantCode {
				t.Errorf("expected %d, got %d (%s)", tt.wantCode, w.Code, w.Body.String())
			}
		})
	}
}
