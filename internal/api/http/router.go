package http

import (
	"log"
	"net/http"
	"os"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

func SetupRouter() *gin.Engine {
	r := gin.Default()

	sessionSecret := os.Getenv("SESSION_SECRET")
	if sessionSecret == "" {
		log.Fatal("SESSION_SECRET environment variable is required but was not set")
	}
	store := cookie.NewStore([]byte(sessionSecret))
	
	store.Options(sessions.Options{
		Path:     "/",
		HttpOnly: true,
		Secure:   os.Getenv("ENV") == "production" || os.Getenv("COOKIE_SECURE") == "true",
		SameSite: http.SameSiteLaxMode,
	})

	r.Use(sessions.Sessions("mysession", store))

	r.Static("/static", "web/static")
	r.StaticFile("/favicon.ico", "web/static/favicon.svg")
	r.StaticFile("/favicon.svg", "web/static/favicon.svg")

	r.LoadHTMLGlob("web/template/*")

	RegisterHandlers(r)

	return r
}
