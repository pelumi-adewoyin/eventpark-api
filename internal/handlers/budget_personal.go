package handlers

import (
	"fmt"
	"net/http"

	"github.com/eventpark/api/internal/middleware"
	"github.com/eventpark/api/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PersonalBudgetHandler struct {
	db *pgxpool.Pool
}

func NewPersonalBudgetHandler(db *pgxpool.Pool) *PersonalBudgetHandler {
	return &PersonalBudgetHandler{db: db}
}

// GET /budgets — list user's budgets with categories and spent totals
func (h *PersonalBudgetHandler) List(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	rows, err := h.db.Query(r.Context(), `
		SELECT b.id, b.name, b.scope, b.event_id, b.total_ngn,
		       b.period_start::text, b.period_end::text,
		       b.alert_at_70, b.alert_at_90, b.created_at, b.updated_at,
		       COALESCE(SUM(e.amount_ngn), 0) AS total_spent
		FROM personal_budgets b
		LEFT JOIN budget_expenses e ON e.budget_id = b.id
		WHERE b.user_id = $1
		GROUP BY b.id
		ORDER BY b.created_at DESC
	`, u.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to fetch budgets")
		return
	}
	defer rows.Close()

	var budgets []models.PersonalBudget
	for rows.Next() {
		var b models.PersonalBudget
		if err := rows.Scan(
			&b.ID, &b.Name, &b.Scope, &b.EventID, &b.TotalNGN,
			&b.PeriodStart, &b.PeriodEnd,
			&b.AlertAt70, &b.AlertAt90, &b.CreatedAt, &b.UpdatedAt,
			&b.TotalSpent,
		); err == nil {
			budgets = append(budgets, b)
		}
	}
	if budgets == nil {
		budgets = []models.PersonalBudget{}
	}

	// Fetch categories for each budget
	for i, b := range budgets {
		cats, _ := h.fetchCategories(r, b.ID)
		budgets[i].Categories = cats
	}

	writeJSON(w, http.StatusOK, budgets)
}

// POST /budgets — create a budget with categories
func (h *PersonalBudgetHandler) Create(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var body struct {
		Name        string  `json:"name"`
		Scope       string  `json:"scope"`
		EventID     *string `json:"event_id"`
		TotalNGN    int64   `json:"total_ngn"`
		PeriodStart *string `json:"period_start"`
		PeriodEnd   *string `json:"period_end"`
		AlertAt70   bool    `json:"alert_at_70"`
		AlertAt90   bool    `json:"alert_at_90"`
		Categories  []struct {
			Name         string `json:"name"`
			AllocatedNGN int64  `json:"allocated_ngn"`
		} `json:"categories"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	if body.Name == "" || body.TotalNGN <= 0 {
		writeErr(w, http.StatusBadRequest, "name and total_ngn are required")
		return
	}

	tx, err := h.db.Begin(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}

	var budgetID string
	err = tx.QueryRow(r.Context(), `
		INSERT INTO personal_budgets (user_id, name, scope, event_id, total_ngn, period_start, period_end, alert_at_70, alert_at_90)
		VALUES ($1, $2, $3, $4, $5, $6::date, $7::date, $8, $9)
		RETURNING id
	`, u.ID, body.Name, body.Scope, body.EventID, body.TotalNGN,
		body.PeriodStart, body.PeriodEnd, body.AlertAt70, body.AlertAt90,
	).Scan(&budgetID)
	if err != nil {
		_ = tx.Rollback(r.Context())
		writeErr(w, http.StatusInternalServerError, "failed to create budget")
		return
	}

	for _, cat := range body.Categories {
		if cat.Name == "" {
			continue
		}
		_, err = tx.Exec(r.Context(), `
			INSERT INTO budget_categories (budget_id, name, allocated_ngn)
			VALUES ($1, $2, $3)
		`, budgetID, cat.Name, cat.AllocatedNGN)
		if err != nil {
			_ = tx.Rollback(r.Context())
			writeErr(w, http.StatusInternalServerError, "failed to create category")
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, http.StatusInternalServerError, "commit failed")
		return
	}

	// Return created budget
	var b models.PersonalBudget
	_ = h.db.QueryRow(r.Context(), `
		SELECT id, name, scope, event_id, total_ngn, period_start::text, period_end::text,
		       alert_at_70, alert_at_90, created_at, updated_at, 0
		FROM personal_budgets WHERE id = $1
	`, budgetID).Scan(
		&b.ID, &b.Name, &b.Scope, &b.EventID, &b.TotalNGN,
		&b.PeriodStart, &b.PeriodEnd, &b.AlertAt70, &b.AlertAt90,
		&b.CreatedAt, &b.UpdatedAt, &b.TotalSpent,
	)
	b.UserID = u.ID.String()
	b.Categories, _ = h.fetchCategories(r, budgetID)

	writeJSON(w, http.StatusCreated, b)
}

// DELETE /budgets/{id} — delete a budget
func (h *PersonalBudgetHandler) Delete(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	id := chi.URLParam(r, "id")
	res, err := h.db.Exec(r.Context(),
		`DELETE FROM personal_budgets WHERE id = $1 AND user_id = $2`, id, u.ID)
	if err != nil || res.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "budget not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}

// GET /budgets/{id}/expenses — list expenses for a budget
func (h *PersonalBudgetHandler) ListExpenses(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	budgetID := chi.URLParam(r, "id")

	// Verify ownership
	var ownerID uuid.UUID
	if err := h.db.QueryRow(r.Context(), `SELECT user_id FROM personal_budgets WHERE id = $1`, budgetID).Scan(&ownerID); err != nil || ownerID != u.ID {
		writeErr(w, http.StatusNotFound, "budget not found")
		return
	}

	rows, err := h.db.Query(r.Context(), `
		SELECT id, budget_id, category_id, description, amount_ngn,
		       expense_date::text, payment_method, notes, created_at
		FROM budget_expenses
		WHERE budget_id = $1
		ORDER BY expense_date DESC, created_at DESC
	`, budgetID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to fetch expenses")
		return
	}
	defer rows.Close()

	var expenses []models.BudgetExpense
	for rows.Next() {
		var e models.BudgetExpense
		if err := rows.Scan(&e.ID, &e.BudgetID, &e.CategoryID, &e.Description, &e.AmountNGN,
			&e.ExpenseDate, &e.PaymentMethod, &e.Notes, &e.CreatedAt); err == nil {
			expenses = append(expenses, e)
		}
	}
	if expenses == nil {
		expenses = []models.BudgetExpense{}
	}
	writeJSON(w, http.StatusOK, expenses)
}

// POST /budgets/{id}/expenses — add an expense
func (h *PersonalBudgetHandler) AddExpense(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	budgetID := chi.URLParam(r, "id")

	// Verify ownership
	var ownerID uuid.UUID
	if err := h.db.QueryRow(r.Context(), `SELECT user_id FROM personal_budgets WHERE id = $1`, budgetID).Scan(&ownerID); err != nil || ownerID != u.ID {
		writeErr(w, http.StatusNotFound, "budget not found")
		return
	}

	var body struct {
		CategoryID    *string `json:"category_id"`
		Description   string  `json:"description"`
		AmountNGN     int64   `json:"amount_ngn"`
		ExpenseDate   string  `json:"expense_date"`
		PaymentMethod string  `json:"payment_method"`
		Notes         *string `json:"notes"`
	}
	if err := decode(r, &body); err != nil || body.Description == "" || body.AmountNGN <= 0 {
		writeErr(w, http.StatusBadRequest, "description and amount_ngn are required")
		return
	}
	if body.ExpenseDate == "" {
		body.ExpenseDate = "today"
	}
	if body.PaymentMethod == "" {
		body.PaymentMethod = "other"
	}

	var e models.BudgetExpense
	err := h.db.QueryRow(r.Context(), `
		INSERT INTO budget_expenses (budget_id, category_id, description, amount_ngn, expense_date, payment_method, notes)
		VALUES ($1, $2, $3, $4, $5::date, $6, $7)
		RETURNING id, budget_id, category_id, description, amount_ngn, expense_date::text, payment_method, notes, created_at
	`, budgetID, body.CategoryID, body.Description, body.AmountNGN,
		body.ExpenseDate, body.PaymentMethod, body.Notes,
	).Scan(&e.ID, &e.BudgetID, &e.CategoryID, &e.Description, &e.AmountNGN,
		&e.ExpenseDate, &e.PaymentMethod, &e.Notes, &e.CreatedAt)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Sprintf("failed to add expense: %v", err))
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

// DELETE /budgets/{id}/expenses/{expenseId} — delete an expense
func (h *PersonalBudgetHandler) DeleteExpense(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	budgetID := chi.URLParam(r, "id")
	expenseID := chi.URLParam(r, "expenseId")

	var ownerID uuid.UUID
	if err := h.db.QueryRow(r.Context(), `SELECT user_id FROM personal_budgets WHERE id = $1`, budgetID).Scan(&ownerID); err != nil || ownerID != u.ID {
		writeErr(w, http.StatusNotFound, "budget not found")
		return
	}

	res, err := h.db.Exec(r.Context(), `DELETE FROM budget_expenses WHERE id = $1 AND budget_id = $2`, expenseID, budgetID)
	if err != nil || res.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "expense not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}

// fetchCategories loads categories for a budget with spent totals
func (h *PersonalBudgetHandler) fetchCategories(r *http.Request, budgetID string) ([]models.BudgetCategory, error) {
	rows, err := h.db.Query(r.Context(), `
		SELECT c.id, c.budget_id, c.name, c.allocated_ngn,
		       COALESCE(SUM(e.amount_ngn), 0) AS spent_ngn
		FROM budget_categories c
		LEFT JOIN budget_expenses e ON e.category_id = c.id
		WHERE c.budget_id = $1
		GROUP BY c.id
		ORDER BY c.name
	`, budgetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cats []models.BudgetCategory
	for rows.Next() {
		var c models.BudgetCategory
		if err := rows.Scan(&c.ID, &c.BudgetID, &c.Name, &c.AllocatedNGN, &c.SpentNGN); err == nil {
			cats = append(cats, c)
		}
	}
	if cats == nil {
		cats = []models.BudgetCategory{}
	}
	return cats, nil
}
