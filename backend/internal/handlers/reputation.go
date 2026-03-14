package handlers

import (
    "math"
    "net/http"

    "reviewshield/internal/database"

    "github.com/gin-gonic/gin"
)

type ReputationResult struct {
    Score           int     `json:"score"`
    Grade           string  `json:"grade"`
    RatingScore     int     `json:"rating_score"`
    VolumeScore     int     `json:"volume_score"`
    SentimentScore  int     `json:"sentiment_score"`
    ResponseScore   int     `json:"response_score"`
    TotalReviews    int     `json:"total_reviews"`
    AverageRating   float64 `json:"average_rating"`
    PositivePercent float64 `json:"positive_percent"`
    ResponseRate    float64 `json:"response_rate"`
    Trend           string  `json:"trend"`
    Message         string  `json:"message"`
}

func calculateReputationScore(averageRating float64, totalReviews int, positiveReviews int, negativeReviews int, respondedReviews int) ReputationResult {
    ratingScore := int(math.Round((averageRating / 5.0) * 40))
    volumeScore := 0
    switch {
    case totalReviews >= 100:
        volumeScore = 20
    case totalReviews >= 50:
        volumeScore = 15
    case totalReviews >= 20:
        volumeScore = 12
    case totalReviews >= 10:
        volumeScore = 10
    case totalReviews >= 5:
        volumeScore = 7
    case totalReviews >= 1:
        volumeScore = 4
    }
    sentimentScore := 0
    positivePercent := 0.0
    if totalReviews > 0 {
        positivePercent = float64(positiveReviews) / float64(totalReviews) * 100
        sentimentScore = int(math.Round((positivePercent / 100.0) * 30))
    }
    responseScore := 0
    responseRate := 0.0
    if totalReviews > 0 {
        responseRate = float64(respondedReviews) / float64(totalReviews) * 100
        responseScore = int(math.Round((responseRate / 100.0) * 10))
    }
    total := ratingScore + volumeScore + sentimentScore + responseScore
    if total > 100 {
        total = 100
    }
    grade := "F"
    message := ""
    switch {
    case total >= 90:
        grade = "A+"
        message = "Outstanding reputation! Keep it up."
    case total >= 80:
        grade = "A"
        message = "Excellent reputation. Customers trust you."
    case total >= 70:
        grade = "B"
        message = "Good reputation. A few areas to improve."
    case total >= 60:
        grade = "C"
        message = "Average reputation. Focus on negative reviews."
    case total >= 50:
        grade = "D"
        message = "Needs improvement. Respond to reviews urgently."
    default:
        grade = "F"
        message = "Critical. Take immediate action on reviews."
    }
    trend := "stable"
    if positivePercent >= 70 {
        trend = "improving"
    } else if positivePercent < 40 {
        trend = "declining"
    }
    return ReputationResult{Score: total, Grade: grade, RatingScore: ratingScore, VolumeScore: volumeScore, SentimentScore: sentimentScore, ResponseScore: responseScore, TotalReviews: totalReviews, AverageRating: averageRating, PositivePercent: positivePercent, ResponseRate: responseRate, Trend: trend, Message: message}
}

func GetReputationScore(c *gin.Context) {
    businessID := c.GetInt("business_id")
    var totalReviews, positiveReviews, negativeReviews, respondedReviews int
    var averageRating float64
    row := database.DB.QueryRow(`
        SELECT COUNT(*) as total, COALESCE(AVG(rating), 0) as avg_rating,
        COUNT(CASE WHEN sentiment = 'positive' THEN 1 END) as positive,
        COUNT(CASE WHEN sentiment = 'negative' THEN 1 END) as negative,
        COUNT(CASE WHEN is_responded = true THEN 1 END) as responded
        FROM reviews WHERE business_id = $1
    `, businessID)
    err := row.Scan(&totalReviews, &averageRating, &positiveReviews, &negativeReviews, &respondedReviews)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch stats"})
        return
    }
    result := calculateReputationScore(averageRating, totalReviews, positiveReviews, negativeReviews, respondedReviews)
    c.JSON(http.StatusOK, gin.H{"reputation": result})
}
