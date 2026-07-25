package http

import (
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"taunewlety/internal/domain/models"
	"taunewlety/internal/platform/database"

	"github.com/gin-gonic/gin"
)

func importCSV(t *testing.T, h *Handler, rows []string) *httptest.ResponseRecorder {
	t.Helper()

	var buf strings.Builder
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", "subs.csv")
	if err != nil {
		t.Fatalf("CreateFormFile failed: %v", err)
	}
	if _, err := part.Write([]byte(strings.Join(rows, "\n"))); err != nil {
		t.Fatalf("writing CSV body failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("closing multipart writer failed: %v", err)
	}

	r := gin.New()
	r.POST("/subscribers/import", h.SubscribersImport)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/subscribers/import", strings.NewReader(buf.String()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	r.ServeHTTP(w, req)
	return w
}

// TestSubscribersImport_NormalizesAddress verifies a row written with a
// display name is stored as the bare address, so it matches the duplicate
// check and produces a usable RCPT TO.
func TestSubscribersImport_NormalizesAddress(t *testing.T) {
	os.Setenv("DB_PATH", ":memory:")
	defer os.Unsetenv("DB_PATH")
	gin.SetMode(gin.TestMode)

	db, err := database.InitDB()
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	db.Exec("DELETE FROM subscribers")

	w := importCSV(t, NewHandler(db), []string{
		"email,name,active",
		"Bob Example <bob@example.com>,,true",
		"<carol@example.com>,,true",
	})
	if w.Code != http.StatusFound {
		t.Fatalf("expected a redirect after import, got %d: %s", w.Code, w.Body.String())
	}

	var stored []models.Subscriber
	db.Order("id ASC").Find(&stored)
	if len(stored) != 2 {
		t.Fatalf("expected 2 subscribers, got %d", len(stored))
	}
	want := []string{"bob@example.com", "carol@example.com"}
	for i, sub := range stored {
		if sub.Email != want[i] {
			t.Errorf("stored email %d = %q, want the bare address %q", i, sub.Email, want[i])
		}
	}
}

// TestSubscribersImport_DisplayNameIsADuplicate verifies the normalized form
// is what the duplicate check compares, so the same person cannot be imported
// twice under two spellings.
func TestSubscribersImport_DisplayNameIsADuplicate(t *testing.T) {
	os.Setenv("DB_PATH", ":memory:")
	defer os.Unsetenv("DB_PATH")
	gin.SetMode(gin.TestMode)

	db, err := database.InitDB()
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	db.Exec("DELETE FROM subscribers")

	w := importCSV(t, NewHandler(db), []string{
		"email,name,active",
		"bob@example.com,,true",
		"Bob Example <bob@example.com>,,true",
	})
	if w.Code != http.StatusFound {
		t.Fatalf("expected a redirect after import, got %d: %s", w.Code, w.Body.String())
	}

	location := w.Header().Get("Location")
	if !strings.Contains(location, "imported=1") || !strings.Contains(location, "skipped=1") {
		t.Errorf("expected the display-name row to be skipped as a duplicate, redirect was %q", location)
	}

	var count int64
	db.Model(&models.Subscriber{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 stored subscriber, got %d", count)
	}
}

// TestSubscriberAdd_NormalizesAddress covers the form path, which must
// normalize identically to the import path — otherwise the same person could
// be stored twice through two different entry points.
func TestSubscriberAdd_NormalizesAddress(t *testing.T) {
	os.Setenv("DB_PATH", ":memory:")
	defer os.Unsetenv("DB_PATH")
	gin.SetMode(gin.TestMode)

	db, err := database.InitDB()
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	db.Exec("DELETE FROM subscribers")

	r := gin.New()
	r.POST("/subscribers", NewHandler(db).SubscriberAdd)

	post := func(email string) *httptest.ResponseRecorder {
		form := url.Values{}
		form.Set("email", email)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/subscribers", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.ServeHTTP(w, req)
		return w
	}

	if w := post("Bob Example <bob@example.com>"); w.Code != http.StatusFound {
		t.Fatalf("expected a redirect after add, got %d: %s", w.Code, w.Body.String())
	}

	var stored models.Subscriber
	db.First(&stored)
	if stored.Email != "bob@example.com" {
		t.Errorf("stored email = %q, want the bare address %q", stored.Email, "bob@example.com")
	}

	// The same address in bare form is now a recognizable duplicate.
	if w := post("bob@example.com"); w.Code != http.StatusConflict {
		t.Errorf("expected 409 for the same address in bare form, got %d: %s", w.Code, w.Body.String())
	}

	var count int64
	db.Model(&models.Subscriber{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 stored subscriber, got %d", count)
	}
}
