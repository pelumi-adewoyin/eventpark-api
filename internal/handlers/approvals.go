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

type ApprovalsHandler struct {
	db *pgxpool.Pool
}

func NewApprovalsHandler(db *pgxpool.Pool) *ApprovalsHandler {
	return &ApprovalsHandler{db: db}
}

// ─── LIST ─────────────────────────────────────────────────────────────────────

// GET /orgs/:orgId/approvals?status=pending&type=po
func (h *ApprovalsHandler) ListApprovals(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(chi.URLParam(r, "orgId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid org id")
		return
	}

	status := r.URL.Query().Get("status")
	itemType := r.URL.Query().Get("type")

	query := `SELECT a.id, a.org_id, a.event_id, a.item_type, a.item_id, a.item_ref,
		  a.title, a.amount, a.currency, a.requested_by, a.notes, a.urgency,
		  a.status, a.resolved_at, a.created_at, a.updated_at,
		  u.full_name AS requester_name
		FROM approval_requests a
		JOIN users u ON u.id = a.requested_by
		WHERE a.org_id = $1`
	args := []any{orgID}

	if status != "" {
		args = append(args, status)
		query += ` AND a.status = $` + itoa(len(args))
	}
	if itemType != "" {
		args = append(args, itemType)
		query += ` AND a.item_type = $` + itoa(len(args))
	}
	query += ` ORDER BY a.created_at DESC`

	rows, err := h.db.Query(r.Context(), query, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to list approvals")
		return
	}
	defer rows.Close()

	approvals := []models.ApprovalRequest{}
	for rows.Next() {
		var a models.ApprovalRequest
		if err := rows.Scan(
			&a.ID, &a.OrgID, &a.EventID, &a.ItemType, &a.ItemID, &a.ItemRef,
			&a.Title, &a.Amount, &a.Currency, &a.RequestedBy, &a.Notes, &a.Urgency,
			&a.Status, &a.ResolvedAt, &a.CreatedAt, &a.UpdatedAt,
			&a.RequesterName,
		); err == nil {
			approvals = append(approvals, a)
		}
	}
	writeJSON(w, http.StatusOK, approvals)
}

// GET /orgs/:orgId/approvals/:id — with action history
func (h *ApprovalsHandler) GetApproval(w http.ResponseWriter, r *http.Request) {
	orgID, _ := uuid.Parse(chi.URLParam(r, "orgId"))
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}

	var a models.ApprovalRequest
	err = h.db.QueryRow(r.Context(),
		`SELECT a.id, a.org_id, a.event_id, a.item_type, a.item_id, a.item_ref,
		   a.title, a.amount, a.currency, a.requested_by, a.notes, a.urgency,
		   a.status, a.resolved_at, a.created_at, a.updated_at,
		   u.full_name
		 FROM approval_requests a
		 JOIN users u ON u.id = a.requested_by
		 WHERE a.id = $1 AND a.org_id = $2`,
		id, orgID,
	).Scan(
		&a.ID, &a.OrgID, &a.EventID, &a.ItemType, &a.ItemID, &a.ItemRef,
		&a.Title, &a.Amount, &a.Currency, &a.RequestedBy, &a.Notes, &a.Urgency,
		&a.Status, &a.ResolvedAt, &a.CreatedAt, &a.UpdatedAt,
		&a.RequesterName,
	)
	if err != nil {
		writeErr(w, http.StatusNotFound, "approval not found")
		return
	}

	// Fetch action history
	rows, _ := h.db.Query(r.Context(),
		`SELECT aa.id, aa.request_id, aa.actor_id, aa.action, aa.note,
		   aa.delegated_to, aa.created_at, u.full_name
		 FROM approval_actions aa
		 JOIN users u ON u.id = aa.actor_id
		 WHERE aa.request_id = $1 ORDER BY aa.created_at`,
		id,
	)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var act models.ApprovalAction
			if err := rows.Scan(
				&act.ID, &act.RequestID, &act.ActorID, &act.Action, &act.Note,
				&act.DelegatedTo, &act.CreatedAt, &act.ActorName,
			); err == nil {
				a.Actions = append(a.Actions, act)
			}
		}
	}

	writeJSON(w, http.StatusOK, a)
}

// ─── SUBMIT ───────────────────────────────────────────────────────────────────

// POST /orgs/:orgId/approvals — submit new approval request
func (h *ApprovalsHandler) SubmitApproval(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	orgID, err := uuid.Parse(chi.URLParam(r, "orgId"))
	if err != nil || u == nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}

	var body struct {
		ItemType string     `json:"item_type"`
		ItemID   uuid.UUID  `json:"item_id"`
		ItemRef  *string    `json:"item_ref"`
		Title    string     `json:"title"`
		Amount   int64      `json:"amount"`
		EventID  *uuid.UUID `json:"event_id"`
		Notes    *string    `json:"notes"`
		Urgency  string     `json:"urgency"`
	}
	if err := decode(r, &body); err != nil || body.Title == "" || body.ItemType == "" {
		writeErr(w, http.StatusBadRequest, "item_type and title are required")
		return
	}
	if body.Urgency == "" {
		body.Urgency = "medium"
	}

	var a models.ApprovalRequest
	err = h.db.QueryRow(r.Context(),
		`INSERT INTO approval_requests
		   (org_id, event_id, item_type, item_id, item_ref, title, amount, requested_by, notes, urgency)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		 RETURNING id, org_id, event_id, item_type, item_id, item_ref,
		   title, amount, currency, requested_by, notes, urgency,
		   status, resolved_at, created_at, updated_at`,
		orgID, body.EventID, body.ItemType, body.ItemID, body.ItemRef,
		body.Title, body.Amount, u.ID, body.Notes, body.Urgency,
	).Scan(
		&a.ID, &a.OrgID, &a.EventID, &a.ItemType, &a.ItemID, &a.ItemRef,
		&a.Title, &a.Amount, &a.Currency, &a.RequestedBy, &a.Notes, &a.Urgency,
		&a.Status, &a.ResolvedAt, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to submit approval")
		return
	}

	// Write to audit log
	writeAuditLog(r.Context(), h.db, orgID, u.ID, "Approval Submitted", "approval_request", a.ID, body.Title)

	writeJSON(w, http.StatusCreated, a)
}

// ─── APPROVE ──────────────────────────────────────────────────────────────────

// POST /orgs/:orgId/approvals/:id/approve
func (h *ApprovalsHandler) Approve(w http.ResponseWriter, r *http.Request) {
	h.actOnApproval(w, r, "approved")
}

// POST /orgs/:orgId/approvals/:id/reject
func (h *ApprovalsHandler) Reject(w http.ResponseWriter, r *http.Request) {
	h.actOnApproval(w, r, "rejected")
}

// POST /orgs/:orgId/approvals/:id/request-changes
func (h *ApprovalsHandler) RequestChanges(w http.ResponseWriter, r *http.Request) {
	h.actOnApproval(w, r, "changes_requested")
}

func (h *ApprovalsHandler) actOnApproval(w http.ResponseWriter, r *http.Request, action string) {
	u := middleware.GetUser(r)
	orgID, _ := uuid.Parse(chi.URLParam(r, "orgId"))
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil || u == nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}

	var body struct {
		Note        *string    `json:"note"`
		DelegateTo  *uuid.UUID `json:"delegate_to"`
	}
	_ = decode(r, &body)

	// Record action
	_, err = h.db.Exec(r.Context(),
		`INSERT INTO approval_actions (request_id, actor_id, action, note, delegated_to)
		 VALUES ($1, $2, $3, $4, $5)`,
		id, u.ID, action, body.Note, body.DelegateTo,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to record action")
		return
	}

	// Update status (changes_requested stays pending in parent)
	newStatus := action
	if action == "changes_requested" {
		newStatus = "changes_requested"
	}

	_, err = h.db.Exec(r.Context(),
		`UPDATE approval_requests SET
		   status = $3,
		   resolved_at = CASE WHEN $3 IN ('approved','rejected') THEN NOW() ELSE NULL END,
		   updated_at = NOW()
		 WHERE id = $1 AND org_id = $2`,
		id, orgID, newStatus,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to update approval status")
		return
	}

	// If PO approval — update PO status too
	var req models.ApprovalRequest
	_ = h.db.QueryRow(r.Context(),
		`SELECT item_type, item_id, title FROM approval_requests WHERE id = $1`, id,
	).Scan(&req.ItemType, &req.ItemID, &req.Title)

	if req.ItemType == "po" && action == "approved" {
		_, _ = h.db.Exec(r.Context(),
			`UPDATE purchase_orders SET status = 'approved', approved_by = $2, approved_at = NOW(), updated_at = NOW()
			 WHERE id = $1`, req.ItemID, u.ID,
		)
	}
	if req.ItemType == "invoice" && action == "approved" {
		_, _ = h.db.Exec(r.Context(),
			`UPDATE vendor_invoices SET status = 'approved', approved_by = $2, updated_at = NOW()
			 WHERE id = $1`, req.ItemID, u.ID,
		)
	}

	// Write to audit log
	writeAuditLog(r.Context(), h.db, orgID, u.ID,
		"Approval "+action, "approval_request", id, req.Title)

	writeJSON(w, http.StatusOK, map[string]string{"status": newStatus})
}

// POST /orgs/:orgId/approvals/:id/delegate
func (h *ApprovalsHandler) Delegate(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	orgID, _ := uuid.Parse(chi.URLParam(r, "orgId"))
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil || u == nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}

	var body struct {
		DelegateTo uuid.UUID `json:"delegate_to"`
		Note       *string   `json:"note"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "delegate_to is required")
		return
	}

	_, err = h.db.Exec(r.Context(),
		`INSERT INTO approval_actions (request_id, actor_id, action, note, delegated_to)
		 VALUES ($1, $2, 'delegated', $3, $4)`,
		id, u.ID, body.Note, body.DelegateTo,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to delegate")
		return
	}

	writeAuditLog(r.Context(), h.db, orgID, u.ID, "Approval Delegated", "approval_request", id, "")
	writeJSON(w, http.StatusOK, map[string]string{"message": "delegated"})
}

// ─── HELPER ───────────────────────────────────────────────────────────────────

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}
