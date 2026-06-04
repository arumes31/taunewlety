package http

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

var (
	getwd    = os.Getwd
	logFatal = log.Fatal
)

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

func SetupRouter() *gin.Engine {
	r := gin.Default()

	// Best practice: Set trusted proxies to nil to disable by default.
	// This prevents spoofing of client IP addresses.
	_ = r.SetTrustedProxies(nil)

	sessionSecret := os.Getenv("SESSION_SECRET")
	if sessionSecret == "" {
		logFatal("SESSION_SECRET environment variable is required but was not set")
		return nil
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

	r.LoadHTMLGlob(filepath.Join(webDir, "template", "*"))

	RegisterHandlers(r)

	return r
}
