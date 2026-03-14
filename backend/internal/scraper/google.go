package scraper

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"reviewshield/internal/database"
	"time"
)

type GoogleReview struct {
	AuthorName string  `json:"author_name"`
	Rating     float64 `json:"rating"`
	Text       string  `json:"text"`
	Time       int64   `json:"time"`
}

type PlaceDetails struct {
	Result struct {
		Name    string         `json:"name"`
		Rating  float64        `json:"rating"`
		Reviews []GoogleReview `json:"reviews"`
	} `json:"result"`
	Status string `json:"status"`
}

// FetchGoogleReviews — fetches reviews from Google Places API
func FetchGoogleReviews(businessID int, placeID string) error {
	apiKey := os.Getenv("GOOGLE_PLACES_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("Google API key not configured")
	}

	apiURL := fmt.Sprintf(
		"https://maps.googleapis.com/maps/api/place/details/json?place_id=%s&fields=name,rating,reviews&key=%s",
		url.QueryEscape(placeID),
		apiKey,
	)

	resp, err := http.Get(apiURL)
	if err != nil {
		return fmt.Errorf("failed to fetch Google reviews: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var details PlaceDetails
	if err := json.Unmarshal(body, &details); err != nil {
		return fmt.Errorf("failed to parse response: %v", err)
	}

	if details.Status != "OK" {
		return fmt.Errorf("Google API error: %s", details.Status)
	}

	// Save each review to database
	saved := 0
	for _, review := range details.Result.Reviews {
		reviewDate := time.Unix(review.Time, 0)

		// Skip if review already exists
		var exists bool
		database.DB.QueryRow(`
			SELECT EXISTS(
				SELECT 1 FROM reviews 
				WHERE business_id=$1 
				AND reviewer_name=$2 
				AND review_date=$3
				AND platform='google'
			)`, businessID, review.AuthorName, reviewDate,
		).Scan(&exists)

		if exists {
			continue
		}

		_, err := database.DB.Exec(`
			INSERT INTO reviews 
			  (business_id, platform, reviewer_name, rating, review_text, review_date)
			VALUES ($1,$2,$3,$4,$5,$6)`,
			businessID, "google", review.AuthorName,
			review.Rating, review.Text, reviewDate,
		)
		if err != nil {
			log.Printf("Failed to save review: %v", err)
			continue
		}
		saved++
	}

	// Update scrape job timestamp
	database.DB.Exec(`
		UPDATE scrape_jobs 
		SET last_run=$1, next_run=$2, status='completed'
		WHERE business_id=$3 AND platform='google'`,
		time.Now(), time.Now().Add(6*time.Hour), businessID,
	)

	log.Printf("✅ Scraped %d new Google reviews for business %d", saved, businessID)
	return nil
}

// GetOrCreateScrapeJob — ensures a scrape job exists
func GetOrCreateScrapeJob(businessID int, platform string) {
	var exists bool
	database.DB.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM scrape_jobs 
			WHERE business_id=$1 AND platform=$2
		)`, businessID, platform,
	).Scan(&exists)

	if !exists {
		database.DB.Exec(`
			INSERT INTO scrape_jobs (business_id, platform, next_run)
			VALUES ($1, $2, $3)`,
			businessID, platform, time.Now(),
		)
	}
}