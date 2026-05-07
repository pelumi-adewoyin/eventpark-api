package handlers

import (
	"net/http"
	"time"

	"github.com/eventpark/api/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type BookingExtHandler struct{ db *pgxpool.Pool }

func NewBookingExtHandler(db *pgxpool.Pool) *BookingExtHandler {
	return &BookingExtHandler{db: db}
}

type bookingRow struct {
	ID             uuid.UUID  `json:"id"`
	EventID        uuid.UUID  `json:"event_id"`
	VendorID       uuid.UUID  `json:"vendor_id"`
	ServiceID      *uuid.UUID `json:"service_id,omitempty"`
	ClientID       uuid.UUID  `json:"client_id"`
	Status         string     `json:"status"`
	TotalAmount    int64      `json:"total_amount"`
	EscrowAmount   int64      `json:"escrow_amount"`
	EscrowReleased bool       `json:"escrow_released"`
	PaymentStatus  string     `json:"payment_status"`
	QuoteStatus    *string    `json:"quote_status,omitempty"`
	Notes          *string    `json:"notes,omitempty"`
	EventDate      *time.Time `json:"event_date,omitempty"`
	ServiceDate    *string    `json:"service_date,omitempty"`
	Headcount      *int       `json:"headcount,omitempty"`
	BudgetHint     *int64     `json:"budget_hint,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	// Enriched
	VendorName *string `json:"vendor_name,omitempty"`
	EventTitle *string `json:"event_title,omitempty"`
}

// GET /bookings — list bookings where the user is client OR vendor owner
func (h *BookingExtHandler) List(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	rows, err := h.db.Query(r.Context(), `
		SELECT
			b.id, b.event_id, b.vendor_id, b.service_id, b.client_id,
			b.status, b.total_amount, b.escrow_amount, b.escrow_released,
			b.payment_status, b.quote_status, b.notes, b.event_date,
			b.service_date::text, b.headcount, b.budget_hint,
			b.created_at, b.updated_at,
			v.business_name AS vendor_name,
			e.title AS event_title
		FROM bookings b
		JOIN vendors v ON v.id = b.vendor_id
		JOIN events e ON e.id = b.event_id
		WHERE b.client_id = $1 OR v.user_id = $1
		ORDER BY b.created_at DESC
	`, u.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to fetch bookings")
		return
	}
	defer rows.Close()

	result := []bookingRow{}
	for rows.Next() {
		var b bookingRow
		if err := rows.Scan(
			&b.ID, &b.EventID, &b.VendorID, &b.ServiceID, &b.ClientID,
			&b.Status, &b.TotalAmount, &b.EscrowAmount, &b.EscrowReleased,
			&b.PaymentStatus, &b.QuoteStatus, &b.Notes, &b.EventDate,
			&b.ServiceDate, &b.Headcount, &b.BudgetHint,
			&b.CreatedAt, &b.UpdatedAt,
			&b.VendorName, &b.EventTitle,
		); err == nil {
			result = append(result, b)
		}
	}
	writeJSON(w, http.StatusOK, result)
}

// GET /bookings/{id} — get a single booking with vendor info and latest quote
func (h *BookingExtHandler) Get(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	bookingID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid booking id")
		return
	}

	var b bookingRow
	err = h.db.QueryRow(r.Context(), `
		SELECT
			b.id, b.event_id, b.vendor_id, b.service_id, b.client_id,
			b.status, b.total_amount, b.escrow_amount, b.escrow_released,
			b.payment_status, b.quote_status, b.notes, b.event_date,
			b.service_date::text, b.headcount, b.budget_hint,
			b.created_at, b.updated_at,
			v.business_name AS vendor_name,
			e.title AS event_title
		FROM bookings b
		JOIN vendors v ON v.id = b.vendor_id
		JOIN events e ON e.id = b.event_id
		WHERE b.id = $1 AND (b.client_id = $2 OR v.user_id = $2)
	`, bookingID, u.ID).Scan(
		&b.ID, &b.EventID, &b.VendorID, &b.ServiceID, &b.ClientID,
		&b.Status, &b.TotalAmount, &b.EscrowAmount, &b.EscrowReleased,
		&b.PaymentStatus, &b.QuoteStatus, &b.Notes, &b.EventDate,
		&b.ServiceDate, &b.Headcount, &b.BudgetHint,
		&b.CreatedAt, &b.UpdatedAt,
		&b.VendorName, &b.EventTitle,
	)
	if err != nil {
		writeErr(w, http.StatusNotFound, "booking not found")
		return
	}

	// Enrich with latest quote if available
	type quoteInfo struct {
		ID          uuid.UUID `json:"id"`
		QuoteType   string    `json:"quote_type"`
		LineItems   []byte    `json:"line_items"`
		SubtotalNGN int64     `json:"subtotal_ngn"`
		VATNGN      int64     `json:"vat_ngn"`
		TotalNGN    int64     `json:"total_ngn"`
		Notes       *string   `json:"notes,omitempty"`
		ValidUntil  *string   `json:"valid_until,omitempty"`
		CreatedAt   time.Time `json:"created_at"`
	}

	type response struct {
		bookingRow
		LatestQuote *quoteInfo `json:"latest_quote,omitempty"`
	}

	resp := response{bookingRow: b}

	var q quoteInfo
	var validUntil *time.Time
	qErr := h.db.QueryRow(r.Context(), `
		SELECT id, quote_type, line_items, subtotal_ngn, vat_ngn, total_ngn, notes, valid_until, created_at
		FROM booking_quotes
		WHERE booking_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`, bookingID).Scan(
		&q.ID, &q.QuoteType, &q.LineItems, &q.SubtotalNGN, &q.VATNGN, &q.TotalNGN,
		&q.Notes, &validUntil, &q.CreatedAt,
	)
	if qErr == nil {
		if validUntil != nil {
			s := validUntil.Format(time.RFC3339)
			q.ValidUntil = &s
		}
		resp.LatestQuote = &q
	}

	writeJSON(w, http.StatusOK, resp)
}

// POST /bookings/{id}/quote/respond — accept or reject a quote
func (h *BookingExtHandler) RespondQuote(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	bookingID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid booking id")
		return
	}

	var body struct {
		Action string  `json:"action"` // accept | reject | counter
		Notes  *string `json:"notes"`
	}
	if err := decode(r, &body); err != nil || (body.Action != "accept" && body.Action != "reject" && body.Action != "counter") {
		writeErr(w, http.StatusBadRequest, "action must be 'accept', 'reject', or 'counter'")
		return
	}

	// Verify the caller is the client
	var clientID uuid.UUID
	if err := h.db.QueryRow(r.Context(), `SELECT client_id FROM bookings WHERE id = $1`, bookingID).Scan(&clientID); err != nil {
		writeErr(w, http.StatusNotFound, "booking not found")
		return
	}
	if clientID != u.ID {
		writeErr(w, http.StatusForbidden, "only the client can respond to a quote")
		return
	}

	quoteStatus := body.Action + "ed" // accepted | rejected | countered
	newStatus := "pending"
	if body.Action == "accept" {
		newStatus = "confirmed"
	}

	_, err = h.db.Exec(r.Context(), `
		UPDATE bookings SET quote_status = $2, status = $3, updated_at = NOW() WHERE id = $1
	`, bookingID, quoteStatus, newStatus)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to update booking")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"message":      "quote " + quoteStatus,
		"quote_status": quoteStatus,
		"status":       newStatus,
	})
}

// POST /bookings/{id}/pay — record payment intent / mark as paid
func (h *BookingExtHandler) Pay(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	bookingID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid booking id")
		return
	}

	var body struct {
		Reference string `json:"reference"` // payment reference from gateway
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Verify the caller is the client
	var clientID uuid.UUID
	if err := h.db.QueryRow(r.Context(), `SELECT client_id FROM bookings WHERE id = $1`, bookingID).Scan(&clientID); err != nil {
		writeErr(w, http.StatusNotFound, "booking not found")
		return
	}
	if clientID != u.ID {
		writeErr(w, http.StatusForbidden, "only the client can record payment")
		return
	}

	_, err = h.db.Exec(r.Context(), `
		UPDATE bookings SET payment_status = 'paid', status = 'confirmed', updated_at = NOW() WHERE id = $1
	`, bookingID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to update payment status")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"message":        "payment recorded",
		"payment_status": "paid",
	})
}
