package database

import (
	"errors"
	"os"
	"testing"

	"taunewlety/internal/domain/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestInitDB(t *testing.T) {
	os.Setenv("DB_PATH", ":memory:")
	defer os.Unsetenv("DB_PATH")

	// Backup and restore globals
	oldDB := DB
	oldOpenDB := OpenDB
	oldFatalf := logFatalf
	oldPrintf := logPrintf
	defer func() {
		DB = oldDB
		OpenDB = oldOpenDB
		logFatalf = oldFatalf
		logPrintf = oldPrintf
	}()

	t.Run("Success_SeedDefault", func(t *testing.T) {
		logFatalf = func(format string, v ...interface{}) { t.Errorf("logFatalf called unexpectedly: "+format, v...) }
		logPrintf = func(format string, v ...interface{}) { t.Errorf("logPrintf called unexpectedly: "+format, v...) }
		OpenDB = func() (*gorm.DB, error) {
			return gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		}

		InitDB()

		if DB == nil {
			t.Fatal("DB should not be nil")
		}

		var count int64
		DB.Model(&models.Config{}).Count(&count)
		if count != 1 {
			t.Errorf("expected 1 config, got %d", count)
		}
	})

	t.Run("Success_AlreadySeeded", func(t *testing.T) {
		logFatalf = func(format string, v ...interface{}) { t.Errorf("logFatalf called unexpectedly: "+format, v...) }
		logPrintf = func(format string, v ...interface{}) { t.Errorf("logPrintf called unexpectedly: "+format, v...) }

		sharedDSN := "file::memory:?cache=shared"
		OpenDB = func() (*gorm.DB, error) {
			return gorm.Open(sqlite.Open(sharedDSN), &gorm.Config{})
		}

		// First call seeds
		InitDB()
		// Second call skips seeding (same shared DB)
		InitDB()

		var count int64
		DB.Model(&models.Config{}).Count(&count)
		if count != 1 {
			t.Errorf("expected 1 config, got %d", count)
		}
	})

	t.Run("OpenDB_Error", func(t *testing.T) {
		var fatalCalled bool
		logFatalf = func(format string, v ...interface{}) { fatalCalled = true }
		OpenDB = func() (*gorm.DB, error) { return nil, errors.New("open error") }

		InitDB()

		if !fatalCalled {
			t.Error("expected logFatalf to be called")
		}
	})

	t.Run("AutoMigrate_Error", func(t *testing.T) {
		var fatalCalled bool
		logFatalf = func(format string, v ...interface{}) { fatalCalled = true }
		OpenDB = func() (*gorm.DB, error) {
			db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			// Close the database to make AutoMigrate fail
			sqlDB, _ := db.DB()
			_ = sqlDB.Close()
			return db, nil
		}

		InitDB()

		if !fatalCalled {
			t.Error("expected logFatalf to be called")
		}
	})

	t.Run("Count_Error", func(t *testing.T) {
		var printfCalled bool
		logPrintf = func(format string, v ...interface{}) { printfCalled = true }
		OpenDB = func() (*gorm.DB, error) {
			db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			// Fail Count query
			_ = db.Callback().Query().Before("gorm:query").Register("fail_query", func(d *gorm.DB) {

				_ = d.AddError(errors.New("query error"))

			})
			return db, nil
		}

		InitDB()

		if !printfCalled {
			t.Error("expected logPrintf to be called")
		}
	})

	t.Run("Create_Error", func(t *testing.T) {
		var printfCalled bool
		logPrintf = func(format string, v ...interface{}) { printfCalled = true }
		OpenDB = func() (*gorm.DB, error) {
			db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			// Fail Create
			_ = db.Callback().Create().Before("gorm:create").Register("fail_create", func(d *gorm.DB) {
				_ = d.AddError(errors.New("create error"))

			})
			return db, nil
		}

		InitDB()

		if !printfCalled {
			t.Error("expected logPrintf to be called")
		}
	})

	t.Run("Create_RowsAffectedZero", func(t *testing.T) {
		var printfCalled bool
		logPrintf = func(format string, v ...interface{}) { printfCalled = true }
		OpenDB = func() (*gorm.DB, error) {
			db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			// Set RowsAffected to 0 after create
			_ = db.Callback().Create().After("gorm:create").Register("zero_rows", func(d *gorm.DB) {

				d.RowsAffected = 0
			})
			return db, nil
		}

		InitDB()

		if !printfCalled {
			t.Error("expected logPrintf to be called")
		}
	})

	t.Run("RealOpenDB_Postgres", func(t *testing.T) {
		os.Setenv("DB_TYPE", "postgres")
		os.Setenv("DB_HOST", "localhost")
		os.Setenv("DB_USER", "user")
		os.Setenv("DB_PASSWORD", "pass")
		os.Setenv("DB_NAME", "db")
		defer func() {
			os.Unsetenv("DB_TYPE")
			os.Unsetenv("DB_HOST")
			os.Unsetenv("DB_USER")
			os.Unsetenv("DB_PASSWORD")
			os.Unsetenv("DB_NAME")
		}()

		// This will fail to connect but should cover the branch
		_, err := oldOpenDB()
		if err == nil {
			t.Error("expected error connecting to non-existent postgres, got nil")
		}
	})
}

func TestGetConfig(t *testing.T) {
	oldDB := DB
	defer func() { DB = oldDB }()

	t.Run("Success", func(t *testing.T) {
		db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		DB = db
		_ = DB.AutoMigrate(&models.Config{})

		DB.Create(&models.Config{Language: "en"})

		cfg, err := GetConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Language != "en" {
			t.Errorf("expected en, got %s", cfg.Language)
		}
	})

	t.Run("Error", func(t *testing.T) {
		db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		DB = db
		// No migration, table doesn't exist
		_, err := GetConfig()
		if err == nil {
			t.Error("expected error, got nil")
		}
	})
}

func TestSaveConfig(t *testing.T) {
	oldDB := DB
	defer func() { DB = oldDB }()

	t.Run("Create_Success", func(t *testing.T) {
		db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		DB = db
		_ = DB.AutoMigrate(&models.Config{})
		err := SaveConfig(&models.Config{Language: "fr"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var cfg models.Config
		DB.First(&cfg)
		if cfg.Language != "fr" {
			t.Errorf("expected fr, got %s", cfg.Language)
		}
	})

	t.Run("Update_Success", func(t *testing.T) {
		db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		DB = db
		_ = DB.AutoMigrate(&models.Config{})

		DB.Create(&models.Config{Language: "en"})

		err := SaveConfig(&models.Config{Language: "es"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var cfg models.Config
		DB.First(&cfg)
		if cfg.Language != "es" {
			t.Errorf("expected es, got %s", cfg.Language)
		}
		if cfg.ID != 1 {
			t.Errorf("expected ID 1, got %d", cfg.ID)
		}
	})

	t.Run("Count_Error", func(t *testing.T) {
		db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		DB = db
		// No table
		err := SaveConfig(&models.Config{})
		if err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("Create_Error", func(t *testing.T) {
		db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		DB = db
		_ = DB.AutoMigrate(&models.Config{})

		_ = db.Callback().Create().Before("gorm:create").Register("fail", func(d *gorm.DB) {
			_ = d.AddError(errors.New("fail"))

		})

		err := SaveConfig(&models.Config{})
		if err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("Save_Error", func(t *testing.T) {
		db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		DB = db
		_ = DB.AutoMigrate(&models.Config{})

		DB.Create(&models.Config{Language: "en"})

		_ = DB.Callback().Update().Before("gorm:update").Register("fail", func(d *gorm.DB) {
			_ = d.AddError(errors.New("fail"))

		})

		err := SaveConfig(&models.Config{Language: "es"})
		if err == nil {
			t.Error("expected error, got nil")
		}
	})
}
