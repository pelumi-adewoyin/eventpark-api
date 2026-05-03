package handlers

import (
	"net/http"

	"github.com/eventpark/api/internal/middleware"
	"github.com/eventpark/api/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type VendorsHandler struct {
	db *pgxpool.Pool
}

func NewVendorsHandler(db *pgxpool.Pool) *VendorsHandler {
	return &VendorsHandler{db: db}
}

// POST /vendors — create vendor profile
func (h *VendorsHandler) CreateVendor(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var body struct {
		BusinessName string  `json:"business_name"`
		Category     string  `json:"category"`
		Bio          *string `json:"bio"`
		City         *string `json:"city"`
		State        *string `json:"state"`
	}
	if err := decode(r, &body); err != nil || body.BusinessName == "" || body.Category == "" {
		writeErr(w, http.StatusBadRequest, "business_name and category are required")
		return
	}

	var vendor models.Vendor
	err := h.db.QueryRow(r.Context(),
		`INSERT INTO vendors (user_id, business_name, category, bio, city, state)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (user_id) DO UPDATE SET
		   business_name = EXCLUDED.business_name,
		   category = EXCLUDED.category,
		   bio = COALESCE(EXCLUDED.bio, vendors.bio),
		   city = COALESCE(EXCLUDED.city, vendors.city),
		   state = COALESCE(EXCLUDED.state, vendors.state)
		 RETURNING id, user_id, business_name, category, bio, city, state, avatar_url, cover_url, rating, review_count, verified, created_at`,
		u.ID, body.BusinessName, body.Category, body.Bio, body.City, body.State,
	).Scan(
		&vendor.ID, &vendor.UserID, &vendor.BusinessName, &vendor.Category,
		&vendor.Bio, &vendor.City, &vendor.State, &vendor.AvatarURL, &vendor.CoverURL,
		&vendor.Rating, &vendor.ReviewCount, &vendor.Verified, &vendor.CreatedAt,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to create vendor profile")
		return
	}
	writeJSON(w, http.StatusCreated, vendor)
}

// GET /vendors/:id
func (h *VendorsHandler) GetVendor(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid vendor id")
		return
	}

	var vendor models.Vendor
	err = h.db.QueryRow(r.Context(),
		`SELECT id, user_id, business_name, category, bio, city, state, avatar_url, cover_url, rating, review_count, verified, created_at
		 FROM vendors WHERE id = $1`, id,
	).Scan(
		&vendor.ID, &vendor.UserID, &vendor.BusinessName, &vendor.Category,
		&vendor.Bio, &vendor.City, &vendor.State, &vendor.AvatarURL, &vendor.CoverURL,
		&vendor.Rating, &vendor.ReviewCount, &vendor.Verified, &vendor.CreatedAt,
	)
	if err != nil {
		writeErr(w, http.StatusNotFound, "vendor not found")
		return
	}

	// Fetch services
	svcRows, _ := h.db.Query(r.Context(),
		`SELECT id, vendor_id, name, description, price_from, price_to, unit, created_at FROM vendor_services WHERE vendor_id = $1`,
		id,
	)
	defer svcRows.Close()
	for svcRows.Next() {
		var s models.VendorService
		if err := svcRows.Scan(&s.ID, &s.VendorID, &s.Name, &s.Description, &s.PriceFrom, &s.PriceTo, &s.Unit, &s.CreatedAt); err == nil {
			vendor.Services = append(vendor.Services, s)
		}
	}

	// Fetch portfolio
	portRows, _ := h.db.Query(r.Context(),
		`SELECT id, vendor_id, image_url, caption, created_at FROM vendor_portfolio WHERE vendor_id = $1 LIMIT 20`,
		id,
	)
	defer portRows.Close()
	for portRows.Next() {
		var p models.VendorPortfolio
		if err := portRows.Scan(&p.ID, &p.VendorID, &p.ImageURL, &p.Caption, &p.CreatedAt); err == nil {
			vendor.Portfolio = append(vendor.Portfolio, p)
		}
	}

	writeJSON(w, http.StatusOK, vendor)
}

// POST /vendors/:id/services
func (h *VendorsHandler) AddService(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	vendorID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil || u == nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}

	// Verify ownership
	var ownerID uuid.UUID
	_ = h.db.QueryRow(r.Context(), `SELECT user_id FROM vendors WHERE id = $1`, vendorID).Scan(&ownerID)
	if ownerID != u.ID {
		writeErr(w, http.StatusForbidden, "not your vendor profile")
		return
	}

	var body struct {
		Name        string  `json:"name"`
		Description *string `json:"description"`
		PriceFrom   int64   `json:"price_from"`
		PriceTo     *int64  `json:"price_to"`
		Unit        *string `json:"unit"`
	}
	if err := decode(r, &body); err != nil || body.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}

	var svc models.VendorService
	err = h.db.QueryRow(r.Context(),
		`INSERT INTO vendor_services (vendor_id, name, description, price_from, price_to, unit)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, vendor_id, name, description, price_from, price_to, unit, created_at`,
		vendorID, body.Name, body.Description, body.PriceFrom, body.PriceTo, body.Unit,
	).Scan(&svc.ID, &svc.VendorID, &svc.Name, &svc.Description, &svc.PriceFrom, &svc.PriceTo, &svc.Unit, &svc.CreatedAt)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to add service")
		return
	}
	writeJSON(w, http.StatusCreated, svc)
}

// POST /bookings — request a vendor booking
func (h *VendorsHandler) CreateBooking(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var body struct {
		EventID     uuid.UUID  `json:"event_id"`
		VendorID    uuid.UUID  `json:"vendor_id"`
		ServiceID   *uuid.UUID `json:"service_id"`
		TotalAmount int64      `json:"total_amount"` // kobo
		Notes       *string    `json:"notes"`
		EventDate   *string    `json:"event_date"`
	}
	if err := decode(r, &body); err != nil || body.TotalAmount <= 0 {
		writeErr(w, http.StatusBadRequest, "event_id, vendor_id and total_amount required")
		return
	}

	// 50% escrow hold
	escrowAmount := body.TotalAmount / 2

	// Deduct escrow from wallet
	var walletID uuid.UUID
	var balance int64
	err := h.db.QueryRow(r.Context(),
		`SELECT id, balance FROM wallets WHERE user_id = $1`, u.ID,
	).Scan(&walletID, &balance)
	if err != nil || balance < escrowAmount {
		writeErr(w, http.StatusBadRequest, "insufficient wallet balance for escrow")
		return
	}

	tx, _ := h.db.Begin(r.Context())
	defer tx.Rollback(r.Context())

	_, err = tx.Exec(r.Context(),
		`UPDATE wallets SET balance = balance - $2, escrow_held = escrow_held + $2, updated_at = NOW() WHERE id = $1`,
		walletID, escrowAmount,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "escrow deduction failed")
		return
	}

	var booking models.Booking
	err = tx.QueryRow(r.Context(),
		`INSERT INTO bookings (event_id, vendor_id, service_id, client_id, total_amount, escrow_amount, notes, event_date)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8::timestamptz)
		 RETURNING id, event_id, vendor_id, service_id, client_id, status, total_amount, escrow_amount, escrow_released, notes, event_date, created_at, updated_at`,
		body.EventID, body.VendorID, body.ServiceID, u.ID, body.TotalAmount, escrowAmount, body.Notes, body.EventDate,
	).Scan(
		&booking.ID, &booking.EventID, &booking.VendorID, &booking.ServiceID,
		&booking.ClientID, &booking.Status, &booking.TotalAmount, &booking.EscrowAmount,
		&booking.EscrowReleased, &booking.Notes, &booking.EventDate, &booking.CreatedAt, &booking.UpdatedAt,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to create booking")
		return
	}

	// Record escrow transaction
	_, _ = tx.Exec(r.Context(),
		`INSERT INTO wallet_transactions (wallet_id, type, amount, status, description, booking_id)
		 VALUES ($1, 'escrow_hold', $2, 'success', 'Escrow held for vendor booking', $3)`,
		walletID, escrowAmount, booking.ID,
	)

	_ = tx.Commit(r.Context())
	writeJSON(w, http.StatusCreated, booking)
}

// POST /bookings/:id/release-escrow — release after check-in confirmation
func (h *VendorsHandler) ReleaseEscrow(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	bookingID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil || u == nil {
		writeErr(w, http.StatusBadRequest, "invalid booking id")
		return
	}

	var booking models.Booking
	err = h.db.QueryRow(r.Context(),
		`SELECT id, vendor_id, client_id, escrow_amount, escrow_released FROM bookings WHERE id = $1`,
		bookingID,
	).Scan(&booking.ID, &booking.VendorID, &booking.ClientID, &booking.EscrowAmount, &booking.EscrowReleased)
	if err != nil {
		writeErr(w, http.StatusNotFound, "booking not found")
		return
	}

	if booking.EscrowReleased {
		writeErr(w, http.StatusConflict, "escrow already released")
		return
	}
	if booking.ClientID != u.ID {
		writeErr(w, http.StatusForbidden, "only the client can release escrow")
		return
	}

	// Release escrow: credit vendor's wallet
	tx, _ := h.db.Begin(r.Context())
	defer tx.Rollback(r.Context())

	// Get vendor's wallet
	var vendorUserID uuid.UUID
	_ = h.db.QueryRow(r.Context(), `SELECT user_id FROM vendors WHERE id = $1`, booking.VendorID).Scan(&vendorUserID)

	var vendorWalletID uuid.UUID
	_ = h.db.QueryRow(r.Context(), `SELECT id FROM wallets WHERE user_id = $1`, vendorUserID).Scan(&vendorWalletID)

	// Deduct from client escrow_held, credit vendor balance
	var clientWalletID uuid.UUID
	_ = h.db.QueryRow(r.Context(), `SELECT id FROM wallets WHERE user_id = $1`, booking.ClientID).Scan(&clientWalletID)

	_, _ = tx.Exec(r.Context(),
		`UPDATE wallets SET escrow_held = escrow_held - $2, updated_at = NOW() WHERE id = $1`,
		clientWalletID, booking.EscrowAmount,
	)
	_, _ = tx.Exec(r.Context(),
		`UPDATE wallets SET balance = balance + $2, updated_at = NOW() WHERE id = $1`,
		vendorWalletID, booking.EscrowAmount,
	)
	_, _ = tx.Exec(r.Context(),
		`UPDATE bookings SET escrow_released = true, status = 'completed', updated_at = NOW() WHERE id = $1`,
		bookingID,
	)
	_, _ = tx.Exec(r.Context(),
		`INSERT INTO wallet_transactions (wallet_id, type, amount, status, description, booking_id)
		 VALUES ($1, 'escrow_release', $2, 'success', 'Escrow released to vendor', $3)`,
		vendorWalletID, booking.EscrowAmount, bookingID,
	)

	_ = tx.Commit(r.Context())
	writeJSON(w, http.StatusOK, map[string]string{"message": "escrow released to vendor"})
}
