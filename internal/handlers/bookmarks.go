package handlers

import (
	"net/http"
	"time"

	"github.com/eventpark/api/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type BookmarkHandler struct{ db *pgxpool.Pool }

func NewBookmarkHandler(db *pgxpool.Pool) *BookmarkHandler {
	return &BookmarkHandler{db: db}
}

type bookmark struct {
	ID        string    `json:"id"`
	ItemType  string    `json:"item_type"`
	ItemID    string    `json:"item_id"`
	CreatedAt time.Time `json:"created_at"`
}

// POST /bookmarks — create bookmark
func (h *BookmarkHandler) Create(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var body struct {
		ItemType string `json:"item_type"`
		ItemID   string `json:"item_id"`
	}
	if err := decode(r, &body); err != nil || body.ItemType == "" || body.ItemID == "" {
		writeErr(w, http.StatusBadRequest, "item_type and item_id required")
		return
	}

	var id string
	err := h.db.QueryRow(r.Context(), `
		INSERT INTO bookmarks (user_id, item_type, item_id)
		VALUES ($1, $2, $3::uuid)
		ON CONFLICT (user_id, item_type, item_id) DO UPDATE SET created_at = NOW()
		RETURNING id
	`, u.ID, body.ItemType, body.ItemID).Scan(&id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to save bookmark")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

// GET /bookmarks?type=vendor — list bookmarks
func (h *BookmarkHandler) List(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	itemType := r.URL.Query().Get("type")

	var (
		rows interface {
			Next() bool
			Scan(...any) error
			Close()
			Err() error
		}
		err error
	)

	if itemType != "" {
		rows, err = h.db.Query(r.Context(), `
			SELECT id, item_type, item_id::text, created_at
			FROM bookmarks WHERE user_id = $1 AND item_type = $2
			ORDER BY created_at DESC
		`, u.ID, itemType)
	} else {
		rows, err = h.db.Query(r.Context(), `
			SELECT id, item_type, item_id::text, created_at
			FROM bookmarks WHERE user_id = $1
			ORDER BY created_at DESC
		`, u.ID)
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to fetch bookmarks")
		return
	}
	defer rows.Close()

	result := []bookmark{}
	for rows.Next() {
		var b bookmark
		if err := rows.Scan(&b.ID, &b.ItemType, &b.ItemID, &b.CreatedAt); err == nil {
			result = append(result, b)
		}
	}
	writeJSON(w, http.StatusOK, result)
}

// DELETE /bookmarks/{type}/{id} — remove bookmark
func (h *BookmarkHandler) Delete(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	itemType := chi.URLParam(r, "type")
	itemID := chi.URLParam(r, "id")

	_, err := h.db.Exec(r.Context(), `
		DELETE FROM bookmarks WHERE user_id = $1 AND item_type = $2 AND item_id = $3::uuid
	`, u.ID, itemType, itemID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to remove bookmark")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "removed"})
}
