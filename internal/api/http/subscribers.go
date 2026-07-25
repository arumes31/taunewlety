package http

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"taunewlety/internal/domain/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handler) SubscriberAdd(c *gin.Context) {
	email := c.PostForm("email")
	if email == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Email address is required"})
		return
	}

	// Validate the email format, and store the parsed address so this path
	// normalizes exactly as the CSV import does — otherwise the same person
	// could be stored twice in two different spellings.
	parsed, err := mail.ParseAddress(email)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Invalid email address format"})
		return
	}
	email = parsed.Address

	// Check if already exists to avoid duplicates
	var count int64
	err = h.DB.Model(&models.Subscriber{}).Where("email = ?", email).Count(&count).Error
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Database error checking subscriber existence"})
		return
	}
	if count > 0 {
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{"error": "Subscriber with this email already exists"})
		return
	}

	res := h.DB.Create(&models.Subscriber{Email: email})
	if res.Error != nil {
		errMsg := res.Error.Error()
		if strings.Contains(errMsg, "UNIQUE constraint") || strings.Contains(errMsg, "duplicate key") {
			c.AbortWithStatusJSON(http.StatusConflict, gin.H{"error": "Subscriber with this email already exists"})
		} else {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to add subscriber: " + errMsg})
		}
		return
	}

	c.Redirect(http.StatusFound, "/")
}

func (h *Handler) SubscriberDelete(c *gin.Context) {
	idStr := c.PostForm("id")
	if idStr == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Subscriber ID is required"})
		return
	}

	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Invalid subscriber ID format"})
		return
	}

	// Hard-delete so the unique email index is freed and the address can be
	// added again later (a soft delete would block re-adding the same email).
	res := h.DB.Unscoped().Delete(&models.Subscriber{}, id)
	if res.Error != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete subscriber: " + res.Error.Error()})
		return
	}
	if res.RowsAffected == 0 {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "Subscriber not found"})
		return
	}

	c.Redirect(http.StatusFound, "/")
}

// SubscribersToggle toggles the Active field of a subscriber by ID.
func (h *Handler) SubscribersToggle(c *gin.Context) {
	idStr := c.PostForm("id")
	if idStr == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Subscriber ID is required"})
		return
	}

	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Invalid subscriber ID format"})
		return
	}

	var subscriber models.Subscriber
	if err := h.DB.First(&subscriber, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "Subscriber not found"})
			return
		}
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch subscriber: " + err.Error()})
		return
	}

	subscriber.Active = !subscriber.Active
	if err := h.DB.Save(&subscriber).Error; err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to toggle subscriber: " + err.Error()})
		return
	}

	c.Redirect(http.StatusFound, "/")
}

// BlacklistDelete removes a blacklist entry by ID.
func (h *Handler) BlacklistDelete(c *gin.Context) {
	idStr := c.PostForm("id")
	if idStr == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Blacklist entry ID is required"})
		return
	}

	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Invalid blacklist entry ID format"})
		return
	}

	res := h.DB.Unscoped().Delete(&models.Blacklist{}, id)
	if res.Error != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete blacklist entry: " + res.Error.Error()})
		return
	}
	if res.RowsAffected == 0 {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "Blacklist entry not found"})
		return
	}

	c.Redirect(http.StatusFound, "/")
}

// SubscribersExport returns a CSV file of all active subscribers.
func (h *Handler) SubscribersExport(c *gin.Context) {
	var subscribers []models.Subscriber
	if err := h.DB.Where("active = ?", true).Find(&subscribers).Error; err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch subscribers: " + err.Error()})
		return
	}

	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", "attachment; filename=subscribers.csv")

	writer := csv.NewWriter(c.Writer)
	// Write header
	if err := writer.Write([]string{"email", "name", "active"}); err != nil {
		log.Printf("Failed to write CSV header: %v", err)
		return
	}

	for _, sub := range subscribers {
		active := "true"
		if !sub.Active {
			active = "false"
		}
		if err := writer.Write([]string{sub.Email, "", active}); err != nil {
			log.Printf("Failed to write CSV row: %v", err)
			return
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		log.Printf("CSV flush error: %v", err)
	}
}

// SubscribersImport reads a CSV file from form upload and imports subscribers,
// skipping duplicates by checking if the email already exists.
func (h *Handler) SubscribersImport(c *gin.Context) {
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "CSV file is required"})
		return
	}
	defer file.Close()

	reader := csv.NewReader(file)

	// Read header row (skip it)
	if _, err := reader.Read(); err != nil {
		if err == io.EOF {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "CSV file is empty"})
			return
		}
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Failed to read CSV header: " + err.Error()})
		return
	}

	var imported, skipped int
	row := 1 // header was row 1; data rows start at 2
	for {
		row++
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			// A row with the wrong number of columns is a bad row, not a bad
			// file: skip it so the rest of the import still lands.
			if errors.Is(err, csv.ErrFieldCount) {
				log.Printf("Skipping malformed CSV row %d: %v", row, err)
				skipped++
				continue
			}
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Failed to read CSV row: " + err.Error()})
			return
		}

		if len(record) == 0 {
			continue
		}

		email := strings.TrimSpace(record[0])
		if email == "" {
			continue
		}

		// Validate the email format. The address itself is never logged.
		// Store the parsed address so a row written as `Bob <bob@x.com>`
		// normalizes to `bob@x.com` and matches the duplicate check.
		parsed, err := mail.ParseAddress(email)
		if err != nil {
			log.Printf("Skipping invalid email address on CSV row %d", row)
			skipped++
			continue
		}
		email = parsed.Address

		// Check if already exists
		var count int64
		if err := h.DB.Model(&models.Subscriber{}).Where("email = ?", email).Count(&count).Error; err != nil {
			log.Printf("Failed to check for an existing subscriber on CSV row %d: %v", row, err)
			skipped++
			continue
		}
		if count > 0 {
			skipped++
			continue
		}

		if err := h.DB.Create(&models.Subscriber{Email: email, Active: true}).Error; err != nil {
			log.Printf("Failed to import subscriber on CSV row %d: %v", row, err)
			skipped++
			continue
		}
		imported++
	}

	c.Redirect(http.StatusFound, fmt.Sprintf("/?imported=%d&skipped=%d", imported, skipped))
}
