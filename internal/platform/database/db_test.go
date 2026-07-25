package database

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"taunewlety/internal/domain/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestInitDB(t *testing.T) {
	os.Setenv("DB_PATH", ":memory:")
	defer os.Unsetenv("DB_PATH")

	// Backup and restore globals
	oldDB := GetDB()
	oldOpenDB := OpenDB
	oldPrintf := logPrintf
	defer func() {
		SetDB(oldDB)
		OpenDB = oldOpenDB
		logPrintf = oldPrintf
	}()

	t.Run("Success_SeedDefault", func(t *testing.T) {
		logPrintf = func(format string, v ...interface{}) {
			// Migration progress messages are expected
			msg := fmt.Sprintf(format, v...)
			if !strings.Contains(msg, "Running migration") && !strings.Contains(msg, "applied successfully") {
				t.Errorf("logPrintf called unexpectedly: %s", msg)
			}
		}
		OpenDB = func() (*gorm.DB, error) {
			return gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		}

		db, err := InitDB()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if db == nil {
			t.Fatal("DB should not be nil")
		}
		if GetDB() == nil {
			t.Fatal("global DB should not be nil after InitDB")
		}

		var count int64
		GetDB().Model(&models.Config{}).Count(&count)
		if count != 1 {
			t.Errorf("expected 1 config, got %d", count)
		}
	})

	t.Run("Success_AlreadySeeded", func(t *testing.T) {
		logPrintf = func(format string, v ...interface{}) {
			// Migration progress messages are expected
			msg := fmt.Sprintf(format, v...)
			if !strings.Contains(msg, "Running migration") && !strings.Contains(msg, "applied successfully") {
				t.Errorf("logPrintf called unexpectedly: %s", msg)
			}
		}

		sharedDSN := "file::memory:?cache=shared"
		OpenDB = func() (*gorm.DB, error) {
			return gorm.Open(sqlite.Open(sharedDSN), &gorm.Config{})
		}

		// First call seeds
		_, err := InitDB()
		if err != nil {
			t.Fatalf("first InitDB failed: %v", err)
		}
		// Second call skips seeding (same shared DB)
		_, err = InitDB()
		if err != nil {
			t.Fatalf("second InitDB failed: %v", err)
		}

		var count int64
		GetDB().Model(&models.Config{}).Count(&count)
		if count != 1 {
			t.Errorf("expected 1 config, got %d", count)
		}
	})

	t.Run("OpenDB_Error", func(t *testing.T) {
		OpenDB = func() (*gorm.DB, error) { return nil, errors.New("open error") }

		_, err := InitDB()

		if err == nil {
			t.Error("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "open error") {
			t.Errorf("expected error to contain 'open error', got %v", err)
		}
	})

	t.Run("AutoMigrate_Error", func(t *testing.T) {
		OpenDB = func() (*gorm.DB, error) {
			db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			// Close the database to make AutoMigrate fail
			sqlDB, _ := db.DB()
			_ = sqlDB.Close()
			return db, nil
		}

		_, err := InitDB()

		if err == nil {
			t.Error("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to run migrations") {
			t.Errorf("expected error about migrations, got %v", err)
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

		_, err := InitDB()
		// Count error is logged but not fatal
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
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

		_, err := InitDB()
		// Create error now causes RunMigrations to fail, which causes InitDB to return an error
		if err == nil {
			t.Fatal("expected error from InitDB, got nil")
		}
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

		_, err := InitDB()
		// RowsAffected=0 is logged but not fatal
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
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
	oldDB := GetDB()
	defer func() { SetDB(oldDB) }()

	t.Run("Success", func(t *testing.T) {
		db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		SetDB(db)
		_ = GetDB().AutoMigrate(&models.Config{})

		GetDB().Create(&models.Config{Language: "en"})

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
		SetDB(db)
		// No migration, table doesn't exist
		_, err := GetConfig()
		if err == nil {
			t.Error("expected error, got nil")
		}
	})
}

func TestSaveConfig(t *testing.T) {
	oldDB := GetDB()
	defer func() { SetDB(oldDB) }()

	t.Run("Create_Success", func(t *testing.T) {
		db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		SetDB(db)
		_ = GetDB().AutoMigrate(&models.Config{})
		err := SaveConfig(&models.Config{Language: "fr"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var cfg models.Config
		GetDB().First(&cfg)
		if cfg.Language != "fr" {
			t.Errorf("expected fr, got %s", cfg.Language)
		}
	})

	t.Run("Update_Success", func(t *testing.T) {
		db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		SetDB(db)
		_ = GetDB().AutoMigrate(&models.Config{})

		GetDB().Create(&models.Config{Language: "en"})

		err := SaveConfig(&models.Config{Language: "es"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var cfg models.Config
		GetDB().First(&cfg)
		if cfg.Language != "es" {
			t.Errorf("expected es, got %s", cfg.Language)
		}
		if cfg.ID != 1 {
			t.Errorf("expected ID 1, got %d", cfg.ID)
		}
	})

	t.Run("Count_Error", func(t *testing.T) {
		db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		SetDB(db)
		// No table
		err := SaveConfig(&models.Config{})
		if err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("Create_Error", func(t *testing.T) {
		db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		SetDB(db)
		_ = GetDB().AutoMigrate(&models.Config{})

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
		SetDB(db)
		_ = GetDB().AutoMigrate(&models.Config{})

		GetDB().Create(&models.Config{Language: "en"})

		_ = db.Callback().Update().Before("gorm:update").Register("fail", func(d *gorm.DB) {
			_ = d.AddError(errors.New("fail"))

		})

		err := SaveConfig(&models.Config{Language: "es"})
		if err == nil {
			t.Error("expected error, got nil")
		}
	})
}

func TestRunMigrations(t *testing.T) {
	oldDB := GetDB()
	defer func() { SetDB(oldDB) }()

	t.Run("CreatesMigrationVersionsTable", func(t *testing.T) {
		db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		SetDB(db)

		err := RunMigrations(db)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !db.Migrator().HasTable(&models.MigrationVersion{}) {
			t.Error("expected migration_versions table to exist")
		}
	})

	t.Run("RecordsAppliedMigrations", func(t *testing.T) {
		db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		SetDB(db)

		err := RunMigrations(db)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var count int64
		db.Model(&models.MigrationVersion{}).Count(&count)
		if count == 0 {
			t.Error("expected migration versions to be recorded")
		}
	})

	t.Run("Idempotent_RunsTwiceNoError", func(t *testing.T) {
		db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		SetDB(db)

		err := RunMigrations(db)
		if err != nil {
			t.Fatalf("first run failed: %v", err)
		}

		err = RunMigrations(db)
		if err != nil {
			t.Fatalf("second run failed: %v", err)
		}

		// Should not duplicate migration records
		var count int64
		db.Model(&models.MigrationVersion{}).Where("version = ?", 0).Count(&count)
		if count != 1 {
			t.Errorf("expected 1 record for migration 0, got %d", count)
		}
	})

	t.Run("MigrationFailure", func(t *testing.T) {
		db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		SetDB(db)

		// Save and restore the original migrations
		originalMigrations := migrations
		defer func() { migrations = originalMigrations }()

		migrations = []migrationDef{
			{
				Version: 0,
				Name:    "baseline",
				Up: func(db *gorm.DB) error {
					return db.AutoMigrate(&models.Config{})
				},
			},
			{
				Version: 99,
				Name:    "failing_migration",
				Up: func(db *gorm.DB) error {
					return errors.New("intentional failure")
				},
			},
		}

		err := RunMigrations(db)
		if err == nil {
			t.Error("expected error from failing migration, got nil")
		}
		if !strings.Contains(err.Error(), "intentional failure") {
			t.Errorf("expected error to contain 'intentional failure', got %v", err)
		}
	})
}

func TestEscapeDSNValue(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "Plain value", input: "taunewlety", want: `'taunewlety'`},
		{name: "Empty value", input: "", want: `''`},
		{name: "Value with spaces", input: "my db name", want: `'my db name'`},
		{name: "Single quote is backslash escaped", input: "pa'ss", want: `'pa\'ss'`},
		{name: "Backslash is doubled", input: `pa\ss`, want: `'pa\\ss'`},
		{name: "Backslash before quote", input: `pa\'ss`, want: `'pa\\\'ss'`},
		{name: "Newline preserved verbatim", input: "pa\nss", want: "'pa\nss'"},
		{name: "Carriage return preserved verbatim", input: "pa\rss", want: "'pa\rss'"},
		{name: "Quote injection attempt", input: "x' host='evil", want: `'x\' host=\'evil'`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := escapeDSNValue(tt.input); got != tt.want {
				t.Errorf("escapeDSNValue(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
