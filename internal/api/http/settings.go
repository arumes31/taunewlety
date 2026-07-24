package http

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strconv"
	"taunewlety/internal/domain/models"
	"taunewlety/internal/platform/database"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type EditableConfigDTO struct {
	TautulliURL    string `form:"tautulli_url" binding:"required,url"`
	TautulliAPIKey string `form:"tautulli_api_key" binding:"required"`
	PlexURL        string `form:"plex_url" binding:"omitempty,url"`
	PlexToken      string `form:"plex_token" binding:"omitempty"`
	OllamaURL      string `form:"ollama_url" binding:"required,url"`
	OllamaModel    string `form:"ollama_model" binding:"required"`
	SMTPHost       string `form:"smtp_host" binding:"required"`
	SMTPPort       int    `form:"smtp_port" binding:"required,min=1,max=65535"`
	SMTPUser       string `form:"smtp_user" binding:"required"`
	SMTPPass       string `form:"smtp_pass" binding:"required"`
	SMTPSender     string `form:"smtp_sender" binding:"required,email"`
	SMTPEncryption string `form:"smtp_encryption" binding:"omitempty,oneof=starttls tls none"`
	AppBaseURL     string `form:"app_base_url" binding:"required,url"`
	DiscordWebhook string `form:"discord_webhook" binding:"omitempty,url"`
	TelegramBotTok string `form:"telegram_bot_token" binding:"omitempty"`
	TelegramChatID string `form:"telegram_chat_id" binding:"omitempty"`
	NewsletterTime string `form:"newsletter_time" binding:"required"`
	RecCount       int    `form:"rec_count" binding:"required,min=1,max=100"`
	Language       string `form:"language" binding:"required"`
}

func (h *Handler) DashboardGet(c *gin.Context) {
	config, err := database.GetConfig()
	if err != nil || config == nil {
		log.Printf("Error loading config: %v", err)
		config = &models.Config{}
	}

	// Pagination parameters with defaults
	pageStr := c.DefaultQuery("page", "1")
	perPageStr := c.DefaultQuery("per_page", "50")

	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}
	perPage, err := strconv.Atoi(perPageStr)
	if err != nil || perPage < 1 {
		perPage = 50
	}

	offset := (page - 1) * perPage

	// Count total subscribers
	var totalSubscribers int64
	if err := h.DB.Model(&models.Subscriber{}).Count(&totalSubscribers).Error; err != nil {
		log.Printf("Failed to count subscribers: %v", err)
		c.HTML(http.StatusInternalServerError, "index.html", gin.H{
			"config":      config,
			"subscribers": []models.Subscriber{},
		})
		return
	}

	// Fetch paginated subscribers
	var subscribers []models.Subscriber
	if err := h.DB.Offset(offset).Limit(perPage).Order("id ASC").Find(&subscribers).Error; err != nil {
		log.Printf("Failed to fetch subscribers: %v", err)
		c.HTML(http.StatusInternalServerError, "index.html", gin.H{
			"config":      config,
			"subscribers": []models.Subscriber{},
		})
		return
	}

	var totalTokens int64
	var nullTokens sql.NullInt64
	err = h.DB.Model(&models.TokenUsage{}).Select("COALESCE(sum(total_tokens), 0)").Row().Scan(&nullTokens)
	if err != nil {
		log.Printf("Error scanning total tokens: %v", err)
	}
	if nullTokens.Valid {
		totalTokens = nullTokens.Int64
	}

	// Fetch blacklist entries
	var blacklist []models.Blacklist
	if err := h.DB.Order("id ASC").Find(&blacklist).Error; err != nil {
		log.Printf("Failed to fetch blacklist: %v", err)
		blacklist = []models.Blacklist{}
	}

	// Calculate total pages
	totalPages := int(totalSubscribers) / perPage
	if int(totalSubscribers)%perPage != 0 {
		totalPages++
	}
	if totalPages < 1 {
		totalPages = 1
	}

	// Retrieve CSRF token set by the middleware
	csrfToken, _ := c.Get("csrf_token")

	c.HTML(http.StatusOK, "index.html", gin.H{
		"config":           config,
		"subscribers":      subscribers,
		"totalTokens":      totalTokens,
		"csrf_token":       csrfToken,
		"blacklist":        blacklist,
		"page":             page,
		"per_page":         perPage,
		"total_subscribers": totalSubscribers,
		"total_pages":      totalPages,
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
		if errors.Is(err, gorm.ErrRecordNotFound) {
			config = &models.Config{
				RecCount:       10,
				NewsletterTime: "09:00",
			}
		} else {
			log.Printf("Error loading config: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load existing config: " + err.Error()})
			return
		}
	}

	// Copy only allowed fields from the DTO to loaded config
	config.TautulliURL = dto.TautulliURL
	config.TautulliAPIKey = dto.TautulliAPIKey
	config.PlexURL = dto.PlexURL
	config.PlexToken = dto.PlexToken
	config.OllamaURL = dto.OllamaURL
	config.OllamaModel = dto.OllamaModel
	config.SMTPHost = dto.SMTPHost
	config.SMTPPort = dto.SMTPPort
	config.SMTPUser = dto.SMTPUser
	config.SMTPPass = dto.SMTPPass
	config.SMTPSender = dto.SMTPSender
	config.SMTPEncryption = dto.SMTPEncryption
	config.AppBaseURL = dto.AppBaseURL
	config.DiscordWebhook = dto.DiscordWebhook
	config.TelegramBotTok = dto.TelegramBotTok
	config.TelegramChatID = dto.TelegramChatID
	config.NewsletterTime = dto.NewsletterTime
	config.RecCount = dto.RecCount
	config.Language = dto.Language

	if err := database.SaveConfig(config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save settings: " + err.Error()})
		return
	}

	c.Redirect(http.StatusFound, "/")
}
