package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

type SentimentResult struct {
	Sentiment  string   `json:"sentiment"`   // positive, neutral, negative
	Score      float64  `json:"score"`       // 0.0 to 1.0
	Patterns   []string `json:"patterns"`    // ["slow_service", "cold_food"]
	Summary    string   `json:"summary"`
}

type ResponseResult struct {
	Response   string `json:"response"`
	Tone       string `json:"tone"`      // professional, empathetic, apologetic
}

// callClaude — sends a prompt to Claude API
func callClaude(prompt string) (string, error) {
	body := map[string]interface{}{
		"model":      "claude-sonnet-4-20250514",
		"max_tokens": 1024,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}

	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST",
		"https://api.anthropic.com/v1/messages",
		bytes.NewBuffer(jsonBody),
	)

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", os.Getenv("ANTHROPIC_API_KEY"))
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var result map[string]interface{}
	json.Unmarshal(respBody, &result)

	content, ok := result["content"].([]interface{})
	if !ok || len(content) == 0 {
		return "", fmt.Errorf("empty response from Claude")
	}

	text := content[0].(map[string]interface{})["text"].(string)
	return text, nil
}

// AnalyzeSentiment — uses Claude to analyze review sentiment
func AnalyzeSentiment(reviewText string, rating float64) (*SentimentResult, error) {
	prompt := fmt.Sprintf(`Analyze this customer review for a business.

Review: "%s"
Star Rating: %.1f out of 5

Respond in this exact JSON format only, no extra text:
{
  "sentiment": "positive|neutral|negative",
  "score": 0.0-1.0,
  "patterns": ["issue1", "issue2"],
  "summary": "one sentence summary"
}

For patterns, identify specific issues like: slow_service, cold_food, rude_staff, 
wrong_order, cleanliness, noise, pricing, parking, wait_time, portion_size.
Only include patterns that are clearly mentioned.`, reviewText, rating)

	response, err := callClaude(prompt)
	if err != nil {
		return fallbackSentiment(rating), nil
	}

	// Clean response - remove markdown if present
	response = strings.TrimSpace(response)
	if strings.HasPrefix(response, "```") {
		lines := strings.Split(response, "\n")
		response = strings.Join(lines[1:len(lines)-1], "\n")
	}

	var result SentimentResult
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return fallbackSentiment(rating), nil
	}

	return &result, nil
}

// GenerateResponse — uses Claude to write a professional reply
func GenerateResponse(reviewText string, sentiment string, businessName string) (*ResponseResult, error) {
	prompt := fmt.Sprintf(`You are a professional customer service manager for "%s".

Write a response to this customer review:
Review: "%s"
Sentiment: %s

Rules:
- Be professional and empathetic
- Keep it under 100 words
- Don't be defensive
- If negative, acknowledge the issue and offer to make it right
- If positive, thank them warmly
- Sound human, not robotic

Respond in this exact JSON format only:
{
  "response": "your response here",
  "tone": "empathetic|professional|apologetic|grateful"
}`, businessName, reviewText, sentiment)

	response, err := callClaude(prompt)
	if err != nil {
		return &ResponseResult{
			Response: "Thank you for your feedback! We truly value your experience and will use this to improve our service.",
			Tone:     "professional",
		}, nil
	}

	response = strings.TrimSpace(response)
	if strings.HasPrefix(response, "```") {
		lines := strings.Split(response, "\n")
		response = strings.Join(lines[1:len(lines)-1], "\n")
	}

	var result ResponseResult
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return &ResponseResult{
			Response: response,
			Tone:     "professional",
		}, nil
	}

	return &result, nil
}

// DetectPatterns — finds recurring complaints across multiple reviews
func DetectPatterns(reviews []string) ([]string, error) {
	if len(reviews) == 0 {
		return []string{}, nil
	}

	combined := strings.Join(reviews, "\n---\n")
	prompt := fmt.Sprintf(`Analyze these customer reviews and find recurring complaint patterns.

Reviews:
%s

Identify the TOP 5 most frequent issues mentioned.
Respond in this exact JSON format only:
{
  "patterns": [
    {"type": "slow_service", "frequency": 5, "severity": "high"},
    {"type": "cold_food", "frequency": 3, "severity": "medium"}
  ]
}

Use these pattern types: slow_service, cold_food, rude_staff, wrong_order, 
cleanliness, noise, pricing, wait_time, portion_size, parking, ambiance`, combined)

	response, err := callClaude(prompt)
	if err != nil {
		return []string{}, nil
	}

	response = strings.TrimSpace(response)
	if strings.HasPrefix(response, "```") {
		lines := strings.Split(response, "\n")
		response = strings.Join(lines[1:len(lines)-1], "\n")
	}

	var result struct {
		Patterns []struct {
			Type      string `json:"type"`
			Frequency int    `json:"frequency"`
			Severity  string `json:"severity"`
		} `json:"patterns"`
	}

	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return []string{}, nil
	}

	var patterns []string
	for _, p := range result.Patterns {
		patterns = append(patterns, p.Type)
	}
	return patterns, nil
}

// fallbackSentiment — used when AI is unavailable
func fallbackSentiment(rating float64) *SentimentResult {
	sentiment := "positive"
	score := 0.8
	if rating <= 2 {
		sentiment = "negative"
		score = 0.2
	} else if rating == 3 {
		sentiment = "neutral"
		score = 0.5
	}
	return &SentimentResult{
		Sentiment: sentiment,
		Score:     score,
		Patterns:  []string{},
		Summary:   "Rating-based analysis",
	}
}