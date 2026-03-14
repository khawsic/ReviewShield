package main

import (
	"log"
	"os"

	"reviewshield/internal/cache"
	"reviewshield/internal/database"
	"reviewshield/internal/handlers"
	"reviewshield/internal/middleware"
	ws "reviewshield/internal/websocket"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/joho/godotenv"
	"time"
)

func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := c.GetHeader("Authorization")
		if tokenStr == "" {
			c.JSON(401, gin.H{"error": "No token provided"})
			c.Abort()
			return
		}
		if len(tokenStr) > 7 && tokenStr[:7] == "Bearer " {
			tokenStr = tokenStr[7:]
		}
		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			return []byte(os.Getenv("JWT_SECRET")), nil
		})
		if err != nil || !token.Valid {
			c.JSON(401, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}
		claims := token.Claims.(jwt.MapClaims)
		c.Set("business_id", int(claims["business_id"].(float64)))
		c.Next()
	}
}

func main() {
	godotenv.Load()
	database.Connect()
	cache.Connect()

	// Start WebSocket hub
	go ws.H.Run()

	if os.Getenv("ENV") == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.Default()

	// Global middleware
	r.Use(middleware.CORS())
	r.Use(middleware.SecurityHeaders())

	// Public routes with rate limiting
	public := r.Group("/api")
	public.Use(middleware.RateLimit(10, time.Minute)) // 10 req/min
	{
		public.POST("/register", handlers.Register)
		public.POST("/login",    handlers.Login)
		public.GET("/health",    func(c *gin.Context) {
			c.JSON(200, gin.H{
				"status":  "ReviewShield API ✅",
				"version": "2.0.0",
				"features": []string{
					"AI Sentiment Analysis",
					"WebSockets",
					"Redis Caching",
					"Rate Limiting",
					"Google Scraping",
				},
			})
		})
	}

	// Protected routes
	api := r.Group("/api", authMiddleware())
	api.Use(middleware.RateLimit(100, time.Minute)) // 100 req/min
	{
		api.GET("/dashboard",          handlers.GetDashboardStats)
		api.GET("/reviews",            handlers.GetReviews)
		api.POST("/reviews",           handlers.AddReview)
		api.POST("/reviews/respond",   handlers.GenerateAIResponse)
		api.GET("/ws",                 ws.ServeWS)
		api.GET("/reputation", handlers.GetReputationScore)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("🚀 ReviewShield API v2.0 running on port %s", port)
	log.Println("   Features: AI + WebSockets + Redis + Rate Limiting")
	r.Run(":" + port)
}