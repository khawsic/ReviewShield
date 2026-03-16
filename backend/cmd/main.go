package main

import (
    "log"
    "os"
    "time"

    "reviewshield/internal/cache"
    "reviewshield/internal/database"
    "reviewshield/internal/handlers"
    "reviewshield/internal/middleware"
    ws "reviewshield/internal/websocket"

    "github.com/gin-gonic/gin"
    "github.com/golang-jwt/jwt/v5"
    "github.com/joho/godotenv"
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

    go ws.H.Run()

    if os.Getenv("ENV") == "production" {
        gin.SetMode(gin.ReleaseMode)
    }

    r := gin.Default()

    r.Use(middleware.CORS())
    r.Use(middleware.SecurityHeaders())

    // Public routes
    public := r.Group("/api")
    public.Use(middleware.RateLimit(10, time.Minute))
    {
        public.POST("/register",            handlers.Register)
        public.POST("/login",               handlers.Login)
        public.POST("/user/register",       handlers.UserRegister)
        public.POST("/user/login",          handlers.UserLogin)
        public.GET("/explore",              handlers.GetNearbyBusinesses)
        public.POST("/reviews/submit",      handlers.SubmitUserReview)
        public.GET("/business/:id/reviews", handlers.GetBusinessReviews)
        public.GET("/health", func(c *gin.Context) {
            c.JSON(200, gin.H{
                "status":  "ReviewShield API",
                "version": "2.0.0",
                "features": []string{
                    "AI Sentiment Analysis",
                    "WebSockets",
                    "Redis Caching",
                    "Rate Limiting",
                    "Dual Login (Owner + User)",
                    "User Reviews with Photo Proof",
                },
            })
        })
    }

    // Protected owner routes
    api := r.Group("/api", authMiddleware())
    api.Use(middleware.RateLimit(100, time.Minute))
    {
        api.GET("/dashboard",               handlers.GetDashboardStats)
        api.GET("/reviews",                 handlers.GetReviews)
        api.POST("/reviews",                handlers.AddReview)
        api.POST("/reviews/respond",        handlers.GenerateAIResponse)
        api.GET("/ws",                      ws.ServeWS)
        api.GET("/reputation",              handlers.GetReputationScore)
        api.POST("/verify/send-otp",        handlers.SendOTP)
        api.POST("/verify/otp",             handlers.VerifyOTP)
        api.POST("/verify/upload-document", handlers.UploadDocument)
        api.GET("/verify/status",           handlers.GetVerificationStatus)
		api.GET("/onboarding/status",  handlers.GetOnboardingStatus)
api.POST("/onboarding/save",   handlers.SaveOnboarding)
api.POST("/onboarding/cover",  handlers.UploadCoverImage)
    }

    // Admin routes
    r.POST("/api/admin/verify/:id", handlers.AdminApproveVerification)

    port := os.Getenv("PORT")
    if port == "" {
        port = "8080"
    }

    log.Printf("ReviewShield API v2.0 running on port %s", port)
    log.Println("Features: AI + WebSockets + Redis + Rate Limiting + Dual Login + Photo Reviews")
    r.Run(":" + port)
}
