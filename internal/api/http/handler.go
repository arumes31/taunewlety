package http

import (
	"net/http"
	"taunewlety/internal/domain/models"
	"taunewlety/internal/service/newsletter"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Handler carries injected dependencies for all HTTP handlers.
type Handler struct {
	DB *gorm.DB
}

// NewHandler creates a Handler with the given database connection.
func NewHandler(db *gorm.DB) *Handler {
	return &Handler{DB: db}
}

// newNewsletterService constructs a NewsletterService with the injected DB
// and the current config from the database. Returns nil if no config is found.
func (h *Handler) newNewsletterService() *newsletter.NewsletterService {
	var config models.Config
	result := h.DB.First(&config)
	if result.Error != nil {
		return nil
	}
	return newsletter.NewNewsletterService(h.DB, &config)
}

// HealthCheck is a lightweight endpoint for Docker healthchecks and
// load-balancers. It verifies database connectivity and returns a
// JSON status — no authentication required.
func (h *Handler) HealthCheck(c *gin.Context) {
	dbOk := true
	sqlDB, err := h.DB.DB()
	if err != nil {
		dbOk = false
	} else if err = sqlDB.Ping(); err != nil {
		dbOk = false
	}
	if dbOk {
		c.JSON(http.StatusOK, gin.H{"status": "healthy", "db": "ok"})
	} else {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unhealthy", "db": "error"})
	}
}

// RegisterHandlers registers all application routes on the given engine.
func RegisterHandlers(r *gin.Engine, db *gorm.DB) {
	h := NewHandler(db)

	// Public routes (no auth required)
	public := r.Group("/")
	{
		public.GET("/health", h.HealthCheck)
		public.GET("/login", h.LoginGet)
		public.POST("/login", LoginRateLimit(), h.LoginPost)
		public.GET("/unsubscribe", h.UnsubscribeGet)
		public.POST("/unsubscribe", h.UnsubscribePost)
		public.GET("/api/logs", h.LogsGet)
	}

	// Authorized routes (auth + CSRF required)
	authorized := r.Group("/")
	authorized.Use(AuthRequired())
	authorized.Use(CSRFProtection())
	{
		authorized.GET("/", h.DashboardGet)
		authorized.GET("/logout", h.LogoutGet)
		authorized.POST("/settings", h.SettingsPost)
		authorized.POST("/subscribers", h.SubscriberAdd)
		authorized.POST("/subscribers/delete", h.SubscriberDelete)
		authorized.POST("/subscribers/toggle", h.SubscribersToggle)
		authorized.GET("/subscribers/export", h.SubscribersExport)
		authorized.POST("/subscribers/import", h.SubscribersImport)
		authorized.GET("/preview", h.NewsletterPreview)
		authorized.POST("/send", h.NewsletterSendManual)
		authorized.POST("/blacklist/delete", h.BlacklistDelete)
	}
}
