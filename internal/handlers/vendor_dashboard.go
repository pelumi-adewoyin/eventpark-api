package handlers

import (
	"net/http"

	"github.com/eventpark/api/internal/middleware"
	"github.com/eventpark/api/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// VendorDashboardHandler handles all /vendor/* routes — endpoints only accessible
// to users whose role='vendor' and who have an associated vendors record.
type VendorDashboardHandler struct {
	db *pgxpool.Pool
}

func NewVendorDashboardHandler(db *pgxpool.Pool) *VendorDashboardHandler {
	return &VendorDashboardHandler{db: db}
}

// resolveVendorID looks up the vendors.id for the authenticated user.
// Returns uuid.Nil and writes an error response if not found.
func (h *VendorDashboardHandler) resolveVendorID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return uuid.Nil, false
	}
	var vendorID uuid.UUID
	err := h.db.QueryRow(r.Context(), `SELECT id FROM vendors WHERE user_id = $1`, u.ID).Scan(&vendorID)
	if err != nil {
		writeErr(w, http.StatusForbidden, "no vendor profile found — complete vendor registration first")
		return uuid.Nil, false
	}
	return vendorID, true
}

// ─── VENDOR PROFILE ───────────────────────────────────────────────────────────

// GET /vendor/me
func (h *VendorDashboardHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	vendorID, ok := h.resolveVendorID(w, r)
	if !ok {
		return
	}

	var v models.Vendor
	err := h.db.QueryRow(r.Context(),
		`SELECT id, user_id, business_name, category, bio, city, state,
		        avatar_url, cover_url, rating, review_count, verified,
		        vendor_type, verification_status, verification_tier, is_registered,
		        cac_rc_number, address, postal_code,
		        tagline, highlight_1, highlight_2, highlight_3,
		        years_experience, events_completed, website, instagram, twitter, whatsapp,
		        created_at, updated_at
		 FROM vendors WHERE id = $1`, vendorID,
	).Scan(
		&v.ID, &v.UserID, &v.BusinessName, &v.Category, &v.Bio, &v.City, &v.State,
		&v.AvatarURL, &v.CoverURL, &v.Rating, &v.ReviewCount, &v.Verified,
		&v.VendorType, &v.VerificationStatus, &v.VerificationTier, &v.IsRegistered,
		&v.CACRCNumber, &v.Address, &v.PostalCode,
		&v.Tagline, &v.Highlight1, &v.Highlight2, &v.Highlight3,
		&v.YearsExperience, &v.EventsCompleted, &v.Website, &v.Instagram, &v.Twitter, &v.WhatsApp,
		&v.CreatedAt, &v.UpdatedAt,
	)
	if err != nil {
		writeErr(w, http.StatusNotFound, "vendor not found")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// PATCH /vendor/me — update storefront fields
func (h *VendorDashboardHandler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	vendorID, ok := h.resolveVendorID(w, r)
	if !ok {
		return
	}

	var body struct {
		Tagline         *string `json:"tagline"`
		Bio             *string `json:"bio"`
		Website         *string `json:"website"`
		Instagram       *string `json:"instagram"`
		Twitter         *string `json:"twitter"`
		WhatsApp        *string `json:"whatsapp"`
		YearsExperience *int    `json:"years_experience"`
		EventsCompleted *int    `json:"events_completed"`
		Highlight1      *string `json:"highlight_1"`
		Highlight2      *string `json:"highlight_2"`
		Highlight3      *string `json:"highlight_3"`
		Address         *string `json:"address"`
		PostalCode      *string `json:"postal_code"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}

	_, err := h.db.Exec(r.Context(),
		`UPDATE vendors SET
		   tagline          = COALESCE($2, tagline),
		   bio              = COALESCE($3, bio),
		   website          = COALESCE($4, website),
		   instagram        = COALESCE($5, instagram),
		   twitter          = COALESCE($6, twitter),
		   whatsapp         = COALESCE($7, whatsapp),
		   years_experience = COALESCE($8, years_experience),
		   events_completed = COALESCE($9, events_completed),
		   highlight_1      = COALESCE($10, highlight_1),
		   highlight_2      = COALESCE($11, highlight_2),
		   highlight_3      = COALESCE($12, highlight_3),
		   address          = COALESCE($13, address),
		   postal_code      = COALESCE($14, postal_code),
		   updated_at       = NOW()
		 WHERE id = $1`,
		vendorID, body.Tagline, body.Bio, body.Website, body.Instagram, body.Twitter,
		body.WhatsApp, body.YearsExperience, body.EventsCompleted,
		body.Highlight1, body.Highlight2, body.Highlight3,
		body.Address, body.PostalCode,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to update profile")
		return
	}
	h.GetMe(w, r)
}

// ─── PRODUCTS (product vendors) ───────────────────────────────────────────────

// GET /vendor/products
func (h *VendorDashboardHandler) ListProducts(w http.ResponseWriter, r *http.Request) {
	vendorID, ok := h.resolveVendorID(w, r)
	if !ok {
		return
	}

	rows, err := h.db.Query(r.Context(),
		`SELECT id, vendor_id, name, description, price, category, image_url, stock, active,
		        min_order_qty, lead_time_days, free_delivery_above, created_at, updated_at
		 FROM products WHERE vendor_id = $1 ORDER BY created_at DESC`, vendorID,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to fetch products")
		return
	}
	defer rows.Close()

	var products []models.Product
	for rows.Next() {
		var p models.Product
		if err := rows.Scan(
			&p.ID, &p.VendorID, &p.Name, &p.Description, &p.Price, &p.Category,
			&p.ImageURL, &p.Stock, &p.Active,
			&p.MinOrderQty, &p.LeadTimeDays, &p.FreeDeliveryAbove,
			&p.CreatedAt, &p.UpdatedAt,
		); err == nil {
			products = append(products, p)
		}
	}
	if products == nil {
		products = []models.Product{}
	}
	writeJSON(w, http.StatusOK, products)
}

// POST /vendor/products
func (h *VendorDashboardHandler) CreateProduct(w http.ResponseWriter, r *http.Request) {
	vendorID, ok := h.resolveVendorID(w, r)
	if !ok {
		return
	}

	// Tier 1 cap: max 1 product
	var verificationStatus string
	var productCount int
	_ = h.db.QueryRow(r.Context(), `SELECT verification_status FROM vendors WHERE id = $1`, vendorID).Scan(&verificationStatus)
	_ = h.db.QueryRow(r.Context(), `SELECT COUNT(*) FROM products WHERE vendor_id = $1`, vendorID).Scan(&productCount)
	if verificationStatus == "tier_1" && productCount >= 1 {
		writeErr(w, http.StatusForbidden, "tier_1 vendors are limited to 1 product listing — apply for Tier 2 to add more")
		return
	}

	var body struct {
		Name              string  `json:"name"`
		Description       *string `json:"description"`
		Price             int64   `json:"price"`
		Category          *string `json:"category"`
		ImageURL          *string `json:"image_url"`
		Stock             *int    `json:"stock"`
		MinOrderQty       int     `json:"min_order_qty"`
		LeadTimeDays      int     `json:"lead_time_days"`
		FreeDeliveryAbove *int64  `json:"free_delivery_above"`
	}
	if err := decode(r, &body); err != nil || body.Name == "" || body.Price <= 0 {
		writeErr(w, http.StatusBadRequest, "name and price are required")
		return
	}
	if body.MinOrderQty < 1 {
		body.MinOrderQty = 1
	}
	if body.LeadTimeDays < 0 {
		body.LeadTimeDays = 3
	}

	var p models.Product
	err := h.db.QueryRow(r.Context(),
		`INSERT INTO products (vendor_id, name, description, price, category, image_url, stock, min_order_qty, lead_time_days, free_delivery_above)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 RETURNING id, vendor_id, name, description, price, category, image_url, stock, active,
		           min_order_qty, lead_time_days, free_delivery_above, created_at, updated_at`,
		vendorID, body.Name, body.Description, body.Price, body.Category, body.ImageURL,
		body.Stock, body.MinOrderQty, body.LeadTimeDays, body.FreeDeliveryAbove,
	).Scan(
		&p.ID, &p.VendorID, &p.Name, &p.Description, &p.Price, &p.Category,
		&p.ImageURL, &p.Stock, &p.Active,
		&p.MinOrderQty, &p.LeadTimeDays, &p.FreeDeliveryAbove,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to create product")
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

// PATCH /vendor/products/:id
func (h *VendorDashboardHandler) UpdateProduct(w http.ResponseWriter, r *http.Request) {
	vendorID, ok := h.resolveVendorID(w, r)
	if !ok {
		return
	}
	productID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid product id")
		return
	}

	var body struct {
		Name              *string `json:"name"`
		Description       *string `json:"description"`
		Price             *int64  `json:"price"`
		Category          *string `json:"category"`
		Stock             *int    `json:"stock"`
		MinOrderQty       *int    `json:"min_order_qty"`
		LeadTimeDays      *int    `json:"lead_time_days"`
		FreeDeliveryAbove *int64  `json:"free_delivery_above"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}

	_, err = h.db.Exec(r.Context(),
		`UPDATE products SET
		   name               = COALESCE($3, name),
		   description        = COALESCE($4, description),
		   price              = COALESCE($5, price),
		   category           = COALESCE($6, category),
		   stock              = COALESCE($7, stock),
		   min_order_qty      = COALESCE($8, min_order_qty),
		   lead_time_days     = COALESCE($9, lead_time_days),
		   free_delivery_above = COALESCE($10, free_delivery_above),
		   updated_at         = NOW()
		 WHERE id = $1 AND vendor_id = $2`,
		productID, vendorID, body.Name, body.Description, body.Price, body.Category,
		body.Stock, body.MinOrderQty, body.LeadTimeDays, body.FreeDeliveryAbove,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to update product")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "updated"})
}

// PATCH /vendor/products/:id/toggle — toggle active
func (h *VendorDashboardHandler) ToggleProduct(w http.ResponseWriter, r *http.Request) {
	vendorID, ok := h.resolveVendorID(w, r)
	if !ok {
		return
	}
	productID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid product id")
		return
	}

	var active bool
	err = h.db.QueryRow(r.Context(),
		`UPDATE products SET active = NOT active, updated_at = NOW()
		 WHERE id = $1 AND vendor_id = $2 RETURNING active`,
		productID, vendorID,
	).Scan(&active)
	if err != nil {
		writeErr(w, http.StatusNotFound, "product not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"active": active})
}

// DELETE /vendor/products/:id
func (h *VendorDashboardHandler) DeleteProduct(w http.ResponseWriter, r *http.Request) {
	vendorID, ok := h.resolveVendorID(w, r)
	if !ok {
		return
	}
	productID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid product id")
		return
	}

	_, err = h.db.Exec(r.Context(),
		`DELETE FROM products WHERE id = $1 AND vendor_id = $2`, productID, vendorID,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to delete product")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── SERVICES (service vendors) ───────────────────────────────────────────────

// GET /vendor/services
func (h *VendorDashboardHandler) ListServices(w http.ResponseWriter, r *http.Request) {
	vendorID, ok := h.resolveVendorID(w, r)
	if !ok {
		return
	}

	rows, err := h.db.Query(r.Context(),
		`SELECT id, vendor_id, name, description, price_from, price_to, unit,
		        pricing_model, is_active, min_notice_hours, max_advance_days, response_time_hrs,
		        created_at, updated_at
		 FROM vendor_services WHERE vendor_id = $1 ORDER BY created_at DESC`, vendorID,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to fetch services")
		return
	}
	defer rows.Close()

	var services []models.VendorService
	for rows.Next() {
		var s models.VendorService
		if err := rows.Scan(
			&s.ID, &s.VendorID, &s.Name, &s.Description, &s.PriceFrom, &s.PriceTo, &s.Unit,
			&s.PricingModel, &s.IsActive, &s.MinNoticeHours, &s.MaxAdvanceDays, &s.ResponseTimeHrs,
			&s.CreatedAt, &s.UpdatedAt,
		); err == nil {
			services = append(services, s)
		}
	}
	if services == nil {
		services = []models.VendorService{}
	}
	writeJSON(w, http.StatusOK, services)
}

// POST /vendor/services
func (h *VendorDashboardHandler) CreateService(w http.ResponseWriter, r *http.Request) {
	vendorID, ok := h.resolveVendorID(w, r)
	if !ok {
		return
	}

	// Tier 1 cap: max 1 service
	var verificationStatus string
	var serviceCount int
	_ = h.db.QueryRow(r.Context(), `SELECT verification_status FROM vendors WHERE id = $1`, vendorID).Scan(&verificationStatus)
	_ = h.db.QueryRow(r.Context(), `SELECT COUNT(*) FROM vendor_services WHERE vendor_id = $1`, vendorID).Scan(&serviceCount)
	if verificationStatus == "tier_1" && serviceCount >= 1 {
		writeErr(w, http.StatusForbidden, "tier_1 vendors are limited to 1 service listing — apply for Tier 2 to add more")
		return
	}

	var body struct {
		Name            string  `json:"name"`
		Description     *string `json:"description"`
		PriceFrom       int64   `json:"price_from"`
		PriceTo         *int64  `json:"price_to"`
		Unit            *string `json:"unit"`
		PricingModel    string  `json:"pricing_model"`
		MinNoticeHours  int     `json:"min_notice_hours"`
		MaxAdvanceDays  int     `json:"max_advance_days"`
		ResponseTimeHrs int     `json:"response_time_hrs"`
	}
	if err := decode(r, &body); err != nil || body.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	if body.PricingModel == "" {
		body.PricingModel = "fixed"
	}
	if body.MinNoticeHours == 0 {
		body.MinNoticeHours = 24
	}
	if body.MaxAdvanceDays == 0 {
		body.MaxAdvanceDays = 90
	}
	if body.ResponseTimeHrs == 0 {
		body.ResponseTimeHrs = 2
	}

	var s models.VendorService
	err := h.db.QueryRow(r.Context(),
		`INSERT INTO vendor_services (vendor_id, name, description, price_from, price_to, unit,
		                             pricing_model, min_notice_hours, max_advance_days, response_time_hrs)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 RETURNING id, vendor_id, name, description, price_from, price_to, unit,
		           pricing_model, is_active, min_notice_hours, max_advance_days, response_time_hrs,
		           created_at, updated_at`,
		vendorID, body.Name, body.Description, body.PriceFrom, body.PriceTo, body.Unit,
		body.PricingModel, body.MinNoticeHours, body.MaxAdvanceDays, body.ResponseTimeHrs,
	).Scan(
		&s.ID, &s.VendorID, &s.Name, &s.Description, &s.PriceFrom, &s.PriceTo, &s.Unit,
		&s.PricingModel, &s.IsActive, &s.MinNoticeHours, &s.MaxAdvanceDays, &s.ResponseTimeHrs,
		&s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to create service")
		return
	}
	writeJSON(w, http.StatusCreated, s)
}

// PATCH /vendor/services/:id
func (h *VendorDashboardHandler) UpdateService(w http.ResponseWriter, r *http.Request) {
	vendorID, ok := h.resolveVendorID(w, r)
	if !ok {
		return
	}
	serviceID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid service id")
		return
	}

	var body struct {
		Name            *string `json:"name"`
		Description     *string `json:"description"`
		PriceFrom       *int64  `json:"price_from"`
		PriceTo         *int64  `json:"price_to"`
		Unit            *string `json:"unit"`
		PricingModel    *string `json:"pricing_model"`
		MinNoticeHours  *int    `json:"min_notice_hours"`
		MaxAdvanceDays  *int    `json:"max_advance_days"`
		ResponseTimeHrs *int    `json:"response_time_hrs"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}

	_, err = h.db.Exec(r.Context(),
		`UPDATE vendor_services SET
		   name             = COALESCE($3, name),
		   description      = COALESCE($4, description),
		   price_from       = COALESCE($5, price_from),
		   price_to         = COALESCE($6, price_to),
		   unit             = COALESCE($7, unit),
		   pricing_model    = COALESCE($8, pricing_model),
		   min_notice_hours = COALESCE($9, min_notice_hours),
		   max_advance_days = COALESCE($10, max_advance_days),
		   response_time_hrs = COALESCE($11, response_time_hrs),
		   updated_at       = NOW()
		 WHERE id = $1 AND vendor_id = $2`,
		serviceID, vendorID, body.Name, body.Description, body.PriceFrom, body.PriceTo,
		body.Unit, body.PricingModel, body.MinNoticeHours, body.MaxAdvanceDays, body.ResponseTimeHrs,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to update service")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "updated"})
}

// PATCH /vendor/services/:id/toggle
func (h *VendorDashboardHandler) ToggleService(w http.ResponseWriter, r *http.Request) {
	vendorID, ok := h.resolveVendorID(w, r)
	if !ok {
		return
	}
	serviceID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid service id")
		return
	}

	var active bool
	err = h.db.QueryRow(r.Context(),
		`UPDATE vendor_services SET is_active = NOT is_active, updated_at = NOW()
		 WHERE id = $1 AND vendor_id = $2 RETURNING is_active`,
		serviceID, vendorID,
	).Scan(&active)
	if err != nil {
		writeErr(w, http.StatusNotFound, "service not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"is_active": active})
}

// DELETE /vendor/services/:id
func (h *VendorDashboardHandler) DeleteService(w http.ResponseWriter, r *http.Request) {
	vendorID, ok := h.resolveVendorID(w, r)
	if !ok {
		return
	}
	serviceID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid service id")
		return
	}

	_, err = h.db.Exec(r.Context(),
		`DELETE FROM vendor_services WHERE id = $1 AND vendor_id = $2`, serviceID, vendorID,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to delete service")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── ORDERS (product vendors) ──────────────────────────────────────────────────

// GET /vendor/orders?status=new
func (h *VendorDashboardHandler) ListOrders(w http.ResponseWriter, r *http.Request) {
	vendorID, ok := h.resolveVendorID(w, r)
	if !ok {
		return
	}

	status := r.URL.Query().Get("status") // empty string = all statuses

	rows, err := h.db.Query(r.Context(),
		`SELECT o.id, o.vendor_id, o.customer_id, o.status, o.total_amount, o.escrow_amount,
		        o.escrow_released, o.delivery_address, o.delivery_zone, o.notes, o.created_at, o.updated_at,
		        u.full_name, u.phone
		 FROM orders o
		 JOIN users u ON u.id = o.customer_id
		 WHERE o.vendor_id = $1
		   AND ($2::text = '' OR o.status = $2::text)
		 ORDER BY o.created_at DESC LIMIT 100`,
		vendorID, status,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to fetch orders")
		return
	}
	defer rows.Close()

	var orders []models.Order
	for rows.Next() {
		var o models.Order
		if err := rows.Scan(
			&o.ID, &o.VendorID, &o.CustomerID, &o.Status, &o.TotalAmount, &o.EscrowAmount,
			&o.EscrowReleased, &o.DeliveryAddress, &o.DeliveryZone, &o.Notes,
			&o.CreatedAt, &o.UpdatedAt,
			&o.CustomerName, &o.CustomerPhone,
		); err == nil {
			orders = append(orders, o)
		}
	}
	if orders == nil {
		orders = []models.Order{}
	}
	writeJSON(w, http.StatusOK, orders)
}

// PATCH /vendor/orders/:id/status
func (h *VendorDashboardHandler) UpdateOrderStatus(w http.ResponseWriter, r *http.Request) {
	vendorID, ok := h.resolveVendorID(w, r)
	if !ok {
		return
	}
	orderID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid order id")
		return
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := decode(r, &body); err != nil || body.Status == "" {
		writeErr(w, http.StatusBadRequest, "status is required")
		return
	}

	// Valid transitions
	valid := map[string]bool{
		"processing":       true,
		"ready":            true,
		"out_for_delivery": true,
		"delivered":        true,
		"confirmed":        true,
		"cancelled":        true,
	}
	if !valid[body.Status] {
		writeErr(w, http.StatusBadRequest, "invalid status value")
		return
	}

	var newStatus string
	err = h.db.QueryRow(r.Context(),
		`UPDATE orders SET status = $3, updated_at = NOW()
		 WHERE id = $1 AND vendor_id = $2 RETURNING status`,
		orderID, vendorID, body.Status,
	).Scan(&newStatus)
	if err != nil {
		writeErr(w, http.StatusNotFound, "order not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": newStatus})
}

// ─── BOOKINGS (service vendors) ───────────────────────────────────────────────

// GET /vendor/bookings?status=pending
func (h *VendorDashboardHandler) ListBookings(w http.ResponseWriter, r *http.Request) {
	vendorID, ok := h.resolveVendorID(w, r)
	if !ok {
		return
	}

	status := r.URL.Query().Get("status") // empty string = all statuses

	rows, err := h.db.Query(r.Context(),
		`SELECT b.id, b.event_id, b.vendor_id, b.service_id, b.client_id, b.status,
		        b.total_amount, b.escrow_amount, b.escrow_released, b.notes, b.event_date,
		        b.created_at, b.updated_at,
		        u.full_name, u.phone
		 FROM bookings b
		 JOIN users u ON u.id = b.client_id
		 WHERE b.vendor_id = $1
		   AND ($2::text = '' OR b.status = $2::text)
		 ORDER BY b.created_at DESC LIMIT 100`,
		vendorID, status,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to fetch bookings")
		return
	}
	defer rows.Close()

	type BookingRow struct {
		models.Booking
		ClientName  *string `json:"client_name,omitempty"`
		ClientPhone *string `json:"client_phone,omitempty"`
	}

	var bookings []BookingRow
	for rows.Next() {
		var b BookingRow
		if err := rows.Scan(
			&b.ID, &b.EventID, &b.VendorID, &b.ServiceID, &b.ClientID, &b.Status,
			&b.TotalAmount, &b.EscrowAmount, &b.EscrowReleased, &b.Notes, &b.EventDate,
			&b.CreatedAt, &b.UpdatedAt,
			&b.ClientName, &b.ClientPhone,
		); err == nil {
			bookings = append(bookings, b)
		}
	}
	if bookings == nil {
		bookings = []BookingRow{}
	}
	writeJSON(w, http.StatusOK, bookings)
}

// PATCH /vendor/bookings/:id/accept
func (h *VendorDashboardHandler) AcceptBooking(w http.ResponseWriter, r *http.Request) {
	vendorID, ok := h.resolveVendorID(w, r)
	if !ok {
		return
	}
	bookingID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid booking id")
		return
	}

	_, err = h.db.Exec(r.Context(),
		`UPDATE bookings SET status = 'accepted', updated_at = NOW()
		 WHERE id = $1 AND vendor_id = $2 AND status = 'pending'`,
		bookingID, vendorID,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to accept booking")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "accepted"})
}

// PATCH /vendor/bookings/:id/decline
func (h *VendorDashboardHandler) DeclineBooking(w http.ResponseWriter, r *http.Request) {
	vendorID, ok := h.resolveVendorID(w, r)
	if !ok {
		return
	}
	bookingID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid booking id")
		return
	}

	var body struct {
		Reason *string `json:"reason"`
	}
	_ = decode(r, &body)

	_, err = h.db.Exec(r.Context(),
		`UPDATE bookings SET status = 'declined', notes = COALESCE($3, notes), updated_at = NOW()
		 WHERE id = $1 AND vendor_id = $2 AND status = 'pending'`,
		bookingID, vendorID, body.Reason,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to decline booking")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "declined"})
}

// ─── VENDOR WALLET ────────────────────────────────────────────────────────────

// GET /vendor/wallet — returns 35/65 split view
func (h *VendorDashboardHandler) GetWallet(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var wallet models.Wallet
	err := h.db.QueryRow(r.Context(),
		`SELECT id, user_id, balance, escrow_held, updated_at FROM wallets WHERE user_id = $1`, u.ID,
	).Scan(&wallet.ID, &wallet.UserID, &wallet.Balance, &wallet.EscrowHeld, &wallet.UpdatedAt)
	if err != nil {
		// Return empty wallet instead of error
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"balance":     0,
			"escrow_held": 0,
			"available":   0,
		})
		return
	}

	// 35% available, 65% escrow from new earnings — show the breakdown
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":          wallet.ID,
		"balance":     wallet.Balance,
		"escrow_held": wallet.EscrowHeld,
		"available":   wallet.Balance,
		"updated_at":  wallet.UpdatedAt,
	})
}

// GET /vendor/transactions
func (h *VendorDashboardHandler) ListTransactions(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var walletID uuid.UUID
	_ = h.db.QueryRow(r.Context(), `SELECT id FROM wallets WHERE user_id = $1`, u.ID).Scan(&walletID)

	rows, err := h.db.Query(r.Context(),
		`SELECT id, wallet_id, type, amount, status, reference, description, booking_id, created_at
		 FROM wallet_transactions WHERE wallet_id = $1
		 ORDER BY created_at DESC LIMIT 50`, walletID,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to fetch transactions")
		return
	}
	defer rows.Close()

	var txns []models.WalletTransaction
	for rows.Next() {
		var t models.WalletTransaction
		if err := rows.Scan(
			&t.ID, &t.WalletID, &t.Type, &t.Amount, &t.Status,
			&t.Reference, &t.Description, &t.BookingID, &t.CreatedAt,
		); err == nil {
			txns = append(txns, t)
		}
	}
	if txns == nil {
		txns = []models.WalletTransaction{}
	}
	writeJSON(w, http.StatusOK, txns)
}

// POST /vendor/withdraw
func (h *VendorDashboardHandler) Withdraw(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var body struct {
		Amount          int64     `json:"amount"`
		BankAccountID   uuid.UUID `json:"bank_account_id"`
	}
	if err := decode(r, &body); err != nil || body.Amount <= 0 {
		writeErr(w, http.StatusBadRequest, "amount is required")
		return
	}

	// Check balance
	var walletID uuid.UUID
	var balance int64
	err := h.db.QueryRow(r.Context(),
		`SELECT id, balance FROM wallets WHERE user_id = $1`, u.ID,
	).Scan(&walletID, &balance)
	if err != nil || balance < body.Amount {
		writeErr(w, http.StatusBadRequest, "insufficient available balance")
		return
	}

	// Deduct balance and record transaction
	tx, _ := h.db.Begin(r.Context())
	defer tx.Rollback(r.Context())

	_, err = tx.Exec(r.Context(),
		`UPDATE wallets SET balance = balance - $2, updated_at = NOW() WHERE id = $1`,
		walletID, body.Amount,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "withdrawal failed")
		return
	}

	ref := uuid.New().String()
	_, err = tx.Exec(r.Context(),
		`INSERT INTO wallet_transactions (wallet_id, type, amount, status, reference, description)
		 VALUES ($1, 'withdrawal', $2, 'pending', $3, 'Vendor withdrawal request')`,
		walletID, body.Amount, ref,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to record withdrawal")
		return
	}

	_ = tx.Commit(r.Context())
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":   "withdrawal initiated",
		"amount":    body.Amount,
		"reference": ref,
	})
}

// ─── BANK ACCOUNTS ────────────────────────────────────────────────────────────

// GET /vendor/bank-accounts
func (h *VendorDashboardHandler) ListBankAccounts(w http.ResponseWriter, r *http.Request) {
	vendorID, ok := h.resolveVendorID(w, r)
	if !ok {
		return
	}

	rows, err := h.db.Query(r.Context(),
		`SELECT id, vendor_id, bank_name, account_number, account_name, is_default, created_at
		 FROM vendor_bank_accounts WHERE vendor_id = $1 ORDER BY is_default DESC, created_at ASC`,
		vendorID,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to fetch bank accounts")
		return
	}
	defer rows.Close()

	var accounts []models.VendorBankAccount
	for rows.Next() {
		var a models.VendorBankAccount
		if err := rows.Scan(&a.ID, &a.VendorID, &a.BankName, &a.AccountNumber, &a.AccountName, &a.IsDefault, &a.CreatedAt); err == nil {
			accounts = append(accounts, a)
		}
	}
	if accounts == nil {
		accounts = []models.VendorBankAccount{}
	}
	writeJSON(w, http.StatusOK, accounts)
}

// POST /vendor/bank-accounts
func (h *VendorDashboardHandler) AddBankAccount(w http.ResponseWriter, r *http.Request) {
	vendorID, ok := h.resolveVendorID(w, r)
	if !ok {
		return
	}

	var body struct {
		BankName      string `json:"bank_name"`
		AccountNumber string `json:"account_number"`
		AccountName   string `json:"account_name"`
		IsDefault     bool   `json:"is_default"`
	}
	if err := decode(r, &body); err != nil || body.BankName == "" || body.AccountNumber == "" || body.AccountName == "" {
		writeErr(w, http.StatusBadRequest, "bank_name, account_number, and account_name are required")
		return
	}

	tx, _ := h.db.Begin(r.Context())
	defer tx.Rollback(r.Context())

	// If setting as default, clear existing defaults
	if body.IsDefault {
		_, _ = tx.Exec(r.Context(),
			`UPDATE vendor_bank_accounts SET is_default = false WHERE vendor_id = $1`, vendorID,
		)
	}

	var account models.VendorBankAccount
	err := tx.QueryRow(r.Context(),
		`INSERT INTO vendor_bank_accounts (vendor_id, bank_name, account_number, account_name, is_default)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, vendor_id, bank_name, account_number, account_name, is_default, created_at`,
		vendorID, body.BankName, body.AccountNumber, body.AccountName, body.IsDefault,
	).Scan(&account.ID, &account.VendorID, &account.BankName, &account.AccountNumber, &account.AccountName, &account.IsDefault, &account.CreatedAt)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to add bank account")
		return
	}

	_ = tx.Commit(r.Context())
	writeJSON(w, http.StatusCreated, account)
}

// ─── VERIFICATION ─────────────────────────────────────────────────────────────

// GET /vendor/verification
func (h *VendorDashboardHandler) GetVerification(w http.ResponseWriter, r *http.Request) {
	vendorID, ok := h.resolveVendorID(w, r)
	if !ok {
		return
	}

	rows, err := h.db.Query(r.Context(),
		`SELECT id, vendor_id, target_tier, cac_rc_number, cac_doc_url, id_type, id_doc_url,
		        bank_stmt_url, notes, status, reviewer_notes, submitted_at, reviewed_at
		 FROM vendor_verifications WHERE vendor_id = $1 ORDER BY submitted_at DESC`, vendorID,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to fetch verifications")
		return
	}
	defer rows.Close()

	var verifications []models.VendorVerification
	for rows.Next() {
		var v models.VendorVerification
		if err := rows.Scan(
			&v.ID, &v.VendorID, &v.TargetTier, &v.CACRCNumber, &v.CACDocURL, &v.IDType, &v.IDDocURL,
			&v.BankStmtURL, &v.Notes, &v.Status, &v.ReviewerNotes, &v.SubmittedAt, &v.ReviewedAt,
		); err == nil {
			verifications = append(verifications, v)
		}
	}
	if verifications == nil {
		verifications = []models.VendorVerification{}
	}
	writeJSON(w, http.StatusOK, verifications)
}

// POST /vendor/verification/apply
func (h *VendorDashboardHandler) ApplyVerification(w http.ResponseWriter, r *http.Request) {
	vendorID, ok := h.resolveVendorID(w, r)
	if !ok {
		return
	}

	var body struct {
		TargetTier  int     `json:"target_tier"` // 2 or 3
		CACRCNumber *string `json:"cac_rc_number"`
		CACDocURL   *string `json:"cac_doc_url"`
		IDType      *string `json:"id_type"`
		IDDocURL    *string `json:"id_doc_url"`
		BankStmtURL *string `json:"bank_stmt_url"`
		Notes       *string `json:"notes"`
	}
	if err := decode(r, &body); err != nil || (body.TargetTier != 2 && body.TargetTier != 3) {
		writeErr(w, http.StatusBadRequest, "target_tier must be 2 or 3")
		return
	}

	// Check current verification status
	var currentStatus string
	_ = h.db.QueryRow(r.Context(), `SELECT verification_status FROM vendors WHERE id = $1`, vendorID).Scan(&currentStatus)
	if body.TargetTier == 3 && currentStatus != "tier_2_approved" {
		writeErr(w, http.StatusBadRequest, "must be Tier 2 approved before applying for Tier 3")
		return
	}

	// Check for existing pending application
	var pendingCount int
	_ = h.db.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM vendor_verifications WHERE vendor_id = $1 AND target_tier = $2 AND status = 'pending'`,
		vendorID, body.TargetTier,
	).Scan(&pendingCount)
	if pendingCount > 0 {
		writeErr(w, http.StatusConflict, "you already have a pending verification application for this tier")
		return
	}

	tx, _ := h.db.Begin(r.Context())
	defer tx.Rollback(r.Context())

	var verification models.VendorVerification
	err := tx.QueryRow(r.Context(),
		`INSERT INTO vendor_verifications (vendor_id, target_tier, cac_rc_number, cac_doc_url, id_type, id_doc_url, bank_stmt_url, notes)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id, vendor_id, target_tier, cac_rc_number, cac_doc_url, id_type, id_doc_url, bank_stmt_url, notes, status, reviewer_notes, submitted_at, reviewed_at`,
		vendorID, body.TargetTier, body.CACRCNumber, body.CACDocURL, body.IDType, body.IDDocURL, body.BankStmtURL, body.Notes,
	).Scan(
		&verification.ID, &verification.VendorID, &verification.TargetTier,
		&verification.CACRCNumber, &verification.CACDocURL, &verification.IDType, &verification.IDDocURL,
		&verification.BankStmtURL, &verification.Notes, &verification.Status, &verification.ReviewerNotes,
		&verification.SubmittedAt, &verification.ReviewedAt,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to submit verification application")
		return
	}

	// Update vendor verification_status to pending
	pendingStatus := "tier_2_pending"
	if body.TargetTier == 3 {
		pendingStatus = "tier_3_pending"
	}
	_, _ = tx.Exec(r.Context(),
		`UPDATE vendors SET verification_status = $2, updated_at = NOW() WHERE id = $1`,
		vendorID, pendingStatus,
	)

	_ = tx.Commit(r.Context())
	writeJSON(w, http.StatusCreated, verification)
}

// ─── AVAILABILITY ──────────────────────────────────────────────────────────────

// GET /vendor/availability
func (h *VendorDashboardHandler) GetAvailability(w http.ResponseWriter, r *http.Request) {
	vendorID, ok := h.resolveVendorID(w, r)
	if !ok {
		return
	}

	var av models.VendorAvailability
	err := h.db.QueryRow(r.Context(),
		`SELECT id, vendor_id, working_days, blocked_dates, updated_at
		 FROM vendor_availability WHERE vendor_id = $1`, vendorID,
	).Scan(&av.ID, &av.VendorID, &av.WorkingDays, &av.BlockedDates, &av.UpdatedAt)
	if err != nil {
		// Return sensible defaults if no row yet
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"vendor_id":    vendorID,
			"working_days": []int{1, 2, 3, 4, 5},
			"blocked_dates": []string{},
		})
		return
	}
	writeJSON(w, http.StatusOK, av)
}

// PATCH /vendor/availability
func (h *VendorDashboardHandler) UpdateAvailability(w http.ResponseWriter, r *http.Request) {
	vendorID, ok := h.resolveVendorID(w, r)
	if !ok {
		return
	}

	var body struct {
		WorkingDays  []int    `json:"working_days"`
		BlockedDates []string `json:"blocked_dates"` // "YYYY-MM-DD"
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}

	_, err := h.db.Exec(r.Context(),
		`INSERT INTO vendor_availability (vendor_id, working_days, blocked_dates)
		 VALUES ($1, $2, $3::date[])
		 ON CONFLICT (vendor_id) DO UPDATE SET
		   working_days  = EXCLUDED.working_days,
		   blocked_dates = EXCLUDED.blocked_dates,
		   updated_at    = NOW()`,
		vendorID, body.WorkingDays, body.BlockedDates,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to update availability")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "availability updated"})
}
