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

func AdminLogin(c *gin.Context) {
    var req struct {
        Email    string `json:"email" binding:"required"`
        Password string `json:"password" binding:"required"`
    }
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
        return
    }
    var id int
    var name, email, password, role string
    err := database.DB.QueryRow(`
        SELECT id, name, email, password, role FROM admins WHERE email = $1`, req.Email,
    ).Scan(&id, &name, &email, &password, &role)
    if err == sql.ErrNoRows {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
        return
    }
    if err := bcrypt.CompareHashAndPassword([]byte(password), []byte(req.Password)); err != nil {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
        return
    }
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
        "admin_id": id,
        "email":    email,
        "role":     role,
        "exp":      time.Now().Add(24 * time.Hour).Unix(),
    })
    tokenString, err := token.SignedString([]byte(os.Getenv("JWT_SECRET")))
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not generate token"})
        return
    }
    c.JSON(http.StatusOK, gin.H{
        "token": tokenString,
        "admin": gin.H{"id": id, "name": name, "email": email, "role": role},
    })
}

func AdminGetStats(c *gin.Context) {
    var totalBusinesses, pendingVerification, totalUsers, pendingReviews int
    database.DB.QueryRow(`SELECT COUNT(*) FROM businesses`).Scan(&totalBusinesses)
    database.DB.QueryRow(`SELECT COUNT(*) FROM businesses WHERE verification_status = 'pending_review'`).Scan(&pendingVerification)
    database.DB.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&totalUsers)
    database.DB.QueryRow(`SELECT COUNT(*) FROM user_reviews WHERE is_approved = false`).Scan(&pendingReviews)
    c.JSON(http.StatusOK, gin.H{
        "total_businesses":    totalBusinesses,
        "pending_verification": pendingVerification,
        "total_users":         totalUsers,
        "pending_reviews":     pendingReviews,
    })
}

func AdminGetPendingBusinesses(c *gin.Context) {
    rows, err := database.DB.Query(`
        SELECT id, name, email, phone, city, category,
               verification_status, onboarding_completed,
               created_at, document_url
        FROM businesses
        WHERE verification_status IN ('pending_review', 'otp_verified', 'unverified')
        ORDER BY created_at DESC
    `)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch businesses"})
        return
    }
    defer rows.Close()
    var businesses []map[string]interface{}
    for rows.Next() {
        var id int
        var name, email, status string
        var phone, city, category, docURL *string
        var onboarded bool
        var createdAt interface{}
        rows.Scan(&id, &name, &email, &phone, &city, &category, &status, &onboarded, &createdAt, &docURL)
        businesses = append(businesses, map[string]interface{}{
            "id": id, "name": name, "email": email,
            "phone": phone, "city": city, "category": category,
            "verification_status": status,
            "onboarding_completed": onboarded,
            "created_at": createdAt, "document_url": docURL,
        })
    }
    if businesses == nil {
        businesses = []map[string]interface{}{}
    }
    c.JSON(http.StatusOK, gin.H{"businesses": businesses})
}

func AdminGetAllBusinesses(c *gin.Context) {
    rows, err := database.DB.Query(`
        SELECT id, name, email, COALESCE(phone,''), COALESCE(city,''),
               COALESCE(category,''), verification_status,
               is_verified, onboarding_completed,
               COALESCE(average_rating,0), COALESCE(total_reviews,0),
               created_at
        FROM businesses
        ORDER BY created_at DESC
    `)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch businesses"})
        return
    }
    defer rows.Close()
    var businesses []map[string]interface{}
    for rows.Next() {
        var id, totalReviews int
        var name, email, phone, city, category, status string
        var isVerified, onboarded bool
        var avgRating float64
        var createdAt interface{}
        rows.Scan(&id, &name, &email, &phone, &city, &category, &status, &isVerified, &onboarded, &avgRating, &totalReviews, &createdAt)
        businesses = append(businesses, map[string]interface{}{
            "id": id, "name": name, "email": email,
            "phone": phone, "city": city, "category": category,
            "verification_status": status, "is_verified": isVerified,
            "onboarding_completed": onboarded,
            "average_rating": avgRating, "total_reviews": totalReviews,
            "created_at": createdAt,
        })
    }
    if businesses == nil {
        businesses = []map[string]interface{}{}
    }
    c.JSON(http.StatusOK, gin.H{"businesses": businesses})
}

func AdminVerifyBusiness(c *gin.Context) {
    businessID := c.Param("id")
    action := c.Query("action")
    if action == "approve" {
        database.DB.Exec(`
            UPDATE businesses SET
                is_verified = true,
                verification_status = 'verified',
                verified_at = NOW(),
                updated_at = NOW()
            WHERE id = $1`, businessID)
        c.JSON(http.StatusOK, gin.H{"message": "Business verified successfully"})
    } else if action == "reject" {
        database.DB.Exec(`
            UPDATE businesses SET
                verification_status = 'rejected',
                updated_at = NOW()
            WHERE id = $1`, businessID)
        c.JSON(http.StatusOK, gin.H{"message": "Business verification rejected"})
    } else {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid action. Use approve or reject"})
    }
}

func AdminGetPendingReviews(c *gin.Context) {
    rows, err := database.DB.Query(`
        SELECT ur.id, ur.reviewer_name, ur.rating, ur.review_text,
               ur.photo_url, ur.photo_url2, ur.photo_url3,
               ur.created_at, b.name as business_name, b.city
        FROM user_reviews ur
        JOIN businesses b ON ur.business_id = b.id
        WHERE ur.is_approved = false
        ORDER BY ur.created_at DESC
    `)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch reviews"})
        return
    }
    defer rows.Close()
    var reviews []map[string]interface{}
    for rows.Next() {
        var id, rating int
        var name, text, businessName, city string
        var photo, photo2, photo3 *string
        var createdAt interface{}
        rows.Scan(&id, &name, &rating, &text, &photo, &photo2, &photo3, &createdAt, &businessName, &city)
        reviews = append(reviews, map[string]interface{}{
            "id": id, "reviewer_name": name, "rating": rating,
            "review_text": text, "photo_url": photo,
            "photo_url2": photo2, "photo_url3": photo3,
            "created_at": createdAt,
            "business_name": businessName, "city": city,
        })
    }
    if reviews == nil {
        reviews = []map[string]interface{}{}
    }
    c.JSON(http.StatusOK, gin.H{"reviews": reviews})
}

func AdminApproveReview(c *gin.Context) {
    reviewID := c.Param("id")
    action := c.Query("action")
    if action == "approve" {
        database.DB.Exec(`UPDATE user_reviews SET is_approved = true WHERE id = $1`, reviewID)
        database.DB.Exec(`
            UPDATE businesses b SET
                total_reviews = (SELECT COUNT(*) FROM user_reviews WHERE business_id = b.id AND is_approved = true),
                average_rating = (SELECT COALESCE(AVG(rating),0) FROM user_reviews WHERE business_id = b.id AND is_approved = true)
            WHERE b.id = (SELECT business_id FROM user_reviews WHERE id = $1)
        `, reviewID)
        c.JSON(http.StatusOK, gin.H{"message": "Review approved"})
    } else {
        database.DB.Exec(`DELETE FROM user_reviews WHERE id = $1`, reviewID)
        c.JSON(http.StatusOK, gin.H{"message": "Review rejected and deleted"})
    }
}

func AdminBanBusiness(c *gin.Context) {
    businessID := c.Param("id")
    database.DB.Exec(`UPDATE businesses SET is_active = false, updated_at = NOW() WHERE id = $1`, businessID)
    c.JSON(http.StatusOK, gin.H{"message": "Business banned successfully"})
}
