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

type CorpWalletHandler struct {
	db *pgxpool.Pool
}

func NewCorpWalletHandler(db *pgxpool.Pool) *CorpWalletHandler {
	return &CorpWalletHandler{db: db}
}

// ─── GET WALLET ───────────────────────────────────────────────────────────────

// GET /orgs/:orgId/wallet
func (h *CorpWalletHandler) GetWallet(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(chi.URLParam(r, "orgId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid org id")
		return
	}

	var wallet models.OrgWallet
	err = h.db.QueryRow(r.Context(),
		`SELECT id, org_id, balance, pending_out, updated_at
		 FROM org_wallets WHERE org_id = $1`,
		orgID,
	).Scan(&wallet.ID, &wallet.OrgID, &wallet.Balance, &wallet.PendingOut, &wallet.UpdatedAt)
	if err != nil {
		writeErr(w, http.StatusNotFound, "org wallet not found")
		return
	}
	writeJSON(w, http.StatusOK, wallet)
}

// ─── TRANSACTIONS ─────────────────────────────────────────────────────────────

// GET /orgs/:orgId/wallet/transactions
func (h *CorpWalletHandler) ListTransactions(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(chi.URLParam(r, "orgId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid org id")
		return
	}

	// Get wallet ID
	var walletID uuid.UUID
	err = h.db.QueryRow(r.Context(),
		`SELECT id FROM org_wallets WHERE org_id = $1`, orgID,
	).Scan(&walletID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "org wallet not found")
		return
	}

	rows, err := h.db.Query(r.Context(),
		`SELECT id, org_wallet_id, type, amount, balance_after,
		   reference, description, invoice_id, initiated_by, status, created_at
		 FROM org_wallet_transactions
		 WHERE org_wallet_id = $1
		 ORDER BY created_at DESC
		 LIMIT 100`,
		walletID,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to list transactions")
		return
	}
	defer rows.Close()

	txns := []models.OrgWalletTransaction{}
	for rows.Next() {
		var tx models.OrgWalletTransaction
		if err := rows.Scan(
			&tx.ID, &tx.OrgWalletID, &tx.Type, &tx.Amount, &tx.BalanceAfter,
			&tx.Reference, &tx.Description, &tx.InvoiceID, &tx.InitiatedBy,
			&tx.Status, &tx.CreatedAt,
		); err == nil {
			txns = append(txns, tx)
		}
	}
	writeJSON(w, http.StatusOK, txns)
}

// ─── TOP-UP ───────────────────────────────────────────────────────────────────

// POST /orgs/:orgId/wallet/topup — record a top-up (bank transfer received)
func (h *CorpWalletHandler) TopUp(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	orgID, err := uuid.Parse(chi.URLParam(r, "orgId"))
	if err != nil || u == nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}

	var body struct {
		Amount    int64   `json:"amount"`     // in kobo
		Reference string  `json:"reference"`
		Note      *string `json:"note"`
	}
	if err := decode(r, &body); err != nil || body.Amount <= 0 {
		writeErr(w, http.StatusBadRequest, "amount and reference are required")
		return
	}

	var walletID uuid.UUID
	var currentBalance int64
	err = h.db.QueryRow(r.Context(),
		`SELECT id, balance FROM org_wallets WHERE org_id = $1`, orgID,
	).Scan(&walletID, &currentBalance)
	if err != nil {
		writeErr(w, http.StatusNotFound, "org wallet not found")
		return
	}

	newBalance := currentBalance + body.Amount
	_, err = h.db.Exec(r.Context(),
		`UPDATE org_wallets SET balance = $2, updated_at = NOW() WHERE id = $1`,
		walletID, newBalance,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to credit wallet")
		return
	}

	desc := "Bank transfer top-up"
	if body.Note != nil {
		desc = *body.Note
	}

	_, _ = h.db.Exec(r.Context(),
		`INSERT INTO org_wallet_transactions
		   (org_wallet_id, type, amount, balance_after, reference, description, initiated_by)
		 VALUES ($1, 'credit', $2, $3, $4, $5, $6)`,
		walletID, body.Amount, newBalance, body.Reference, desc, u.ID,
	)

	writeAuditLog(r.Context(), h.db, orgID, u.ID, "Wallet Top-up",
		"org_wallet", walletID,
		fmt.Sprintf("₦%d credited – ref %s", body.Amount, body.Reference))

	writeJSON(w, http.StatusOK, map[string]any{
		"balance":   newBalance,
		"credited":  body.Amount,
		"reference": body.Reference,
	})
}

// ─── WITHDRAWAL REQUEST (multi-sig) ──────────────────────────────────────────

// POST /orgs/:orgId/wallet/withdrawal-requests
func (h *CorpWalletHandler) RequestWithdrawal(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	orgID, err := uuid.Parse(chi.URLParam(r, "orgId"))
	if err != nil || u == nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}

	var body struct {
		Amount int64   `json:"amount"`
		Reason *string `json:"reason"`
	}
	if err := decode(r, &body); err != nil || body.Amount <= 0 {
		writeErr(w, http.StatusBadRequest, "amount is required")
		return
	}

	// Check balance
	var balance int64
	_ = h.db.QueryRow(r.Context(),
		`SELECT balance FROM org_wallets WHERE org_id = $1`, orgID,
	).Scan(&balance)
	if balance < body.Amount {
		writeErr(w, http.StatusUnprocessableEntity, "insufficient balance")
		return
	}

	// Determine signatories required
	const multiSigThreshold = 500_000_00 // ₦5,000,000 in kobo
	approvalsNeeded := 1
	if body.Amount > multiSigThreshold {
		approvalsNeeded = 2
	}

	var req models.OrgWithdrawalRequest
	err = h.db.QueryRow(r.Context(),
		`INSERT INTO org_withdrawal_requests
		   (org_id, amount, reason, requested_by, approvals_needed)
		 VALUES ($1,$2,$3,$4,$5)
		 RETURNING id, org_id, amount, reason, requested_by, status, approvals_needed, created_at`,
		orgID, body.Amount, body.Reason, u.ID, approvalsNeeded,
	).Scan(
		&req.ID, &req.OrgID, &req.Amount, &req.Reason,
		&req.RequestedBy, &req.Status, &req.ApprovalsNeeded, &req.CreatedAt,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to create withdrawal request")
		return
	}

	writeAuditLog(r.Context(), h.db, orgID, u.ID, "Withdrawal Requested",
		"withdrawal_request", req.ID,
		fmt.Sprintf("₦%d – requires %d signatory approvals", body.Amount, approvalsNeeded))

	writeJSON(w, http.StatusCreated, req)
}

// GET /orgs/:orgId/wallet/withdrawal-requests
func (h *CorpWalletHandler) ListWithdrawalRequests(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(chi.URLParam(r, "orgId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid org id")
		return
	}

	rows, err := h.db.Query(r.Context(),
		`SELECT id, org_id, amount, reason, requested_by, status, approvals_needed, created_at, executed_at
		 FROM org_withdrawal_requests
		 WHERE org_id = $1 ORDER BY created_at DESC`,
		orgID,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to list withdrawal requests")
		return
	}
	defer rows.Close()

	reqs := []models.OrgWithdrawalRequest{}
	for rows.Next() {
		var req models.OrgWithdrawalRequest
		if err := rows.Scan(
			&req.ID, &req.OrgID, &req.Amount, &req.Reason,
			&req.RequestedBy, &req.Status, &req.ApprovalsNeeded,
			&req.CreatedAt, &req.ExecutedAt,
		); err == nil {
			reqs = append(reqs, req)
		}
	}
	writeJSON(w, http.StatusOK, reqs)
}

// POST /orgs/:orgId/wallet/withdrawal-requests/:id/sign
func (h *CorpWalletHandler) SignWithdrawal(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	orgID, _ := uuid.Parse(chi.URLParam(r, "orgId"))
	reqID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil || u == nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}

	var body struct {
		Action string `json:"action"` // approved | rejected
	}
	if err := decode(r, &body); err != nil || (body.Action != "approved" && body.Action != "rejected") {
		writeErr(w, http.StatusBadRequest, "action must be 'approved' or 'rejected'")
		return
	}

	_, err = h.db.Exec(r.Context(),
		`INSERT INTO org_withdrawal_approvals (request_id, signer_id, action)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (request_id, signer_id) DO UPDATE SET action = EXCLUDED.action, signed_at = NOW()`,
		reqID, u.ID, body.Action,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to record signature")
		return
	}

	// Check if we have enough approvals to execute
	var req models.OrgWithdrawalRequest
	_ = h.db.QueryRow(r.Context(),
		`SELECT id, org_id, amount, approvals_needed, status FROM org_withdrawal_requests WHERE id = $1`,
		reqID,
	).Scan(&req.ID, &req.OrgID, &req.Amount, &req.ApprovalsNeeded, &req.Status)

	if body.Action == "rejected" {
		_, _ = h.db.Exec(r.Context(),
			`UPDATE org_withdrawal_requests SET status = 'rejected' WHERE id = $1`, reqID,
		)
		writeJSON(w, http.StatusOK, map[string]string{"status": "rejected"})
		return
	}

	var approvalCount int
	_ = h.db.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM org_withdrawal_approvals WHERE request_id = $1 AND action = 'approved'`,
		reqID,
	).Scan(&approvalCount)

	if approvalCount >= req.ApprovalsNeeded && req.Status == "pending" {
		// Execute withdrawal
		var walletID uuid.UUID
		var balance int64
		_ = h.db.QueryRow(r.Context(),
			`SELECT id, balance FROM org_wallets WHERE org_id = $1`, orgID,
		).Scan(&walletID, &balance)

		newBalance := balance - req.Amount
		if newBalance >= 0 {
			_, _ = h.db.Exec(r.Context(),
				`UPDATE org_wallets SET balance = $2, updated_at = NOW() WHERE id = $1`,
				walletID, newBalance,
			)
			_, _ = h.db.Exec(r.Context(),
				`INSERT INTO org_wallet_transactions
				   (org_wallet_id, type, amount, balance_after, description, initiated_by)
				 VALUES ($1, 'debit', $2, $3, 'Withdrawal executed – multi-sig approved', $4)`,
				walletID, req.Amount, newBalance, u.ID,
			)
			_, _ = h.db.Exec(r.Context(),
				`UPDATE org_withdrawal_requests SET status = 'executed', executed_at = NOW() WHERE id = $1`, reqID,
			)
			writeAuditLog(r.Context(), h.db, orgID, u.ID, "Withdrawal Executed",
				"withdrawal_request", reqID, fmt.Sprintf("₦%d withdrawn", req.Amount))
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "executed", "balance": newBalance})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":           "pending",
		"approvals_count":  approvalCount,
		"approvals_needed": req.ApprovalsNeeded,
	})
}

// ─── SIGNATORIES ─────────────────────────────────────────────────────────────

// GET /orgs/:orgId/wallet/signatories
func (h *CorpWalletHandler) ListSignatories(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(chi.URLParam(r, "orgId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid org id")
		return
	}

	rows, err := h.db.Query(r.Context(),
		`SELECT s.id, s.org_id, s.user_id, s.threshold, s.added_at,
		   u.full_name, u.email
		 FROM org_wallet_signatories s
		 JOIN users u ON u.id = s.user_id
		 WHERE s.org_id = $1`,
		orgID,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to list signatories")
		return
	}
	defer rows.Close()

	type Signatory struct {
		ID        uuid.UUID `json:"id"`
		OrgID     uuid.UUID `json:"org_id"`
		UserID    uuid.UUID `json:"user_id"`
		Threshold int64     `json:"threshold"`
		AddedAt   string    `json:"added_at"`
		FullName  *string   `json:"full_name"`
		Email     *string   `json:"email"`
	}
	sigs := []Signatory{}
	for rows.Next() {
		var s Signatory
		var addedAt interface{}
		if err := rows.Scan(
			&s.ID, &s.OrgID, &s.UserID, &s.Threshold, &addedAt, &s.FullName, &s.Email,
		); err == nil {
			sigs = append(sigs, s)
		}
	}
	writeJSON(w, http.StatusOK, sigs)
}

// POST /orgs/:orgId/wallet/signatories
func (h *CorpWalletHandler) AddSignatory(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	orgID, err := uuid.Parse(chi.URLParam(r, "orgId"))
	if err != nil || u == nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}

	var body struct {
		UserID    uuid.UUID `json:"user_id"`
		Threshold int64     `json:"threshold"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "user_id is required")
		return
	}
	if body.Threshold == 0 {
		body.Threshold = 500_000_00 // ₦5M default
	}

	_, err = h.db.Exec(r.Context(),
		`INSERT INTO org_wallet_signatories (org_id, user_id, threshold)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (org_id, user_id) DO UPDATE SET threshold = EXCLUDED.threshold`,
		orgID, body.UserID, body.Threshold,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to add signatory")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"message": "signatory added"})
}
