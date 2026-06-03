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
	TautulliURL    string `form:"tautulli_url"`
	TautulliAPIKey string `form:"tautulli_api_key"`
	OllamaURL      string `form:"ollama_url"`
	OllamaModel    string `form:"ollama_model"`
	SMTPHost       string `form:"smtp_host"`
	SMTPPort       int    `form:"smtp_port"`
	SMTPUser       string `form:"smtp_user"`
	SMTPPass       string `form:"smtp_pass"`
	SMTPSender     string `form:"smtp_sender"`
	AppBaseURL     string `form:"app_base_url"`
	DiscordWebhook string `form:"discord_webhook"`
	TelegramBotTok string `form:"telegram_bot_token"`
	TelegramChatID string `form:"telegram_chat_id"`
	Language       string `form:"language"`
}

func (h *Handler) DashboardGet(c *gin.Context) {
	config, _ := database.GetConfig()
	var subscribers []models.Subscriber
	database.DB.Find(&subscribers)
	
	var totalTokens int64
	var nullTokens sql.NullInt64
	err := database.DB.Model(&models.TokenUsage{}).Select("COALESCE(sum(total_tokens), 0)").Row().Scan(&nullTokens)
	if err != nil {
		log.Printf("Error scanning total tokens: %v", err)
	}
	if nullTokens.Valid {
		totalTokens = nullTokens.Int64
	}

	c.HTML(http.StatusOK, "index.html", gin.H{
		"config":      config,
		"subscribers": subscribers,
		"totalTokens": totalTokens,
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
