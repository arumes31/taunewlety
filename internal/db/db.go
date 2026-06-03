package db

import (
	"log"
	"taunewlety/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

var DB *gorm.DB

func InitDB(dbPath string) {
	var err error
	DB, err = gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	// Auto-migrate the schema
	err = DB.AutoMigrate(&models.Config{}, &models.Blacklist{}, &models.Subscriber{}, &models.RecommendationStat{}, &models.TokenUsage{})
	if err != nil {
		log.Fatalf("failed to migrate database: %v", err)
	}

	// Seed default config if empty
	var count int64
	DB.Model(&models.Config{}).Count(&count)
	if count == 0 {
		defaultConfig := models.Config{
			OllamaURL:   "http://ollama:11434",
			OllamaModel: "llama3.2:3b",
			Language:    "en_US",
			RecCount:    10,
		}
		DB.Create(&defaultConfig)
	}
}

func GetConfig() (*models.Config, error) {
	var config models.Config
	result := DB.First(&config)
	if result.Error != nil {
		return nil, result.Error
	}
	return &config, nil
}

func SaveConfig(config *models.Config) error {
	var count int64
	DB.Model(&models.Config{}).Count(&count)
	if count == 0 {
		return DB.Create(config).Error
	}
	// Assuming ID 1 for simplicity as there's only one config
	config.ID = 1
	return DB.Save(config).Error
}
