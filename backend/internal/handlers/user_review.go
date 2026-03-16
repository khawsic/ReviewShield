package handlers

import (
    "fmt"
    "net/http"
    "os"
    "path/filepath"
    "strconv"
    "time"

    "reviewshield/internal/database"

    "github.com/gin-gonic/gin"
)

func SubmitUserReview(c *gin.Context) {
    businessIDStr := c.PostForm("business_id")
    reviewerName := c.PostForm("reviewer_name")
    ratingStr := c.PostForm("rating")
    reviewText := c.PostForm("review_text")
    visitDate := c.PostForm("visit_date")

    if businessIDStr == "" || reviewerName == "" || ratingStr == "" || reviewText == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Business, name, rating and review are required"})
        return
    }

    businessID, err := strconv.Atoi(businessIDStr)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
        return
    }

    rating, err := strconv.Atoi(ratingStr)
    if err != nil || rating < 1 || rating > 5 {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Rating must be between 1 and 5"})
        return
    }

    // Must upload at least one photo
    file, err := c.FormFile("photo")
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "At least one photo is required to submit a review"})
        return
    }

    // Validate file type
    ext := filepath.Ext(file.Filename)
    allowed := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true}
    if !allowed[ext] {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Only JPG, PNG or WEBP photos allowed"})
        return
    }

    // Validate file size (5MB max)
    if file.Size > 5*1024*1024 {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Photo too large. Maximum 5MB"})
        return
    }

    // Save photo
    uploadDir := "./uploads/reviews"
    os.MkdirAll(uploadDir, 0755)
    filename := fmt.Sprintf("review_%d_%d%s", businessID, time.Now().Unix(), ext)
    savePath := filepath.Join(uploadDir, filename)
    if err := c.SaveUploadedFile(file, savePath); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save photo"})
        return
    }
    photoURL := "/uploads/reviews/" + filename

    // Save optional second photo
    photoURL2 := ""
    file2, err2 := c.FormFile("photo2")
    if err2 == nil {
        ext2 := filepath.Ext(file2.Filename)
        if allowed[ext2] && file2.Size <= 5*1024*1024 {
            filename2 := fmt.Sprintf("review_%d_%d_2%s", businessID, time.Now().Unix(), ext2)
            savePath2 := filepath.Join(uploadDir, filename2)
            if c.SaveUploadedFile(file2, savePath2) == nil {
                photoURL2 = "/uploads/reviews/" + filename2
            }
        }
    }

    // Save optional third photo
    photoURL3 := ""
    file3, err3 := c.FormFile("photo3")
    if err3 == nil {
        ext3 := filepath.Ext(file3.Filename)
        if allowed[ext3] && file3.Size <= 5*1024*1024 {
            filename3 := fmt.Sprintf("review_%d_%d_3%s", businessID, time.Now().Unix(), ext3)
            savePath3 := filepath.Join(uploadDir, filename3)
            if c.SaveUploadedFile(file3, savePath3) == nil {
                photoURL3 = "/uploads/reviews/" + filename3
            }
        }
    }

    // Get user_id if logged in
    userID := 0
    authHeader := c.GetHeader("Authorization")
    if len(authHeader) > 7 {
        userID, _ = strconv.Atoi(fmt.Sprintf("%v", c.GetInt("user_id")))
    }
    _ = authHeader

    // Insert review
    var reviewID int
    var visitDateVal interface{}
    if visitDate != "" {
        visitDateVal = visitDate
    }

    err = database.DB.QueryRow(`
        INSERT INTO user_reviews
          (business_id, user_id, reviewer_name, rating, review_text, photo_url, photo_url2, photo_url3, visit_date, is_approved)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, false)
        RETURNING id`,
        businessID,
        func() interface{} {
            if userID > 0 {
                return userID
            }
            return nil
        }(),
        reviewerName, rating, reviewText, photoURL, photoURL2, photoURL3, visitDateVal,
    ).Scan(&reviewID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save review"})
        return
    }

    c.JSON(http.StatusCreated, gin.H{
        "message":   "Review submitted! It will appear after verification.",
        "review_id": reviewID,
        "photo_url": photoURL,
    })
}

func GetBusinessReviews(c *gin.Context) {
    businessIDStr := c.Param("id")
    businessID, err := strconv.Atoi(businessIDStr)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
        return
    }

    rows, err := database.DB.Query(`
        SELECT id, reviewer_name, rating, review_text,
               photo_url, photo_url2, photo_url3,
               COALESCE(visit_date::text, ''),
               created_at
        FROM user_reviews
        WHERE business_id = $1 AND is_approved = true
        ORDER BY created_at DESC
        LIMIT 50
    `, businessID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch reviews"})
        return
    }
    defer rows.Close()

    var reviews []map[string]interface{}
    for rows.Next() {
        var id, rating int
        var name, text, photo, photo2, photo3, visitDate string
        var createdAt interface{}
        rows.Scan(&id, &name, &rating, &text, &photo, &photo2, &photo3, &visitDate, &createdAt)
        reviews = append(reviews, map[string]interface{}{
            "id": id, "reviewer_name": name, "rating": rating,
            "review_text": text, "photo_url": photo,
            "photo_url2": photo2, "photo_url3": photo3,
            "visit_date": visitDate, "created_at": createdAt,
        })
    }
    if reviews == nil {
        reviews = []map[string]interface{}{}
    }
    c.JSON(http.StatusOK, gin.H{"reviews": reviews})
}
