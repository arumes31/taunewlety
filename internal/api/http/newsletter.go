package http

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"os"
	"taunewlety/internal/platform/database"
	"taunewlety/internal/service/newsletter"
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
	err = svc.SendEmail(os.Getenv("NOTIFY_EMAIL"), subject, body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "Sent"})
}
