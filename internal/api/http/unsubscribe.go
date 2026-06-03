package http

import (
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
	"taunewlety/internal/domain/models"
	"taunewlety/internal/platform/database"
	"taunewlety/pkg"
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
	
	// Generate CSRF token for unsubscribe
	csrfToken := generateCSRFToken()
	session.Set("csrf_token_unsub", csrfToken)
	_ = session.Save()

	c.HTML(http.StatusOK, "unsubscribe.html", gin.H{
		"email":     email,
		"question":  captcha.Question,
		"CSRFToken": csrfToken,
	})
}

func (h *Handler) UnsubscribePost(c *gin.Context) {
	session := sessions.Default(c)

	// Verify CSRF
	csrfInput := c.PostForm("csrf_token")
	csrfSession := session.Get("csrf_token_unsub")
	csrfSessionStr, ok := csrfSession.(string)
	if !ok || csrfInput == "" || csrfInput != csrfSessionStr {
		c.String(http.StatusForbidden, "Invalid CSRF token")
		return
	}

	email := c.PostForm("email")
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

	// Remove captcha and CSRF from session
	session.Delete("captcha_answer")
	session.Delete("csrf_token_unsub")
	_ = session.Save()

	// Hard-delete so the unique email index is freed and the user can
	// re-subscribe later (a soft delete would leave the row in place and
	// cause a UNIQUE constraint failure on re-subscription).
	res := database.DB.Unscoped().Where("email = ?", email).Delete(&models.Subscriber{})
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
