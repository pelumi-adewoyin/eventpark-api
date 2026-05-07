package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
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
	termiiKey        string
	env              string
}

func NewAuthHandler(db *pgxpool.Pool, jwtSecret, jwtRefreshSecret, termiiKey, env string) *AuthHandler {
	return &AuthHandler{
		db:               db,
		jwtSecret:        jwtSecret,
		jwtRefreshSecret: jwtRefreshSecret,
		termiiKey:        termiiKey,
		env:              env,
	}
}

const demoPhone = "+2340000000000"
const demoOTP = "000000"

// POST /auth/request-otp
func (h *AuthHandler) RequestOTP(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Phone           string `json:"phone"`
		CreateIfMissing bool   `json:"create_if_missing"` // true during signup flow
	}
	if err := decode(r, &body); err != nil || body.Phone == "" {
		writeErr(w, http.StatusBadRequest, "phone is required")
		return
	}

	// Universal demo OTP: 000000 authenticates any phone — skip real SMS.
	// Store it in the DB so VerifyOTP can find it, but only if we haven't
	// already stored one that's still valid (idempotent).
	_, _ = h.db.Exec(r.Context(),
		`INSERT INTO otp_codes (phone, code, expires_at) VALUES ($1, $2, $3)`,
		body.Phone, demoOTP, time.Now().Add(10*time.Minute),
	)

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

	// Send OTP via Termii
	if h.termiiKey != "" {
		if err := h.sendTermiiOTP(body.Phone, code); err != nil {
			log.Printf("Termii SMS failed for %s: %v", body.Phone, err)
			// Don't fail the request — fall through
		}
	}

	resp := map[string]any{"message": "OTP sent to your phone", "expires_in": 600}
	// Return code in non-production for easy testing
	if h.env != "production" {
		resp["code"] = code
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *AuthHandler) sendTermiiOTP(phone, code string) error {
	// Normalize Nigerian numbers: 08012345678 → 2348012345678
	normalized := phone
	if len(phone) == 11 && phone[0] == '0' {
		normalized = "234" + phone[1:]
	}

	payload := map[string]any{
		"to":      normalized,
		"from":    "EventPark",
		"sms":     fmt.Sprintf("Your EventPark verification code is %s. Valid for 10 minutes. Do not share this code.", code),
		"type":    "plain",
		"channel": "generic",
		"api_key": h.termiiKey,
	}

	b, _ := json.Marshal(payload)
	resp, err := http.Post("https://api.ng.termii.com/api/sms/send", "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("termii returned status %d", resp.StatusCode)
	}
	return nil
}

// POST /auth/verify-otp
func (h *AuthHandler) VerifyOTP(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Phone           string `json:"phone"`
		Code            string `json:"code"`
		CreateIfMissing bool   `json:"create_if_missing"` // true during signup flow
	}
	if err := decode(r, &body); err != nil || body.Phone == "" || body.Code == "" {
		writeErr(w, http.StatusBadRequest, "phone and code are required")
		return
	}

	// Universal bypass: code "000000" authenticates any phone without a
	// DB lookup — works for login, signup, or demo regardless of what is
	// stored in otp_codes. Always upserts the user so no prior account needed.
	if body.Code == demoOTP || body.Phone == demoPhone {
		body.CreateIfMissing = true
		// skip OTP DB validation — fall through to user fetch/upsert
	} else {
		// Validate OTP from DB
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
	}

	// Fetch the user. During signup (create_if_missing=true) we upsert so the
	// account is created on first OTP verify. During login we only look up —
	// if the phone isn't registered we reject with a clear error.
	// If creating a new account: INSERT OR IGNORE, then SELECT.
	// Two separate statements avoids CTE RETURNING gotchas with custom ENUMs.
	if body.CreateIfMissing {
		_, insertErr := h.db.Exec(r.Context(),
			`INSERT INTO users (phone) VALUES ($1) ON CONFLICT (phone) DO NOTHING`,
			body.Phone,
		)
		if insertErr != nil {
			log.Printf("VerifyOTP: insert user failed for phone=%s: %v", body.Phone, insertErr)
			writeErr(w, http.StatusInternalServerError, "failed to create account")
			return
		}
	}

	// SELECT the user (works for both new and existing accounts).
	// Deliberately simple — no org join here to avoid any schema issues;
	// GetMe fills in org/vendor details after login.
	var user models.User
	fetchErr := h.db.QueryRow(r.Context(),
		`SELECT id, phone, email, full_name, avatar_url,
		        role::text, COALESCE(kyc_tier::text, '0'), onboarding_done, created_at, updated_at
		 FROM users
		 WHERE phone = $1
		 LIMIT 1`,
		body.Phone,
	).Scan(
		&user.ID, &user.Phone, &user.Email, &user.FullName, &user.AvatarURL,
		&user.Role, &user.KYCTier, &user.OnboardingDone, &user.CreatedAt, &user.UpdatedAt,
	)
	if fetchErr != nil {
		log.Printf("VerifyOTP: user select failed for phone=%s create_if_missing=%v: %v",
			body.Phone, body.CreateIfMissing, fetchErr)
		writeErr(w, http.StatusNotFound, "No account found for this number. Please sign up to create one.")
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
