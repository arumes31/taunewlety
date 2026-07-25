package http

import (
	"errors"
	"log"
	"net/http"
	"os"
	"sync"
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
	if err := h.DB.Where("active = ?", true).Find(&subscribers).Error; err != nil {
		log.Printf("Manual send: failed to load subscribers: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load subscribers"})
		return
	}

	recipients := make([]string, 0, len(subscribers)+1)
	for _, sub := range subscribers {
		recipients = append(recipients, sub.Email)
	}
	// Also send to admin notification email as a backup
	if notifyEmail := os.Getenv("NOTIFY_EMAIL"); notifyEmail != "" {
		recipients = append(recipients, notifyEmail)
	}

	go sendBulkEmail(svc, recipients, subject, body)

	c.JSON(http.StatusOK, gin.H{"status": "Sent", "recipients": len(recipients)})
}

// maxConcurrentSends bounds how many SMTP conversations run at once so a
// large subscriber list cannot exhaust sockets or trip the provider's
// connection limits.
const maxConcurrentSends = 5

// sendBulkEmail delivers the newsletter to every recipient using a bounded
// pool of workers. Failures are counted rather than logged individually so
// subscriber addresses never reach the log.
func sendBulkEmail(svc *newsletter.NewsletterService, recipients []string, subject, body string) {
	sem := make(chan struct{}, maxConcurrentSends)
	var wg sync.WaitGroup
	var mu sync.Mutex
	failed := 0

	for _, addr := range recipients {
		wg.Add(1)
		sem <- struct{}{}
		go func(a string) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := svc.SendEmail(a, subject, body); err != nil {
				mu.Lock()
				failed++
				mu.Unlock()
			}
		}(addr)
	}
	wg.Wait()

	if failed > 0 {
		log.Printf("Manual send: %d of %d messages failed to deliver", failed, len(recipients))
	}
}
