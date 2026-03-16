package handlers

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"net/http"
	"net/smtp"
	"os"
	"path/filepath"
	"time"

	"reviewshield/internal/database"

	"github.com/gin-gonic/gin"
)

func generateOTP() string {
	max := big.NewInt(999999)
	n, _ := rand.Int(rand.Reader, max)
	return fmt.Sprintf("%06d", n.Int64())
}

func sendOTPEmail(toEmail, businessName, otp string) error {
	smtpHost := os.Getenv("SMTP_HOST")
	smtpPort := os.Getenv("SMTP_PORT")
	smtpUser := os.Getenv("SMTP_USER")
	smtpPass := os.Getenv("SMTP_PASS")
	if smtpHost == "" {
		fmt.Printf("\n OTP for %s (%s): %s\n\n", businessName, toEmail, otp)
		return nil
	}
	auth := smtp.PlainAuth("", smtpUser, smtpPass, smtpHost)
	subject := "ReviewShield - Your Verification Code"
	body := fmt.Sprintf("Hi %s,\n\nYour verification code is: %s\n\nExpires in 10 minutes.\n\n- ReviewShield Team", businessName, otp)
	msg := fmt.Sprintf("From: ReviewShield <%s>\r\nTo: %s\r\nSubject: %s\r\n\r\n%s", smtpUser, toEmail, subject, body)
	return smtp.SendMail(smtpHost+":"+smtpPort, auth, smtpUser, []string{toEmail}, []byte(msg))
}

func SendOTP(c *gin.Context) {
	businessID := c.GetInt("business_id")
	var email, name string
	err := database.DB.QueryRow(`SELECT email, name FROM businesses WHERE id = $1`, businessID).Scan(&email, &name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Business not found"})
		return
	}
	database.DB.Exec(`UPDATE otps SET verified = true WHERE business_id = $1 AND verified = false`, businessID)
	otp := generateOTP()
	expiresAt := time.Now().Add(10 * time.Minute)
	_, err = database.DB.Exec(`INSERT INTO otps (business_id, type, otp_code, expires_at) VALUES ($1, 'email', $2, $3)`, businessID, otp, expiresAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create OTP"})
		return
	}
	if err := sendOTPEmail(email, name, otp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send OTP email"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "OTP sent to " + email, "email": email})
}

func VerifyOTP(c *gin.Context) {
	businessID := c.GetInt("business_id")
	var req struct {
		OTP string `json:"otp" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "OTP is required"})
		return
	}
	var otpID int
	err := database.DB.QueryRow(`
		SELECT id FROM otps
		WHERE business_id = $1 AND otp_code = $2 AND verified = false AND expires_at > NOW()
		ORDER BY created_at DESC LIMIT 1
	`, businessID, req.OTP).Scan(&otpID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or expired OTP"})
		return
	}
	database.DB.Exec(`UPDATE otps SET verified = true WHERE id = $1`, otpID)
	database.DB.Exec(`UPDATE businesses SET email_verified = true, verification_status = 'otp_verified', updated_at = NOW() WHERE id = $1`, businessID)
	c.JSON(http.StatusOK, gin.H{"message": "OTP verified! Now upload your business document.", "next": "upload_document"})
}

func UploadDocument(c *gin.Context) {
	businessID := c.GetInt("business_id")
	file, err := c.FormFile("document")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No document uploaded"})
		return
	}
	ext := filepath.Ext(file.Filename)
	allowed := map[string]bool{".pdf": true, ".jpg": true, ".jpeg": true, ".png": true}
	if !allowed[ext] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Only PDF, JPG, PNG files allowed"})
		return
	}
	if file.Size > 5*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File too large. Maximum 5MB allowed"})
		return
	}
	uploadDir := "./uploads/documents"
	os.MkdirAll(uploadDir, 0755)
	filename := fmt.Sprintf("business_%d_%d%s", businessID, time.Now().Unix(), ext)
	savePath := filepath.Join(uploadDir, filename)
	if err := c.SaveUploadedFile(file, savePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save document"})
		return
	}
	database.DB.Exec(`
		INSERT INTO verification_docs (business_id, doc_type, file_name, file_path, file_size, status)
		VALUES ($1, 'business_doc', $2, $3, $4, 'pending')
	`, businessID, file.Filename, savePath, file.Size)
	database.DB.Exec(`UPDATE businesses SET verification_status = 'pending_review', updated_at = NOW() WHERE id = $1`, businessID)
	c.JSON(http.StatusOK, gin.H{"message": "Document uploaded! Pending admin review.", "status": "pending_review"})
}

func GetVerificationStatus(c *gin.Context) {
	businessID := c.GetInt("business_id")
	var isVerified, emailVerified bool
	var status string
	database.DB.QueryRow(`SELECT is_verified, email_verified, verification_status FROM businesses WHERE id = $1`, businessID).Scan(&isVerified, &emailVerified, &status)
	c.JSON(http.StatusOK, gin.H{"is_verified": isVerified, "email_verified": emailVerified, "verification_status": status})
}

func AdminApproveVerification(c *gin.Context) {
	adminKey := c.GetHeader("X-Admin-Key")
	if adminKey != os.Getenv("ADMIN_SECRET_KEY") {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	businessID := c.Param("id")
	action := c.Query("action")
	if action == "approve" {
		database.DB.Exec(`UPDATE businesses SET is_verified = true, doc_verified = true, verification_status = 'verified', verified_at = NOW(), updated_at = NOW() WHERE id = $1`, businessID)
		c.JSON(http.StatusOK, gin.H{"message": "Business verified successfully"})
	} else {
		database.DB.Exec(`UPDATE businesses SET verification_status = 'rejected', updated_at = NOW() WHERE id = $1`, businessID)
		c.JSON(http.StatusOK, gin.H{"message": "Verification rejected"})
	}
}
