package http

import (
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
	"taunewlety/internal/domain/models"
	"taunewlety/internal/platform/database"
	"taunewlety/pkg"
)

func (h *Handler) UnsubscribeGet(c *gin.Context) {
	email := c.Query("email")
	captcha := pkg.GenerateCaptcha()
	
	session := sessions.Default(c)
	session.Set("captcha_answer", captcha.Answer)
	_ = session.Save()

	c.HTML(http.StatusOK, "unsubscribe.html", gin.H{
		"email":    email,
		"question": captcha.Question,
	})
}

func (h *Handler) UnsubscribePost(c *gin.Context) {
	email := c.PostForm("email")
	answerStr := c.PostForm("answer")
	answer, _ := strconv.Atoi(answerStr)

	session := sessions.Default(c)
	correctAnswer := session.Get("captcha_answer")

	if correctAnswer != nil && answer == correctAnswer.(int) {
		database.DB.Where("email = ?", email).Delete(&models.Subscriber{})
		c.String(http.StatusOK, "You have been successfully unsubscribed.")
	} else {
		c.String(http.StatusUnauthorized, "Invalid captcha answer. Please try again.")
	}
}
