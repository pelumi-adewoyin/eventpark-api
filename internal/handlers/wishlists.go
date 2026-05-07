package handlers

import (
	"net/http"
	"time"

	"github.com/eventpark/api/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type WishlistHandler struct{ db *pgxpool.Pool }

func NewWishlistHandler(db *pgxpool.Pool) *WishlistHandler {
	return &WishlistHandler{db: db}
}

type wishlistItem struct {
	ID          uuid.UUID  `json:"id"`
	WishlistID  uuid.UUID  `json:"wishlist_id"`
	Name        string     `json:"name"`
	Description *string    `json:"description,omitempty"`
	PriceNGN    *int64     `json:"price_ngn,omitempty"`
	URL         *string    `json:"url,omitempty"`
	ImageURL    *string    `json:"image_url,omitempty"`
	IsReserved  bool       `json:"is_reserved"`
	ReservedBy  *uuid.UUID `json:"reserved_by,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

type wishlist struct {
	ID          uuid.UUID      `json:"id"`
	UserID      uuid.UUID      `json:"user_id"`
	Name        string         `json:"name"`
	Slug        *string        `json:"slug,omitempty"`
	Description *string        `json:"description,omitempty"`
	IsPublic    bool           `json:"is_public"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	Items       []wishlistItem `json:"items,omitempty"`
}

// GET /wishlists — list user's wishlists
func (h *WishlistHandler) List(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	rows, err := h.db.Query(r.Context(), `
		SELECT id, user_id, name, slug, description, is_public, created_at, updated_at
		FROM wishlists WHERE user_id = $1
		ORDER BY created_at DESC
	`, u.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to fetch wishlists")
		return
	}
	defer rows.Close()

	result := []wishlist{}
	for rows.Next() {
		var wl wishlist
		if err := rows.Scan(
			&wl.ID, &wl.UserID, &wl.Name, &wl.Slug, &wl.Description,
			&wl.IsPublic, &wl.CreatedAt, &wl.UpdatedAt,
		); err == nil {
			result = append(result, wl)
		}
	}
	writeJSON(w, http.StatusOK, result)
}

// POST /wishlists — create wishlist
func (h *WishlistHandler) Create(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var body struct {
		Name        string  `json:"name"`
		Description *string `json:"description"`
		IsPublic    bool    `json:"is_public"`
	}
	if err := decode(r, &body); err != nil || body.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}

	var wl wishlist
	err := h.db.QueryRow(r.Context(), `
		INSERT INTO wishlists (user_id, name, description, is_public)
		VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, name, slug, description, is_public, created_at, updated_at
	`, u.ID, body.Name, body.Description, body.IsPublic).Scan(
		&wl.ID, &wl.UserID, &wl.Name, &wl.Slug, &wl.Description,
		&wl.IsPublic, &wl.CreatedAt, &wl.UpdatedAt,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to create wishlist")
		return
	}
	wl.Items = []wishlistItem{}
	writeJSON(w, http.StatusCreated, wl)
}

// GET /wishlists/{id} — get a wishlist with its items
func (h *WishlistHandler) Get(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	wishlistID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid wishlist id")
		return
	}

	var wl wishlist
	err = h.db.QueryRow(r.Context(), `
		SELECT id, user_id, name, slug, description, is_public, created_at, updated_at
		FROM wishlists WHERE id = $1 AND (user_id = $2 OR is_public = true)
	`, wishlistID, u.ID).Scan(
		&wl.ID, &wl.UserID, &wl.Name, &wl.Slug, &wl.Description,
		&wl.IsPublic, &wl.CreatedAt, &wl.UpdatedAt,
	)
	if err != nil {
		writeErr(w, http.StatusNotFound, "wishlist not found")
		return
	}

	// Fetch items
	itemRows, err := h.db.Query(r.Context(), `
		SELECT id, wishlist_id, name, description, price_ngn, url, image_url, is_reserved, reserved_by, created_at
		FROM wishlist_items WHERE wishlist_id = $1 ORDER BY created_at ASC
	`, wishlistID)
	if err == nil {
		defer itemRows.Close()
		for itemRows.Next() {
			var item wishlistItem
			if err := itemRows.Scan(
				&item.ID, &item.WishlistID, &item.Name, &item.Description,
				&item.PriceNGN, &item.URL, &item.ImageURL,
				&item.IsReserved, &item.ReservedBy, &item.CreatedAt,
			); err == nil {
				wl.Items = append(wl.Items, item)
			}
		}
	}
	if wl.Items == nil {
		wl.Items = []wishlistItem{}
	}

	writeJSON(w, http.StatusOK, wl)
}

// POST /wishlists/{id}/items — add item to wishlist
func (h *WishlistHandler) AddItem(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	wishlistID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid wishlist id")
		return
	}

	// Verify ownership
	var ownerID uuid.UUID
	if err := h.db.QueryRow(r.Context(), `SELECT user_id FROM wishlists WHERE id = $1`, wishlistID).Scan(&ownerID); err != nil || ownerID != u.ID {
		writeErr(w, http.StatusNotFound, "wishlist not found")
		return
	}

	var body struct {
		Name        string  `json:"name"`
		Description *string `json:"description"`
		PriceNGN    *int64  `json:"price_ngn"`
		URL         *string `json:"url"`
		ImageURL    *string `json:"image_url"`
	}
	if err := decode(r, &body); err != nil || body.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}

	var item wishlistItem
	err = h.db.QueryRow(r.Context(), `
		INSERT INTO wishlist_items (wishlist_id, name, description, price_ngn, url, image_url)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, wishlist_id, name, description, price_ngn, url, image_url, is_reserved, reserved_by, created_at
	`, wishlistID, body.Name, body.Description, body.PriceNGN, body.URL, body.ImageURL).Scan(
		&item.ID, &item.WishlistID, &item.Name, &item.Description,
		&item.PriceNGN, &item.URL, &item.ImageURL,
		&item.IsReserved, &item.ReservedBy, &item.CreatedAt,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to add item")
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

// DELETE /wishlists/{id}/items/{itemId} — remove item from wishlist
func (h *WishlistHandler) DeleteItem(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	wishlistID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid wishlist id")
		return
	}

	itemID, err := uuid.Parse(chi.URLParam(r, "itemId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid item id")
		return
	}

	// Verify ownership of wishlist
	var ownerID uuid.UUID
	if err := h.db.QueryRow(r.Context(), `SELECT user_id FROM wishlists WHERE id = $1`, wishlistID).Scan(&ownerID); err != nil || ownerID != u.ID {
		writeErr(w, http.StatusNotFound, "wishlist not found")
		return
	}

	res, err := h.db.Exec(r.Context(), `
		DELETE FROM wishlist_items WHERE id = $1 AND wishlist_id = $2
	`, itemID, wishlistID)
	if err != nil || res.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "item not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}
