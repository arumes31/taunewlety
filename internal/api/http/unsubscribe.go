package http

import (
	"log"
	"net/http"
	"strconv"
	"taunewlety/internal/domain/models"
	"taunewlety/internal/platform/database"
	"taunewlety/internal/service/newsletter"
	"taunewlety/pkg"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// sendUnsubscribeEmail is a package-level variable to allow mocking in tests.
// #nosec G107
var sendUnsubscribeEmail = sendUnsubscribeConfirmationEmail

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

	if err := session.Save(); err != nil {
		c.String(http.StatusInternalServerError, "Failed to save session")
		return
	}

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

	// Verify that the form-submitted email matches the session email
	formEmail := c.PostForm("email")
	if formEmail != "" && formEmail != email {
		c.String(http.StatusForbidden, "Email mismatch")
		return
	}

	// Rate limit: reject if last unsubscribe attempt was within 5 seconds
	lastAttemptVal := session.Get("last_unsubscribe_attempt")
	if lastAttemptVal != nil {
		switch v := lastAttemptVal.(type) {
		case int64:
			if time.Now().Unix()-v < 5 {
				c.String(http.StatusTooManyRequests, "Too many attempts. Please wait a few seconds and try again.")
				return
			}
		case float64:
			if time.Now().Unix()-int64(v) < 5 {
				c.String(http.StatusTooManyRequests, "Too many attempts. Please wait a few seconds and try again.")
				return
			}
		}
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

	// Store rate limit timestamp
	session.Set("last_unsubscribe_attempt", time.Now().Unix())

	// Remove captcha and email from session
	session.Delete("captcha_answer")
	session.Delete("csrf_token")
	session.Delete("unsubscribe_email")
	if err := session.Save(); err != nil {
		c.String(http.StatusInternalServerError, "Failed to save session")
		return
	}

	// Hard-delete so the unique email index is freed and the user can
	// re-subscribe later (a soft delete would leave the row in place and
	// cause a UNIQUE constraint failure on re-subscription).
	res := h.DB.Unscoped().Where("email = ?", email).Delete(&models.Subscriber{})
	if res.Error != nil {
		c.String(http.StatusInternalServerError, "Failed to unsubscribe: "+res.Error.Error())
		return
	}
	if res.RowsAffected == 0 {
		c.String(http.StatusNotFound, "Subscriber not found.")
		return
	}

	// Send confirmation email in the background (best-effort, non-blocking)
	go sendUnsubscribeEmail(email)

	c.String(http.StatusOK, "You have been successfully unsubscribed. A confirmation email has been sent to %s.", email)
}

// sendUnsubscribeConfirmationEmail sends a best-effort notification to the
// subscriber confirming that they have been unsubscribed. Errors are logged
// but do not affect the unsubscribe operation. Delivery goes through the
// newsletter service's mailer so SMTPEncryption is honoured here too.
// Recipient addresses are deliberately kept out of the logs.
func sendUnsubscribeConfirmationEmail(email string) {
	config, err := database.GetConfig()
	if err != nil || config == nil {
		log.Printf("Warning: could not load config for unsubscribe confirmation email: %v", err)
		return
	}

	if config.SMTPHost == "" || config.SMTPUser == "" {
		log.Printf("Warning: SMTP not configured, skipping unsubscribe confirmation email")
		return
	}

	// The unsubscribe link is rendered by the shared mailer, which replaces
	// this placeholder with a per-recipient URL.
	body := "<html><body><p>Hello,</p>" +
		"<p>You have been successfully unsubscribed from the TauNewlety newsletter. " +
		"If you did not request this, you can re-subscribe through the application.</p>" +
		"<p>If this was not you, please contact the server administrator.</p>" +
		"<hr><p><small>Manage your subscription: " +
		"<a href=\"{{.UnsubscribeURL}}\">{{.UnsubscribeURL}}</a></small></p>" +
		"</body></html>"

	svc := newsletter.NewNewsletterService(database.GetDB(), config)
	if err := svc.SendEmail(email, "Unsubscribe Confirmation", body); err != nil {
		log.Printf("Warning: failed to send unsubscribe confirmation email: %v", err)
	}
}
