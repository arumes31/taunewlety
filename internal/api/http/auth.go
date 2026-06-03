package http

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"os"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

func generateCSRFToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func generateAndStoreCSRF(session sessions.Session) string {
	token := generateCSRFToken()
	session.Set("csrf_token", token)
	_ = session.Save()
	return token
}

func (h *Handler) LoginGet(c *gin.Context) {
	session := sessions.Default(c)
	token := generateAndStoreCSRF(session)
	c.HTML(http.StatusOK, "login.html", gin.H{
		"csrfToken": token,
	})
}

func (h *Handler) LoginPost(c *gin.Context) {
	session := sessions.Default(c)
	
	// CSRF validation
	csrfInput := c.PostForm("csrf_token")
	csrfSession := session.Get("csrf_token")
	if csrfSession == nil || csrfInput == "" || csrfInput != csrfSession.(string) {
		token := generateAndStoreCSRF(session)
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
		token := generateAndStoreCSRF(session)
		c.HTML(http.StatusInternalServerError, "login.html", gin.H{
			"error":     "Server authentication not configured",
			"csrfToken": token,
		})
		return
	}

	userMatch := subtle.ConstantTimeCompare([]byte(user), []byte(envUser)) == 1
	passMatch := subtle.ConstantTimeCompare([]byte(pass), []byte(envPass)) == 1

	if userMatch && passMatch {
		session.Set("user", user)
		if err := session.Save(); err != nil {
			token := generateAndStoreCSRF(session)
			c.HTML(http.StatusInternalServerError, "login.html", gin.H{
				"error":     "Failed to save session",
				"csrfToken": token,
			})
			return
		}
		c.Redirect(http.StatusFound, "/")
	} else {
		token := generateAndStoreCSRF(session)
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{
			"error":     "Invalid credentials",
			"csrfToken": token,
		})
	}
}
