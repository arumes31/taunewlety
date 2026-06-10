package http

import (
	"net/http"
	"strconv"
	"taunewlety/internal/domain/models"
	"taunewlety/internal/platform/database"
	"taunewlety/pkg"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

func (h *Handler) UnsubscribeGet(c *gin.Context) {
	email := c.Query("email")
	captcha, err := pkg.GenerateCaptcha()
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to generate captcha: %v", err)
		return
	}

	session := sessions.Default(c)
	session.Set("captcha_answer", captcha.Answer)
	session.Set("unsubscribe_email", email)

	// Use the CSRF token from the middleware (already stored in session
	// and set in context by CSRFProtection). Fall back to generating one
	// if the middleware was not applied (e.g. direct handler test).
	csrfToken, _ := c.Get("csrf_token")
	csrfStr, ok := csrfToken.(string)
	if !ok || csrfStr == "" {
		csrfStr, err = generateCSRFToken()
		if err != nil {
			c.String(http.StatusInternalServerError, "Failed to generate CSRF token: %v", err)
			return
		}
		session.Set("csrf_token", csrfStr)
	}

	_ = session.Save()

	c.HTML(http.StatusOK, "unsubscribe.html", gin.H{
		"email":      email,
		"question":   captcha.Question,
		"csrf_token": csrfStr,
	})
}

func (h *Handler) UnsubscribePost(c *gin.Context) {
	session := sessions.Default(c)

	// Verify CSRF — the centralized middleware validates the token for
	// POST requests on authorized routes. For the public unsubscribe
	// endpoint (no middleware), we validate manually using the session
	// key "csrf_token" set by the GET handler or middleware.
	csrfInput := c.PostForm("csrf_token")
	if csrfInput == "" {
		csrfInput = c.GetHeader("X-CSRF-Token")
	}
	csrfSession := session.Get("csrf_token")
	csrfSessionStr, ok := csrfSession.(string)
	if !ok || csrfInput == "" || csrfInput != csrfSessionStr {
		c.String(http.StatusForbidden, "Invalid CSRF token")
		return
	}

	email, _ := session.Get("unsubscribe_email").(string)
	if email == "" {
		c.String(http.StatusBadRequest, "Email not found in session")
		return
	}
	answerStr := c.PostForm("answer")
	answer, err := strconv.Atoi(answerStr)
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid answer format")
		return
	}

	correctAnswerVal := session.Get("captcha_answer")
	if correctAnswerVal == nil {
		c.String(http.StatusUnauthorized, "Captcha answer not found in session")
		return
	}

	var correctAnswerInt int
	matched := false
	switch v := correctAnswerVal.(type) {
	case int:
		correctAnswerInt = v
		matched = true
	case float64:
		correctAnswerInt = int(v)
		matched = true
	case string:
		if parsed, err := strconv.Atoi(v); err == nil {
			correctAnswerInt = parsed
			matched = true
		}
	}

	if !matched || answer != correctAnswerInt {
		c.String(http.StatusUnauthorized, "Invalid captcha answer. Please try again.")
		return
	}

	// Remove captcha and email from session
	session.Delete("captcha_answer")
	session.Delete("csrf_token")
	session.Delete("unsubscribe_email")
	_ = session.Save()

	// Hard-delete so the unique email index is freed and the user can
	// re-subscribe later (a soft delete would leave the row in place and
	// cause a UNIQUE constraint failure on re-subscription).
	res := database.GetDB().Unscoped().Where("email = ?", email).Delete(&models.Subscriber{})
	if res.Error != nil {
		c.String(http.StatusInternalServerError, "Failed to unsubscribe: "+res.Error.Error())
		return
	}
	if res.RowsAffected == 0 {
		c.String(http.StatusNotFound, "Subscriber not found.")
		return
	}

	c.String(http.StatusOK, "You have been successfully unsubscribed.")
}
