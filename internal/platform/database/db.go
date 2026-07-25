package database

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"taunewlety/internal/domain/models"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// escapeDSNValue quotes a value for a PostgreSQL DSN key=value string.
// Inside a single-quoted value libpq treats a backslash as an escape
// character, so both backslashes and single quotes are backslash-escaped —
// SQL-style quote doubling would terminate the value early. Every other
// character, including whitespace and newlines, is preserved verbatim.
func escapeDSNValue(val string) string {
	escaped := strings.ReplaceAll(val, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "'", "\\'")
	return "'" + escaped + "'"
}

var (
	db *gorm.DB

	mu sync.RWMutex

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
	logPrintf = log.Printf
)

// GetDB returns the global database instance, protected by a read lock.
// Callers should use this instead of accessing the DB variable directly.
func GetDB() *gorm.DB {
	mu.RLock()
	defer mu.RUnlock()
	return db
}

// SetDB sets the global database instance, protected by a write lock.
// This is primarily used for testing.
func SetDB(newDB *gorm.DB) {
	mu.Lock()
	defer mu.Unlock()
	db = newDB
}

// InitDB opens the database, runs migrations, and seeds default data.
// It returns the initialized *gorm.DB so callers can inject it where needed.
// On error the global DB is left unchanged and the error is returned.
func InitDB() (*gorm.DB, error) {
	mu.Lock()
	defer mu.Unlock()

	newDB, err := OpenDB()
	if err != nil {
		return nil, fmt.Errorf("failed to connect database: %w", err)
	}

	// Best practice: Configure connection pool settings
	sqlDB, err := newDB.DB()
	if err != nil {
		logPrintf("failed to get underlying sql.DB for connection pool configuration: %v", err)
	} else {
		sqlDB.SetMaxIdleConns(10)
		sqlDB.SetMaxOpenConns(100)
		sqlDB.SetConnMaxLifetime(time.Hour)
	}

	// Run migrations (AutoMigrate is migration 0 / baseline)
	if err := RunMigrations(newDB); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	// Seed default config if empty
	var count int64
	result := newDB.Model(&models.Config{}).Count(&count)
	if result.Error != nil {
		logPrintf("failed to count configurations: %v", result.Error)
	} else if count == 0 {
		defaultConfig := models.Config{
			OllamaURL:      "http://ollama:11434",
			OllamaModel:    "llama3.2:3b",
			Language:       "en_US",
			RecCount:       10,
			NewsletterTime: "09:00",
		}
		res := newDB.Create(&defaultConfig)
		if res.Error != nil {
			logPrintf("failed to seed default configuration: %v", res.Error)
		} else if res.RowsAffected == 0 {
			logPrintf("default configuration not seeded (rows affected = 0)")
		}
	}

	db = newDB
	return newDB, nil
}

// migrationDef defines a numbered migration function with a descriptive name.
type migrationDef struct {
	Version int
	Name    string
	Up      func(db *gorm.DB) error
}

// migrations is the ordered list of application-level migrations.
// Migration 0 is the baseline AutoMigrate. Add new migrations here
// as the schema evolves (column renames, type changes, data migrations, etc.).
var migrations = []migrationDef{
	{
		Version: 0,
		Name:    "baseline_auto_migrate",
		Up: func(db *gorm.DB) error {
			return db.AutoMigrate(
				&models.Config{},
				&models.Blacklist{},
				&models.Subscriber{},
				&models.RecommendationStat{},
				&models.TokenUsage{},
				&models.MigrationVersion{},
			)
		},
	},
	{
		Version: 1,
		Name:    "template_for_future_migrations",
		Up: func(db *gorm.DB) error {
			// This is a placeholder migration to demonstrate the pattern.
			// Replace with actual schema changes when needed, e.g.:
			//   return db.Exec("ALTER TABLE configs ADD COLUMN IF NOT EXISTS new_field VARCHAR(255)").Error
			return nil
		},
	},
}

// RunMigrations executes all pending migrations in order, tracking which
// versions have already been applied in the migration_versions table.
func RunMigrations(db *gorm.DB) error {
	// Ensure the migration_versions table exists first
	if err := db.AutoMigrate(&models.MigrationVersion{}); err != nil {
		return fmt.Errorf("failed to create migration_versions table: %w", err)
	}

	for _, m := range migrations {
		// Check if this migration has already been applied
		var count int64
		db.Model(&models.MigrationVersion{}).Where("version = ?", m.Version).Count(&count)
		if count > 0 {
			continue // already applied
		}

		logPrintf("Running migration %d: %s", m.Version, m.Name)
		if err := m.Up(db); err != nil {
			return fmt.Errorf("migration %d (%s) failed: %w", m.Version, m.Name, err)
		}

		// Record that this migration was applied
		if err := db.Create(&models.MigrationVersion{
			Version: m.Version,
			Name:    m.Name,
		}).Error; err != nil {
			return fmt.Errorf("failed to record migration %d: %w", m.Version, err)
		}
		logPrintf("Migration %d (%s) applied successfully", m.Version, m.Name)
	}

	return nil
}

func GetConfig() (*models.Config, error) {
	var config models.Config
	result := GetDB().First(&config)
	if result.Error != nil {
		return nil, result.Error
	}
	return &config, nil
}

func SaveConfig(config *models.Config) error {
	d := GetDB()
	var existingConfig models.Config
	result := d.FirstOrCreate(&existingConfig)
	if result.Error != nil {
		return result.Error
	}
	// Copy fields from the incoming config to the existing record
	existingConfig.TautulliURL = config.TautulliURL
	existingConfig.TautulliAPIKey = config.TautulliAPIKey
	existingConfig.PlexURL = config.PlexURL
	existingConfig.PlexToken = config.PlexToken
	existingConfig.OllamaURL = config.OllamaURL
	existingConfig.OllamaModel = config.OllamaModel
	existingConfig.SMTPHost = config.SMTPHost
	existingConfig.SMTPPort = config.SMTPPort
	existingConfig.SMTPUser = config.SMTPUser
	existingConfig.SMTPPass = config.SMTPPass
	existingConfig.SMTPSender = config.SMTPSender
	existingConfig.SMTPEncryption = config.SMTPEncryption
	existingConfig.AppBaseURL = config.AppBaseURL
	existingConfig.DiscordWebhook = config.DiscordWebhook
	existingConfig.TelegramBotTok = config.TelegramBotTok
	existingConfig.TelegramChatID = config.TelegramChatID
	existingConfig.NewsletterTime = config.NewsletterTime
	existingConfig.RecCount = config.RecCount
	existingConfig.Language = config.Language
	*config = existingConfig
	return d.Save(config).Error
}
