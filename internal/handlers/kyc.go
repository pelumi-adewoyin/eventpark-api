package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/eventpark/api/internal/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type KYCHandler struct {
	db             *pgxpool.Pool
	dojahAppID     string
	dojahPrivKey   string
}

func NewKYCHandler(db *pgxpool.Pool, dojahAppID, dojahPrivKey string) *KYCHandler {
	return &KYCHandler{db: db, dojahAppID: dojahAppID, dojahPrivKey: dojahPrivKey}
}

// GET /kyc/status
func (h *KYCHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var kycTier, status string
	err := h.db.QueryRow(r.Context(),
		`SELECT u.kyc_tier, COALESCE(kv.status::text, 'none')
		 FROM users u
		 LEFT JOIN kyc_verifications kv ON kv.user_id = u.id
		 WHERE u.id = $1
		 ORDER BY kv.created_at DESC LIMIT 1`,
		u.ID,
	).Scan(&kycTier, &status)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to get KYC status")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"kyc_tier": kycTier,
		"status":   status,
	})
}

// POST /kyc/verify-bvn — Tier 1 verification
func (h *KYCHandler) VerifyBVN(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var body struct {
		BVN       string `json:"bvn"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
		DOB       string `json:"dob"` // YYYY-MM-DD
	}
	if err := decode(r, &body); err != nil || body.BVN == "" {
		writeErr(w, http.StatusBadRequest, "bvn is required")
		return
	}

	// Hash BVN for storage (never store raw)
	bvnHash, _ := bcrypt.GenerateFromPassword([]byte(body.BVN), 10)

	// Record verification attempt
	var kycID string
	_ = h.db.QueryRow(r.Context(),
		`INSERT INTO kyc_verifications (user_id, tier, status, bvn_hash)
		 VALUES ($1, '1', 'processing', $2)
		 RETURNING id`,
		u.ID, string(bvnHash),
	).Scan(&kycID)

	// Call Dojah BVN lookup
	dojahResp, err := h.callDojah("/api/v1/kyc/bvn/full", map[string]any{
		"bvn": body.BVN,
	}, r)

	if err != nil || !dojahResp.Success {
		reason := "BVN verification failed"
		if dojahResp.Error != "" {
			reason = dojahResp.Error
		}
		_, _ = h.db.Exec(r.Context(),
			`UPDATE kyc_verifications SET status = 'rejected', failure_reason = $2 WHERE id = $1`,
			kycID, reason,
		)
		writeErr(w, http.StatusBadRequest, reason)
		return
	}

	// Approve: upgrade user to tier 1
	now := time.Now()
	_, _ = h.db.Exec(r.Context(),
		`UPDATE kyc_verifications SET status = 'approved', dojah_ref = $2, verified_at = $3 WHERE id = $1`,
		kycID, dojahResp.Ref, now,
	)
	_, _ = h.db.Exec(r.Context(),
		`UPDATE users SET kyc_tier = '1', updated_at = NOW() WHERE id = $1`, u.ID,
	)

	writeJSON(w, http.StatusOK, map[string]any{
		"message":  "BVN verified successfully",
		"kyc_tier": "1",
	})
}

// POST /kyc/verify-nin — Tier 2 verification
func (h *KYCHandler) VerifyNIN(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var body struct {
		NIN string `json:"nin"`
	}
	if err := decode(r, &body); err != nil || body.NIN == "" {
		writeErr(w, http.StatusBadRequest, "nin is required")
		return
	}

	ninHash, _ := bcrypt.GenerateFromPassword([]byte(body.NIN), 10)

	var kycID string
	_ = h.db.QueryRow(r.Context(),
		`INSERT INTO kyc_verifications (user_id, tier, status, nin_hash)
		 VALUES ($1, '2', 'processing', $2)
		 RETURNING id`,
		u.ID, string(ninHash),
	).Scan(&kycID)

	dojahResp, err := h.callDojah("/api/v1/kyc/nin/full", map[string]any{
		"nin": body.NIN,
	}, r)

	if err != nil || !dojahResp.Success {
		reason := "NIN verification failed"
		if dojahResp.Error != "" {
			reason = dojahResp.Error
		}
		_, _ = h.db.Exec(r.Context(),
			`UPDATE kyc_verifications SET status = 'rejected', failure_reason = $2 WHERE id = $1`,
			kycID, reason,
		)
		writeErr(w, http.StatusBadRequest, reason)
		return
	}

	now := time.Now()
	_, _ = h.db.Exec(r.Context(),
		`UPDATE kyc_verifications SET status = 'approved', dojah_ref = $2, verified_at = $3 WHERE id = $1`,
		kycID, dojahResp.Ref, now,
	)
	_, _ = h.db.Exec(r.Context(),
		`UPDATE users SET kyc_tier = '2', updated_at = NOW() WHERE id = $1`, u.ID,
	)

	writeJSON(w, http.StatusOK, map[string]any{
		"message":  "NIN verified successfully",
		"kyc_tier": "2",
	})
}

type dojahResponse struct {
	Success bool   `json:"-"`
	Ref     string `json:"-"`
	Error   string `json:"-"`
}

func (h *KYCHandler) callDojah(path string, payload map[string]any, r *http.Request) (*dojahResponse, error) {
	b, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost,
		"https://api.dojah.io"+path, bytes.NewReader(b))
	if err != nil {
		return &dojahResponse{}, err
	}
	req.Header.Set("AppId", h.dojahAppID)
	req.Header.Set("Authorization", h.dojahPrivKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &dojahResponse{}, err
	}
	defer resp.Body.Close()

	var raw map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&raw)

	result := &dojahResponse{}
	if resp.StatusCode == http.StatusOK {
		result.Success = true
		if ref, ok := raw["entity"].(map[string]any)["ref"].(string); ok {
			result.Ref = ref
		}
	} else {
		if errMsg, ok := raw["error"].(string); ok {
			result.Error = errMsg
		}
	}
	return result, nil
}
