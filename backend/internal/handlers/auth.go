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

// Register — creates a new business account
func Register(c *gin.Context) {
	var req models.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}

	// Hash the password
	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not hash password"})
		return
	}

	// Insert into database
	var id int
	err = database.DB.QueryRow(`
		INSERT INTO businesses (name, email, password, phone, city)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`,
		req.Name, req.Email, string(hashed), req.Phone, req.City,
	).Scan(&id)

	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Email already exists"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Account created successfully!",
		"id":      id,
	})
}

// Login — returns a JWT token
func Login(c *gin.Context) {
	var req models.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}

	// Find business by email
	var b models.Business
	err := database.DB.QueryRow(`
		SELECT id, name, email, password, phone, plan, city
		FROM businesses WHERE email = $1`, req.Email,
	).Scan(&b.ID, &b.Name, &b.Email, &b.Password, &b.Phone, &b.Plan, &b.City)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
		return
	}

	// Check password
	err = bcrypt.CompareHashAndPassword([]byte(b.Password), []byte(req.Password))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
		return
	}

	// Generate JWT token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"business_id": b.ID,
		"email":       b.Email,
		"exp":         time.Now().Add(7 * 24 * time.Hour).Unix(), // 7 days
	})

	tokenString, err := token.SignedString([]byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":    tokenString,
		"business": b,
	})
}