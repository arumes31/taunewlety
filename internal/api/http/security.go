package http

import "github.com/gin-gonic/gin"

// contentSecurityPolicy is deliberately strict about scripts: the app renders
// LLM-generated HTML in the newsletter preview, so an injected <script> must
// never execute. All dashboard JavaScript lives in /static/js, which means
// script-src needs no 'unsafe-inline'. Inline styles are still permitted
// because the templates carry style attributes throughout.
const contentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: https:; connect-src 'self'; object-src 'none'; base-uri 'self'; form-action 'self'"

// SecurityHeaders returns a middleware that sets common security headers
// on every response, including Content-Security-Policy which is critical
// since the app renders user-generated HTML from the LLM.
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Content-Security-Policy", contentSecurityPolicy)
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")

		// HSTS is only meaningful — and only honoured by browsers — over a
		// TLS connection, whether terminated here or at a reverse proxy.
		if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		c.Next()
	}
}
