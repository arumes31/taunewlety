package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"taunewlety/internal/api"
	"taunewlety/internal/db"
	"taunewlety/internal/logic"
	"taunewlety/internal/models"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

func main() {
	_ = godotenv.Load()

	logger, _ := zap.NewProduction()
	defer logger.Sync()

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "taunewlety.db"
	}
	db.InitDB(dbPath)

	r := gin.Default()
	sessionSecret := os.Getenv("SESSION_SECRET")
	if sessionSecret == "" {
		sessionSecret = "taunewlety-default-secret-2026"
	}
	store := cookie.NewStore([]byte(sessionSecret))
	r.Use(sessions.Sessions("mysession", store))

	r.Static("/static", "web/static")
	r.StaticFile("/favicon.ico", "web/static/favicon.svg")

	r.LoadHTMLGlob("web/templates/*")
	api.RegisterHandlers(r)

	c := cron.New()
	_, err := c.AddFunc("0 9 * * *", func() {
		config, _ := db.GetConfig()
		if config != nil {
			svc := logic.NewNewsletterService(config)
			subject, body, err := svc.GenerateNewsletter()
			if err != nil {
				logger.Error("Generation failed", zap.Error(err))
				return
			}
			
			// Send to all active subscribers
			var subs []models.Subscriber
			db.DB.Where("active = ?", true).Find(&subs)
			for _, sub := range subs {
				_ = svc.SendEmail(sub.Email, subject, body)
			}
		}
	})
	if err != nil {
		logger.Fatal("Cron error", zap.Error(err))
	}
	c.Start()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("listen", zap.Error(err))
		}
	}()

	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatal("Server forced to shutdown:", zap.Error(err))
	}

	logger.Info("Server exiting")
}
