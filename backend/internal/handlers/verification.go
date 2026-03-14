package handlers

import (
	"crypto/rand"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/smtp"
	"os"
	"path/filepath"
	"time"

	"reviewshield/internal/database"

	"github.com/gin-gonic/gin"
)

// ── Helpers ──────────────────────────────────────────────

func generateOTP() string {
	digits := "0123456789"
	otp := make([]byte, 6)
	for i := range otp {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
		otp[i] = digits[n.Int64()]
	}
	return string(otp)
}

func sendEmailOTP(toEmail, businessName, otp string) error {
	smtpHost := os.Getenv("SMTP_HOST")
	smtpPort := os.Getenv("SMTP_PORT")
	smtpUser := os.Getenv("SMTP_USER")
	smtpPass := os.Getenv("SMTP_PASS")

	if smtpHost == "" {
		// Dev mode — just print OTP
		fmt.Printf("📧 DEV MODE — OTP for %s: %s\n", toEmail, otp)
		return nil
	}

	auth := smtp.PlainAuth("", smtpUser, smtpPass, smtpHost)
	subject := "ReviewShield — Your Verification Code"
	body := fmt.Sprintf(`
Hello %s,

Your ReviewShield email verification code is:

  %s

This code expires in 10 minutes.

If you did not request this, please ignore this email.

— ReviewShield Team
`, businessName, otp)

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s",
		smtpUser, toEmail, subject, body)

	return smtp.SendMail(smtpHost+":"+smtpPort, auth, smtpUser, []string{toEmail}, []byte(msg))
}

// ── Send Email OTP — POST /api/verify/email/send ─────────

func SendEmailOTP(c *gin.Context) {
	businessID := c.GetInt("business_id")

	// Get business email
	var email, name string
	err := database.DB.QueryRow(`SELECT email, name FROM businesses WHERE id = $1`, businessID).
		Scan(&email, &name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Business not found"})
		return
	}

	// Check if already verified
	var emailVerified bool
	database.DB.QueryRow(`SELECT email_verified FROM businesses WHERE id = $1`, businessID).
		Scan(&emailVerified)
	if emailVerified {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Email already verified"})
		return
	}

	// Delete old OTPs
	database.DB.Exec(`DELETE FROM otps WHERE business_id = $1 AND type = 'email'`, businessID)

	// Generate and save OTP
	otp := generateOTP()
	expiresAt := time.Now().Add(10 * time.Minute)
	_, err = database.DB.Exec(`
		INSERT INTO otps (business_id, type, otp_code, expires_at)
		VALUES ($1, 'email', $2, $3)
	`, businessID, otp, expiresAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create OTP"})
		return
	}

	// Send email
	if err := sendEmailOTP(email, name, otp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send email"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": fmt.Sprintf("OTP sent to %s", maskEmail(email)),
		"masked":  maskEmail(email),
	})
}

// ── Verify Email OTP — POST /api/verify/email/confirm ────

func VerifyEmailOTP(c *gin.Context) {
	businessID := c.GetInt("business_id")
	var req struct {
		OTP string `json:"otp" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "OTP is required"})
		return
	}

	// Check OTP
	var otpID int
	var expiresAt time.Time
	err := database.DB.QueryRow(`
		SELECT id, expires_at FROM otps
		WHERE business_id = $1 AND type = 'email' AND otp_code = $2 AND verified = FALSE
		ORDER BY created_at DESC LIMIT 1
	`, businessID, req.OTP).Scan(&otpID, &expiresAt)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid OTP"})
		return
	}
	if time.Now().After(expiresAt) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "OTP has expired. Please request a new one."})
		return
	}

	// Mark OTP as used
	database.DB.Exec(`UPDATE otps SET verified = TRUE WHERE id = $1`, otpID)

	// Mark email as verified
	database.DB.Exec(`UPDATE businesses SET email_verified = TRUE WHERE id = $1`, businessID)

	// Update overall verification status
	updateVerificationStatus(businessID)

	c.JSON(http.StatusOK, gin.H{"message": "Email verified successfully!", "email_verified": true})
}

// ── Upload Document — POST /api/verify/document ──────────

func UploadDocument(c *gin.Context) {
	businessID := c.GetInt("business_id")

	file, header, err := c.Request.FormFile("document")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded"})
		return
	}
	defer file.Close()

	docType := c.PostForm("doc_type") // gst, shop_license, pan, other
	if docType == "" {
		docType = "other"
	}

	// Validate file size (max 5MB)
	if header.Size > 5*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File too large. Max 5MB allowed."})
		return
	}

	// Validate file type
	ext := filepath.Ext(header.Filename)
	allowed := map[string]bool{".pdf": true, ".jpg": true, ".jpeg": true, ".png": true}
	if !allowed[ext] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Only PDF, JPG, PNG files allowed"})
		return
	}

	// Save file to uploads directory
	uploadDir := "./uploads/verification"
	os.MkdirAll(uploadDir, 0755)

	fileName := fmt.Sprintf("%d_%s_%d%s", businessID, docType, time.Now().Unix(), ext)
	filePath := filepath.Join(uploadDir, fileName)

	dst, err := os.Create(filePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
		return
	}
	defer dst.Close()
	io.Copy(dst, file)

	// Save record to DB
	var docID int
	err = database.DB.QueryRow(`
		INSERT INTO verification_docs (business_id, doc_type, file_name, file_path, file_size)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, businessID, docType, header.Filename, filePath, header.Size).Scan(&docID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save document record"})
		return
	}

	// Update verification status to pending
	database.DB.Exec(`
		UPDATE businesses SET verification_status = 'pending', doc_verified = FALSE WHERE id = $1
	`, businessID)

	c.JSON(http.StatusOK, gin.H{
		"message": "Document uploaded successfully! Our team will review it within 24 hours.",
		"doc_id":  docID,
	})
}

// ── Get Verification Status — GET /api/verify/status ─────

func GetVerificationStatus(c *gin.Context) {
	businessID := c.GetInt("business_id")

	var emailVerified, phoneVerified, docVerified, isVerified bool
	var verificationStatus string
	database.DB.QueryRow(`
		SELECT email_verified, phone_verified, doc_verified, is_verified, verification_status
		FROM businesses WHERE id = $1
	`, businessID).Scan(&emailVerified, &phoneVerified, &docVerified, &isVerified, &verificationStatus)

	// Get uploaded docs
	rows, _ := database.DB.Query(`
		SELECT id, doc_type, file_name, status, admin_note, uploaded_at
		FROM verification_docs WHERE business_id = $1
		ORDER BY uploaded_at DESC
	`, businessID)
	var docs []map[string]interface{}
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var id int
			var docType, fileName, status string
			var adminNote *string
			var uploadedAt time.Time
			rows.Scan(&id, &docType, &fileName, &status, &adminNote, &uploadedAt)
			note := ""
			if adminNote != nil {
				note = *adminNote
			}
			docs = append(docs, map[string]interface{}{
				"id": id, "doc_type": docType, "file_name": fileName,
				"status": status, "admin_note": note, "uploaded_at": uploadedAt,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"email_verified":      emailVerified,
		"phone_verified":      phoneVerified,
		"doc_verified":        docVerified,
		"is_verified":         isVerified,
		"verification_status": verificationStatus,
		"documents":           docs,
	})
}

// ── Helpers ───────────────────────────────────────────────

func maskEmail(email string) string {
	for i, c := range email {
		if c == '@' {
			if i <= 2 {
				return email[:1] + "***" + email[i:]
			}
			return email[:2] + "***" + email[i:]
		}
	}
	return email
}

func updateVerificationStatus(businessID int) {
	var emailVerified, docVerified bool
	database.DB.QueryRow(`
		SELECT email_verified, doc_verified FROM businesses WHERE id = $1
	`, businessID).Scan(&emailVerified, &docVerified)

	if emailVerified && docVerified {
		database.DB.Exec(`
			UPDATE businesses SET is_verified = TRUE, verification_status = 'verified', verified_at = NOW()
			WHERE id = $1
		`, businessID)
	} else if emailVerified {
		database.DB.Exec(`
			UPDATE businesses SET verification_status = 'pending' WHERE id = $1
		`, businessID)
	}
}
