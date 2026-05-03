package handlers

import (
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/eventpark/api/internal/auth"
	"github.com/eventpark/api/internal/middleware"
	"github.com/eventpark/api/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AuthHandler struct {
	db               *pgxpool.Pool
	jwtSecret        string
	jwtRefreshSecret string
}

func NewAuthHandler(db *pgxpool.Pool, jwtSecret, jwtRefreshSecret string) *AuthHandler {
	return &AuthHandler{db: db, jwtSecret: jwtSecret, jwtRefreshSecret: jwtRefreshSecret}
}

// POST /auth/request-otp
func (h *AuthHandler) RequestOTP(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Phone string `json:"phone"`
	}
	if err := decode(r, &body); err != nil || body.Phone == "" {
		writeErr(w, http.StatusBadRequest, "phone is required")
		return
	}

	// Generate 6-digit OTP
	code := fmt.Sprintf("%06d", rand.Intn(1000000))
	expires := time.Now().Add(10 * time.Minute)

	_, err := h.db.Exec(r.Context(),
		`INSERT INTO otp_codes (phone, code, expires_at) VALUES ($1, $2, $3)`,
		body.Phone, code, expires,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to create OTP")
		return
	}

	// In production: send via Termii/Twilio SMS
	// For development: return code in response
	resp := map[string]any{"message": "OTP sent", "expires_in": 600}
	if r.Header.Get("X-Dev-Mode") == "true" {
		resp["code"] = code // only expose in dev mode
	}
	writeJSON(w, http.StatusOK, resp)
}

// POST /auth/verify-otp
func (h *AuthHandler) VerifyOTP(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Phone string `json:"phone"`
		Code  string `json:"code"`
	}
	if err := decode(r, &body); err != nil || body.Phone == "" || body.Code == "" {
		writeErr(w, http.StatusBadRequest, "phone and code are required")
		return
	}

	// Validate OTP
	var otpID uuid.UUID
	err := h.db.QueryRow(r.Context(),
		`SELECT id FROM otp_codes
		 WHERE phone = $1 AND code = $2 AND used = false AND expires_at > NOW()
		 ORDER BY created_at DESC LIMIT 1`,
		body.Phone, body.Code,
	).Scan(&otpID)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid or expired OTP")
		return
	}

	// Mark OTP used
	_, _ = h.db.Exec(r.Context(), `UPDATE otp_codes SET used = true WHERE id = $1`, otpID)

	// Upsert user
	var user models.User
	err = h.db.QueryRow(r.Context(),
		`INSERT INTO users (phone) VALUES ($1)
		 ON CONFLICT (phone) DO UPDATE SET updated_at = NOW()
		 RETURNING id, phone, email, full_name, avatar_url, role, kyc_tier, onboarding_done, created_at, updated_at`,
		body.Phone,
	).Scan(
		&user.ID, &user.Phone, &user.Email, &user.FullName, &user.AvatarURL,
		&user.Role, &user.KYCTier, &user.OnboardingDone, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to upsert user")
		return
	}

	role := ""
	if user.Role != nil {
		role = *user.Role
	}

	accessToken, err := auth.GenerateAccessToken(user.ID, user.Phone, role, h.jwtSecret)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	refreshToken, tokenHash, err := auth.GenerateRefreshToken(user.ID, user.Phone, role, h.jwtRefreshSecret)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to generate refresh token")
		return
	}

	_, _ = h.db.Exec(r.Context(),
		`INSERT INTO refresh_tokens (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
		user.ID, tokenHash, time.Now().Add(30*24*time.Hour),
	)

	writeJSON(w, http.StatusOK, map[string]any{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"user":          user,
	})
}

// POST /auth/refresh
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := decode(r, &body); err != nil || body.RefreshToken == "" {
		writeErr(w, http.StatusBadRequest, "refresh_token is required")
		return
	}

	tokenHash := auth.HashToken(body.RefreshToken)

	var userID uuid.UUID
	var phone, role string
	err := h.db.QueryRow(r.Context(),
		`SELECT u.id, u.phone, COALESCE(u.role::text, '') FROM refresh_tokens rt
		 JOIN users u ON u.id = rt.user_id
		 WHERE rt.token_hash = $1 AND rt.expires_at > NOW()`,
		tokenHash,
	).Scan(&userID, &phone, &role)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid or expired refresh token")
		return
	}

	accessToken, err := auth.GenerateAccessToken(userID, phone, role, h.jwtSecret)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"access_token": accessToken})
}

// POST /auth/logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	authUser := middleware.GetUser(r)
	if authUser == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	_, _ = h.db.Exec(r.Context(),
		`DELETE FROM refresh_tokens WHERE user_id = $1`, authUser.ID,
	)
	writeJSON(w, http.StatusOK, map[string]string{"message": "logged out"})
}
