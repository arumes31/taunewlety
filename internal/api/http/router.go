package http

import (
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"taunewlety/internal/platform/sanitize"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var getwd = os.Getwd

// resolveWebDir locates the "web" asset directory so the server works
// regardless of the current working directory (project root in production,
// the package directory under `go test`). It walks up from the working
// directory looking for a "web/template" directory, falling back to "web".
func resolveWebDir() string {
	dir, err := getwd()
	if err != nil {
		return "web"
	}
	for i := 0; i < 8; i++ {
		candidate := filepath.Join(dir, "web")
		if info, err := os.Stat(filepath.Join(candidate, "template")); err == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "web"
}

// templateFuncMap returns the set of functions available in Go templates.
func templateFuncMap() template.FuncMap {
	return template.FuncMap{
		"sanitizeHTML": func(input string) template.HTML {
			// #nosec G203
			return template.HTML(sanitize.HTML(input))
		},
		"iteratePages": func(totalPages int) []int {
			pages := make([]int, totalPages)
			for i := range pages {
				pages[i] = i + 1
			}
			return pages
		},
		"add": func(a, b int) int {
			return a + b
		},
		"sub": func(a, b int) int {
			return a - b
		},
		"formatTime": func(unix int64) string {
			if unix == 0 {
				return "Never"
			}
			return time.Unix(unix, 0).UTC().Format(time.RFC3339)
		},
		"seq": func(n int) []int {
			result := make([]int, n)
			for i := range result {
				result[i] = i + 1
			}
			return result
		},
	}
}

// SetupRouter creates and configures the gin Engine with all routes.
// It returns an error if required environment variables are missing,
// instead of calling log.Fatal which would bypass deferred cleanup.
// The logger parameter is used for structured HTTP request logging via
// the GinZapLogger middleware. Pass zap.NewNop() in tests to discard logs.
func SetupRouter(db *gorm.DB, logger *zap.Logger) (*gin.Engine, error) {
	r := gin.New()

	// Best practice: Set trusted proxies to nil to disable by default.
	// This prevents spoofing of client IP addresses.
	_ = r.SetTrustedProxies(nil)

	// Replace gin.Default() middleware with zap-integrated logging,
	// gin recovery, and security headers.
	r.Use(GinZapLogger(logger))
	r.Use(gin.Recovery())
	r.Use(SecurityHeaders())

	sessionSecret := os.Getenv("SESSION_SECRET")
	if sessionSecret == "" {
		return nil, fmt.Errorf("SESSION_SECRET environment variable is required but was not set")
	}
	store := cookie.NewStore([]byte(sessionSecret))

	store.Options(sessions.Options{
		Path:     "/",
		HttpOnly: true,
		Secure:   os.Getenv("ENV") == "production" || os.Getenv("COOKIE_SECURE") == "true",
		SameSite: http.SameSiteStrictMode,
	})

	r.Use(sessions.Sessions("mysession", store))

	webDir := resolveWebDir()
	staticDir := filepath.Join(webDir, "static")
	r.Static("/static", staticDir)
	r.StaticFile("/favicon.ico", filepath.Join(staticDir, "favicon.svg"))
	r.StaticFile("/favicon.svg", filepath.Join(staticDir, "favicon.svg"))

	r.SetHTMLTemplate(template.Must(
		template.New("").Funcs(templateFuncMap()).ParseGlob(filepath.Join(webDir, "template", "*")),
	))

	RegisterHandlers(r, db)

	return r, nil
}
