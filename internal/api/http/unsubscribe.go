package http

import (
	"log"
	"net/http"
	"taunewlety/internal/domain/models"
	"taunewlety/internal/platform/database"
	"taunewlety/internal/service/newsletter"
	"taunewlety/pkg"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// sendUnsubscribeEmail is a package-level variable to allow mocking in tests.
// #nosec G107
var sendUnsubscribeEmail = sendUnsubscribeConfirmationEmail

// unsubscribeDoneMessage is returned for every authorized unsubscribe
// request, whether or not the address was on the list, so the response
// never reveals list membership.
const unsubscribeDoneMessage = "If this address was subscribed, it has been unsubscribed. A confirmation email will be sent."

// unsubscribeInvalidLinkMessage is shown for links without a valid token,
// including links from newsletters sent before tokens were introduced.
const unsubscribeInvalidLinkMessage = "This unsubscribe link is invalid. Please use the link from your most recent newsletter, or contact the server administrator."

// UnsubscribeGet renders the confirmation page for a signed unsubscribe link.
// The removal itself happens on POST so that mail scanners prefetching the
// link cannot unsubscribe anyone.
func (h *Handler) UnsubscribeGet(c *gin.Context) {
	email := c.Query("email")
	token := c.Query("token")
	if !pkg.VerifyUnsubscribeToken(pkg.UnsubscribeSecret(), email, token) {
		c.HTML(http.StatusBadRequest, "unsubscribe.html", gin.H{
			"error": unsubscribeInvalidLinkMessage,
		})
		return
	}

	session := sessions.Default(c)

	// Use the CSRF token from the middleware (already stored in session
	// and set in context by CSRFProtection). Fall back to generating one
	// if the middleware was not applied (e.g. direct handler test).
	csrfToken, _ := c.Get("csrf_token")
	csrfStr, ok := csrfToken.(string)
	if !ok || csrfStr == "" {
		var err error
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
		"token":      token,
		"csrf_token": csrfStr,
	})
}

// UnsubscribePost removes the subscriber named in a signed unsubscribe link.
// The HMAC token proves the request comes from the link mailed to that
// address, so no captcha or session state is needed to authorize it.
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

	email := c.PostForm("email")
	token := c.PostForm("token")
	if !pkg.VerifyUnsubscribeToken(pkg.UnsubscribeSecret(), email, token) {
		c.String(http.StatusForbidden, unsubscribeInvalidLinkMessage)
		return
	}

	session.Delete("csrf_token")
	if err := session.Save(); err != nil {
		c.String(http.StatusInternalServerError, "Failed to save session")
		return
	}

	// Hard-delete so the unique email index is freed and the user can
	// re-subscribe later (a soft delete would leave the row in place and
	// cause a UNIQUE constraint failure on re-subscription).
	var subscriber models.Subscriber
	err := h.DB.Unscoped().Where("email = ?", email).Limit(1).Find(&subscriber).Error
	if err == nil && subscriber.ID != 0 {
		err = h.DB.Unscoped().Delete(&subscriber).Error
	}
	if err != nil {
		log.Printf("Unsubscribe: failed to delete subscriber: %v", err)
		c.String(http.StatusInternalServerError, "Failed to unsubscribe. Please try again later.")
		return
	}

	// Only actual removals trigger a confirmation email, addressed to the
	// stored address rather than the request input; the response is the
	// same either way.
	if subscriber.ID != 0 {
		go sendUnsubscribeEmail(subscriber.Email)
	}

	c.String(http.StatusOK, unsubscribeDoneMessage)
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
