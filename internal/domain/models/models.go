package models

import "gorm.io/gorm"

type Config struct {
	gorm.Model
	TautulliURL    string `json:"tautulli_url"`
	TautulliAPIKey string `json:"tautulli_api_key"`
	PlexURL        string `json:"plex_url"`
	PlexToken      string `json:"plex_token"`
	OllamaURL      string `json:"ollama_url"`
	OllamaModel    string `json:"ollama_model"`
	SMTPHost       string `json:"smtp_host"`
	SMTPPort       int    `json:"smtp_port"`
	SMTPUser       string `json:"smtp_user"`
	SMTPPass       string `json:"smtp_pass"`
	SMTPSender     string `json:"smtp_sender"`
	NewsletterTime string `json:"newsletter_time"`
	RecCount       int    `json:"rec_count"`
	Language       string `json:"language"`
	AppBaseURL     string `json:"app_base_url"` // For unsubscribe links
	DiscordWebhook string `json:"discord_webhook"`
	TelegramBotTok string `json:"telegram_bot_token"`
	TelegramChatID string `json:"telegram_chat_id"`
}

type Subscriber struct {
	gorm.Model
	Email  string `gorm:"uniqueIndex"`
	Active bool   `gorm:"default:true"`
}

type RecommendationStat struct {
	gorm.Model
	MediaID string
	Title   string
	SentAt  int64
	Opened  bool `gorm:"default:false"`
	Watched bool `gorm:"default:false"`
}

type TokenUsage struct {
	gorm.Model
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	LLMModel         string
}

type Blacklist struct {
	gorm.Model
	MediaID   string `gorm:"uniqueIndex"`
	MediaType string
	ExpiresAt int64
}
