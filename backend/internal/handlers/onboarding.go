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

func GetOnboardingStatus(c *gin.Context) {
    businessID := c.GetInt("business_id")
    var completed bool
    var status, name, category, address, city, description string
    database.DB.QueryRow(`
        SELECT onboarding_completed, verification_status,
               COALESCE(name,''), COALESCE(category,''),
               COALESCE(address,''), COALESCE(city,''),
               COALESCE(description,'')
        FROM businesses WHERE id = $1`, businessID,
    ).Scan(&completed, &status, &name, &category, &address, &city, &description)
    c.JSON(http.StatusOK, gin.H{
        "onboarding_completed": completed,
        "verification_status":  status,
        "name":                 name,
        "category":             category,
        "address":              address,
        "city":                 city,
        "description":          description,
    })
}

func SaveOnboarding(c *gin.Context) {
    businessID := c.GetInt("business_id")

    var req struct {
        BusinessType  string `json:"business_type"`
        Category      string `json:"category"`
        Description   string `json:"description"`
        Address       string `json:"address"`
        City          string `json:"city"`
        Pincode       string `json:"pincode"`
        Latitude      string `json:"latitude"`
        Longitude     string `json:"longitude"`
        Cuisine       string `json:"cuisine"`
        OpeningHours  string `json:"opening_hours"`
        Website       string `json:"website"`
        PriceRange    string `json:"price_range"`
        SocialInstagram string `json:"social_instagram"`
        SocialFacebook  string `json:"social_facebook"`
        Phone         string `json:"phone"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
        return
    }

    if req.Category == "" || req.Address == "" || req.City == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Category, address and city are required"})
        return
    }

    var lat, lng interface{}
    if req.Latitude != "" {
        if v, err := strconv.ParseFloat(req.Latitude, 64); err == nil {
            lat = v
        }
    }
    if req.Longitude != "" {
        if v, err := strconv.ParseFloat(req.Longitude, 64); err == nil {
            lng = v
        }
    }

    _, err := database.DB.Exec(`
        UPDATE businesses SET
            business_type    = $1,
            category         = $2,
            description      = $3,
            address          = $4,
            city             = $5,
            pincode          = $6,
            latitude         = $7,
            longitude        = $8,
            cuisine          = $9,
            opening_hours    = $10,
            website          = $11,
            price_range      = $12,
            social_instagram = $13,
            social_facebook  = $14,
            phone            = $15,
            onboarding_completed = true,
            updated_at       = NOW()
        WHERE id = $16`,
        req.BusinessType, req.Category, req.Description,
        req.Address, req.City, req.Pincode,
        lat, lng, req.Cuisine, req.OpeningHours,
        req.Website, req.PriceRange,
        req.SocialInstagram, req.SocialFacebook,
        req.Phone, businessID,
    )

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save business details"})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "message": "Business details saved successfully!",
        "onboarding_completed": true,
    })
}

func UploadCoverImage(c *gin.Context) {
    businessID := c.GetInt("business_id")

    file, err := c.FormFile("cover_image")
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "No image uploaded"})
        return
    }

    ext := filepath.Ext(file.Filename)
    allowed := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true}
    if !allowed[ext] {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Only JPG, PNG or WEBP allowed"})
        return
    }

    if file.Size > 5*1024*1024 {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Image too large. Maximum 5MB"})
        return
    }

    uploadDir := "./uploads/covers"
    os.MkdirAll(uploadDir, 0755)
    filename := fmt.Sprintf("cover_%d_%d%s", businessID, time.Now().Unix(), ext)
    savePath := filepath.Join(uploadDir, filename)

    if err := c.SaveUploadedFile(file, savePath); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save image"})
        return
    }

    coverURL := "/uploads/covers/" + filename
    database.DB.Exec(`UPDATE businesses SET cover_image = $1, updated_at = NOW() WHERE id = $2`, coverURL, businessID)

    c.JSON(http.StatusOK, gin.H{"cover_image": coverURL, "message": "Cover image uploaded!"})
}
