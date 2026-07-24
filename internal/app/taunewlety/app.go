package taunewlety

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
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
	logger, err := zap.NewProduction()
	if err != nil {
		panic("failed to initialize logger: " + err.Error())
	}

	return &App{
		logger: logger,
		cron:   cron.New(),
	}
}

func (a *App) Run(ctx context.Context) error {
	defer func() { _ = a.logger.Sync() }()

	// Fail fast if authentication credentials are not configured, rather than
	// surfacing the error lazily on the first login attempt.
	if os.Getenv("APP_USER") == "" || os.Getenv("APP_PASS") == "" {
		a.logger.Error("APP_USER and APP_PASS environment variables are required but were not set")
		return errors.New("APP_USER and APP_PASS environment variables are required")
	}
	db, err := database.InitDB()
	if err != nil {
		a.logger.Error("Failed to initialize database", zap.Error(err))
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	r, err := api_http.SetupRouter(db, a.logger)
	if err != nil {
		a.logger.Error("Failed to setup router", zap.Error(err))
		return fmt.Errorf("failed to setup router: %w", err)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	a.srv = &http.Server{
		Addr:              ":" + port,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	a.setupScheduler()
	a.cron.Start()

	tlsCert := os.Getenv("TLS_CERT")
	tlsKey := os.Getenv("TLS_KEY")

	go func() {
		if tlsCert != "" && tlsKey != "" {
			a.logger.Info("Starting server with TLS", zap.String("port", port))
			if err := a.srv.ListenAndServeTLS(tlsCert, tlsKey); err != nil && err != http.ErrServerClosed {
				loggerFatal(a.logger, "listen_tls", zap.Error(err))
			}
		} else {
			if err := a.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				loggerFatal(a.logger, "listen", zap.Error(err))
			}
		}
	}()

	proto := "http"
	if tlsCert != "" && tlsKey != "" {
		proto = "https"
	}
	a.logger.Info("TauNewlety started", zap.String("version", Version), zap.String("port", port), zap.String("protocol", proto))

	// Wait for context cancellation
	<-ctx.Done()

	return nil
}

// buildCronSchedule converts a "HH:MM" time string into a cron expression
// that runs daily at the specified time. Returns a default of "0 9 * * *"
// if the input is invalid or empty.
func buildCronSchedule(newsletterTime string) string {
	if newsletterTime == "" {
		return "0 9 * * *"
	}

	parts := strings.SplitN(newsletterTime, ":", 2)
	if len(parts) != 2 {
		return "0 9 * * *"
	}

	hour := parts[0]
	minute := parts[1]

	// Basic validation: hour 00-23, minute 00-59
	if len(hour) < 1 || len(hour) > 2 || len(minute) != 2 {
		return "0 9 * * *"
	}

	return fmt.Sprintf("%s %s * * *", minute, hour)
}

func (a *App) setupScheduler() {
	// Load the newsletter time from the config; fall back to "09:00" if unavailable.
	schedule := "0 9 * * *"
	config, err := database.GetConfig()
	if err == nil && config != nil && config.NewsletterTime != "" {
		schedule = buildCronSchedule(config.NewsletterTime)
	}

	_, err = a.cron.AddFunc(schedule, func() {
		config, err := database.GetConfig()
		if err != nil {
			a.logger.Error("Failed to load config for scheduled job", zap.Error(err))
			return
		}
		if config != nil {
			svc := newsletter.NewNewsletterService(database.GetDB(), config)
			subject, body, err := svc.GenerateNewsletter()
			if err != nil {
				a.logger.Error("Scheduled generation failed", zap.Error(err))
				return
			}

			var subs []models.Subscriber
			database.GetDB().Where("active = ?", true).Find(&subs)
			for _, sub := range subs {
				if err := svc.SendEmail(sub.Email, subject, body); err != nil {
					a.logger.Error("Failed to send email", zap.String("email", sub.Email), zap.Error(err))
				}
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

	if a.srv == nil {
		a.logger.Info("Server was not started, skipping shutdown")
		return nil
	}

	if err := a.srv.Shutdown(ctx); err != nil {
		return err
	}

	a.logger.Info("TauNewlety stopped")
	return nil
}
