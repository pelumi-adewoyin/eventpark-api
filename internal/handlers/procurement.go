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

type ProcurementHandler struct {
	db *pgxpool.Pool
}

func NewProcurementHandler(db *pgxpool.Pool) *ProcurementHandler {
	return &ProcurementHandler{db: db}
}

// ─── PURCHASE ORDERS ──────────────────────────────────────────────────────────

// GET /orgs/:orgId/pos
func (h *ProcurementHandler) ListPOs(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(chi.URLParam(r, "orgId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid org id")
		return
	}

	rows, err := h.db.Query(r.Context(),
		`SELECT po.id, po.org_id, po.event_id, po.budget_line_id, po.vendor_id,
		   po.ref, po.scope, po.amount, po.currency, po.status,
		   po.issued_at::text, po.due_at::text,
		   po.created_by, po.approved_by, po.approved_at,
		   po.created_at, po.updated_at,
		   cv.name AS vendor_name,
		   EXISTS(SELECT 1 FROM goods_receipts gr WHERE gr.po_id = po.id) AS gr_done
		 FROM purchase_orders po
		 JOIN corp_vendors cv ON cv.id = po.vendor_id
		 WHERE po.org_id = $1
		 ORDER BY po.created_at DESC`,
		orgID,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to list POs")
		return
	}
	defer rows.Close()

	pos := []models.PurchaseOrder{}
	for rows.Next() {
		var po models.PurchaseOrder
		if err := rows.Scan(
			&po.ID, &po.OrgID, &po.EventID, &po.BudgetLineID, &po.VendorID,
			&po.Ref, &po.Scope, &po.Amount, &po.Currency, &po.Status,
			&po.IssuedAt, &po.DueAt,
			&po.CreatedBy, &po.ApprovedBy, &po.ApprovedAt,
			&po.CreatedAt, &po.UpdatedAt,
			&po.VendorName, &po.GRDone,
		); err == nil {
			pos = append(pos, po)
		}
	}
	writeJSON(w, http.StatusOK, pos)
}

// GET /orgs/:orgId/pos/:id
func (h *ProcurementHandler) GetPO(w http.ResponseWriter, r *http.Request) {
	orgID, _ := uuid.Parse(chi.URLParam(r, "orgId"))
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid po id")
		return
	}

	var po models.PurchaseOrder
	err = h.db.QueryRow(r.Context(),
		`SELECT po.id, po.org_id, po.event_id, po.budget_line_id, po.vendor_id,
		   po.ref, po.scope, po.amount, po.currency, po.status,
		   po.issued_at::text, po.due_at::text,
		   po.created_by, po.approved_by, po.approved_at,
		   po.created_at, po.updated_at,
		   cv.name AS vendor_name,
		   EXISTS(SELECT 1 FROM goods_receipts gr WHERE gr.po_id = po.id) AS gr_done
		 FROM purchase_orders po
		 JOIN corp_vendors cv ON cv.id = po.vendor_id
		 WHERE po.id = $1 AND po.org_id = $2`,
		id, orgID,
	).Scan(
		&po.ID, &po.OrgID, &po.EventID, &po.BudgetLineID, &po.VendorID,
		&po.Ref, &po.Scope, &po.Amount, &po.Currency, &po.Status,
		&po.IssuedAt, &po.DueAt,
		&po.CreatedBy, &po.ApprovedBy, &po.ApprovedAt,
		&po.CreatedAt, &po.UpdatedAt,
		&po.VendorName, &po.GRDone,
	)
	if err != nil {
		writeErr(w, http.StatusNotFound, "PO not found")
		return
	}

	// Fetch line items
	rows, _ := h.db.Query(r.Context(),
		`SELECT id, po_id, description, quantity, unit, unit_price, total
		 FROM po_line_items WHERE po_id = $1`, id,
	)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var li models.POLineItem
			if err := rows.Scan(
				&li.ID, &li.POID, &li.Description, &li.Quantity,
				&li.Unit, &li.UnitPrice, &li.Total,
			); err == nil {
				po.LineItems = append(po.LineItems, li)
			}
		}
	}

	writeJSON(w, http.StatusOK, po)
}

// POST /orgs/:orgId/pos
func (h *ProcurementHandler) CreatePO(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	orgID, err := uuid.Parse(chi.URLParam(r, "orgId"))
	if err != nil || u == nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}

	var body struct {
		VendorID     uuid.UUID  `json:"vendor_id"`
		EventID      *uuid.UUID `json:"event_id"`
		BudgetLineID *uuid.UUID `json:"budget_line_id"`
		Scope        *string    `json:"scope"`
		Amount       int64      `json:"amount"`
		IssuedAt     *string    `json:"issued_at"`
		DueAt        *string    `json:"due_at"`
		LineItems    []struct {
			Description string  `json:"description"`
			Quantity    float64 `json:"quantity"`
			Unit        *string `json:"unit"`
			UnitPrice   int64   `json:"unit_price"`
		} `json:"line_items"`
	}
	if err := decode(r, &body); err != nil || body.Amount == 0 {
		writeErr(w, http.StatusBadRequest, "vendor_id and amount are required")
		return
	}

	// Generate ref
	var seq int
	_ = h.db.QueryRow(r.Context(), `SELECT nextval('po_ref_seq')`).Scan(&seq)
	ref := fmt.Sprintf("PO-%d-%04d", 2026, seq)

	var po models.PurchaseOrder
	err = h.db.QueryRow(r.Context(),
		`INSERT INTO purchase_orders
		   (org_id, event_id, budget_line_id, vendor_id, ref, scope, amount, issued_at, due_at, created_by, status)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'pending_approval')
		 RETURNING id, org_id, event_id, budget_line_id, vendor_id, ref, scope, amount, currency,
		   status, issued_at::text, due_at::text, created_by, approved_by, approved_at, created_at, updated_at`,
		orgID, body.EventID, body.BudgetLineID, body.VendorID, ref, body.Scope,
		body.Amount, body.IssuedAt, body.DueAt, u.ID,
	).Scan(
		&po.ID, &po.OrgID, &po.EventID, &po.BudgetLineID, &po.VendorID,
		&po.Ref, &po.Scope, &po.Amount, &po.Currency, &po.Status,
		&po.IssuedAt, &po.DueAt,
		&po.CreatedBy, &po.ApprovedBy, &po.ApprovedAt,
		&po.CreatedAt, &po.UpdatedAt,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to create PO")
		return
	}

	// Insert line items
	for _, li := range body.LineItems {
		total := int64(li.Quantity * float64(li.UnitPrice))
		_, _ = h.db.Exec(r.Context(),
			`INSERT INTO po_line_items (po_id, description, quantity, unit, unit_price, total)
			 VALUES ($1,$2,$3,$4,$5,$6)`,
			po.ID, li.Description, li.Quantity, li.Unit, li.UnitPrice, total,
		)
	}

	// Auto-submit approval request
	var vendorName string
	_ = h.db.QueryRow(r.Context(), `SELECT name FROM corp_vendors WHERE id = $1`, body.VendorID).Scan(&vendorName)

	_, _ = h.db.Exec(r.Context(),
		`INSERT INTO approval_requests (org_id, event_id, item_type, item_id, item_ref, title, amount, requested_by, urgency)
		 VALUES ($1,$2,'po',$3,$4,$5,$6,$7,'medium')`,
		orgID, body.EventID, po.ID, ref,
		fmt.Sprintf("%s – %s", ref, vendorName),
		body.Amount, u.ID,
	)

	writeAuditLog(r.Context(), h.db, orgID, u.ID, "PO Created", "purchase_order", po.ID, ref)
	writeJSON(w, http.StatusCreated, po)
}

// ─── GOODS RECEIPTS ───────────────────────────────────────────────────────────

// POST /orgs/:orgId/pos/:id/goods-receipt
func (h *ProcurementHandler) RecordGoodsReceipt(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	orgID, _ := uuid.Parse(chi.URLParam(r, "orgId"))
	poID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil || u == nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}

	var body struct {
		ReceivedAt string  `json:"received_at"`
		Notes      *string `json:"notes"`
		Partial    bool    `json:"partial"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.ReceivedAt == "" {
		body.ReceivedAt = "today"
	}

	var gr models.GoodsReceipt
	err = h.db.QueryRow(r.Context(),
		`INSERT INTO goods_receipts (po_id, received_by, received_at, notes, partial)
		 VALUES ($1, $2, COALESCE($3::date, CURRENT_DATE), $4, $5)
		 RETURNING id, po_id, received_by, received_at::text, notes, partial, created_at`,
		poID, u.ID, body.ReceivedAt, body.Notes, body.Partial,
	).Scan(
		&gr.ID, &gr.POID, &gr.ReceivedBy, &gr.ReceivedAt,
		&gr.Notes, &gr.Partial, &gr.CreatedAt,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to record goods receipt")
		return
	}

	// Re-run 3-way match for any invoice linked to this PO
	h.runThreeWayMatch(r, poID, orgID)

	writeAuditLog(r.Context(), h.db, orgID, u.ID, "Goods Receipt Recorded", "goods_receipt", poID, "")
	writeJSON(w, http.StatusCreated, gr)
}

// ─── INVOICES ─────────────────────────────────────────────────────────────────

// GET /orgs/:orgId/invoices
func (h *ProcurementHandler) ListInvoices(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(chi.URLParam(r, "orgId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid org id")
		return
	}

	rows, err := h.db.Query(r.Context(),
		`SELECT vi.id, vi.org_id, vi.po_id, vi.vendor_id, vi.ref,
		   vi.amount, vi.vat_amount, vi.wht_amount, vi.net_payable, vi.currency,
		   vi.invoice_date::text, vi.due_date::text,
		   vi.status, vi.match_status, vi.paid_at,
		   vi.uploaded_by, vi.approved_by, vi.created_at, vi.updated_at,
		   cv.name AS vendor_name, po.ref AS po_ref,
		   EXISTS(SELECT 1 FROM goods_receipts gr WHERE gr.po_id = vi.po_id) AS gr_exists
		 FROM vendor_invoices vi
		 JOIN corp_vendors cv ON cv.id = vi.vendor_id
		 LEFT JOIN purchase_orders po ON po.id = vi.po_id
		 WHERE vi.org_id = $1
		 ORDER BY vi.created_at DESC`,
		orgID,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to list invoices")
		return
	}
	defer rows.Close()

	invoices := []models.VendorInvoice{}
	for rows.Next() {
		var inv models.VendorInvoice
		if err := rows.Scan(
			&inv.ID, &inv.OrgID, &inv.POID, &inv.VendorID, &inv.Ref,
			&inv.Amount, &inv.VATAmount, &inv.WHTAmount, &inv.NetPayable, &inv.Currency,
			&inv.InvoiceDate, &inv.DueDate,
			&inv.Status, &inv.MatchStatus, &inv.PaidAt,
			&inv.UploadedBy, &inv.ApprovedBy, &inv.CreatedAt, &inv.UpdatedAt,
			&inv.VendorName, &inv.PORef, &inv.GRExists,
		); err == nil {
			invoices = append(invoices, inv)
		}
	}
	writeJSON(w, http.StatusOK, invoices)
}

// POST /orgs/:orgId/invoices — upload/create invoice
func (h *ProcurementHandler) CreateInvoice(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	orgID, err := uuid.Parse(chi.URLParam(r, "orgId"))
	if err != nil || u == nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}

	var body struct {
		POID        *uuid.UUID `json:"po_id"`
		VendorID    uuid.UUID  `json:"vendor_id"`
		Ref         string     `json:"ref"`
		Amount      int64      `json:"amount"`    // gross, in kobo
		WHTRate     float64    `json:"wht_rate"`  // 0.05 or 0.10
		InvoiceDate string     `json:"invoice_date"`
		DueDate     *string    `json:"due_date"`
	}
	if err := decode(r, &body); err != nil || body.Amount == 0 || body.Ref == "" {
		writeErr(w, http.StatusBadRequest, "vendor_id, ref, and amount are required")
		return
	}

	// Compute VAT (7.5%) and WHT
	vatAmount := int64(float64(body.Amount) * 0.075)
	if body.WHTRate == 0 {
		body.WHTRate = 0.05
	}
	whtAmount := int64(float64(body.Amount) * body.WHTRate)
	netPayable := body.Amount + vatAmount - whtAmount

	var inv models.VendorInvoice
	err = h.db.QueryRow(r.Context(),
		`INSERT INTO vendor_invoices
		   (org_id, po_id, vendor_id, ref, amount, vat_amount, wht_amount, net_payable,
		    invoice_date, due_date, uploaded_by)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,
		   COALESCE($9::date, CURRENT_DATE), $10::date, $11)
		 RETURNING id, org_id, po_id, vendor_id, ref,
		   amount, vat_amount, wht_amount, net_payable, currency,
		   invoice_date::text, due_date::text,
		   status, match_status, paid_at, uploaded_by, approved_by, created_at, updated_at`,
		orgID, body.POID, body.VendorID, body.Ref, body.Amount,
		vatAmount, whtAmount, netPayable,
		body.InvoiceDate, body.DueDate, u.ID,
	).Scan(
		&inv.ID, &inv.OrgID, &inv.POID, &inv.VendorID, &inv.Ref,
		&inv.Amount, &inv.VATAmount, &inv.WHTAmount, &inv.NetPayable, &inv.Currency,
		&inv.InvoiceDate, &inv.DueDate,
		&inv.Status, &inv.MatchStatus, &inv.PaidAt,
		&inv.UploadedBy, &inv.ApprovedBy, &inv.CreatedAt, &inv.UpdatedAt,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to create invoice")
		return
	}

	// Run 3-way match if PO supplied
	if body.POID != nil {
		h.runThreeWayMatch(r, *body.POID, orgID)
		// Re-fetch match_status
		_ = h.db.QueryRow(r.Context(),
			`SELECT match_status FROM vendor_invoices WHERE id = $1`, inv.ID,
		).Scan(&inv.MatchStatus)
	}

	// Submit approval request
	_, _ = h.db.Exec(r.Context(),
		`INSERT INTO approval_requests
		   (org_id, item_type, item_id, item_ref, title, amount, requested_by, urgency)
		 VALUES ($1,'invoice',$2,$3,$4,$5,$6,'medium')`,
		orgID, inv.ID, body.Ref,
		fmt.Sprintf("Invoice %s", body.Ref),
		body.Amount, u.ID,
	)

	writeAuditLog(r.Context(), h.db, orgID, u.ID, "Invoice Uploaded", "vendor_invoice", inv.ID, body.Ref)
	writeJSON(w, http.StatusCreated, inv)
}

// POST /orgs/:orgId/invoices/:id/pay
func (h *ProcurementHandler) PayInvoice(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	orgID, _ := uuid.Parse(chi.URLParam(r, "orgId"))
	invID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil || u == nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}

	// Fetch invoice
	var inv models.VendorInvoice
	err = h.db.QueryRow(r.Context(),
		`SELECT id, org_id, net_payable, status, match_status FROM vendor_invoices WHERE id = $1 AND org_id = $2`,
		invID, orgID,
	).Scan(&inv.ID, &inv.OrgID, &inv.NetPayable, &inv.Status, &inv.MatchStatus)
	if err != nil {
		writeErr(w, http.StatusNotFound, "invoice not found")
		return
	}
	if inv.MatchStatus != "matched" {
		writeErr(w, http.StatusUnprocessableEntity, "invoice must be 3-way matched before payment")
		return
	}
	if inv.Status == "paid" {
		writeErr(w, http.StatusConflict, "invoice already paid")
		return
	}

	// Debit org wallet
	var walletID uuid.UUID
	var balance int64
	err = h.db.QueryRow(r.Context(),
		`SELECT id, balance FROM org_wallets WHERE org_id = $1`, orgID,
	).Scan(&walletID, &balance)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "org wallet not found")
		return
	}
	if balance < inv.NetPayable {
		writeErr(w, http.StatusUnprocessableEntity, "insufficient wallet balance")
		return
	}

	newBalance := balance - inv.NetPayable
	_, err = h.db.Exec(r.Context(),
		`UPDATE org_wallets SET balance = $2, updated_at = NOW() WHERE id = $1`,
		walletID, newBalance,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to debit wallet")
		return
	}

	// Record wallet transaction
	_, _ = h.db.Exec(r.Context(),
		`INSERT INTO org_wallet_transactions
		   (org_wallet_id, type, amount, balance_after, description, invoice_id, initiated_by)
		 VALUES ($1, 'debit', $2, $3, $4, $5, $6)`,
		walletID, inv.NetPayable, newBalance,
		fmt.Sprintf("Invoice payment – %s", inv.Ref),
		invID, u.ID,
	)

	// Mark invoice paid
	_, _ = h.db.Exec(r.Context(),
		`UPDATE vendor_invoices SET status = 'paid', paid_at = NOW(), updated_at = NOW() WHERE id = $1`,
		invID,
	)

	// Update vendor total_paid
	_, _ = h.db.Exec(r.Context(),
		`UPDATE corp_vendors SET total_paid = total_paid + $2, updated_at = NOW()
		 WHERE id = (SELECT vendor_id FROM vendor_invoices WHERE id = $1)`,
		invID, inv.NetPayable,
	)

	writeAuditLog(r.Context(), h.db, orgID, u.ID, "Invoice Paid", "vendor_invoice", invID,
		fmt.Sprintf("₦%d paid", inv.NetPayable))

	writeJSON(w, http.StatusOK, map[string]any{
		"message":       "payment processed",
		"amount_paid":   inv.NetPayable,
		"wallet_balance": newBalance,
	})
}

// ─── 3-WAY MATCH ──────────────────────────────────────────────────────────────

// runThreeWayMatch checks PO + GR + Invoice alignment and updates match_status
func (h *ProcurementHandler) runThreeWayMatch(r *http.Request, poID uuid.UUID, orgID uuid.UUID) {
	// Check if GR exists for this PO
	var grExists bool
	_ = h.db.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM goods_receipts WHERE po_id = $1)`, poID,
	).Scan(&grExists)

	// Check invoice against PO amount (within 5% tolerance)
	var poAmount, invAmount int64
	_ = h.db.QueryRow(r.Context(),
		`SELECT amount FROM purchase_orders WHERE id = $1`, poID,
	).Scan(&poAmount)
	_ = h.db.QueryRow(r.Context(),
		`SELECT COALESCE(SUM(amount), 0) FROM vendor_invoices WHERE po_id = $1 AND status != 'cancelled'`, poID,
	).Scan(&invAmount)

	var matchStatus string
	switch {
	case !grExists:
		matchStatus = "unmatched" // no GR yet
	case invAmount == 0:
		matchStatus = "unmatched" // no invoice yet
	case poAmount > 0 && abs64(invAmount-poAmount)*100/poAmount <= 5:
		matchStatus = "matched" // within 5% tolerance
	default:
		matchStatus = "partial"
	}

	_, _ = h.db.Exec(r.Context(),
		`UPDATE vendor_invoices SET match_status = $2, updated_at = NOW() WHERE po_id = $1`,
		poID, matchStatus,
	)

	if matchStatus == "matched" {
		writeAuditLog(r.Context(), h.db, orgID, nil, "3-Way Match Passed", "purchase_order", poID, "PO + GR + Invoice aligned")
	}
}

// GET /orgs/:orgId/invoices/:id/match — get match status details
func (h *ProcurementHandler) GetMatchStatus(w http.ResponseWriter, r *http.Request) {
	orgID, _ := uuid.Parse(chi.URLParam(r, "orgId"))
	invID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid invoice id")
		return
	}

	var inv models.VendorInvoice
	err = h.db.QueryRow(r.Context(),
		`SELECT vi.id, vi.po_id, vi.amount, vi.match_status,
		   po.ref AS po_ref, po.amount AS po_amount,
		   EXISTS(SELECT 1 FROM goods_receipts gr WHERE gr.po_id = vi.po_id) AS gr_exists
		 FROM vendor_invoices vi
		 LEFT JOIN purchase_orders po ON po.id = vi.po_id
		 WHERE vi.id = $1 AND vi.org_id = $2`,
		invID, orgID,
	).Scan(&inv.ID, &inv.POID, &inv.Amount, &inv.MatchStatus, &inv.PORef, &inv.VATAmount, &inv.GRExists)
	if err != nil {
		writeErr(w, http.StatusNotFound, "invoice not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"invoice_id":   inv.ID,
		"po_ref":       inv.PORef,
		"po_matched":   inv.POID != nil,
		"gr_exists":    inv.GRExists,
		"amount_match": inv.MatchStatus == "matched",
		"match_status": inv.MatchStatus,
	})
}

func abs64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}
