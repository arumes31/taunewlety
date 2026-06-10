package http

import (
	"net/http"
	"os"
	"taunewlety/internal/domain/models"
	"taunewlety/internal/platform/database"
	"taunewlety/internal/service/newsletter"

	"github.com/gin-gonic/gin"
)

func (h *Handler) NewsletterPreview(c *gin.Context) {
	config, _ := database.GetConfig()
	if config == nil {
		c.String(http.StatusBadRequest, "Configure settings first")
		return
	}
	svc := newsletter.NewNewsletterService(config)
	subject, body, err := svc.GenerateNewsletter()
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	c.HTML(http.StatusOK, "preview.html", gin.H{"subject": subject, "content": body})
}

func (h *Handler) NewsletterSendManual(c *gin.Context) {
	config, err := database.GetConfig()
	if err != nil || config == nil {
		c.String(http.StatusBadRequest, "Configure settings first")
		return
	}
	svc := newsletter.NewNewsletterService(config)
	subject, body, err := svc.GenerateNewsletter()
	if err != nil {
		if err == newsletter.ErrNoRecommendations {
			c.JSON(http.StatusOK, gin.H{"status": "Skipped", "message": "No recommendations found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	// Send to all active subscribers
	var subscribers []models.Subscriber
	database.GetDB().Where("active = ?", true).Find(&subscribers)
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
