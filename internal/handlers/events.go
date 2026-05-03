package handlers

import (
	"net/http"

	"github.com/eventpark/api/internal/middleware"
	"github.com/eventpark/api/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type EventsHandler struct {
	db *pgxpool.Pool
}

func NewEventsHandler(db *pgxpool.Pool) *EventsHandler {
	return &EventsHandler{db: db}
}

// POST /events
func (h *EventsHandler) CreateEvent(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var body struct {
		Title       string  `json:"title"`
		Description *string `json:"description"`
		EventType   string  `json:"event_type"`
		Visibility  string  `json:"visibility"`
		VenueName   *string `json:"venue_name"`
		VenueAddress *string `json:"venue_address"`
		VenueCity   *string `json:"venue_city"`
		VenueState  *string `json:"venue_state"`
		StartAt     *string `json:"start_at"`
		EndAt       *string `json:"end_at"`
		MaxGuests   *int    `json:"max_guests"`
		BudgetTotal int64   `json:"budget_total"`
		TicketPrice int64   `json:"ticket_price"`
	}
	if err := decode(r, &body); err != nil || body.Title == "" {
		writeErr(w, http.StatusBadRequest, "title is required")
		return
	}

	if body.EventType == "" {
		body.EventType = "other"
	}
	if body.Visibility == "" {
		body.Visibility = "private"
	}

	var event models.Event
	err := h.db.QueryRow(r.Context(),
		`INSERT INTO events (owner_id, title, description, event_type, visibility, venue_name,
		  venue_address, venue_city, venue_state, start_at, end_at, max_guests, budget_total, ticket_price)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		 RETURNING id, owner_id, title, description, event_type, visibility, status,
		   start_at, end_at, venue_name, venue_address, venue_city, venue_state,
		   cover_url, max_guests, budget_total, ticket_price, approval_status, created_at, updated_at`,
		u.ID, body.Title, body.Description, body.EventType, body.Visibility,
		body.VenueName, body.VenueAddress, body.VenueCity, body.VenueState,
		body.StartAt, body.EndAt, body.MaxGuests, body.BudgetTotal, body.TicketPrice,
	).Scan(
		&event.ID, &event.OwnerID, &event.Title, &event.Description, &event.EventType,
		&event.Visibility, &event.Status, &event.StartAt, &event.EndAt,
		&event.VenueName, &event.VenueAddress, &event.VenueCity, &event.VenueState,
		&event.CoverURL, &event.MaxGuests, &event.BudgetTotal, &event.TicketPrice,
		&event.ApprovalStatus, &event.CreatedAt, &event.UpdatedAt,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to create event")
		return
	}

	writeJSON(w, http.StatusCreated, event)
}

// GET /events — list user's events
func (h *EventsHandler) ListMyEvents(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	rows, err := h.db.Query(r.Context(),
		`SELECT e.id, e.owner_id, e.title, e.description, e.event_type, e.visibility, e.status,
		   e.start_at, e.end_at, e.venue_name, e.venue_address, e.venue_city, e.venue_state,
		   e.cover_url, e.max_guests, e.budget_total, e.ticket_price, e.approval_status,
		   e.created_at, e.updated_at,
		   COUNT(g.id) FILTER (WHERE g.status != 'invited') AS guest_count,
		   COUNT(g.id) FILTER (WHERE g.status = 'checked_in') AS checked_in
		 FROM events e
		 LEFT JOIN guests g ON g.event_id = e.id
		 WHERE e.owner_id = $1
		 GROUP BY e.id
		 ORDER BY e.created_at DESC`,
		u.ID,
	)
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
		); err != nil {
			continue
		}
		events = append(events, e)
	}
	writeJSON(w, http.StatusOK, events)
}

// GET /events/:id
func (h *EventsHandler) GetEvent(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid event id")
		return
	}

	var e models.Event
	err = h.db.QueryRow(r.Context(),
		`SELECT e.id, e.owner_id, e.title, e.description, e.event_type, e.visibility, e.status,
		   e.start_at, e.end_at, e.venue_name, e.venue_address, e.venue_city, e.venue_state,
		   e.cover_url, e.max_guests, e.budget_total, e.ticket_price, e.approval_status,
		   e.created_at, e.updated_at,
		   COUNT(g.id) FILTER (WHERE g.status != 'invited') AS guest_count,
		   COUNT(g.id) FILTER (WHERE g.status = 'checked_in') AS checked_in
		 FROM events e
		 LEFT JOIN guests g ON g.event_id = e.id
		 WHERE e.id = $1
		 GROUP BY e.id`, id,
	).Scan(
		&e.ID, &e.OwnerID, &e.Title, &e.Description, &e.EventType,
		&e.Visibility, &e.Status, &e.StartAt, &e.EndAt,
		&e.VenueName, &e.VenueAddress, &e.VenueCity, &e.VenueState,
		&e.CoverURL, &e.MaxGuests, &e.BudgetTotal, &e.TicketPrice,
		&e.ApprovalStatus, &e.CreatedAt, &e.UpdatedAt,
		&e.GuestCount, &e.CheckedIn,
	)
	if err != nil {
		writeErr(w, http.StatusNotFound, "event not found")
		return
	}
	writeJSON(w, http.StatusOK, e)
}

// PATCH /events/:id
func (h *EventsHandler) UpdateEvent(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil || u == nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}

	var body struct {
		Title        *string `json:"title"`
		Description  *string `json:"description"`
		VenueName    *string `json:"venue_name"`
		VenueAddress *string `json:"venue_address"`
		VenueCity    *string `json:"venue_city"`
		VenueState   *string `json:"venue_state"`
		StartAt      *string `json:"start_at"`
		EndAt        *string `json:"end_at"`
		MaxGuests    *int    `json:"max_guests"`
		BudgetTotal  *int64  `json:"budget_total"`
		TicketPrice  *int64  `json:"ticket_price"`
		CoverURL     *string `json:"cover_url"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}

	_, err = h.db.Exec(r.Context(),
		`UPDATE events SET
		  title         = COALESCE($3, title),
		  description   = COALESCE($4, description),
		  venue_name    = COALESCE($5, venue_name),
		  venue_address = COALESCE($6, venue_address),
		  venue_city    = COALESCE($7, venue_city),
		  venue_state   = COALESCE($8, venue_state),
		  start_at      = COALESCE($9::timestamptz, start_at),
		  end_at        = COALESCE($10::timestamptz, end_at),
		  max_guests    = COALESCE($11, max_guests),
		  budget_total  = COALESCE($12, budget_total),
		  ticket_price  = COALESCE($13, ticket_price),
		  cover_url     = COALESCE($14, cover_url),
		  updated_at    = NOW()
		 WHERE id = $1 AND owner_id = $2`,
		id, u.ID,
		body.Title, body.Description, body.VenueName, body.VenueAddress,
		body.VenueCity, body.VenueState, body.StartAt, body.EndAt,
		body.MaxGuests, body.BudgetTotal, body.TicketPrice, body.CoverURL,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to update event")
		return
	}

	h.GetEvent(w, r)
}

// POST /events/:id/publish
func (h *EventsHandler) PublishEvent(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil || u == nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}

	// Check event ownership and visibility
	var visibility string
	var ticketPrice int64
	err = h.db.QueryRow(r.Context(),
		`SELECT visibility, ticket_price FROM events WHERE id = $1 AND owner_id = $2`,
		id, u.ID,
	).Scan(&visibility, &ticketPrice)
	if err != nil {
		writeErr(w, http.StatusNotFound, "event not found")
		return
	}

	// Public/ticketed events require KYC tier 1+
	if visibility == "public" || ticketPrice > 0 {
		var kycTier string
		_ = h.db.QueryRow(r.Context(),
			`SELECT kyc_tier FROM users WHERE id = $1`, u.ID,
		).Scan(&kycTier)
		if kycTier == "0" {
			writeErr(w, http.StatusForbidden, "kyc_required:tier_1")
			return
		}
	}

	_, err = h.db.Exec(r.Context(),
		`UPDATE events SET status = 'published', updated_at = NOW() WHERE id = $1 AND owner_id = $2`,
		id, u.ID,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to publish event")
		return
	}

	h.GetEvent(w, r)
}

// DELETE /events/:id
func (h *EventsHandler) DeleteEvent(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil || u == nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}

	_, err = h.db.Exec(r.Context(),
		`UPDATE events SET status = 'cancelled', updated_at = NOW() WHERE id = $1 AND owner_id = $2`,
		id, u.ID,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to cancel event")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "event cancelled"})
}
