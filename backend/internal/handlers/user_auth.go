package handlers

import (
    "database/sql"
    "net/http"
    "os"
    "time"

    "reviewshield/internal/database"

    "github.com/gin-gonic/gin"
    "github.com/golang-jwt/jwt/v5"
    "golang.org/x/crypto/bcrypt"
)

func UserRegister(c *gin.Context) {
    var req struct {
        Name     string `json:"name" binding:"required"`
        Email    string `json:"email" binding:"required"`
        Password string `json:"password" binding:"required"`
        Phone    string `json:"phone"`
        City     string `json:"city"`
    }
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Name, email and password are required"})
        return
    }
    hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not hash password"})
        return
    }
    var id int
    err = database.DB.QueryRow(`
        INSERT INTO users (name, email, password, phone, city)
        VALUES ($1, $2, $3, $4, $5) RETURNING id`,
        req.Name, req.Email, string(hashed), req.Phone, req.City,
    ).Scan(&id)
    if err != nil {
        c.JSON(http.StatusConflict, gin.H{"error": "Email already exists"})
        return
    }
    c.JSON(http.StatusCreated, gin.H{"message": "Account created! Please sign in.", "id": id})
}

func UserLogin(c *gin.Context) {
    var req struct {
        Email    string `json:"email" binding:"required"`
        Password string `json:"password" binding:"required"`
    }
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
        return
    }
    var id int
    var name, email, password, phone, city string
    err := database.DB.QueryRow(`
        SELECT id, name, email, password, COALESCE(phone,''), COALESCE(city,'')
        FROM users WHERE email = $1`, req.Email,
    ).Scan(&id, &name, &email, &password, &phone, &city)
    if err == sql.ErrNoRows {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
        return
    }
    if err := bcrypt.CompareHashAndPassword([]byte(password), []byte(req.Password)); err != nil {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
        return
    }
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
        "user_id": id,
        "email":   email,
        "role":    "user",
        "exp":     time.Now().Add(7 * 24 * time.Hour).Unix(),
    })
    tokenString, err := token.SignedString([]byte(os.Getenv("JWT_SECRET")))
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not generate token"})
        return
    }
    c.JSON(http.StatusOK, gin.H{
        "token": tokenString,
        "user": gin.H{
            "id": id, "name": name, "email": email,
            "phone": phone, "city": city, "role": "user",
        },
    })
}

func GetNearbyBusinesses(c *gin.Context) {
    lat := c.Query("lat")
    lng := c.Query("lng")
    category := c.Query("category")
    search := c.Query("search")

    query := `
        SELECT id, name, COALESCE(description,''), COALESCE(address,''),
               COALESCE(city,''), COALESCE(category,''), COALESCE(phone,''),
               COALESCE(price_range,''), COALESCE(cover_image,''),
               COALESCE(average_rating, 0), COALESCE(total_reviews, 0),
               is_verified,
               CASE WHEN $1 != '' AND $2 != ''
                    THEN ROUND(CAST(
                         6371 * acos(
                           cos(radians(CAST($1 AS float))) *
                           cos(radians(CAST(latitude AS float))) *
                           cos(radians(CAST(longitude AS float)) - radians(CAST($2 AS float))) +
                           sin(radians(CAST($1 AS float))) *
                           sin(radians(CAST(latitude AS float)))
                         ) AS numeric), 1)
                    ELSE NULL END as distance_km
        FROM businesses
        WHERE is_active = true
          AND is_verified = true
          AND ($3 = '' OR category ILIKE '%' || $3 || '%')
          AND ($4 = '' OR name ILIKE '%' || $4 || '%' OR description ILIKE '%' || $4 || '%')
        ORDER BY
          CASE WHEN $1 != '' AND $2 != '' AND latitude IS NOT NULL
               THEN 6371 * acos(
                      cos(radians(CAST($1 AS float))) *
                      cos(radians(CAST(latitude AS float))) *
                      cos(radians(CAST(longitude AS float)) - radians(CAST($2 AS float))) +
                      sin(radians(CAST($1 AS float))) *
                      sin(radians(CAST(latitude AS float)))
                    )
               ELSE average_rating END
        LIMIT 20`

    rows, err := database.DB.Query(query, lat, lng, category, search)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch businesses"})
        return
    }
    defer rows.Close()

    var businesses []map[string]interface{}
    for rows.Next() {
        var id, totalReviews int
        var name, desc, address, city, category, phone, priceRange, coverImage string
        var avgRating float64
        var isVerified bool
        var distanceKm *float64
        rows.Scan(&id, &name, &desc, &address, &city, &category, &phone,
            &priceRange, &coverImage, &avgRating, &totalReviews, &isVerified, &distanceKm)
        businesses = append(businesses, map[string]interface{}{
            "id": id, "name": name, "description": desc,
            "address": address, "city": city, "category": category,
            "phone": phone, "price_range": priceRange, "cover_image": coverImage,
            "average_rating": avgRating, "total_reviews": totalReviews,
            "is_verified": isVerified, "distance_km": distanceKm,
        })
    }
    if businesses == nil {
        businesses = []map[string]interface{}{}
    }
    c.JSON(http.StatusOK, gin.H{"businesses": businesses})
}
