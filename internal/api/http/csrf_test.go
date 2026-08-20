package http

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

func TestCSRFProtection_GetGeneratesToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	store := cookie.NewStore([]byte("secret"))
	r.Use(sessions.Sessions("mysession", store))
	r.Use(CSRFProtection())

	r.GET("/test", func(c *gin.Context) {
		token, _ := c.Get("csrf_token")
		c.String(http.StatusOK, "token=%v", token)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.HasPrefix(body, "token=") || len(body) <= 6 {
		t.Errorf("expected a token in body, got %q", body)
	}
}

func TestCSRFProtection_PostValidatesToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	store := cookie.NewStore([]byte("secret"))
	r.Use(sessions.Sessions("mysession", store))
	r.Use(CSRFProtection())

	r.POST("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	r.GET("/test", func(c *gin.Context) {
		token, _ := c.Get("csrf_token")
		c.String(http.StatusOK, "token=%v", token)
	})

	// First, GET to obtain a token and session cookie
	wGet := httptest.NewRecorder()
	reqGet, _ := http.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(wGet, reqGet)

	// Extract token from body
	body := wGet.Body.String()
	token := strings.TrimPrefix(body, "token=")
	if token == "" || token == "<nil>" {
		t.Fatal("failed to get CSRF token from GET response")
	}

	// POST with valid token
	wPost := httptest.NewRecorder()
	form := url.Values{}
	form.Set("csrf_token", token)
	reqPost, _ := http.NewRequest(http.MethodPost, "/test", strings.NewReader(form.Encode()))
	reqPost.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range wGet.Result().Cookies() {
		reqPost.AddCookie(c)
	}
	r.ServeHTTP(wPost, reqPost)

	if wPost.Code != http.StatusOK {
		t.Errorf("expected status 200 with valid CSRF token, got %d", wPost.Code)
	}
}

func TestCSRFProtection_PostRejectsInvalidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	store := cookie.NewStore([]byte("secret"))
	r.Use(sessions.Sessions("mysession", store))
	r.Use(CSRFProtection())

	r.POST("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	r.GET("/test", func(c *gin.Context) {
		token, _ := c.Get("csrf_token")
		c.String(http.StatusOK, "token=%v", token)
	})

	// GET to establish session
	wGet := httptest.NewRecorder()
	reqGet, _ := http.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(wGet, reqGet)

	// POST with wrong token
	wPost := httptest.NewRecorder()
	form := url.Values{}
	form.Set("csrf_token", "wrong-token")
	reqPost, _ := http.NewRequest(http.MethodPost, "/test", strings.NewReader(form.Encode()))
	reqPost.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range wGet.Result().Cookies() {
		reqPost.AddCookie(c)
	}
	r.ServeHTTP(wPost, reqPost)

	if wPost.Code != http.StatusForbidden {
		t.Errorf("expected status 403 with invalid CSRF token, got %d", wPost.Code)
	}
}

func TestCSRFProtection_PostRejectsMissingToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	store := cookie.NewStore([]byte("secret"))
	r.Use(sessions.Sessions("mysession", store))
	r.Use(CSRFProtection())

	r.POST("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	r.GET("/test", func(c *gin.Context) {
		token, _ := c.Get("csrf_token")
		c.String(http.StatusOK, "token=%v", token)
	})

	// GET to establish session
	wGet := httptest.NewRecorder()
	reqGet, _ := http.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(wGet, reqGet)

	// POST without token
	wPost := httptest.NewRecorder()
	reqPost, _ := http.NewRequest(http.MethodPost, "/test", strings.NewReader(""))
	reqPost.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range wGet.Result().Cookies() {
		reqPost.AddCookie(c)
	}
	r.ServeHTTP(wPost, reqPost)

	if wPost.Code != http.StatusForbidden {
		t.Errorf("expected status 403 with missing CSRF token, got %d", wPost.Code)
	}
}

func TestCSRFProtection_HeaderToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	store := cookie.NewStore([]byte("secret"))
	r.Use(sessions.Sessions("mysession", store))
	r.Use(CSRFProtection())

	r.POST("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	r.GET("/test", func(c *gin.Context) {
		token, _ := c.Get("csrf_token")
		c.String(http.StatusOK, "token=%v", token)
	})

	// GET to obtain token
	wGet := httptest.NewRecorder()
	reqGet, _ := http.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(wGet, reqGet)

	body := wGet.Body.String()
	token := strings.TrimPrefix(body, "token=")
	if token == "" || token == "<nil>" {
		t.Fatal("failed to get CSRF token from GET response")
	}

	// POST with X-CSRF-Token header
	wPost := httptest.NewRecorder()
	reqPost, _ := http.NewRequest(http.MethodPost, "/test", nil)
	reqPost.Header.Set("X-CSRF-Token", token)
	for _, c := range wGet.Result().Cookies() {
		reqPost.AddCookie(c)
	}
	r.ServeHTTP(wPost, reqPost)

	if wPost.Code != http.StatusOK {
		t.Errorf("expected status 200 with valid X-CSRF-Token header, got %d", wPost.Code)
	}
}

func TestCSRFProtection_NoSession(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	store := cookie.NewStore([]byte("secret"))
	r.Use(sessions.Sessions("mysession", store))
	r.Use(CSRFProtection())

	r.POST("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	// POST without any prior session
	wPost := httptest.NewRecorder()
	form := url.Values{}
	form.Set("csrf_token", "some-token")
	reqPost, _ := http.NewRequest(http.MethodPost, "/test", strings.NewReader(form.Encode()))
	reqPost.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.ServeHTTP(wPost, reqPost)

	if wPost.Code != http.StatusForbidden {
		t.Errorf("expected status 403 without session, got %d", wPost.Code)
	}
}
