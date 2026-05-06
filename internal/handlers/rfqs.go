package handlers

import (
	"fmt"
	"net/http"

	"github.com/eventpark/api/internal/middleware"
	"github.com/eventpark/api/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RFQsHandler struct {
	db *pgxpool.Pool
}

func NewRFQsHandler(db *pgxpool.Pool) *RFQsHandler {
	return &RFQsHandler{db: db}
}

// ─── LIST ─────────────────────────────────────────────────────────────────────

// GET /orgs/:orgId/rfqs?status=open&event_id=xxx
func (h *RFQsHandler) ListRFQs(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(chi.URLParam(r, "orgId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid org id")
		return
	}

	rows, err := h.db.Query(r.Context(),
		`SELECT r.id, r.org_id, r.event_id, r.ref, r.title, r.description,
		   r.deadline, r.status, r.awarded_vendor, r.created_by, r.created_at, r.updated_at,
		   COUNT(rr.id) AS response_count,
		   cv.name AS awarded_vendor_name
		 FROM rfqs r
		 LEFT JOIN rfq_responses rr ON rr.rfq_id = r.id
		 LEFT JOIN corp_vendors cv ON cv.id = r.awarded_vendor
		 WHERE r.org_id = $1
		 GROUP BY r.id, cv.name
		 ORDER BY r.created_at DESC`,
		orgID,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to list RFQs")
		return
	}
	defer rows.Close()

	rfqs := []models.RFQ{}
	for rows.Next() {
		var rfq models.RFQ
		if err := rows.Scan(
			&rfq.ID, &rfq.OrgID, &rfq.EventID, &rfq.Ref, &rfq.Title, &rfq.Description,
			&rfq.Deadline, &rfq.Status, &rfq.AwardedVendor, &rfq.CreatedBy,
			&rfq.CreatedAt, &rfq.UpdatedAt,
			&rfq.ResponseCount, &rfq.AwardedVendorName,
		); err == nil {
			rfqs = append(rfqs, rfq)
		}
	}
	writeJSON(w, http.StatusOK, rfqs)
}

// GET /orgs/:orgId/rfqs/:id — with responses
func (h *RFQsHandler) GetRFQ(w http.ResponseWriter, r *http.Request) {
	orgID, _ := uuid.Parse(chi.URLParam(r, "orgId"))
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid rfq id")
		return
	}

	var rfq models.RFQ
	err = h.db.QueryRow(r.Context(),
		`SELECT r.id, r.org_id, r.event_id, r.ref, r.title, r.description,
		   r.deadline, r.status, r.awarded_vendor, r.created_by, r.created_at, r.updated_at,
		   COUNT(rr.id) AS response_count,
		   cv.name AS awarded_vendor_name
		 FROM rfqs r
		 LEFT JOIN rfq_responses rr ON rr.rfq_id = r.id
		 LEFT JOIN corp_vendors cv ON cv.id = r.awarded_vendor
		 WHERE r.id = $1 AND r.org_id = $2
		 GROUP BY r.id, cv.name`,
		id, orgID,
	).Scan(
		&rfq.ID, &rfq.OrgID, &rfq.EventID, &rfq.Ref, &rfq.Title, &rfq.Description,
		&rfq.Deadline, &rfq.Status, &rfq.AwardedVendor, &rfq.CreatedBy,
		&rfq.CreatedAt, &rfq.UpdatedAt,
		&rfq.ResponseCount, &rfq.AwardedVendorName,
	)
	if err != nil {
		writeErr(w, http.StatusNotFound, "RFQ not found")
		return
	}

	// Fetch responses
	rows, _ := h.db.Query(r.Context(),
		`SELECT rr.id, rr.rfq_id, rr.vendor_id, rr.amount, rr.delivery_days,
		   rr.notes, rr.score, rr.awarded, rr.submitted_at, cv.name
		 FROM rfq_responses rr
		 JOIN corp_vendors cv ON cv.id = rr.vendor_id
		 WHERE rr.rfq_id = $1 ORDER BY rr.amount`,
		id,
	)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var resp models.RFQResponse
			if err := rows.Scan(
				&resp.ID, &resp.RFQId, &resp.VendorID, &resp.Amount, &resp.DeliveryDays,
				&resp.Notes, &resp.Score, &resp.Awarded, &resp.SubmittedAt, &resp.VendorName,
			); err == nil {
				rfq.Responses = append(rfq.Responses, resp)
			}
		}
	}

	writeJSON(w, http.StatusOK, rfq)
}

// ─── CREATE ───────────────────────────────────────────────────────────────────

// POST /orgs/:orgId/rfqs
func (h *RFQsHandler) CreateRFQ(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	orgID, err := uuid.Parse(chi.URLParam(r, "orgId"))
	if err != nil || u == nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}

	var body struct {
		Title       string     `json:"title"`
		Description *string    `json:"description"`
		EventID     *uuid.UUID `json:"event_id"`
		Deadline    *string    `json:"deadline"`
		VendorIDs   []uuid.UUID `json:"vendor_ids"` // which corp_vendors to invite
	}
	if err := decode(r, &body); err != nil || body.Title == "" {
		writeErr(w, http.StatusBadRequest, "title is required")
		return
	}

	// Generate ref
	var seq int
	_ = h.db.QueryRow(r.Context(), `SELECT nextval('rfq_ref_seq')`).Scan(&seq)
	ref := fmt.Sprintf("RFQ-%d-%04d", 2026, seq)

	var rfq models.RFQ
	err = h.db.QueryRow(r.Context(),
		`INSERT INTO rfqs (org_id, event_id, ref, title, description, deadline, created_by)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)
		 RETURNING id, org_id, event_id, ref, title, description, deadline,
		   status, awarded_vendor, created_by, created_at, updated_at`,
		orgID, body.EventID, ref, body.Title, body.Description, body.Deadline, u.ID,
	).Scan(
		&rfq.ID, &rfq.OrgID, &rfq.EventID, &rfq.Ref, &rfq.Title, &rfq.Description,
		&rfq.Deadline, &rfq.Status, &rfq.AwardedVendor, &rfq.CreatedBy,
		&rfq.CreatedAt, &rfq.UpdatedAt,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to create RFQ")
		return
	}

	// Create invitations
	for _, vID := range body.VendorIDs {
		_, _ = h.db.Exec(r.Context(),
			`INSERT INTO rfq_invitations (rfq_id, vendor_id) VALUES ($1, $2)`,
			rfq.ID, vID,
		)
	}

	writeAuditLog(r.Context(), h.db, orgID, u.ID, "RFQ Created", "rfq", rfq.ID, body.Title)
	writeJSON(w, http.StatusCreated, rfq)
}

// ─── SEND ─────────────────────────────────────────────────────────────────────

// POST /orgs/:orgId/rfqs/:id/send — change status from draft → open
func (h *RFQsHandler) SendRFQ(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	orgID, _ := uuid.Parse(chi.URLParam(r, "orgId"))
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil || u == nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}

	_, err = h.db.Exec(r.Context(),
		`UPDATE rfqs SET status = 'open', updated_at = NOW() WHERE id = $1 AND org_id = $2 AND status = 'draft'`,
		id, orgID,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to send RFQ")
		return
	}

	writeAuditLog(r.Context(), h.db, orgID, u.ID, "RFQ Sent to Vendors", "rfq", id, "")
	writeJSON(w, http.StatusOK, map[string]string{"status": "open"})
}

// ─── RESPOND (vendor submits quote) ──────────────────────────────────────────

// POST /orgs/:orgId/rfqs/:id/respond
func (h *RFQsHandler) SubmitResponse(w http.ResponseWriter, r *http.Request) {
	orgID, _ := uuid.Parse(chi.URLParam(r, "orgId"))
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid rfq id")
		return
	}

	var body struct {
		VendorID     uuid.UUID `json:"vendor_id"`
		Amount       int64     `json:"amount"`
		DeliveryDays *int      `json:"delivery_days"`
		Notes        *string   `json:"notes"`
	}
	if err := decode(r, &body); err != nil || body.Amount == 0 {
		writeErr(w, http.StatusBadRequest, "vendor_id and amount are required")
		return
	}

	var resp models.RFQResponse
	err = h.db.QueryRow(r.Context(),
		`INSERT INTO rfq_responses (rfq_id, vendor_id, amount, delivery_days, notes)
		 VALUES ($1,$2,$3,$4,$5)
		 RETURNING id, rfq_id, vendor_id, amount, delivery_days, notes, score, awarded, submitted_at`,
		id, body.VendorID, body.Amount, body.DeliveryDays, body.Notes,
	).Scan(
		&resp.ID, &resp.RFQId, &resp.VendorID, &resp.Amount, &resp.DeliveryDays,
		&resp.Notes, &resp.Score, &resp.Awarded, &resp.SubmittedAt,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to submit response")
		return
	}

	// Mark invitation as responded
	_, _ = h.db.Exec(r.Context(),
		`UPDATE rfq_invitations SET responded = true WHERE rfq_id = $1 AND vendor_id = $2`,
		id, body.VendorID,
	)

	// Move to evaluating if was open
	_, _ = h.db.Exec(r.Context(),
		`UPDATE rfqs SET status = 'evaluating', updated_at = NOW() WHERE id = $1 AND status = 'open'`, id,
	)

	writeAuditLog(r.Context(), h.db, orgID, nil, "RFQ Response Submitted", "rfq", id, "")
	writeJSON(w, http.StatusCreated, resp)
}

// ─── SCORE ────────────────────────────────────────────────────────────────────

// PATCH /orgs/:orgId/rfqs/:id/responses/:responseId/score
func (h *RFQsHandler) ScoreResponse(w http.ResponseWriter, r *http.Request) {
	responseID, err := uuid.Parse(chi.URLParam(r, "responseId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid response id")
		return
	}

	var body struct {
		Score int `json:"score"`
	}
	if err := decode(r, &body); err != nil || body.Score < 1 || body.Score > 10 {
		writeErr(w, http.StatusBadRequest, "score must be 1-10")
		return
	}

	_, err = h.db.Exec(r.Context(),
		`UPDATE rfq_responses SET score = $2 WHERE id = $1`, responseID, body.Score,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to score response")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"score": body.Score})
}

// ─── AWARD ────────────────────────────────────────────────────────────────────

// POST /orgs/:orgId/rfqs/:id/award
func (h *RFQsHandler) AwardRFQ(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	orgID, _ := uuid.Parse(chi.URLParam(r, "orgId"))
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil || u == nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}

	var body struct {
		VendorID   uuid.UUID `json:"vendor_id"`
		ResponseID uuid.UUID `json:"response_id"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "vendor_id and response_id are required")
		return
	}

	// Mark awarded vendor and response
	_, err = h.db.Exec(r.Context(),
		`UPDATE rfqs SET status = 'awarded', awarded_vendor = $3, updated_at = NOW()
		 WHERE id = $1 AND org_id = $2`,
		id, orgID, body.VendorID,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to award RFQ")
		return
	}

	_, _ = h.db.Exec(r.Context(),
		`UPDATE rfq_responses SET awarded = true WHERE id = $1`, body.ResponseID,
	)

	// Get vendor name for audit
	var vendorName string
	_ = h.db.QueryRow(r.Context(), `SELECT name FROM corp_vendors WHERE id = $1`, body.VendorID).Scan(&vendorName)

	writeAuditLog(r.Context(), h.db, orgID, u.ID, "RFQ Awarded", "rfq", id,
		fmt.Sprintf("Awarded to %s", vendorName))
	writeJSON(w, http.StatusOK, map[string]string{"status": "awarded", "vendor": vendorName})
}

// ─── CANCEL ───────────────────────────────────────────────────────────────────

// POST /orgs/:orgId/rfqs/:id/cancel
func (h *RFQsHandler) CancelRFQ(w http.ResponseWriter, r *http.Request) {
	orgID, _ := uuid.Parse(chi.URLParam(r, "orgId"))
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid rfq id")
		return
	}

	_, err = h.db.Exec(r.Context(),
		`UPDATE rfqs SET status = 'cancelled', updated_at = NOW() WHERE id = $1 AND org_id = $2`,
		id, orgID,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to cancel RFQ")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}
