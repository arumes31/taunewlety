package http

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"taunewlety/internal/domain/models"
	"taunewlety/internal/platform/database"
)

func (h *Handler) DashboardGet(c *gin.Context) {
	config, _ := database.GetConfig()
	var subscribers []models.Subscriber
	database.DB.Find(&subscribers)
	var totalTokens int64
	database.DB.Model(&models.TokenUsage{}).Select("sum(total_tokens)").Row().Scan(&totalTokens)
	c.HTML(http.StatusOK, "index.html", gin.H{
		"config":      config,
		"subscribers": subscribers,
		"totalTokens": totalTokens,
	})
}

func (h *Handler) SettingsPost(c *gin.Context) {
	var config models.Config
	if err := c.ShouldBind(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	_ = database.SaveConfig(&config)
	c.Redirect(http.StatusFound, "/")
}
