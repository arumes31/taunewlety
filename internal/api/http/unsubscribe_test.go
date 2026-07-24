package http

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"taunewlety/internal/domain/models"
	"taunewlety/internal/platform/database"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type failReader struct{}

func (failReader) Read(p []byte) (n int, err error) {
	return 0, fmt.Errorf("simulated random read error")
}

func TestUnsubscribeHandlers(t *testing.T) {
	// Override sendUnsubscribeEmail to prevent goroutine from accessing
	// a stale DB when tests run in sequence.
	originalSendEmail := sendUnsubscribeEmail
	sendUnsubscribeEmail = func(email string) {}
	defer func() { sendUnsubscribeEmail = originalSendEmail }()

	os.Setenv("DB_PATH", ":memory:")
	defer os.Unsetenv("DB_PATH")

	db, err := database.InitDB()
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	os.Setenv("SESSION_SECRET", "test-secret-123")
	defer os.Unsetenv("SESSION_SECRET")

	// Pre-create a subscriber
	database.GetDB().Create(&models.Subscriber{Email: "test@example.com"})

	gin.SetMode(gin.TestMode)
	r, routerErr := SetupRouter(db, zap.NewNop())
	if routerErr != nil {
		t.Fatalf("SetupRouter failed: %v", routerErr)
	}

	// 1. GET /unsubscribe
	wGet := httptest.NewRecorder()
	reqGet, _ := http.NewRequest(http.MethodGet, "/unsubscribe?email=test@example.com", nil)
	r.ServeHTTP(wGet, reqGet)

	if wGet.Code != http.StatusOK {
		t.Fatalf("expected status 200, got: %d", wGet.Code)
	}

	bodyStr := wGet.Body.String()
	cookieVal := wGet.Header().Get("Set-Cookie")

	// Extract CSRF token
	reCSRF := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`)
	matchesCSRF := reCSRF.FindStringSubmatch(bodyStr)
	if len(matchesCSRF) < 2 {
		t.Fatal("could not find CSRF token in unsubscribe form")
	}
	csrfToken := matchesCSRF[1]

	// Extract Captcha question (e.g. "a + b = ?")
	reCaptcha := regexp.MustCompile(`class="captcha-question">(\d+\s+(?:\+|\&\#43;)\s+\d+)\s+=\s+\?`)
	matchesCaptcha := reCaptcha.FindStringSubmatch(bodyStr)
	if len(matchesCaptcha) < 2 {
		t.Fatalf("could not find captcha question in unsubscribe form, body:\n%s", bodyStr)
	}
	questionStr := strings.ReplaceAll(matchesCaptcha[1], "&#43;", "+")

	// Calculate correct answer
	var a, b int
	_, _ = fmt.Sscanf(questionStr, "%d + %d", &a, &b)
	correctAnswer := a + b

	// 2. POST /unsubscribe - Bad CSRF
	wPostBadCsrf := httptest.NewRecorder()
	formBadCsrf := url.Values{}
	formBadCsrf.Set("csrf_token", "bad-csrf")
	formBadCsrf.Set("email", "test@example.com")
	formBadCsrf.Set("answer", fmt.Sprintf("%d", correctAnswer))
	reqPostBadCsrf, _ := http.NewRequest(http.MethodPost, "/unsubscribe", strings.NewReader(formBadCsrf.Encode()))
	reqPostBadCsrf.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqPostBadCsrf.Header.Set("Cookie", cookieVal)
	r.ServeHTTP(wPostBadCsrf, reqPostBadCsrf)

	if wPostBadCsrf.Code != http.StatusForbidden {
		t.Errorf("expected Forbidden (403) on bad CSRF, got: %d", wPostBadCsrf.Code)
	}

	// 3. POST /unsubscribe - Bad Captcha
	wPostBadCaptcha := httptest.NewRecorder()
	formBadCaptcha := url.Values{}
	formBadCaptcha.Set("csrf_token", csrfToken)
	formBadCaptcha.Set("email", "test@example.com")
	formBadCaptcha.Set("answer", fmt.Sprintf("%d", correctAnswer+1))
	reqPostBadCaptcha, _ := http.NewRequest(http.MethodPost, "/unsubscribe", strings.NewReader(formBadCaptcha.Encode()))
	reqPostBadCaptcha.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqPostBadCaptcha.Header.Set("Cookie", cookieVal)
	r.ServeHTTP(wPostBadCaptcha, reqPostBadCaptcha)

	if wPostBadCreds := wPostBadCaptcha; wPostBadCreds.Code != http.StatusUnauthorized {
		t.Errorf("expected Unauthorized (401) on bad captcha, got: %d", wPostBadCreds.Code)
	}

	// 4. POST /unsubscribe - Bad answer format (non-integer)
	wPostBadFormat := httptest.NewRecorder()
	formBadFormat := url.Values{}
	formBadFormat.Set("csrf_token", csrfToken)
	formBadFormat.Set("email", "test@example.com")
	formBadFormat.Set("answer", "not-a-number")
	reqPostBadFormat, _ := http.NewRequest(http.MethodPost, "/unsubscribe", strings.NewReader(formBadFormat.Encode()))
	reqPostBadFormat.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqPostBadFormat.Header.Set("Cookie", cookieVal)
	r.ServeHTTP(wPostBadFormat, reqPostBadFormat)

	if wPostBadFormat.Code != http.StatusBadRequest {
		t.Errorf("expected BadRequest (400) on invalid integer, got: %d", wPostBadFormat.Code)
	}

	// 5. POST /unsubscribe - Success
	wPostSuccess := httptest.NewRecorder()
	formSuccess := url.Values{}
	formSuccess.Set("csrf_token", csrfToken)
	formSuccess.Set("email", "test@example.com")
	formSuccess.Set("answer", fmt.Sprintf("%d", correctAnswer))
	reqPostSuccess, _ := http.NewRequest(http.MethodPost, "/unsubscribe", strings.NewReader(formSuccess.Encode()))
	reqPostSuccess.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqPostSuccess.Header.Set("Cookie", cookieVal)
	r.ServeHTTP(wPostSuccess, reqPostSuccess)

	if wPostSuccess.Code != http.StatusOK {
		t.Errorf("expected Status OK (200), got: %d", wPostSuccess.Code)
	}
	if !strings.Contains(wPostSuccess.Body.String(), "successfully unsubscribed") {
		t.Errorf("expected success message, got: %s", wPostSuccess.Body.String())
	}

	// Verify deleted from DB
	var count int64
	database.GetDB().Model(&models.Subscriber{}).Where("email = ?", "test@example.com").Count(&count)
	if count != 0 {
		t.Error("expected subscriber to be deleted from DB")
	}

	// 6. POST /unsubscribe - Not found if email does not exist (replay/successive call with new captcha)
	// We need a new session context to test this
	wGet2 := httptest.NewRecorder()
	reqGet2, _ := http.NewRequest(http.MethodGet, "/unsubscribe?email=notfound@example.com", nil)
	r.ServeHTTP(wGet2, reqGet2)
	cookieVal2 := wGet2.Header().Get("Set-Cookie")
	bodyStr2 := wGet2.Body.String()

	matchesCSRF2 := reCSRF.FindStringSubmatch(bodyStr2)
	if len(matchesCSRF2) < 2 {
		t.Fatal("could not find CSRF token in second unsubscribe form")
	}
	csrfToken2 := matchesCSRF2[1]
	matchesCaptcha2 := reCaptcha.FindStringSubmatch(bodyStr2)
	if len(matchesCaptcha2) < 2 {
		t.Fatal("could not find captcha question in second unsubscribe form")
	}
	questionStr2 := strings.ReplaceAll(matchesCaptcha2[1], "&#43;", "+")
	_, _ = fmt.Sscanf(questionStr2, "%d + %d", &a, &b)
	correctAnswer2 := a + b

	wPostNotFound := httptest.NewRecorder()
	formNotFound := url.Values{}
	formNotFound.Set("csrf_token", csrfToken2)
	formNotFound.Set("email", "notfound@example.com")
	formNotFound.Set("answer", fmt.Sprintf("%d", correctAnswer2))
	reqPostNotFound, _ := http.NewRequest(http.MethodPost, "/unsubscribe", strings.NewReader(formNotFound.Encode()))
	reqPostNotFound.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqPostNotFound.Header.Set("Cookie", cookieVal2)
	r.ServeHTTP(wPostNotFound, reqPostNotFound)

	if wPostNotFound.Code != http.StatusNotFound {
		t.Errorf("expected NotFound (404) on non-existent email, got: %d", wPostNotFound.Code)
	}
}

func TestUnsubscribeHandlers_CaptchaGenerationError(t *testing.T) {
	// Test GenerateCaptcha error in unsubscribe route
	oldReader := rand.Reader
	rand.Reader = failReader{} // Force GenerateCaptcha to fail
	defer func() { rand.Reader = oldReader }()

	gin.SetMode(gin.TestMode)
	db, _ := database.InitDB()
	h := NewHandler(db)
	r := gin.New()
	r.GET("/unsubscribe", h.UnsubscribeGet)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/unsubscribe?email=test@example.com", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected InternalServerError on captcha generator failure, got: %d", w.Code)
	}
}

func TestUnsubscribePost_MissingCaptchaInSession(t *testing.T) {
	os.Setenv("DB_PATH", ":memory:")
	defer os.Unsetenv("DB_PATH")

	db, _ := database.InitDB()
	gin.SetMode(gin.TestMode)
	h := NewHandler(db)

	r := gin.New()
	store := cookie.NewStore([]byte("secret"))
	r.Use(sessions.Sessions("mysession", store))

	r.POST("/unsubscribe", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("csrf_token", "valid-csrf")
		session.Set("unsubscribe_email", "test@example.com")
		_ = session.Save()
		c.Next()
	}, h.UnsubscribePost)

	w := httptest.NewRecorder()
	form := url.Values{}
	form.Set("csrf_token", "valid-csrf")
	form.Set("email", "test@example.com")
	form.Set("answer", "15")
	req, _ := http.NewRequest(http.MethodPost, "/unsubscribe", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected StatusUnauthorized (401), got: %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Captcha answer not found in session") {
		t.Errorf("expected captcha answer not found message, got: %s", w.Body.String())
	}
}

func TestUnsubscribePost_CaptchaStringAndDBError(t *testing.T) {
	originalSendEmail := sendUnsubscribeEmail
	sendUnsubscribeEmail = func(email string) {}
	defer func() { sendUnsubscribeEmail = originalSendEmail }()

	os.Setenv("DB_PATH", ":memory:")
	defer os.Unsetenv("DB_PATH")

	db, _ := database.InitDB()
	database.GetDB().Create(&models.Subscriber{Email: "test@example.com"})

	gin.SetMode(gin.TestMode)
	h := NewHandler(db)

	r := gin.New()
	store := cookie.NewStore([]byte("secret"))
	r.Use(sessions.Sessions("mysession", store))

	r.POST("/unsubscribe", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("csrf_token", "valid-csrf")
		session.Set("unsubscribe_email", "test@example.com")
		if c.Query("type") == "float" {
			session.Set("captcha_answer", float64(15.0))
		} else {
			session.Set("captcha_answer", "15") // string answer
		}
		_ = session.Save()
		c.Next()
	}, h.UnsubscribePost)

	// Test 1: string answer works
	w1 := httptest.NewRecorder()
	form1 := url.Values{}
	form1.Set("csrf_token", "valid-csrf")
	form1.Set("email", "test@example.com")
	form1.Set("answer", "15")
	req1, _ := http.NewRequest(http.MethodPost, "/unsubscribe", strings.NewReader(form1.Encode()))
	req1.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Errorf("expected StatusOK (200), got: %d", w1.Code)
	}

	// Test 1.5: float64 answer works
	database.GetDB().Create(&models.Subscriber{Email: "test@example.com"})
	w15 := httptest.NewRecorder()
	req15, _ := http.NewRequest(http.MethodPost, "/unsubscribe?type=float", strings.NewReader(form1.Encode()))
	req15.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.ServeHTTP(w15, req15)

	if w15.Code != http.StatusOK {
		t.Errorf("expected StatusOK (200) on float type, got: %d", w15.Code)
	}

	// Test 2: Database delete failure
	database.GetDB().Create(&models.Subscriber{Email: "test@example.com"})
	_ = database.GetDB().Callback().Delete().Before("gorm:delete").Register("fail_subscriber_delete", func(d *gorm.DB) {
		_ = d.AddError(fmt.Errorf("simulated subscriber delete error"))
	})
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodPost, "/unsubscribe", strings.NewReader(form1.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusInternalServerError {
		t.Errorf("expected InternalServerError (500) on database delete error, got: %d", w2.Code)
	}
	if !strings.Contains(w2.Body.String(), "Failed to unsubscribe") {
		t.Errorf("expected Failed to unsubscribe error message, got: %s", w2.Body.String())
	}
}

func TestUnsubscribePost_EdgeCases(t *testing.T) {
	originalSendEmail := sendUnsubscribeEmail
	sendUnsubscribeEmail = func(email string) {}
	defer func() { sendUnsubscribeEmail = originalSendEmail }()

	os.Setenv("DB_PATH", ":memory:")
	defer os.Unsetenv("DB_PATH")

	db, _ := database.InitDB()
	gin.SetMode(gin.TestMode)
	h := NewHandler(db)

	r := gin.New()
	store := cookie.NewStore([]byte("secret"))
	r.Use(sessions.Sessions("mysession", store))

	tests := []struct {
		name           string
		setupSession   func(s sessions.Session)
		csrfInput      string
		answerInput    string
		expectedStatus int
		expectedBody   string
	}{
		{
			name: "Missing CSRF in Session",
			setupSession: func(s sessions.Session) {
				// No CSRF set
			},
			csrfInput:      "some-token",
			expectedStatus: http.StatusForbidden,
			expectedBody:   "Invalid CSRF token",
		},
		{
			name: "Invalid CSRF Type in Session",
			setupSession: func(s sessions.Session) {
				s.Set("csrf_token", 123) // Should be string
			},
			csrfInput:      "some-token",
			expectedStatus: http.StatusForbidden,
			expectedBody:   "Invalid CSRF token",
		},
		{
			name: "Unsupported Captcha Answer Type",
			setupSession: func(s sessions.Session) {
				s.Set("csrf_token", "valid-csrf")
				s.Set("unsubscribe_email", "test@example.com")
				s.Set("captcha_answer", true) // bool not supported
			},
			csrfInput:      "valid-csrf",
			answerInput:    "1",
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   "Invalid captcha answer",
		},
		{
			name: "Non-numeric Captcha Answer String in Session",
			setupSession: func(s sessions.Session) {
				s.Set("csrf_token", "valid-csrf")
				s.Set("unsubscribe_email", "test@example.com")
				s.Set("captcha_answer", "abc") // invalid numeric string
			},
			csrfInput:      "valid-csrf",
			answerInput:    "0",
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   "Invalid captcha answer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rPost := gin.New()
			rPost.Use(sessions.Sessions("mysession", store))
			rPost.POST("/unsubscribe", func(c *gin.Context) {
				session := sessions.Default(c)
				if tt.setupSession != nil {
					tt.setupSession(session)
				}
				_ = session.Save()
				c.Next()
			}, h.UnsubscribePost)

			w := httptest.NewRecorder()
			form := url.Values{}
			form.Set("csrf_token", tt.csrfInput)
			form.Set("answer", tt.answerInput)
			req, _ := http.NewRequest(http.MethodPost, "/unsubscribe", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rPost.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
			if tt.expectedBody != "" && !strings.Contains(w.Body.String(), tt.expectedBody) {
				t.Errorf("expected body to contain %q, got %q", tt.expectedBody, w.Body.String())
			}
		})
	}
}
