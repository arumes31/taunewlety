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

func (h *Handler) LoginPost(c *gin.Context) {
	ip := c.ClientIP()

	// Rate limit check: block if too many failed attempts from this IP
	if !loginLimiter.check(ip) {
		token, err := generateAndStoreCSRF(sessions.Default(c))
		if err != nil {
			c.String(http.StatusInternalServerError, "Failed to generate CSRF token")
			return
		}
		c.HTML(http.StatusTooManyRequests, "login.html", gin.H{
			"error":     "Too many login attempts. Try again later.",
			"csrfToken": token,
		})
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

	userMatch := subtle.ConstantTimeCompare([]byte(user), []byte(envUser)) == 1
	passMatch := subtle.ConstantTimeCompare([]byte(pass), []byte(envPass)) == 1

	if userMatch && passMatch {
		loginLimiter.reset(ip)
		session.Set("user", user)
		if err := session.Save(); err != nil {
			c.String(http.StatusInternalServerError, "Failed to save session")
			return
		}
		c.Redirect(http.StatusFound, "/")
	} else {
		loginLimiter.recordFailure(ip)
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
