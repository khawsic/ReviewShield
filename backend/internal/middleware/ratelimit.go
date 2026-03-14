package middleware

import (
	"fmt"
	"net/http"
	"reviewshield/internal/cache"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimit — max requests per IP per minute
func RateLimit(maxRequests int64, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		endpoint := c.FullPath()
		key := fmt.Sprintf("rate:%s:%s", ip, endpoint)

		count, err := cache.Increment(key, window)
		if err != nil {
			c.Next()
			return
		}

		// Set headers so client knows their limit
		c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", maxRequests))
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", maxRequests-count))

		if count > maxRequests {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "Too many requests",
				"retry_after": window.Seconds(),
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

// SecurityHeaders — adds security headers to every response
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Next()
	}
}

// CORS middleware
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type,Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}