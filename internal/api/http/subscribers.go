package http

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"net/mail"
	"strconv"
	"taunewlety/internal/domain/models"
	"taunewlety/internal/platform/database"
)

func (h *Handler) SubscriberAdd(c *gin.Context) {
	email := c.PostForm("email")
	if email == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Email address is required"})
		return
	}

	// Validate email format
	_, err := mail.ParseAddress(email)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Invalid email address format"})
		return
	}

	// Check if already exists to avoid duplicates
	var count int64
	err = database.DB.Model(&models.Subscriber{}).Where("email = ?", email).Count(&count).Error
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Database error checking subscriber existence"})
		return
	}
	if count > 0 {
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{"error": "Subscriber with this email already exists"})
		return
	}

	res := database.DB.Create(&models.Subscriber{Email: email})
	if res.Error != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to add subscriber: " + res.Error.Error()})
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
	res := database.DB.Unscoped().Delete(&models.Subscriber{}, id)
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
