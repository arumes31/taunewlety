package taunewlety

import (
	"context"
	"net/http"
	"os"
	api_http "taunewlety/internal/api/http"
	"taunewlety/internal/domain/models"
	"taunewlety/internal/platform/database"
	"taunewlety/internal/service/newsletter"
	"time"

	"github.com/joho/godotenv"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

var Version = "v0.1-dev"
var cronSchedule = "0 9 * * *"

var loggerFatal = func(logger *zap.Logger, msg string, fields ...zap.Field) {
	logger.Fatal(msg, fields...)
}

type App struct {
	logger *zap.Logger
	cron   *cron.Cron
	srv    *http.Server
}

func NewApp() *App {
	_ = godotenv.Load()
	logger, _ := zap.NewProduction()
	
	return &App{
		logger: logger,
		cron:   cron.New(),
	}
}

func (a *App) Run(ctx context.Context) error {
	defer func() { _ = a.logger.Sync() }()

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "taunewlety.db"
	}
	database.InitDB(dbPath)

	r := api_http.SetupRouter()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	a.srv = &http.Server{
		Addr:              ":" + port,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

	a.setupScheduler()
	a.cron.Start()

	go func() {
		if err := a.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			loggerFatal(a.logger, "listen", zap.Error(err))
		}
	}()

	a.logger.Info("TauNewlety started", zap.String("version", Version), zap.String("port", port))

	// Wait for context cancellation
	<-ctx.Done()

	return nil
}

func (a *App) setupScheduler() {
	_, err := a.cron.AddFunc(cronSchedule, func() {
		config, _ := database.GetConfig()
		if config != nil {
			svc := newsletter.NewNewsletterService(config)
			subject, body, err := svc.GenerateNewsletter()
			if err != nil {
				a.logger.Error("Scheduled generation failed", zap.Error(err))
				return
			}

			var subs []models.Subscriber
			database.DB.Where("active = ?", true).Find(&subs)
			for _, sub := range subs {
				_ = svc.SendEmail(sub.Email, subject, body)
			}
		}
	})
	if err != nil {
		a.logger.Error("Cron setup failed", zap.Error(err))
	}
}

func (a *App) Shutdown(ctx context.Context) error {
	a.logger.Info("Shutting down TauNewlety...")
	
	a.cron.Stop()
	
	if err := a.srv.Shutdown(ctx); err != nil {
		return err
	}

	a.logger.Info("TauNewlety stopped")
	return nil
}
