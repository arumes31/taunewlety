package http

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"taunewlety/internal/domain/models"
	"taunewlety/internal/platform/database"
)

func (h *Handler) SubscriberAdd(c *gin.Context) {
	email := c.PostForm("email")
	if email != "" {
		database.DB.Create(&models.Subscriber{Email: email})
	}
	c.Redirect(http.StatusFound, "/")
}

func (h *Handler) SubscriberDelete(c *gin.Context) {
	id := c.PostForm("id")
	database.DB.Delete(&models.Subscriber{}, id)
	c.Redirect(http.StatusFound, "/")
}
