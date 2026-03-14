package models

import "time"

type Business struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Password  string    `json:"-"`       // never send password in response
	Phone     string    `json:"phone"`
	Plan      string    `json:"plan"`
	City      string    `json:"city"`
	CreatedAt time.Time `json:"created_at"`
}

type Review struct {
	ID             int       `json:"id"`
	BusinessID     int       `json:"business_id"`
	Platform       string    `json:"platform"`
	ReviewerName   string    `json:"reviewer_name"`
	Rating         float64   `json:"rating"`
	ReviewText     string    `json:"review_text"`
	Sentiment      string    `json:"sentiment"`
	SentimentScore float64   `json:"sentiment_score"`
	IsResponded    bool      `json:"is_responded"`
	ResponseText   string    `json:"response_text"`
	ReviewDate     time.Time `json:"review_date"`
	CreatedAt      time.Time `json:"created_at"`
}

type Alert struct {
	ID         int       `json:"id"`
	BusinessID int       `json:"business_id"`
	AlertType  string    `json:"alert_type"`
	Message    string    `json:"message"`
	IsRead     bool      `json:"is_read"`
	CreatedAt  time.Time `json:"created_at"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type RegisterRequest struct {
	Name     string `json:"name" binding:"required"`
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
	Phone    string `json:"phone"`
	City     string `json:"city"`
}