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

// Pagination bounds for the subscriber list. maxPage keeps
// (page-1)*maxPerPage well inside int32 so the offset cannot overflow on a
// 32-bit build.
const (
	defaultPerPage = 50
	maxPerPage     = 200
	maxPage        = 100000
)

// EditableConfigDTO carries the settings form. Secret fields are optional:
// the form renders them blank, and an empty submission keeps the stored value
// (see SettingsPost), so requiring them would reject every save.
type EditableConfigDTO struct {
	TautulliURL    string `form:"tautulli_url" binding:"required,url"`
	TautulliAPIKey string `form:"tautulli_api_key" binding:"omitempty"`
	PlexURL        string `form:"plex_url" binding:"omitempty,url"`
	PlexToken      string `form:"plex_token" binding:"omitempty"`
	OllamaURL      string `form:"ollama_url" binding:"required,url"`
	OllamaModel    string `form:"ollama_model" binding:"required"`
	SMTPHost       string `form:"smtp_host" binding:"required"`
	SMTPPort       int    `form:"smtp_port" binding:"required,min=1,max=65535"`
	SMTPUser       string `form:"smtp_user" binding:"required"`
	SMTPPass       string `form:"smtp_pass" binding:"omitempty"`
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

// renderDashboardError renders the standalone error page. index.html assumes
// a fully populated context (pagination counters, CSRF token, blacklist), so
// re-rendering it on a database failure would fail inside the template.
func renderDashboardError(c *gin.Context, message string) {
	c.HTML(http.StatusInternalServerError, "error.html", gin.H{
		"title":   "Something went wrong",
		"message": message,
	})
}

func (h *Handler) DashboardGet(c *gin.Context) {
	config, err := database.GetConfig()
	if err != nil || config == nil {
		log.Printf("Error loading config: %v", err)
		config = &models.Config{}
	}

	// Pagination parameters with defaults, clamped so a hostile page or
	// per_page value cannot overflow the offset or ask for an unbounded scan.
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}
	if page > maxPage {
		page = maxPage
	}
	perPage, err := strconv.Atoi(c.DefaultQuery("per_page", strconv.Itoa(defaultPerPage)))
	if err != nil || perPage < 1 {
		perPage = defaultPerPage
	}
	if perPage > maxPerPage {
		perPage = maxPerPage
	}

	offset := (page - 1) * perPage

	// Count total subscribers
	var totalSubscribers int64
	if err := h.DB.Model(&models.Subscriber{}).Count(&totalSubscribers).Error; err != nil {
		log.Printf("Failed to count subscribers: %v", err)
		renderDashboardError(c, "Could not load subscribers.")
		return
	}

	// Fetch paginated subscribers
	var subscribers []models.Subscriber
	if err := h.DB.Offset(offset).Limit(perPage).Order("id ASC").Find(&subscribers).Error; err != nil {
		log.Printf("Failed to fetch subscribers: %v", err)
		renderDashboardError(c, "Could not load subscribers.")
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
		"config":            config,
		"subscribers":       subscribers,
		"totalTokens":       totalTokens,
		"csrf_token":        csrfToken,
		"blacklist":         blacklist,
		"page":              page,
		"per_page":          perPage,
		"total_subscribers": totalSubscribers,
		"total_pages":       totalPages,
	})
}

func (h *Handler) SettingsPost(c *gin.Context) {
	var dto EditableConfigDTO
	if err := c.ShouldBind(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	config, err := database.GetConfig()
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Printf("Error loading config: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load existing config: " + err.Error()})
		return
	}
	// Covers both "no row yet" and a nil config returned without an error.
	if config == nil {
		config = &models.Config{
			RecCount:       10,
			NewsletterTime: "09:00",
		}
	}

	// Copy only allowed fields from the DTO to loaded config
	config.TautulliURL = dto.TautulliURL
	config.PlexURL = dto.PlexURL
	config.OllamaURL = dto.OllamaURL
	config.OllamaModel = dto.OllamaModel
	config.SMTPHost = dto.SMTPHost
	config.SMTPPort = dto.SMTPPort
	config.SMTPUser = dto.SMTPUser
	config.SMTPSender = dto.SMTPSender
	config.SMTPEncryption = dto.SMTPEncryption
	config.AppBaseURL = dto.AppBaseURL
	config.DiscordWebhook = dto.DiscordWebhook
	config.TelegramChatID = dto.TelegramChatID
	config.NewsletterTime = dto.NewsletterTime
	config.RecCount = dto.RecCount
	config.Language = dto.Language

	// Secrets are never rendered back into the form, so a blank submission
	// means "unchanged" rather than "clear it".
	if dto.TautulliAPIKey != "" {
		config.TautulliAPIKey = dto.TautulliAPIKey
	}
	if dto.PlexToken != "" {
		config.PlexToken = dto.PlexToken
	}
	if dto.SMTPPass != "" {
		config.SMTPPass = dto.SMTPPass
	}
	if dto.TelegramBotTok != "" {
		config.TelegramBotTok = dto.TelegramBotTok
	}

	if err := database.SaveConfig(config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save settings: " + err.Error()})
		return
	}

	c.Redirect(http.StatusFound, "/")
}
