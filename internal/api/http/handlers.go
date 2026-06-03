package http

import (
	"net/http"
	"os"
	"strconv"
	"taunewlety/internal/domain/models"
	"taunewlety/internal/platform/database"
	"taunewlety/internal/service/newsletter"
	"taunewlety/pkg"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

func RegisterHandlers(r *gin.Engine) {
	r.GET("/login", func(c *gin.Context) {
		c.HTML(http.StatusOK, "login.html", nil)
	})

	// Public Unsubscribe flow with Captcha
	r.GET("/unsubscribe", func(c *gin.Context) {
		email := c.Query("email")
		captcha := pkg.GenerateCaptcha()
		
		session := sessions.Default(c)
		session.Set("captcha_answer", captcha.Answer)
		_ = session.Save()

		c.HTML(http.StatusOK, "unsubscribe.html", gin.H{
			"email":    email,
			"question": captcha.Question,
		})
	})

	r.POST("/unsubscribe", func(c *gin.Context) {
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
	})

	r.POST("/login", func(c *gin.Context) {
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
	})

	authorized := r.Group("/")
	authorized.Use(AuthRequired())
	{
		authorized.GET("/", func(c *gin.Context) {
			config, _ := database.GetConfig()
			var subscribers []models.Subscriber
			database.DB.Find(&subscribers)
			var totalTokens int64
			database.DB.Model(&models.TokenUsage{}).Select("sum(total_tokens)").Row().Scan(&totalTokens)
			c.HTML(http.StatusOK, "index.html", gin.H{
				"config":      config,
				"subscribers": subscribers,
				"totalTokens": totalTokens,
			})
		})

		authorized.POST("/subscribers", func(c *gin.Context) {
			email := c.PostForm("email")
			if email != "" {
				database.DB.Create(&models.Subscriber{Email: email})
			}
			c.Redirect(http.StatusFound, "/")
		})

		authorized.POST("/subscribers/delete", func(c *gin.Context) {
			id := c.PostForm("id")
			database.DB.Delete(&models.Subscriber{}, id)
			c.Redirect(http.StatusFound, "/")
		})

		authorized.POST("/settings", func(c *gin.Context) {
			var config models.Config
			if err := c.ShouldBind(&config); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			_ = database.SaveConfig(&config)
			c.Redirect(http.StatusFound, "/")
		})

		authorized.GET("/preview", func(c *gin.Context) {
			config, _ := database.GetConfig()
			if config == nil {
				c.String(http.StatusBadRequest, "Configure settings first")
				return
			}
			svc := newsletter.NewNewsletterService(config)
			subject, body, err := svc.GenerateNewsletter()
			if err != nil {
				c.String(http.StatusInternalServerError, err.Error())
				return
			}
			c.HTML(http.StatusOK, "preview.html", gin.H{"subject": subject, "content": body})
		})

		authorized.POST("/send", func(c *gin.Context) {
			config, _ := database.GetConfig()
			svc := newsletter.NewNewsletterService(config)
			subject, body, err := svc.GenerateNewsletter()
			if err != nil {
				if err == newsletter.ErrNoRecommendations {
					c.JSON(http.StatusOK, gin.H{"status": "Skipped", "message": "No recommendations found"})
				} else {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				}
				return
			}
			err = svc.SendEmail(os.Getenv("NOTIFY_EMAIL"), subject, body)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"status": "Sent"})
		})
	}
}

func AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		user := session.Get("user")
		if user == nil {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}
		c.Next()
	}
}
