package database

import (
	"fmt"
	"log"
	"os"
	"strings"
	"taunewlety/internal/domain/models"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// escapeDSNValue escapes values for use in a PostgreSQL DSN key=value string.
func escapeDSNValue(val string) string {
	escaped := strings.ReplaceAll(val, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "'", "''")
	return "'" + escaped + "'"
}

var (
	DB     *gorm.DB
	OpenDB = func() (*gorm.DB, error) {
		dbType := os.Getenv("DB_TYPE")
		if dbType == "postgres" || os.Getenv("DB_HOST") != "" {
			host := os.Getenv("DB_HOST")
			user := os.Getenv("DB_USER")
			password := os.Getenv("DB_PASSWORD")
			dbname := os.Getenv("DB_NAME")

			if host == "" || user == "" || password == "" || dbname == "" {
				return nil, fmt.Errorf("missing required database connection parameters (DB_HOST, DB_USER, DB_PASSWORD, DB_NAME)")
			}

			port := os.Getenv("DB_PORT")
			if port == "" {
				port = "5432"
			}
			// sslmode: defaults to 'require' for security. 
			// For local development without SSL, set DB_SSLMODE=disable.
			sslmode := os.Getenv("DB_SSLMODE")
			if sslmode == "" {
				sslmode = "require"
			}

			dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
				escapeDSNValue(host), escapeDSNValue(user), escapeDSNValue(password), escapeDSNValue(dbname), escapeDSNValue(port), escapeDSNValue(sslmode))
			return gorm.Open(postgres.Open(dsn), &gorm.Config{
				SkipDefaultTransaction: true,
				PrepareStmt:            true,
			})
		}

		dbPath := os.Getenv("DB_PATH")
		if dbPath == "" {
			dbPath = "taunewlety.db"
		}
		return gorm.Open(sqlite.Open(dbPath), &gorm.Config{
			SkipDefaultTransaction: true,
			PrepareStmt:            true,
		})
	}
	logFatalf = log.Fatalf
	logPrintf = log.Printf
)

func InitDB() {
	var err error
	DB, err = OpenDB()
	if err != nil {
		logFatalf("failed to connect database: %v", err)
		return
	}

	// Best practice: Configure connection pool settings
	sqlDB, err := DB.DB()
	if err != nil {
		logPrintf("failed to get underlying sql.DB for connection pool configuration: %v", err)
	} else {
		sqlDB.SetMaxIdleConns(10)
		sqlDB.SetMaxOpenConns(100)
		sqlDB.SetConnMaxLifetime(time.Hour)
	}

	// Auto-migrate the schema
	err = DB.AutoMigrate(&models.Config{}, &models.Blacklist{}, &models.Subscriber{}, &models.RecommendationStat{}, &models.TokenUsage{})
	if err != nil {
		logFatalf("failed to migrate database: %v", err)
		return
	}

	// Seed default config if empty
	var count int64
	result := DB.Model(&models.Config{}).Count(&count)
	if result.Error != nil {
		logPrintf("failed to count configurations: %v", result.Error)
	} else if count == 0 {
		defaultConfig := models.Config{
			OllamaURL:   "http://ollama:11434",
			OllamaModel: "llama3.2:3b",
			Language:    "en_US",
			RecCount:    10,
		}
		res := DB.Create(&defaultConfig)
		if res.Error != nil {
			logPrintf("failed to seed default configuration: %v", res.Error)
		} else if res.RowsAffected == 0 {
			logPrintf("default configuration not seeded (rows affected = 0)")
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
