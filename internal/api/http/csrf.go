package http

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// CSRFProtection is a middleware that generates and validates CSRF tokens.
// On GET/HEAD/OPTIONS requests it generates a new token, stores it in the
// session, and makes it available to templates via c.Set("csrf_token", ...).
// On POST/PUT/DELETE requests it validates the submitted token (form field
// "csrf_token" or header "X-CSRF-Token") against the session value.
func CSRFProtection() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		method := c.Request.Method

		switch method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			token, err := generateCSRFToken()
			if err != nil {
				c.String(http.StatusInternalServerError, "Failed to generate CSRF token")
				c.Abort()
				return
			}
			session.Set("csrf_token", token)
			_ = session.Save()
			c.Set("csrf_token", token)

		case http.MethodPost, http.MethodPut, http.MethodDelete:
			// Read token from form field or header
			csrfInput := c.PostForm("csrf_token")
			if csrfInput == "" {
				csrfInput = c.GetHeader("X-CSRF-Token")
			}

			csrfSession := session.Get("csrf_token")
			csrfStr, ok := csrfSession.(string)
			if csrfSession == nil || !ok || csrfInput == "" || csrfInput != csrfStr {
				c.String(http.StatusForbidden, "Invalid CSRF token")
				c.Abort()
				return
			}
			// Generate a new token for the next request
			newToken, err := generateCSRFToken()
			if err == nil {
				session.Set("csrf_token", newToken)
				_ = session.Save()
				c.Set("csrf_token", newToken)
			}
		}

		c.Next()
	}
}

// generateCSRFToken creates a cryptographically random hex token.
// This is a duplicate of the function previously in auth.go; that copy
// will be removed as part of the CSRF consolidation.
func generateCSRFToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
