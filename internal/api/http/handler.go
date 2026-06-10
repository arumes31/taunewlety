package http

import (
	"github.com/gin-gonic/gin"
)

type Handler struct {
	// Add dependencies here if needed, e.g., NewsletterService
}

func NewHandler() *Handler {
	return &Handler{}
}

func RegisterHandlers(r *gin.Engine) {
	h := NewHandler()

	r.GET("/login", h.LoginGet)
	r.POST("/login", h.LoginPost)

	// Public Unsubscribe flow with Captcha
	r.GET("/unsubscribe", h.UnsubscribeGet)
	r.POST("/unsubscribe", h.UnsubscribePost)

	authorized := r.Group("/")
	authorized.Use(AuthRequired())
	authorized.Use(CSRFProtection())
	{
		authorized.GET("/", h.DashboardGet)
		authorized.POST("/logout", h.LogoutPost)
		authorized.POST("/settings", h.SettingsPost)
		authorized.POST("/subscribers", h.SubscriberAdd)
		authorized.POST("/subscribers/delete", h.SubscriberDelete)
		authorized.GET("/preview", h.NewsletterPreview)
		authorized.POST("/send", h.NewsletterSendManual)
	}
}
