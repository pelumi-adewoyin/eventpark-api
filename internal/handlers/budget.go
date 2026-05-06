package handlers

import (
	"net/http"

	"github.com/eventpark/api/internal/middleware"
	"github.com/eventpark/api/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type BudgetHandler struct {
	db *pgxpool.Pool
}

func NewBudgetHandler(db *pgxpool.Pool) *BudgetHandler {
	return &BudgetHandler{db: db}
}

// ─── LIST ─────────────────────────────────────────────────────────────────────

// GET /events/:id/budget
func (h *BudgetHandler) ListBudgetLines(w http.ResponseWriter, r *http.Request) {
	eventID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid event id")
		return
	}

	rows, err := h.db.Query(r.Context(),
		`SELECT id, event_id, category, label, allocated, spent, notes, created_at, updated_at
		 FROM event_budget_lines
		 WHERE event_id = $1
		 ORDER BY category, label`,
		eventID,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to list budget lines")
		return
	}
	defer rows.Close()

	lines := []models.EventBudgetLine{}
	for rows.Next() {
		var bl models.EventBudgetLine
		if err := rows.Scan(
			&bl.ID, &bl.EventID, &bl.Category, &bl.Label,
			&bl.Allocated, &bl.Spent, &bl.Notes,
			&bl.CreatedAt, &bl.UpdatedAt,
		); err == nil {
			lines = append(lines, bl)
		}
	}

	// Compute totals
	var totalAllocated, totalSpent int64
	for _, l := range lines {
		totalAllocated += l.Allocated
		totalSpent += l.Spent
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"lines":           lines,
		"total_allocated": totalAllocated,
		"total_spent":     totalSpent,
		"total_remaining": totalAllocated - totalSpent,
	})
}

// ─── CREATE ───────────────────────────────────────────────────────────────────

// POST /events/:id/budget
func (h *BudgetHandler) CreateBudgetLine(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	eventID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil || u == nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}

	// Verify ownership or membership
	var ownerID uuid.UUID
	_ = h.db.QueryRow(r.Context(), `SELECT owner_id FROM events WHERE id = $1`, eventID).Scan(&ownerID)
	if ownerID != u.ID {
		writeErr(w, http.StatusForbidden, "not authorized")
		return
	}

	var body struct {
		Category  string  `json:"category"`
		Label     string  `json:"label"`
		Allocated int64   `json:"allocated"`
		Notes     *string `json:"notes"`
	}
	if err := decode(r, &body); err != nil || body.Label == "" || body.Allocated == 0 {
		writeErr(w, http.StatusBadRequest, "label and allocated are required")
		return
	}
	if body.Category == "" {
		body.Category = "miscellaneous"
	}

	var bl models.EventBudgetLine
	err = h.db.QueryRow(r.Context(),
		`INSERT INTO event_budget_lines (event_id, category, label, allocated, notes)
		 VALUES ($1,$2,$3,$4,$5)
		 RETURNING id, event_id, category, label, allocated, spent, notes, created_at, updated_at`,
		eventID, body.Category, body.Label, body.Allocated, body.Notes,
	).Scan(
		&bl.ID, &bl.EventID, &bl.Category, &bl.Label,
		&bl.Allocated, &bl.Spent, &bl.Notes,
		&bl.CreatedAt, &bl.UpdatedAt,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to create budget line")
		return
	}

	// Update event total budget
	_, _ = h.db.Exec(r.Context(),
		`UPDATE events SET budget_total = (
		   SELECT COALESCE(SUM(allocated), 0) FROM event_budget_lines WHERE event_id = $1
		 ), updated_at = NOW() WHERE id = $1`,
		eventID,
	)

	writeJSON(w, http.StatusCreated, bl)
}

// ─── UPDATE ───────────────────────────────────────────────────────────────────

// PATCH /events/:id/budget/:lineId
func (h *BudgetHandler) UpdateBudgetLine(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	eventID, _ := uuid.Parse(chi.URLParam(r, "id"))
	lineID, err := uuid.Parse(chi.URLParam(r, "lineId"))
	if err != nil || u == nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}

	var body struct {
		Category  *string `json:"category"`
		Label     *string `json:"label"`
		Allocated *int64  `json:"allocated"`
		Spent     *int64  `json:"spent"`
		Notes     *string `json:"notes"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}

	_, err = h.db.Exec(r.Context(),
		`UPDATE event_budget_lines SET
		   category  = COALESCE($3, category),
		   label     = COALESCE($4, label),
		   allocated = COALESCE($5, allocated),
		   spent     = COALESCE($6, spent),
		   notes     = COALESCE($7, notes),
		   updated_at = NOW()
		 WHERE id = $1 AND event_id = $2`,
		lineID, eventID,
		body.Category, body.Label, body.Allocated, body.Spent, body.Notes,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to update budget line")
		return
	}

	// Recompute event budget_total
	_, _ = h.db.Exec(r.Context(),
		`UPDATE events SET budget_total = (
		   SELECT COALESCE(SUM(allocated), 0) FROM event_budget_lines WHERE event_id = $1
		 ), updated_at = NOW() WHERE id = $1`,
		eventID,
	)

	writeJSON(w, http.StatusOK, map[string]string{"message": "updated"})
}

// ─── DELETE ───────────────────────────────────────────────────────────────────

// DELETE /events/:id/budget/:lineId
func (h *BudgetHandler) DeleteBudgetLine(w http.ResponseWriter, r *http.Request) {
	eventID, _ := uuid.Parse(chi.URLParam(r, "id"))
	lineID, err := uuid.Parse(chi.URLParam(r, "lineId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid line id")
		return
	}

	_, err = h.db.Exec(r.Context(),
		`DELETE FROM event_budget_lines WHERE id = $1 AND event_id = $2`,
		lineID, eventID,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to delete budget line")
		return
	}

	// Recompute event budget_total
	_, _ = h.db.Exec(r.Context(),
		`UPDATE events SET budget_total = (
		   SELECT COALESCE(SUM(allocated), 0) FROM event_budget_lines WHERE event_id = $1
		 ), updated_at = NOW() WHERE id = $1`,
		eventID,
	)

	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}

// ─── SUMMARY ─────────────────────────────────────────────────────────────────

// GET /events/:id/budget/summary — by category with percentages
func (h *BudgetHandler) BudgetSummary(w http.ResponseWriter, r *http.Request) {
	eventID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid event id")
		return
	}

	rows, err := h.db.Query(r.Context(),
		`SELECT category,
		   SUM(allocated) AS allocated,
		   SUM(spent) AS spent,
		   ROUND(SUM(spent)::numeric / NULLIF(SUM(allocated), 0) * 100, 1) AS pct_used
		 FROM event_budget_lines
		 WHERE event_id = $1
		 GROUP BY category
		 ORDER BY allocated DESC`,
		eventID,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to summarize budget")
		return
	}
	defer rows.Close()

	type CategorySummary struct {
		Category  string  `json:"category"`
		Allocated int64   `json:"allocated"`
		Spent     int64   `json:"spent"`
		PctUsed   float64 `json:"pct_used"`
	}

	summary := []CategorySummary{}
	for rows.Next() {
		var s CategorySummary
		if err := rows.Scan(&s.Category, &s.Allocated, &s.Spent, &s.PctUsed); err == nil {
			summary = append(summary, s)
		}
	}
	writeJSON(w, http.StatusOK, summary)
}
