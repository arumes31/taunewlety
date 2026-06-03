package database

import (
	"errors"
	"testing"

	"taunewlety/internal/domain/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestInitDB(t *testing.T) {
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
		OpenDB = func(dbPath string) (*gorm.DB, error) {
			return gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
		}

		InitDB(":memory:")

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
		OpenDB = func(dbPath string) (*gorm.DB, error) {
			return gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
		}

		// First call seeds
		InitDB(":memory:")
		// Second call skips seeding
		InitDB(":memory:")

		var count int64
		DB.Model(&models.Config{}).Count(&count)
		if count != 1 {
			t.Errorf("expected 1 config, got %d", count)
		}
	})

	t.Run("OpenDB_Error", func(t *testing.T) {
		var fatalCalled bool
		logFatalf = func(format string, v ...interface{}) { fatalCalled = true }
		OpenDB = func(dbPath string) (*gorm.DB, error) { return nil, errors.New("open error") }

		InitDB(":memory:")

		if !fatalCalled {
			t.Error("expected logFatalf to be called")
		}
	})

	t.Run("AutoMigrate_Error", func(t *testing.T) {
		var fatalCalled bool
		logFatalf = func(format string, v ...interface{}) { fatalCalled = true }
		OpenDB = func(dbPath string) (*gorm.DB, error) {
			db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			// Close the database to make AutoMigrate fail
			sqlDB, _ := db.DB()
			sqlDB.Close()
			return db, nil
		}

		InitDB(":memory:")

		if !fatalCalled {
			t.Error("expected logFatalf to be called")
		}
	})

	t.Run("Count_Error", func(t *testing.T) {
		var printfCalled bool
		logPrintf = func(format string, v ...interface{}) { printfCalled = true }
		OpenDB = func(dbPath string) (*gorm.DB, error) {
			db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			// Fail Count query
			db.Callback().Query().Before("gorm:query").Register("fail_query", func(d *gorm.DB) {
				d.AddError(errors.New("query error"))
			})
			return db, nil
		}

		InitDB(":memory:")

		if !printfCalled {
			t.Error("expected logPrintf to be called")
		}
	})

	t.Run("Create_Error", func(t *testing.T) {
		var printfCalled bool
		logPrintf = func(format string, v ...interface{}) { printfCalled = true }
		OpenDB = func(dbPath string) (*gorm.DB, error) {
			db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			// Fail Create
			db.Callback().Create().Before("gorm:create").Register("fail_create", func(d *gorm.DB) {
				d.AddError(errors.New("create error"))
			})
			return db, nil
		}

		InitDB(":memory:")

		if !printfCalled {
			t.Error("expected logPrintf to be called")
		}
	})

	t.Run("Create_RowsAffectedZero", func(t *testing.T) {
		var printfCalled bool
		logPrintf = func(format string, v ...interface{}) { printfCalled = true }
		OpenDB = func(dbPath string) (*gorm.DB, error) {
			db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			// Set RowsAffected to 0 after create
			db.Callback().Create().After("gorm:create").Register("zero_rows", func(d *gorm.DB) {
				d.RowsAffected = 0
			})
			return db, nil
		}

		InitDB(":memory:")

		if !printfCalled {
			t.Error("expected logPrintf to be called")
		}
	})

	t.Run("RealOpenDB", func(t *testing.T) {
		// This covers the real OpenDB function body
		db, err := oldOpenDB(":memory:")
		if err != nil {
			t.Errorf("real OpenDB failed: %v", err)
		}
		if db == nil {
			t.Error("real OpenDB returned nil db")
		}
	})
}

func TestGetConfig(t *testing.T) {
	oldDB := DB
	defer func() { DB = oldDB }()

	t.Run("Success", func(t *testing.T) {
		db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		DB = db
		DB.AutoMigrate(&models.Config{})
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
		DB.AutoMigrate(&models.Config{})

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
		DB.AutoMigrate(&models.Config{})
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
		DB.AutoMigrate(&models.Config{})
		DB.Callback().Create().Before("gorm:create").Register("fail", func(d *gorm.DB) {
			d.AddError(errors.New("fail"))
		})

		err := SaveConfig(&models.Config{})
		if err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("Save_Error", func(t *testing.T) {
		db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		DB = db
		DB.AutoMigrate(&models.Config{})
		DB.Create(&models.Config{Language: "en"})

		DB.Callback().Update().Before("gorm:update").Register("fail", func(d *gorm.DB) {
			d.AddError(errors.New("fail"))
		})

		err := SaveConfig(&models.Config{Language: "es"})
		if err == nil {
			t.Error("expected error, got nil")
		}
	})
}
