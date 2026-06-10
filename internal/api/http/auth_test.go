package http

import (
	"html/template"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"taunewlety/internal/platform/sanitize"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

func TestMain(m *testing.M) {
	// Change working directory to project root so templates can be loaded
	err := os.Chdir("../../..")
	if err != nil {
		log.Printf("Failed to Chdir: %v", err)
	}
	cwd, _ := os.Getwd()
	log.Printf("Current working directory in tests: %s", cwd)
	os.Exit(m.Run())
}

func TestLoginGet(t *testing.T) {
	gin.SetMode(gin.TestMode)
	os.Setenv("SESSION_SECRET", "test-secret")
	defer os.Unsetenv("SESSION_SECRET")

	r := SetupRouter()

	t.Run("renders login page with CSRF token", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/login", nil)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		body := w.Body.String()
		if !strings.Contains(body, "name=\"csrf_token\"") {
			t.Error("body does not contain csrf_token input")
		}
	})
}

func TestLoginPost(t *testing.T) {
	gin.SetMode(gin.TestMode)

	setup := func() (*gin.Engine, string, string) {
		os.Setenv("APP_USER", "admin")
		os.Setenv("APP_PASS", "password")
		os.Setenv("SESSION_SECRET", "test-secret")

		r := SetupRouter()

		// Get a valid CSRF token and cookie
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/login", nil)
		r.ServeHTTP(w, req)

		re := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`)
		matches := re.FindStringSubmatch(w.Body.String())
		csrf := ""
		if len(matches) > 1 {
			csrf = matches[1]
		}
		cookie := w.Header().Get("Set-Cookie")
		return r, csrf, cookie
	}

	tests := []struct {
		name           string
		user           string
		pass           string
		csrf           string
		envUser        string
		envPass        string
		noCookie       bool
		expectedStatus int
		expectedBody   string
		expectedLoc    string
	}{
		{
			name:           "Success",
			user:           "admin",
			pass:           "password",
			envUser:        "admin",
			envPass:        "password",
			expectedStatus: http.StatusFound,
			expectedLoc:    "/",
		},
		{
			name:           "Invalid Credentials",
			user:           "admin",
			pass:           "wrong",
			envUser:        "admin",
			envPass:        "password",
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   "Invalid credentials",
		},
		{
			name:           "Invalid CSRF",
			user:           "admin",
			pass:           "password",
			csrf:           "wrong-csrf",
			envUser:        "admin",
			envPass:        "password",
			expectedStatus: http.StatusForbidden,
			expectedBody:   "Invalid CSRF token",
		},
		{
			name:           "Missing CSRF in session",
			user:           "admin",
			pass:           "password",
			noCookie:       true,
			envUser:        "admin",
			envPass:        "password",
			expectedStatus: http.StatusForbidden,
			expectedBody:   "Invalid CSRF token",
		},
		{
			name:           "Empty CSRF input",
			user:           "admin",
			pass:           "password",
			csrf:           "",
			envUser:        "admin",
			envPass:        "password",
			expectedStatus: http.StatusForbidden,
			expectedBody:   "Invalid CSRF token",
		},
		{
			name:           "Unconfigured Server - Missing APP_USER",
			user:           "admin",
			pass:           "password",
			envUser:        "",
			envPass:        "password",
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   "Server authentication not configured",
		},
		{
			name:           "Unconfigured Server - Missing APP_PASS",
			user:           "admin",
			pass:           "password",
			envUser:        "admin",
			envPass:        "",
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   "Server authentication not configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, validCsrf, cookie := setup()

			// Override env for this test case; t.Setenv restores the prior
			// value automatically after the subtest.
			t.Setenv("APP_USER", tt.envUser)
			t.Setenv("APP_PASS", tt.envPass)

			csrf := tt.csrf
			if csrf == "" && tt.name == "Success" { // Shortcut for success case
				csrf = validCsrf
			}
			// if tt.csrf was explicitly set (even to empty string), we use it.
			// except in Success case where we want the valid one.
			if tt.csrf != "" || tt.name == "Empty CSRF input" {
				csrf = tt.csrf
			} else if tt.name != "Success" && tt.csrf == "" {
				csrf = validCsrf
			}

			form := url.Values{}
			form.Set("csrf_token", csrf)
			form.Set("username", tt.user)
			form.Set("password", tt.pass)

			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if !tt.noCookie {
				req.Header.Set("Cookie", cookie)
			}

			r.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedBody != "" && !strings.Contains(w.Body.String(), tt.expectedBody) {
				t.Errorf("expected body to contain %q, got %q", tt.expectedBody, w.Body.String())
			}

			if tt.expectedLoc != "" && w.Header().Get("Location") != tt.expectedLoc {
				t.Errorf("expected redirect to %q, got %q", tt.expectedLoc, w.Header().Get("Location"))
			}
		})
	}
}

func TestAuthHandlers_SessionSaveFailure(t *testing.T) {
	os.Setenv("APP_USER", "admin")
	os.Setenv("APP_PASS", "password")
	os.Setenv("SESSION_SECRET", "test-secret-123")
	defer func() {
		os.Unsetenv("APP_USER")
		os.Unsetenv("APP_PASS")
		os.Unsetenv("SESSION_SECRET")
	}()

	gin.SetMode(gin.TestMode)

	r := gin.New()
	store := cookie.NewStore([]byte("secret"))
	r.Use(sessions.Sessions("mysession", store))
	r.SetHTMLTemplate(template.Must(
		template.New("").Funcs(template.FuncMap{
			"sanitizeHTML": func(input string) template.HTML {
				return template.HTML(sanitize.HTML(input))
			},
		}).ParseGlob("web/template/*"),
	))

	handler := NewHandler()

	r.POST("/login_bad_save", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("csrf_token", "valid-csrf-token")
		// Force save failure by putting something unserializable
		session.Set("unserializable", make(chan int))
		_ = session.Save()
		c.Next()
	}, handler.LoginPost)

	w := httptest.NewRecorder()
	form := url.Values{}
	form.Set("csrf_token", "valid-csrf-token")
	form.Set("username", "admin")
	form.Set("password", "password")
	req, _ := http.NewRequest(http.MethodPost, "/login_bad_save", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected Internal Server Error (500), got: %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Failed to save session") {
		t.Errorf("expected 'Failed to save session' error page, got: %s", w.Body.String())
	}
}
