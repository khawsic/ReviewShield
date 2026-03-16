package handlers

import (
    "database/sql"
    "net/http"
    "os"
    "time"

    "reviewshield/internal/database"
    "reviewshield/internal/models"

    "github.com/gin-gonic/gin"
    "github.com/golang-jwt/jwt/v5"
    "golang.org/x/crypto/bcrypt"
)

func Register(c *gin.Context) {
    var req models.RegisterRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
        return
    }
    hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not hash password"})
        return
    }
    var id int
    err = database.DB.QueryRow(`
        INSERT INTO businesses (name, email, password, phone, city)
        VALUES ($1, $2, $3, $4, $5) RETURNING id`,
        req.Name, req.Email, string(hashed), req.Phone, req.City,
    ).Scan(&id)
    if err != nil {
        c.JSON(http.StatusConflict, gin.H{"error": "Email already exists"})
        return
    }
    c.JSON(http.StatusCreated, gin.H{"message": "Account created successfully!", "id": id})
}

func Login(c *gin.Context) {
    var req models.LoginRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
        return
    }
    var b models.Business
    var verificationStatus string
    var isVerified, isOnboarded bool
    err := database.DB.QueryRow(`
        SELECT id, name, email, password, phone, plan, city,
               COALESCE(verification_status, 'unverified'),
               COALESCE(is_verified, false),
               COALESCE(onboarding_completed, false)
        FROM businesses WHERE email = $1`, req.Email,
    ).Scan(&b.ID, &b.Name, &b.Email, &b.Password, &b.Phone, &b.Plan, &b.City,
        &verificationStatus, &isVerified, &isOnboarded)
    if err == sql.ErrNoRows {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
        return
    }
    if err := bcrypt.CompareHashAndPassword([]byte(b.Password), []byte(req.Password)); err != nil {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
        return
    }
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
        "business_id": b.ID,
        "email":       b.Email,
        "exp":         time.Now().Add(7 * 24 * time.Hour).Unix(),
    })
    tokenString, err := token.SignedString([]byte(os.Getenv("JWT_SECRET")))
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not generate token"})
        return
    }
    c.JSON(http.StatusOK, gin.H{
        "token": tokenString,
        "business": gin.H{
            "id":                   b.ID,
            "name":                 b.Name,
            "email":                b.Email,
            "phone":                b.Phone,
            "plan":                 b.Plan,
            "city":                 b.City,
            "verification_status":  verificationStatus,
            "is_verified":          isVerified,
            "onboarding_completed": isOnboarded,
        },
    })
}
