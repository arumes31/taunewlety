package http

import (
	"errors"
	"net/http"
	"os"
	"taunewlety/internal/domain/models"
	"taunewlety/internal/service/newsletter"

	"github.com/gin-gonic/gin"
)

func (h *Handler) NewsletterPreview(c *gin.Context) {
	svc := h.newNewsletterService()
	if svc == nil {
		c.String(http.StatusBadRequest, "Configure settings first")
		return
	}
	subject, body, err := svc.GenerateNewsletterWithContext(c.Request.Context())
	if err != nil {
		if errors.Is(err, newsletter.ErrNoRecommendations) {
			c.String(http.StatusUnprocessableEntity, "No recommendations available. Check your Tautulli connection and settings.")
			return
		}
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	c.HTML(http.StatusOK, "preview.html", gin.H{"subject": subject, "content": body})
}

func (h *Handler) NewsletterSendManual(c *gin.Context) {
	svc := h.newNewsletterService()
	if svc == nil {
		c.String(http.StatusBadRequest, "Configure settings first")
		return
	}
	subject, body, err := svc.GenerateNewsletterWithContext(c.Request.Context())
	if err != nil {
		if errors.Is(err, newsletter.ErrNoRecommendations) {
			c.JSON(http.StatusOK, gin.H{"status": "Skipped", "message": "No recommendations found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	// Send to all active subscribers
	var subscribers []models.Subscriber
	h.DB.Where("active = ?", true).Find(&subscribers)
	for _, sub := range subscribers {
		go func(email string) {
			_ = svc.SendEmail(email, subject, body)
		}(sub.Email)
	}

	// Also send to admin notification email as a backup
	if notifyEmail := os.Getenv("NOTIFY_EMAIL"); notifyEmail != "" {
		go func() {
			_ = svc.SendEmail(notifyEmail, subject, body)
		}()
	}

	c.JSON(http.StatusOK, gin.H{"status": "Sent"})
}
