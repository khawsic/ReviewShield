package handlers

import (
	"net/http"
	"reviewshield/internal/ai"
	"reviewshield/internal/cache"
	"reviewshield/internal/database"
	"reviewshield/internal/models"
	ws "reviewshield/internal/websocket"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
)

func GetReviews(c *gin.Context) {
	businessID := c.GetInt("business_id")
	cacheKey := fmt.Sprintf("reviews:%d", businessID)

	// Try cache first
	if cached, err := cache.Get(cacheKey); err == nil {
		var reviews []models.Review
		json.Unmarshal([]byte(cached), &reviews)
		c.JSON(http.StatusOK, gin.H{"reviews": reviews, "source": "cache"})
		return
	}

	rows, err := database.DB.Query(`
		SELECT id, platform, reviewer_name, rating,
		       review_text, sentiment, sentiment_score,
		       is_responded, created_at
		FROM reviews
		WHERE business_id = $1
		ORDER BY created_at DESC
		LIMIT 50`, businessID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not fetch reviews"})
		return
	}
	defer rows.Close()

	var reviews []models.Review
	for rows.Next() {
		var r models.Review
		rows.Scan(&r.ID, &r.Platform, &r.ReviewerName, &r.Rating,
			&r.ReviewText, &r.Sentiment, &r.SentimentScore,
			&r.IsResponded, &r.CreatedAt)
		reviews = append(reviews, r)
	}

	// Cache for 5 minutes
	data, _ := json.Marshal(reviews)
	cache.Set(cacheKey, string(data), 5*time.Minute)

	c.JSON(http.StatusOK, gin.H{"reviews": reviews, "source": "db"})
}

func AddReview(c *gin.Context) {
	businessID := c.GetInt("business_id")

	var r models.Review
	if err := c.ShouldBindJSON(&r); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}

	// 🤖 Real AI sentiment analysis
	sentiment, err := ai.AnalyzeSentiment(r.ReviewText, r.Rating)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "AI analysis failed"})
		return
	}

	var id int
	err = database.DB.QueryRow(`
		INSERT INTO reviews
		  (business_id, platform, reviewer_name, rating,
		   review_text, sentiment, sentiment_score, review_date)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id`,
		businessID, r.Platform, r.ReviewerName, r.Rating,
		r.ReviewText, sentiment.Sentiment, sentiment.Score, time.Now(),
	).Scan(&id)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not save review"})
		return
	}

	// Save detected patterns
	for _, pattern := range sentiment.Patterns {
		database.DB.Exec(`
			INSERT INTO review_patterns (business_id, pattern_type, severity)
			VALUES ($1, $2, 'medium')
			ON CONFLICT DO NOTHING`,
			businessID, pattern,
		)
	}

	// Create alert for negative reviews
	if sentiment.Sentiment == "negative" {
		alertMsg := fmt.Sprintf(
			"Negative review from %s on %s: %s",
			r.ReviewerName, r.Platform, sentiment.Summary,
		)
		database.DB.Exec(`
			INSERT INTO alerts (business_id, alert_type, message)
			VALUES ($1, $2, $3)`,
			businessID, "negative_review", alertMsg,
		)

		// 🔴 Push real-time alert via WebSocket
		ws.H.Broadcast(businessID, ws.Message{
			Type: "new_alert",
			Payload: map[string]interface{}{
				"message":  alertMsg,
				"severity": "high",
			},
		})
	}

	// Push new review via WebSocket to dashboard
	ws.H.Broadcast(businessID, ws.Message{
		Type: "new_review",
		Payload: map[string]interface{}{
			"id":           id,
			"reviewer":     r.ReviewerName,
			"rating":       r.Rating,
			"sentiment":    sentiment.Sentiment,
			"platform":     r.Platform,
		},
	})

	// Invalidate cache
	cache.Delete(fmt.Sprintf("reviews:%d", businessID))

	c.JSON(http.StatusCreated, gin.H{
		"message":   "Review added",
		"id":        id,
		"sentiment": sentiment,
	})
}

func GenerateAIResponse(c *gin.Context) {
	businessID := c.GetInt("business_id")

	var req struct {
		ReviewID int    `json:"review_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}

	// Get review + business name
	var reviewText, sentiment, businessName string
	database.DB.QueryRow(`
		SELECT r.review_text, r.sentiment, b.name
		FROM reviews r
		JOIN businesses b ON b.id = r.business_id
		WHERE r.id = $1 AND r.business_id = $2`,
		req.ReviewID, businessID,
	).Scan(&reviewText, &sentiment, &businessName)

	response, err := ai.GenerateResponse(reviewText, sentiment, businessName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not generate response"})
		return
	}

	// Save to responses table
	database.DB.Exec(`
		INSERT INTO responses (review_id, suggested)
		VALUES ($1, $2)`,
		req.ReviewID, response.Response,
	)

	c.JSON(http.StatusOK, gin.H{"response": response})
}

func GetDashboardStats(c *gin.Context) {
	businessID := c.GetInt("business_id")
	cacheKey := fmt.Sprintf("stats:%d", businessID)

	// Try cache first
	if cached, err := cache.Get(cacheKey); err == nil {
		var stats map[string]interface{}
		json.Unmarshal([]byte(cached), &stats)
		c.JSON(http.StatusOK, gin.H{"stats": stats, "source": "cache"})
		return
	}

	var total, positive, neutral, negative int
	var avgRating float64

	database.DB.QueryRow(`
		SELECT COUNT(*), COALESCE(AVG(rating),0),
		       SUM(CASE WHEN sentiment='positive' THEN 1 ELSE 0 END),
		       SUM(CASE WHEN sentiment='neutral'  THEN 1 ELSE 0 END),
		       SUM(CASE WHEN sentiment='negative' THEN 1 ELSE 0 END)
		FROM reviews WHERE business_id = $1`, businessID,
	).Scan(&total, &avgRating, &positive, &neutral, &negative)

	// Get unread alerts count
	var unreadAlerts int
	database.DB.QueryRow(`
		SELECT COUNT(*) FROM alerts 
		WHERE business_id=$1 AND is_read=false`, businessID,
	).Scan(&unreadAlerts)

	// Get top patterns
	rows, _ := database.DB.Query(`
		SELECT pattern_type, frequency, severity
		FROM review_patterns
		WHERE business_id=$1
		ORDER BY frequency DESC LIMIT 5`, businessID)
	defer rows.Close()

	type Pattern struct {
		Type      string `json:"type"`
		Frequency int    `json:"frequency"`
		Severity  string `json:"severity"`
	}
	var patterns []Pattern
	for rows.Next() {
		var p Pattern
		rows.Scan(&p.Type, &p.Frequency, &p.Severity)
		patterns = append(patterns, p)
	}

	stats := map[string]interface{}{
		"total_reviews":    total,
		"average_rating":   avgRating,
		"positive_reviews": positive,
		"neutral_reviews":  neutral,
		"negative_reviews": negative,
		"unread_alerts":    unreadAlerts,
		"top_patterns":     patterns,
	}

	// Cache for 2 minutes
	data, _ := json.Marshal(stats)
	cache.Set(cacheKey, string(data), 2*time.Minute)

	c.JSON(http.StatusOK, gin.H{"stats": stats, "source": "db"})
}