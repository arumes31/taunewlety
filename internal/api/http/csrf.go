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
// On POST/PUT/PATCH/DELETE requests it validates the submitted token (form
// field "csrf_token" or header "X-CSRF-Token") against the session value.
func CSRFProtection() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		method := c.Request.Method

		// Ensure a CSRF token exists for the session. A session value that is
		// missing, of another type, or empty is treated as "no token yet" so a
		// malformed cookie cannot panic the middleware.
		token, ok := session.Get("csrf_token").(string)
		if !ok || token == "" {
			newToken, err := generateCSRFToken()
			if err != nil {
				c.String(http.StatusInternalServerError, "Failed to generate CSRF token")
				c.Abort()
				return
			}
			session.Set("csrf_token", newToken)
			if err := session.Save(); err != nil {
				c.String(http.StatusInternalServerError, "Failed to save session")
				c.Abort()
				return
			}
			token = newToken
		}
		c.Set("csrf_token", token)

		switch method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			// Read token from form field or header
			csrfInput := c.PostForm("csrf_token")
			if csrfInput == "" {
				csrfInput = c.GetHeader("X-CSRF-Token")
			}

			if csrfInput == "" || csrfInput != token {
				c.String(http.StatusForbidden, "Invalid CSRF token")
				c.Abort()
				return
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
