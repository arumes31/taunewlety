package http

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// GinZapLogger returns a gin.HandlerFunc that logs HTTP requests using zap
// instead of the default gin.Logger which writes to stdout.
func GinZapLogger(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		duration := time.Since(start)
		logger.Info("HTTP Request",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("duration", duration),
			zap.String("client_ip", c.ClientIP()),
		)
	}
}
