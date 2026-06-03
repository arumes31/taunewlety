package database

import (
	"log"
	"taunewlety/internal/domain/models"

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
	result := DB.Model(&models.Config{}).Count(&count)
	if result.Error != nil {
		log.Printf("failed to count configurations: %v", result.Error)
	} else if count == 0 {
		defaultConfig := models.Config{
			OllamaURL:   "http://ollama:11434",
			OllamaModel: "llama3.2:3b",
			Language:    "en_US",
			RecCount:    10,
		}
		res := DB.Create(&defaultConfig)
		if res.Error != nil {
			log.Printf("failed to seed default configuration: %v", res.Error)
		} else if res.RowsAffected == 0 {
			log.Printf("default configuration not seeded (rows affected = 0)")
		}
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
	r := DB.Model(&models.Config{}).Count(&count)
	if r.Error != nil {
		return r.Error
	}
	if count == 0 {
		return DB.Create(config).Error
	}
	// Assuming ID 1 for simplicity as there's only one config
	config.ID = 1
	return DB.Save(config).Error
}
