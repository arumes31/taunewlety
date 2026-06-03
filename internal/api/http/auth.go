package http

import (
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"net/http"
	"os"
)

func (h *Handler) LoginGet(c *gin.Context) {
	c.HTML(http.StatusOK, "login.html", nil)
}

func (h *Handler) LoginPost(c *gin.Context) {
	user := c.PostForm("username")
	pass := c.PostForm("password")

	if user == os.Getenv("APP_USER") && pass == os.Getenv("APP_PASS") {
		session := sessions.Default(c)
		session.Set("user", user)
		_ = session.Save()
		c.Redirect(http.StatusFound, "/")
	} else {
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{"error": "Invalid credentials"})
	}
}
