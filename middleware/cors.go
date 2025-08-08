package middleware

import (
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	ALLOW_ORIGIN string = "ALLOW_ORIGIN"
)

var allowedOrigins = os.Getenv(ALLOW_ORIGIN)

// example env
// ALLOW_ORIGIN=https://example.com, https://app.example.com, https://admin.example.com
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")

		if allowedOrigins != "" {
			// Split multiple origins by comma and check if current origin is allowed
			origins := strings.Split(allowedOrigins, ",")
			originAllowed := false
			for _, allowedOrigin := range origins {
				allowedOrigin = strings.TrimSpace(allowedOrigin)
				if allowedOrigin == origin || allowedOrigin == "*" {
					originAllowed = true
					break
				}
			}
			if originAllowed {
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			}
		} else {
			c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		}

		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Referer, User-Agent, Content-Type, Content-Length, Accept-Language, Accept-Encoding, X-CSRF-Token, Authorization, Accept, Origin, Cache-Control, X-Requested-With, Permission-Token")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, PATCH, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
