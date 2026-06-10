package http

import (
	"database/sql"
	"log"
	"net/http"
	"taunewlety/internal/domain/models"
	"taunewlety/internal/platform/database"

	"github.com/gin-gonic/gin"
)

type EditableConfigDTO struct {
	TautulliURL    string `form:"tautulli_url" binding:"required,url"`
	TautulliAPIKey string `form:"tautulli_api_key" binding:"required"`
	OllamaURL      string `form:"ollama_url" binding:"required,url"`
	OllamaModel    string `form:"ollama_model" binding:"required"`
	SMTPHost       string `form:"smtp_host" binding:"required"`
	SMTPPort       int    `form:"smtp_port" binding:"required,min=1,max=65535"`
	SMTPUser       string `form:"smtp_user" binding:"required"`
	SMTPPass       string `form:"smtp_pass" binding:"required"`
	SMTPSender     string `form:"smtp_sender" binding:"required,email"`
	AppBaseURL     string `form:"app_base_url" binding:"required,url"`
	DiscordWebhook string `form:"discord_webhook" binding:"omitempty,url"`
	TelegramBotTok string `form:"telegram_bot_token" binding:"omitempty"`
	TelegramChatID string `form:"telegram_chat_id" binding:"omitempty"`
	Language       string `form:"language" binding:"required"`
}

func (h *Handler) DashboardGet(c *gin.Context) {
	config, err := database.GetConfig()
	if err != nil || config == nil {
		log.Printf("Error loading config: %v", err)
		config = &models.Config{}
	}
	var subscribers []models.Subscriber
	database.GetDB().Find(&subscribers)

	var totalTokens int64
	var nullTokens sql.NullInt64
	err = database.GetDB().Model(&models.TokenUsage{}).Select("COALESCE(sum(total_tokens), 0)").Row().Scan(&nullTokens)
	if err != nil {
		log.Printf("Error scanning total tokens: %v", err)
	}
	if nullTokens.Valid {
		totalTokens = nullTokens.Int64
	}

	// Retrieve CSRF token set by the middleware
	csrfToken, _ := c.Get("csrf_token")

	c.HTML(http.StatusOK, "index.html", gin.H{
		"config":      config,
		"subscribers": subscribers,
		"totalTokens": totalTokens,
		"csrf_token":  csrfToken,
	})
}

func (h *Handler) SettingsPost(c *gin.Context) {
	var dto EditableConfigDTO
	if err := c.ShouldBind(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	config, err := database.GetConfig()
	if err != nil {
		config = &models.Config{}
	}

	// Copy only allowed fields from the DTO to loaded config
	config.TautulliURL = dto.TautulliURL
	config.TautulliAPIKey = dto.TautulliAPIKey
	config.OllamaURL = dto.OllamaURL
	config.OllamaModel = dto.OllamaModel
	config.SMTPHost = dto.SMTPHost
	config.SMTPPort = dto.SMTPPort
	config.SMTPUser = dto.SMTPUser
	config.SMTPPass = dto.SMTPPass
	config.SMTPSender = dto.SMTPSender
	config.AppBaseURL = dto.AppBaseURL
	config.DiscordWebhook = dto.DiscordWebhook
	config.TelegramBotTok = dto.TelegramBotTok
	config.TelegramChatID = dto.TelegramChatID
	config.Language = dto.Language

	if err := database.SaveConfig(config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save settings: " + err.Error()})
		return
	}

	c.Redirect(http.StatusFound, "/")
}
