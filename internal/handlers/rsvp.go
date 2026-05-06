package handlers

import (
	"net/http"
	"time"

	"github.com/eventpark/api/internal/middleware"
	"github.com/eventpark/api/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RSVPHandler struct {
	db *pgxpool.Pool
}

func NewRSVPHandler(db *pgxpool.Pool) *RSVPHandler {
	return &RSVPHandler{db: db}
}

// ─── LOOKUP BY TOKEN (public, no auth) ───────────────────────────────────────

// GET /rsvp/:token — fetch guest + event info for RSVP page
func (h *RSVPHandler) GetRSVP(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		writeErr(w, http.StatusBadRequest, "token is required")
		return
	}

	var g models.RSVPGuest
	err := h.db.QueryRow(r.Context(),
		`SELECT g.id, g.event_id, g.full_name, g.email, g.phone,
		   g.status, g.rsvp_token, g.rsvp_status, g.rsvp_note,
		   g.plus_one, g.plus_one_allowed, g.plus_one_name,
		   g.dietary_req, g.tags, g.relationship, g.iv_template,
		   g.responded_at, g.ticket_code, g.created_at,
		   e.title AS event_title,
		   e.start_at::text AS event_date,
		   e.venue_name AS event_venue,
		   e.cover_url AS event_cover_url,
		   u.full_name AS host_name
		 FROM guests g
		 JOIN events e ON e.id = g.event_id
		 JOIN users u ON u.id = e.owner_id
		 WHERE g.rsvp_token = $1`,
		token,
	).Scan(
		&g.ID, &g.EventID, &g.FullName, &g.Email, &g.Phone,
		&g.Status, &g.RSVPToken, &g.RSVPStatus, &g.RSVPNote,
		&g.PlusOne, &g.PlusOneAllowed, &g.PlusOneName,
		&g.DietaryReq, &g.Tags, &g.Relationship, &g.IVTemplate,
		&g.RespondedAt, &g.TicketCode, &g.CreatedAt,
		&g.EventTitle, &g.EventDate, &g.EventVenue, &g.EventCoverURL, &g.HostName,
	)
	if err != nil {
		writeErr(w, http.StatusNotFound, "invalid or expired RSVP link")
		return
	}

	writeJSON(w, http.StatusOK, g)
}

// ─── RESPOND ──────────────────────────────────────────────────────────────────

// POST /rsvp/:token/respond — guest accepts or declines
func (h *RSVPHandler) Respond(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		writeErr(w, http.StatusBadRequest, "token is required")
		return
	}

	var body struct {
		Status      string  `json:"status"`      // accepted | declined
		Note        *string `json:"note"`
		PlusOne     bool    `json:"plus_one"`
		PlusOneName *string `json:"plus_one_name"`
		DietaryReq  *string `json:"dietary_req"`
	}
	if err := decode(r, &body); err != nil || (body.Status != "accepted" && body.Status != "declined") {
		writeErr(w, http.StatusBadRequest, "status must be 'accepted' or 'declined'")
		return
	}

	now := time.Now()

	var g models.RSVPGuest
	err := h.db.QueryRow(r.Context(),
		`UPDATE guests SET
		   rsvp_status  = $2,
		   rsvp_note    = COALESCE($3, rsvp_note),
		   plus_one     = $4,
		   plus_one_name = COALESCE($5, plus_one_name),
		   dietary_req  = COALESCE($6, dietary_req),
		   responded_at = $7,
		   status       = CASE WHEN $2 = 'accepted' THEN 'rsvp_yes' ELSE 'rsvp_no' END
		 WHERE rsvp_token = $1
		 RETURNING id, event_id, full_name, email, rsvp_status, rsvp_note,
		   plus_one, plus_one_name, dietary_req, ticket_code, responded_at`,
		token, body.Status, body.Note, body.PlusOne, body.PlusOneName, body.DietaryReq, now,
	).Scan(
		&g.ID, &g.EventID, &g.FullName, &g.Email, &g.RSVPStatus, &g.RSVPNote,
		&g.PlusOne, &g.PlusOneName, &g.DietaryReq, &g.TicketCode, &g.RespondedAt,
	)
	if err != nil {
		writeErr(w, http.StatusNotFound, "invalid RSVP token")
		return
	}

	// Notify event owner (fire-and-forget; real impl would queue an email/push)
	go func() {
		var ownerID interface{}
		var eventTitle string
		_ = h.db.QueryRow(r.Context(),
			`SELECT owner_id, title FROM events WHERE id = $1`, g.EventID,
		).Scan(&ownerID, &eventTitle)

		if ownerID != nil {
			_, _ = h.db.Exec(r.Context(),
				`INSERT INTO notifications (user_id, type, title, body)
				 VALUES ($1, 'rsvp_received', $2, $3)`,
				ownerID,
				"RSVP received — "+g.FullName,
				g.FullName+" has "+body.Status+" their invitation to "+eventTitle,
			)
		}
	}()

	writeJSON(w, http.StatusOK, g)
}

// ─── NOTIFICATIONS (for event owner) ─────────────────────────────────────────

// GET /notifications — user's unread notifications
func (h *RSVPHandler) ListNotifications(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	rows, err := h.db.Query(r.Context(),
		`SELECT id, user_id, type, title, body, read, created_at
		 FROM notifications WHERE user_id = $1
		 ORDER BY created_at DESC LIMIT 50`,
		u.ID,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to fetch notifications")
		return
	}
	defer rows.Close()

	notifs := []models.Notification{}
	for rows.Next() {
		var n models.Notification
		if err := rows.Scan(
			&n.ID, &n.UserID, &n.Type, &n.Title, &n.Body, &n.Read, &n.CreatedAt,
		); err == nil {
			notifs = append(notifs, n)
		}
	}
	writeJSON(w, http.StatusOK, notifs)
}

// POST /notifications/mark-read
func (h *RSVPHandler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	_, _ = h.db.Exec(r.Context(),
		`UPDATE notifications SET read = true WHERE user_id = $1 AND read = false`,
		u.ID,
	)
	writeJSON(w, http.StatusOK, map[string]string{"message": "all notifications marked read"})
}
