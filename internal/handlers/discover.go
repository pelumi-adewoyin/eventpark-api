package handlers

import (
	"net/http"
	"strconv"

	"github.com/eventpark/api/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DiscoverHandler struct {
	db *pgxpool.Pool
}

func NewDiscoverHandler(db *pgxpool.Pool) *DiscoverHandler {
	return &DiscoverHandler{db: db}
}

// GET /discover/events?city=Lagos&type=wedding&page=1&limit=20
func (h *DiscoverHandler) Events(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	city := q.Get("city")
	eventType := q.Get("type")
	search := q.Get("q")
	page, _ := strconv.Atoi(q.Get("page"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}
	offset := (page - 1) * limit

	query := `SELECT e.id, e.owner_id, e.title, e.description,
		e.event_type::text, e.visibility::text, e.status::text,
		e.start_at, e.end_at, e.venue_name, e.venue_address, e.venue_city, e.venue_state,
		e.cover_url, e.max_guests, e.budget_total, e.ticket_price, e.approval_status::text,
		e.created_at, e.updated_at,
		COUNT(g.id) FILTER (WHERE g.status::text = 'rsvp_yes') AS guest_count, 0 AS checked_in
	  FROM events e
	  LEFT JOIN guests g ON g.event_id = e.id
	  WHERE e.status::text = 'published' AND e.visibility::text = 'public'`
	args := []any{}
	argN := 1

	if city != "" {
		query += ` AND e.venue_city ILIKE $` + strconv.Itoa(argN)
		args = append(args, "%"+city+"%")
		argN++
	}
	if eventType != "" {
		query += ` AND e.event_type = $` + strconv.Itoa(argN)
		args = append(args, eventType)
		argN++
	}
	if search != "" {
		query += ` AND (e.title ILIKE $` + strconv.Itoa(argN) + ` OR e.description ILIKE $` + strconv.Itoa(argN) + `)`
		args = append(args, "%"+search+"%")
		argN++
	}

	query += ` GROUP BY e.id ORDER BY e.start_at ASC NULLS LAST
	           LIMIT $` + strconv.Itoa(argN) + ` OFFSET $` + strconv.Itoa(argN+1)
	args = append(args, limit, offset)

	rows, err := h.db.Query(r.Context(), query, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to fetch events")
		return
	}
	defer rows.Close()

	events := []models.Event{}
	for rows.Next() {
		var e models.Event
		if err := rows.Scan(
			&e.ID, &e.OwnerID, &e.Title, &e.Description, &e.EventType,
			&e.Visibility, &e.Status, &e.StartAt, &e.EndAt,
			&e.VenueName, &e.VenueAddress, &e.VenueCity, &e.VenueState,
			&e.CoverURL, &e.MaxGuests, &e.BudgetTotal, &e.TicketPrice,
			&e.ApprovalStatus, &e.CreatedAt, &e.UpdatedAt,
			&e.GuestCount, &e.CheckedIn,
		); err == nil {
			events = append(events, e)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"events": events,
		"page":   page,
		"limit":  limit,
	})
}

// GET /discover/vendors?category=catering&city=Lagos
func (h *DiscoverHandler) Vendors(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	category := q.Get("category")
	city := q.Get("city")
	search := q.Get("q")
	page, _ := strconv.Atoi(q.Get("page"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}
	offset := (page - 1) * limit

	query := `SELECT id, user_id, business_name, category, bio, city, state, avatar_url, cover_url, rating, review_count, verified, created_at
	  FROM vendors WHERE true`
	args := []any{}
	argN := 1

	if category != "" {
		query += ` AND category ILIKE $` + strconv.Itoa(argN)
		args = append(args, "%"+category+"%")
		argN++
	}
	if city != "" {
		query += ` AND city ILIKE $` + strconv.Itoa(argN)
		args = append(args, "%"+city+"%")
		argN++
	}
	if search != "" {
		query += ` AND business_name ILIKE $` + strconv.Itoa(argN)
		args = append(args, "%"+search+"%")
		argN++
	}

	query += ` ORDER BY rating DESC, review_count DESC LIMIT $` + strconv.Itoa(argN) + ` OFFSET $` + strconv.Itoa(argN+1)
	args = append(args, limit, offset)

	rows, err := h.db.Query(r.Context(), query, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to fetch vendors")
		return
	}
	defer rows.Close()

	vendors := []models.Vendor{}
	for rows.Next() {
		var v models.Vendor
		if err := rows.Scan(
			&v.ID, &v.UserID, &v.BusinessName, &v.Category,
			&v.Bio, &v.City, &v.State, &v.AvatarURL, &v.CoverURL,
			&v.Rating, &v.ReviewCount, &v.Verified, &v.CreatedAt,
		); err == nil {
			vendors = append(vendors, v)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"vendors": vendors,
		"page":    page,
		"limit":   limit,
	})
}

// GET /discover/products?category=decor&q=balloon
func (h *DiscoverHandler) Products(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	category := q.Get("category")
	search := q.Get("q")
	page, _ := strconv.Atoi(q.Get("page"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}
	offset := (page - 1) * limit

	query := `SELECT p.id, p.vendor_id, p.name, p.description, p.price, p.category, p.image_url, p.stock, p.active, p.created_at
	  FROM products p WHERE p.active = true`
	args := []any{}
	argN := 1

	if category != "" {
		query += ` AND p.category ILIKE $` + strconv.Itoa(argN)
		args = append(args, "%"+category+"%")
		argN++
	}
	if search != "" {
		query += ` AND p.name ILIKE $` + strconv.Itoa(argN)
		args = append(args, "%"+search+"%")
		argN++
	}

	query += ` ORDER BY p.created_at DESC LIMIT $` + strconv.Itoa(argN) + ` OFFSET $` + strconv.Itoa(argN+1)
	args = append(args, limit, offset)

	rows, err := h.db.Query(r.Context(), query, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to fetch products")
		return
	}
	defer rows.Close()

	products := []models.Product{}
	for rows.Next() {
		var p models.Product
		if err := rows.Scan(
			&p.ID, &p.VendorID, &p.Name, &p.Description,
			&p.Price, &p.Category, &p.ImageURL, &p.Stock, &p.Active, &p.CreatedAt,
		); err == nil {
			products = append(products, p)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"products": products,
		"page":     page,
		"limit":    limit,
	})
}
