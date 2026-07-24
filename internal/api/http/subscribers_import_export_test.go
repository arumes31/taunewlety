package http

import (
	"bytes"
	"encoding/csv"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"taunewlety/internal/domain/models"
	"taunewlety/internal/platform/database"

	"github.com/gin-gonic/gin"
)

func TestSubscribersExport(t *testing.T) {
	os.Setenv("DB_PATH", ":memory:")
	defer os.Unsetenv("DB_PATH")

	gin.SetMode(gin.TestMode)

	t.Run("Success", func(t *testing.T) {
		db, _ := database.InitDB()
		database.GetDB().Create(&models.Subscriber{Email: "export1@example.com", Active: true})
		database.GetDB().Create(&models.Subscriber{Email: "export2@example.com", Active: true})

		w := httptest.NewRecorder()
		_, r := gin.CreateTestContext(w)
		h := &Handler{DB: db}
		r.GET("/subscribers/export", h.SubscribersExport)

		req, _ := http.NewRequest(http.MethodGet, "/subscribers/export", nil)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		contentType := w.Header().Get("Content-Type")
		if contentType != "text/csv" {
			t.Errorf("expected Content-Type text/csv, got %s", contentType)
		}

		disposition := w.Header().Get("Content-Disposition")
		if !strings.Contains(disposition, "subscribers.csv") {
			t.Errorf("expected Content-Disposition to contain subscribers.csv, got %s", disposition)
		}

		body := w.Body.String()
		if !strings.Contains(body, "email") {
			t.Error("expected CSV header with 'email'")
		}
		if !strings.Contains(body, "export1@example.com") {
			t.Error("expected CSV to contain export1@example.com")
		}
		if !strings.Contains(body, "export2@example.com") {
			t.Error("expected CSV to contain export2@example.com")
		}
	})

	t.Run("DatabaseError", func(t *testing.T) {
		db, _ := database.InitDB()
		// Drop the table to cause an error
		database.GetDB().Migrator().DropTable(&models.Subscriber{})

		w := httptest.NewRecorder()
		_, r := gin.CreateTestContext(w)
		h := &Handler{DB: db}
		r.GET("/subscribers/export", h.SubscribersExport)

		req, _ := http.NewRequest(http.MethodGet, "/subscribers/export", nil)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("expected status 500, got %d", w.Code)
		}
	})
}

func TestSubscribersImport(t *testing.T) {
	os.Setenv("DB_PATH", ":memory:")
	defer os.Unsetenv("DB_PATH")

	gin.SetMode(gin.TestMode)

	createCSVBody := func(records [][]string) (*bytes.Buffer, string) {
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		part, _ := writer.CreateFormFile("file", "subscribers.csv")
		csvWriter := csv.NewWriter(part)
		for _, record := range records {
			_ = csvWriter.Write(record)
		}
		csvWriter.Flush()
		writer.Close()
		return &buf, writer.FormDataContentType()
	}

	t.Run("Success", func(t *testing.T) {
		db, _ := database.InitDB()

		records := [][]string{
			{"email", "name", "active"},
			{"import1@example.com", "", "true"},
			{"import2@example.com", "", "true"},
		}
		buf, contentType := createCSVBody(records)

		w := httptest.NewRecorder()
		_, r := gin.CreateTestContext(w)
		h := &Handler{DB: db}
		r.POST("/subscribers/import", h.SubscribersImport)

		req, _ := http.NewRequest(http.MethodPost, "/subscribers/import", buf)
		req.Header.Set("Content-Type", contentType)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusFound {
			t.Errorf("expected status 302, got %d", w.Code)
		}
		loc := w.Header().Get("Location")
		if !strings.Contains(loc, "imported=2") {
			t.Errorf("expected location to contain imported=2, got %s", loc)
		}
	})

	t.Run("SkipsDuplicates", func(t *testing.T) {
		db, _ := database.InitDB()
		// Pre-create a subscriber
		database.GetDB().Create(&models.Subscriber{Email: "existing@example.com", Active: true})

		records := [][]string{
			{"email", "name", "active"},
			{"existing@example.com", "", "true"},
			{"new@example.com", "", "true"},
		}
		buf, contentType := createCSVBody(records)

		w := httptest.NewRecorder()
		_, r := gin.CreateTestContext(w)
		h := &Handler{DB: db}
		r.POST("/subscribers/import", h.SubscribersImport)

		req, _ := http.NewRequest(http.MethodPost, "/subscribers/import", buf)
		req.Header.Set("Content-Type", contentType)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusFound {
			t.Errorf("expected status 302, got %d", w.Code)
		}
		loc := w.Header().Get("Location")
		if !strings.Contains(loc, "imported=1") {
			t.Errorf("expected location to contain imported=1, got %s", loc)
		}
		if !strings.Contains(loc, "skipped=1") {
			t.Errorf("expected location to contain skipped=1, got %s", loc)
		}
	})

	t.Run("NoFile", func(t *testing.T) {
		db, _ := database.InitDB()

		w := httptest.NewRecorder()
		_, r := gin.CreateTestContext(w)
		h := &Handler{DB: db}
		r.POST("/subscribers/import", h.SubscribersImport)

		req, _ := http.NewRequest(http.MethodPost, "/subscribers/import", nil)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", w.Code)
		}
	})

	t.Run("EmptyCSV", func(t *testing.T) {
		db, _ := database.InitDB()

		records := [][]string{
			{"email", "name", "active"},
		}
		buf, contentType := createCSVBody(records)

		w := httptest.NewRecorder()
		_, r := gin.CreateTestContext(w)
		h := &Handler{DB: db}
		r.POST("/subscribers/import", h.SubscribersImport)

		req, _ := http.NewRequest(http.MethodPost, "/subscribers/import", buf)
		req.Header.Set("Content-Type", contentType)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusFound {
			t.Errorf("expected status 302, got %d", w.Code)
		}
		loc := w.Header().Get("Location")
		if !strings.Contains(loc, "imported=0") {
			t.Errorf("expected location to contain imported=0, got %s", loc)
		}
	})

	t.Run("InvalidEmailSkipped", func(t *testing.T) {
		db, _ := database.InitDB()

		records := [][]string{
			{"email", "name", "active"},
			{"not-an-email", "", "true"},
			{"valid@example.com", "", "true"},
		}
		buf, contentType := createCSVBody(records)

		w := httptest.NewRecorder()
		_, r := gin.CreateTestContext(w)
		h := &Handler{DB: db}
		r.POST("/subscribers/import", h.SubscribersImport)

		req, _ := http.NewRequest(http.MethodPost, "/subscribers/import", buf)
		req.Header.Set("Content-Type", contentType)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusFound {
			t.Errorf("expected status 302, got %d", w.Code)
		}
		loc := w.Header().Get("Location")
		if !strings.Contains(loc, "imported=1") {
			t.Errorf("expected location to contain imported=1, got %s", loc)
		}
		if !strings.Contains(loc, "skipped=1") {
			t.Errorf("expected location to contain skipped=1, got %s", loc)
		}
	})
}
