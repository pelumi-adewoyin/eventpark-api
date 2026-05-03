package handlers

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/eventpark/api/internal/middleware"
	"github.com/eventpark/api/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type WalletHandler struct {
	db               *pgxpool.Pool
	paystackSecret   string
}

func NewWalletHandler(db *pgxpool.Pool, paystackSecret string) *WalletHandler {
	return &WalletHandler{db: db, paystackSecret: paystackSecret}
}

// GET /wallet
func (h *WalletHandler) GetWallet(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var wallet models.Wallet
	err := h.db.QueryRow(r.Context(),
		`SELECT id, user_id, balance, escrow_held, updated_at FROM wallets WHERE user_id = $1`,
		u.ID,
	).Scan(&wallet.ID, &wallet.UserID, &wallet.Balance, &wallet.EscrowHeld, &wallet.UpdatedAt)
	if err != nil {
		writeErr(w, http.StatusNotFound, "wallet not found")
		return
	}
	writeJSON(w, http.StatusOK, wallet)
}

// GET /wallet/transactions
func (h *WalletHandler) ListTransactions(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	filter := r.URL.Query().Get("type") // all | topup | withdraw | escrow

	query := `SELECT wt.id, wt.wallet_id, wt.type, wt.amount, wt.status,
		wt.reference, wt.paystack_ref, wt.description, wt.booking_id, wt.created_at
	  FROM wallet_transactions wt
	  JOIN wallets w ON w.id = wt.wallet_id
	  WHERE w.user_id = $1`
	args := []any{u.ID}

	if filter != "" && filter != "all" {
		query += ` AND wt.type = $2`
		args = append(args, filter)
	}
	query += ` ORDER BY wt.created_at DESC LIMIT 50`

	rows, err := h.db.Query(r.Context(), query, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to fetch transactions")
		return
	}
	defer rows.Close()

	txns := []models.WalletTransaction{}
	for rows.Next() {
		var t models.WalletTransaction
		if err := rows.Scan(
			&t.ID, &t.WalletID, &t.Type, &t.Amount, &t.Status,
			&t.Reference, &t.PaystackRef, &t.Description, &t.BookingID, &t.CreatedAt,
		); err == nil {
			txns = append(txns, t)
		}
	}
	writeJSON(w, http.StatusOK, txns)
}

// POST /wallet/topup/initialize — create Paystack payment
func (h *WalletHandler) InitializeTopUp(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var body struct {
		Amount int64  `json:"amount"` // in kobo
		Email  string `json:"email"`
	}
	if err := decode(r, &body); err != nil || body.Amount <= 0 || body.Email == "" {
		writeErr(w, http.StatusBadRequest, "amount (kobo) and email are required")
		return
	}

	ref := fmt.Sprintf("EP-TOPUP-%s-%d", u.ID.String()[:8], time.Now().UnixMilli())

	// Create pending transaction record
	var walletID uuid.UUID
	_ = h.db.QueryRow(r.Context(), `SELECT id FROM wallets WHERE user_id = $1`, u.ID).Scan(&walletID)

	_, _ = h.db.Exec(r.Context(),
		`INSERT INTO wallet_transactions (wallet_id, type, amount, status, reference, description)
		 VALUES ($1, 'topup', $2, 'pending', $3, 'Wallet top-up via Paystack')`,
		walletID, body.Amount, ref,
	)

	// Call Paystack initialize endpoint
	payload := map[string]any{
		"email":     body.Email,
		"amount":    body.Amount,
		"reference": ref,
		"metadata": map[string]any{
			"user_id": u.ID.String(),
			"type":    "wallet_topup",
		},
	}
	payloadBytes, _ := json.Marshal(payload)

	req, _ := http.NewRequestWithContext(r.Context(), http.MethodPost,
		"https://api.paystack.co/transaction/initialize", bytes.NewReader(payloadBytes))
	req.Header.Set("Authorization", "Bearer "+h.paystackSecret)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "failed to reach Paystack")
		return
	}
	defer resp.Body.Close()

	var psResp map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&psResp)

	writeJSON(w, http.StatusOK, psResp)
}

// POST /wallet/topup/verify/:reference — verify Paystack payment
func (h *WalletHandler) VerifyTopUp(w http.ResponseWriter, r *http.Request) {
	ref := r.URL.Query().Get("reference")
	if ref == "" {
		writeErr(w, http.StatusBadRequest, "reference is required")
		return
	}

	req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet,
		"https://api.paystack.co/transaction/verify/"+ref, nil)
	req.Header.Set("Authorization", "Bearer "+h.paystackSecret)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "failed to verify with Paystack")
		return
	}
	defer resp.Body.Close()

	var psResp struct {
		Status bool `json:"status"`
		Data   struct {
			Status    string `json:"status"`
			Amount    int64  `json:"amount"`
			Reference string `json:"reference"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&psResp)

	if !psResp.Status || psResp.Data.Status != "success" {
		writeErr(w, http.StatusPaymentRequired, "payment not successful")
		return
	}

	// Credit wallet
	_, err = h.db.Exec(r.Context(),
		`UPDATE wallet_transactions SET status = 'success', paystack_ref = $2 WHERE reference = $1`,
		ref, ref,
	)

	var walletID uuid.UUID
	_ = h.db.QueryRow(r.Context(),
		`SELECT wallet_id FROM wallet_transactions WHERE reference = $1`, ref,
	).Scan(&walletID)

	_, err = h.db.Exec(r.Context(),
		`UPDATE wallets SET balance = balance + $2, updated_at = NOW() WHERE id = $1`,
		walletID, psResp.Data.Amount,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to credit wallet")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message": "wallet credited",
		"amount":  psResp.Data.Amount,
	})
}

// POST /wallet/topup/webhook — Paystack webhook
func (h *WalletHandler) PaystackWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Verify webhook signature
	sig := r.Header.Get("X-Paystack-Signature")
	mac := hmac.New(sha512.New, []byte(h.paystackSecret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	if sig != expected {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	var event struct {
		Event string `json:"event"`
		Data  struct {
			Status    string `json:"status"`
			Amount    int64  `json:"amount"`
			Reference string `json:"reference"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &event); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if event.Event == "charge.success" && event.Data.Status == "success" {
		ref := event.Data.Reference

		var walletID uuid.UUID
		err := h.db.QueryRow(r.Context(),
			`SELECT wallet_id FROM wallet_transactions WHERE reference = $1 AND status = 'pending'`, ref,
		).Scan(&walletID)

		if err == nil {
			_, _ = h.db.Exec(r.Context(),
				`UPDATE wallet_transactions SET status = 'success' WHERE reference = $1`, ref)
			_, _ = h.db.Exec(r.Context(),
				`UPDATE wallets SET balance = balance + $2, updated_at = NOW() WHERE id = $1`,
				walletID, event.Data.Amount)
		}
	}

	w.WriteHeader(http.StatusOK)
}

// POST /wallet/withdraw
func (h *WalletHandler) Withdraw(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Withdraw requires KYC tier 1+
	var kycTier string
	_ = h.db.QueryRow(r.Context(), `SELECT kyc_tier FROM users WHERE id = $1`, u.ID).Scan(&kycTier)
	if kycTier == "0" {
		writeErr(w, http.StatusForbidden, "kyc_required:tier_1")
		return
	}

	var body struct {
		Amount        int64  `json:"amount"` // in kobo
		BankCode      string `json:"bank_code"`
		AccountNumber string `json:"account_number"`
		AccountName   string `json:"account_name"`
	}
	if err := decode(r, &body); err != nil || body.Amount <= 0 {
		writeErr(w, http.StatusBadRequest, "amount, bank_code, account_number required")
		return
	}

	// Check balance
	var balance int64
	var walletID uuid.UUID
	err := h.db.QueryRow(r.Context(),
		`SELECT id, balance FROM wallets WHERE user_id = $1`, u.ID,
	).Scan(&walletID, &balance)
	if err != nil || balance < body.Amount {
		writeErr(w, http.StatusBadRequest, "insufficient balance")
		return
	}

	ref := fmt.Sprintf("EP-WDR-%s-%d", u.ID.String()[:8], time.Now().UnixMilli())

	tx, _ := h.db.Begin(r.Context())
	defer tx.Rollback(r.Context())

	_, err = tx.Exec(r.Context(),
		`UPDATE wallets SET balance = balance - $2, updated_at = NOW() WHERE id = $1`,
		walletID, body.Amount,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to debit wallet")
		return
	}

	_, err = tx.Exec(r.Context(),
		`INSERT INTO wallet_transactions (wallet_id, type, amount, status, reference, description)
		 VALUES ($1, 'withdraw', $2, 'pending', $3, $4)`,
		walletID, body.Amount, ref,
		fmt.Sprintf("Withdrawal to %s (%s)", body.AccountName, body.AccountNumber),
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to record transaction")
		return
	}

	_ = tx.Commit(r.Context())

	// In production: trigger Paystack Transfer API here
	// POST https://api.paystack.co/transfer with recipient and amount

	writeJSON(w, http.StatusAccepted, map[string]any{
		"message":   "withdrawal initiated",
		"reference": ref,
		"amount":    body.Amount,
	})
}
