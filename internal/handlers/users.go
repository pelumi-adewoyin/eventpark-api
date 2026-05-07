package handlers

import (
	"net/http"

	"github.com/eventpark/api/internal/middleware"
	"github.com/eventpark/api/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UsersHandler struct {
	db *pgxpool.Pool
}

func NewUsersHandler(db *pgxpool.Pool) *UsersHandler {
	return &UsersHandler{db: db}
}

// GET /users/me
func (h *UsersHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var user models.User
	err := h.db.QueryRow(r.Context(),
		`SELECT u.id, u.phone, u.email, u.full_name, u.avatar_url, u.role::text,
		        u.kyc_tier::text, u.onboarding_done, u.created_at, u.updated_at,
		        m.org_id, o.name,
		        v.id, v.vendor_type, v.business_name, v.verification_status, v.verification_tier
		 FROM users u
		 LEFT JOIN org_members m ON m.user_id = u.id AND m.active = true
		 LEFT JOIN orgs o ON o.id = m.org_id
		 LEFT JOIN vendors v ON v.user_id = u.id
		 WHERE u.id = $1
		 LIMIT 1`, u.ID,
	).Scan(
		&user.ID, &user.Phone, &user.Email, &user.FullName, &user.AvatarURL,
		&user.Role, &user.KYCTier, &user.OnboardingDone, &user.CreatedAt, &user.UpdatedAt,
		&user.OrgID, &user.OrgName,
		&user.VendorID, &user.VendorType, &user.BusinessName, &user.VerificationStatus, &user.VerificationTier,
	)
	if err != nil {
		writeErr(w, http.StatusNotFound, "user not found")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

// PATCH /users/me
func (h *UsersHandler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var body struct {
		FullName  *string `json:"full_name"`
		Email     *string `json:"email"`
		AvatarURL *string `json:"avatar_url"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}

	_, err := h.db.Exec(r.Context(),
		`UPDATE users SET
		  full_name  = COALESCE($2, full_name),
		  email      = COALESCE($3, email),
		  avatar_url = COALESCE($4, avatar_url),
		  updated_at = NOW()
		 WHERE id = $1`,
		u.ID, body.FullName, body.Email, body.AvatarURL,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to update user")
		return
	}

	h.GetMe(w, r)
}

// POST /users/onboarding
func (h *UsersHandler) CompleteOnboarding(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var body struct {
		Role     string  `json:"role"`       // diy | planner | corporate
		FullName *string `json:"full_name"`
		Email    *string `json:"email"`
		// Corporate-specific
		OrgName    *string `json:"org_name"`
		RCNumber   *string `json:"rc_number"`
		Industry   *string `json:"industry"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}

	validRoles := map[string]bool{"diy": true, "planner": true, "corporate": true, "vendor": true}
	if !validRoles[body.Role] {
		writeErr(w, http.StatusBadRequest, "role must be diy, planner, corporate, or vendor")
		return
	}

	tx, err := h.db.Begin(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "transaction error")
		return
	}
	defer tx.Rollback(r.Context())

	_, err = tx.Exec(r.Context(),
		`UPDATE users SET role = $2, full_name = COALESCE($3, full_name),
		  email = COALESCE($4, email), onboarding_done = true, updated_at = NOW()
		 WHERE id = $1`,
		u.ID, body.Role, body.FullName, body.Email,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to update user")
		return
	}

	// For corporate users, create an organisation and add the owner as a member
	if body.Role == "corporate" && body.OrgName != nil {
		var orgID uuid.UUID
		err = tx.QueryRow(r.Context(),
			`INSERT INTO organisations (name, rc_number, industry, owner_id)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT DO NOTHING
			 RETURNING id`,
			body.OrgName, body.RCNumber, body.Industry, u.ID,
		).Scan(&orgID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "failed to create organisation")
			return
		}
		// Add owner as an active org member so GET /orgs/me can find this org
		_, _ = tx.Exec(r.Context(),
			`INSERT INTO org_members (org_id, user_id, role, role_enum, active)
			 VALUES ($1, $2, 'owner', 'owner', true) ON CONFLICT DO NOTHING`,
			orgID, u.ID,
		)
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, http.StatusInternalServerError, "commit failed")
		return
	}

	h.GetMe(w, r)
}
