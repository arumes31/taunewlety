package http

import (
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
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

	// The admin notification address is a backup copy, not a second delivery.
	recipients := collectRecipients(subscribers, os.Getenv("NOTIFY_EMAIL"))

	go sendBulkEmail(svc.SendEmail, recipients, subject, body)

	c.JSON(http.StatusOK, gin.H{"status": "Sent", "recipients": len(recipients)})
}

// collectRecipients builds the delivery list from the active subscribers plus
// the optional admin notification address, deduplicated so nobody receives the
// newsletter twice — most commonly when NOTIFY_EMAIL is also a subscriber.
// Comparison ignores case and surrounding whitespace; the original spelling is
// what gets sent.
func collectRecipients(subscribers []models.Subscriber, notifyEmail string) []string {
	recipients := make([]string, 0, len(subscribers)+1)
	seen := make(map[string]bool, len(subscribers)+1)

	add := func(email string) {
		key := strings.ToLower(strings.TrimSpace(email))
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		recipients = append(recipients, email)
	}

	for _, sub := range subscribers {
		add(sub.Email)
	}
	add(notifyEmail)

	return recipients
}

// maxConcurrentSends bounds how many SMTP conversations run at once so a
// large subscriber list cannot exhaust sockets or trip the provider's
// connection limits.
const maxConcurrentSends = 5

// sendBulkEmail delivers the newsletter to every recipient using a bounded
// pool of workers. Failures are counted rather than logged individually so
// subscriber addresses never reach the log. send is normally
// NewsletterService.SendEmail; it is a parameter so tests can drive the
// failure and panic paths without a live SMTP server.
func sendBulkEmail(send func(to, subject, body string) error, recipients []string, subject, body string) {
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
			// This runs detached from the request, so gin's recovery
			// middleware cannot help: an unrecovered panic here would take
			// the whole process down mid-send.
			defer func() {
				if r := recover(); r != nil {
					mu.Lock()
					failed++
					mu.Unlock()
					log.Printf("Manual send: recovered from panic while sending a message: %v", r)
				}
			}()
			if err := send(a, subject, body); err != nil {
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
