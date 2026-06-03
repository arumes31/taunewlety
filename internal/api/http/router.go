package http

import (
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"os"
)

func SetupRouter() *gin.Engine {
	r := gin.Default()

	sessionSecret := os.Getenv("SESSION_SECRET")
	if sessionSecret == "" {
		sessionSecret = "taunewlety-default-secret-2026"
	}
	store := cookie.NewStore([]byte(sessionSecret))
	r.Use(sessions.Sessions("mysession", store))

	r.Static("/static", "web/static")
	r.StaticFile("/favicon.ico", "web/static/favicon.svg")

	r.LoadHTMLGlob("web/template/*")

	RegisterHandlers(r)

	return r
}
