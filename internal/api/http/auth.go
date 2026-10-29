package http

import (
	"crypto/subtle"
	"net/http"
	"os"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

func (h *Handler) LoginGet(c *gin.Context) {
	session := sessions.Default(c)
	token, err := generateAndStoreCSRF(session)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to generate CSRF token")
		return
	}
	c.HTML(http.StatusOK, "login.html", gin.H{
		"csrfToken": token,
	})
}

// maxLoginFormBytes bounds the login body; the form holds only a CSRF
// token, a username and a password.
const maxLoginFormBytes = 8 << 10

// renderLoginThrottled answers a login attempt rejected by the rate limits.
func renderLoginThrottled(c *gin.Context) {
	token, err := generateAndStoreCSRF(sessions.Default(c))
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to generate CSRF token")
		return
	}
	c.HTML(http.StatusTooManyRequests, "login.html", gin.H{
		"error":     "Too many login attempts. Try again later.",
		"csrfToken": token,
	})
}

func (h *Handler) LoginPost(c *gin.Context) {
	ip := c.ClientIP()

	// Cheap early rejection: too many failed attempts from this IP, or the
	// account-wide backoff (independent of IP) is in effect.
	if !loginLimiter.check(ip) || !loginBackoff.allowed() {
		renderLoginThrottled(c)
		return
	}

	// Read the whole (bounded) form up front, so the admission decision
	// below is made after any slow body upload rather than before it.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxLoginFormBytes)
	if err := c.Request.ParseForm(); err != nil {
		c.String(http.StatusRequestEntityTooLarge, "Invalid login request")
		return
	}

	session := sessions.Default(c)

	// CSRF validation (login has its own CSRF handling since there is
	// no authenticated session yet — the middleware is not applied here)
	csrfInput := c.PostForm("csrf_token")
	csrfSession := session.Get("csrf_token")
	csrfStr, ok := csrfSession.(string)
	if csrfSession == nil || !ok || csrfInput == "" || csrfInput != csrfStr {
		token, err := generateAndStoreCSRF(session)
		if err != nil {
			c.String(http.StatusInternalServerError, "Failed to generate CSRF token")
			return
		}
		c.HTML(http.StatusForbidden, "login.html", gin.H{
			"error":     "Invalid CSRF token",
			"csrfToken": token,
		})
		return
	}

	user := c.PostForm("username")
	pass := c.PostForm("password")

	envUser := os.Getenv("APP_USER")
	envPass := os.Getenv("APP_PASS")

	if envUser == "" || envPass == "" {
		token, err := generateAndStoreCSRF(session)
		if err != nil {
			c.String(http.StatusInternalServerError, "Failed to generate CSRF token")
			return
		}
		c.HTML(http.StatusInternalServerError, "login.html", gin.H{
			"error":     "Server authentication not configured",
			"csrfToken": token,
		})
		return
	}

	// Atomically reserve a credential check under both limits. The early
	// check above does not count requests already in flight, so without the
	// reservation a burst of concurrent requests could all reach the
	// comparison before any failure was recorded.
	if !loginLimiter.reserve(ip) {
		renderLoginThrottled(c)
		return
	}
	if !loginBackoff.reserve() {
		loginLimiter.release(ip, false)
		renderLoginThrottled(c)
		return
	}

	userMatch := subtle.ConstantTimeCompare([]byte(user), []byte(envUser)) == 1
	passMatch := subtle.ConstantTimeCompare([]byte(pass), []byte(envPass)) == 1

	if userMatch && passMatch {
		loginLimiter.release(ip, false)
		loginBackoff.release(false)
		loginLimiter.reset(ip)
		loginBackoff.reset()
		if err := startAdminSession(session, user); err != nil {
			c.String(http.StatusInternalServerError, "Failed to start session")
			return
		}
		if err := session.Save(); err != nil {
			c.String(http.StatusInternalServerError, "Failed to save session")
			return
		}
		c.Redirect(http.StatusFound, "/")
	} else {
		loginLimiter.release(ip, true)
		loginBackoff.release(true)
		token, err := generateAndStoreCSRF(session)
		if err != nil {
			c.String(http.StatusInternalServerError, "Failed to generate CSRF token")
			return
		}
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{
			"error":     "Invalid credentials",
			"csrfToken": token,
		})
	}
}

// LogoutGet clears the session and redirects to the login page.
func (h *Handler) LogoutGet(c *gin.Context) {
	session := sessions.Default(c)
	endAdminSession(session)
	session.Clear()
	session.Options(sessions.Options{MaxAge: -1})
	if err := session.Save(); err != nil {
		c.String(http.StatusInternalServerError, "Failed to save session")
		return
	}
	c.Redirect(http.StatusFound, "/login")
}

// generateAndStoreCSRF is kept here for the login flow which does not
// go through the centralized CSRF middleware (no authenticated session yet).
func generateAndStoreCSRF(session sessions.Session) (string, error) {
	token, err := generateCSRFToken()
	if err != nil {
		return "", err
	}
	session.Set("csrf_token", token)
	if err := session.Save(); err != nil {
		return "", err
	}
	return token, nil
}
