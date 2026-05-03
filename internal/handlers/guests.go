package handlers

import (
	"net/http"
	"time"

	"github.com/eventpark/api/internal/middleware"
	"github.com/eventpark/api/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type GuestsHandler struct {
	db *pgxpool.Pool
}

func NewGuestsHandler(db *pgxpool.Pool) *GuestsHandler {
	return &GuestsHandler{db: db}
}

// POST /events/:id/guests — add/invite guests
func (h *GuestsHandler) InviteGuests(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	eventID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil || u == nil {
		writeErr(w, http.StatusBadRequest, "invalid event id")
		return
	}

	// Verify ownership
	var ownerID uuid.UUID
	err = h.db.QueryRow(r.Context(), `SELECT owner_id FROM events WHERE id = $1`, eventID).Scan(&ownerID)
	if err != nil || ownerID != u.ID {
		writeErr(w, http.StatusForbidden, "not the event owner")
		return
	}

	var body struct {
		Guests []struct {
			FullName string  `json:"full_name"`
			Email    *string `json:"email"`
			Phone    *string `json:"phone"`
		} `json:"guests"`
	}
	if err := decode(r, &body); err != nil || len(body.Guests) == 0 {
		writeErr(w, http.StatusBadRequest, "guests array is required")
		return
	}

	added := []models.Guest{}
	for _, g := range body.Guests {
		if g.FullName == "" {
			continue
		}
		var guest models.Guest
		err := h.db.QueryRow(r.Context(),
			`INSERT INTO guests (event_id, full_name, email, phone)
			 VALUES ($1, $2, $3, $4)
			 RETURNING id, event_id, user_id, full_name, email, phone, status, ticket_code, seat_number, checked_in_at, created_at`,
			eventID, g.FullName, g.Email, g.Phone,
		).Scan(
			&guest.ID, &guest.EventID, &guest.UserID, &guest.FullName,
			&guest.Email, &guest.Phone, &guest.Status, &guest.TicketCode,
			&guest.SeatNumber, &guest.CheckedInAt, &guest.CreatedAt,
		)
		if err == nil {
			added = append(added, guest)
		}
	}
	writeJSON(w, http.StatusCreated, added)
}

// GET /events/:id/guests
func (h *GuestsHandler) ListGuests(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	eventID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil || u == nil {
		writeErr(w, http.StatusBadRequest, "invalid event id")
		return
	}

	rows, err := h.db.Query(r.Context(),
		`SELECT id, event_id, user_id, full_name, email, phone, status, ticket_code, seat_number, checked_in_at, created_at
		 FROM guests WHERE event_id = $1 ORDER BY full_name`,
		eventID,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to fetch guests")
		return
	}
	defer rows.Close()

	guests := []models.Guest{}
	for rows.Next() {
		var g models.Guest
		if err := rows.Scan(
			&g.ID, &g.EventID, &g.UserID, &g.FullName,
			&g.Email, &g.Phone, &g.Status, &g.TicketCode,
			&g.SeatNumber, &g.CheckedInAt, &g.CreatedAt,
		); err == nil {
			guests = append(guests, g)
		}
	}
	writeJSON(w, http.StatusOK, guests)
}

// POST /checkin/:eventId — check in by ticket code or guest ID
func (h *GuestsHandler) CheckIn(w http.ResponseWriter, r *http.Request) {
	eventID, err := uuid.Parse(chi.URLParam(r, "eventId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid event id")
		return
	}

	var body struct {
		TicketCode string     `json:"ticket_code"`
		GuestID    *uuid.UUID `json:"guest_id"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}

	now := time.Now()
	var guest models.Guest

	if body.TicketCode != "" {
		err = h.db.QueryRow(r.Context(),
			`UPDATE guests SET status = 'checked_in', checked_in_at = $3
			 WHERE event_id = $1 AND ticket_code = $2
			 RETURNING id, event_id, user_id, full_name, email, phone, status, ticket_code, seat_number, checked_in_at, created_at`,
			eventID, body.TicketCode, now,
		).Scan(
			&guest.ID, &guest.EventID, &guest.UserID, &guest.FullName,
			&guest.Email, &guest.Phone, &guest.Status, &guest.TicketCode,
			&guest.SeatNumber, &guest.CheckedInAt, &guest.CreatedAt,
		)
	} else if body.GuestID != nil {
		err = h.db.QueryRow(r.Context(),
			`UPDATE guests SET status = 'checked_in', checked_in_at = $3
			 WHERE event_id = $1 AND id = $2
			 RETURNING id, event_id, user_id, full_name, email, phone, status, ticket_code, seat_number, checked_in_at, created_at`,
			eventID, body.GuestID, now,
		).Scan(
			&guest.ID, &guest.EventID, &guest.UserID, &guest.FullName,
			&guest.Email, &guest.Phone, &guest.Status, &guest.TicketCode,
			&guest.SeatNumber, &guest.CheckedInAt, &guest.CreatedAt,
		)
	} else {
		writeErr(w, http.StatusBadRequest, "ticket_code or guest_id required")
		return
	}

	if err != nil {
		writeErr(w, http.StatusNotFound, "guest not found")
		return
	}

	// Get live stats
	var total, checkedIn int
	_ = h.db.QueryRow(r.Context(),
		`SELECT COUNT(*), COUNT(*) FILTER (WHERE status = 'checked_in') FROM guests WHERE event_id = $1`,
		eventID,
	).Scan(&total, &checkedIn)

	writeJSON(w, http.StatusOK, map[string]any{
		"guest":      guest,
		"total":      total,
		"checked_in": checkedIn,
	})
}

// GET /checkin/:eventId/stats
func (h *GuestsHandler) CheckInStats(w http.ResponseWriter, r *http.Request) {
	eventID, err := uuid.Parse(chi.URLParam(r, "eventId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid event id")
		return
	}

	var total, checkedIn int
	err = h.db.QueryRow(r.Context(),
		`SELECT COUNT(*), COUNT(*) FILTER (WHERE status = 'checked_in') FROM guests WHERE event_id = $1`,
		eventID,
	).Scan(&total, &checkedIn)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to get stats")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"total":      total,
		"checked_in": checkedIn,
		"remaining":  total - checkedIn,
	})
}
