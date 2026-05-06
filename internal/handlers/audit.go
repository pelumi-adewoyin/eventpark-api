package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/eventpark/api/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AuditHandler struct {
	db *pgxpool.Pool
}

func NewAuditHandler(db *pgxpool.Pool) *AuditHandler {
	return &AuditHandler{db: db}
}

// ─── LIST ─────────────────────────────────────────────────────────────────────

// GET /orgs/:orgId/audit?action=xxx&entity=xxx&from=xxx&to=xxx
func (h *AuditHandler) ListAuditLogs(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(chi.URLParam(r, "orgId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid org id")
		return
	}

	action := r.URL.Query().Get("action")
	entity := r.URL.Query().Get("entity")
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	query := `SELECT id, org_id, user_id, actor_name, action, entity_type, entity_id,
		  detail, previous_hash, entry_hash, created_at
		FROM audit_logs
		WHERE (org_id = $1 OR org_id IS NULL)`
	args := []any{orgID}

	if action != "" {
		args = append(args, "%"+action+"%")
		query += fmt.Sprintf(` AND action ILIKE $%d`, len(args))
	}
	if entity != "" {
		args = append(args, entity)
		query += fmt.Sprintf(` AND entity_type = $%d`, len(args))
	}
	if from != "" {
		args = append(args, from)
		query += fmt.Sprintf(` AND created_at >= $%d::timestamptz`, len(args))
	}
	if to != "" {
		args = append(args, to)
		query += fmt.Sprintf(` AND created_at <= $%d::timestamptz`, len(args))
	}
	query += ` ORDER BY created_at DESC LIMIT 500`

	rows, err := h.db.Query(r.Context(), query, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to query audit log")
		return
	}
	defer rows.Close()

	logs := []models.AuditLog{}
	for rows.Next() {
		var l models.AuditLog
		if err := rows.Scan(
			&l.ID, &l.OrgID, &l.UserID, &l.ActorName, &l.Action,
			&l.EntityType, &l.EntityID, &l.Detail,
			&l.PreviousHash, &l.EntryHash, &l.CreatedAt,
		); err == nil {
			logs = append(logs, l)
		}
	}
	writeJSON(w, http.StatusOK, logs)
}

// GET /orgs/:orgId/audit/verify — verify the hash chain
func (h *AuditHandler) VerifyChain(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(chi.URLParam(r, "orgId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid org id")
		return
	}

	rows, err := h.db.Query(r.Context(),
		`SELECT id, previous_hash, entry_hash, action, created_at
		 FROM audit_logs WHERE org_id = $1 ORDER BY created_at ASC`,
		orgID,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to query audit log")
		return
	}
	defer rows.Close()

	type entry struct {
		ID           uuid.UUID
		PreviousHash *string
		EntryHash    string
		Action       string
		CreatedAt    time.Time
	}

	entries := []entry{}
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.ID, &e.PreviousHash, &e.EntryHash, &e.Action, &e.CreatedAt); err == nil {
			entries = append(entries, e)
		}
	}

	// Verify chain
	valid := true
	broken := []string{}
	var prevHash string

	for _, e := range entries {
		expectedPrev := prevHash
		if e.PreviousHash == nil && expectedPrev != "" {
			valid = false
			broken = append(broken, e.ID.String())
		} else if e.PreviousHash != nil && *e.PreviousHash != expectedPrev {
			valid = false
			broken = append(broken, e.ID.String())
		}
		prevHash = e.EntryHash
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"valid":         valid,
		"entries_count": len(entries),
		"broken_links":  broken,
	})
}

// ─── HELPER: writeAuditLog (used by all handlers) ─────────────────────────────

// writeAuditLog appends an entry to the audit log with hash chaining.
// Call this from any handler after a significant action.
// userID can be nil for system actions.
func writeAuditLog(
	ctx context.Context,
	db *pgxpool.Pool,
	orgID uuid.UUID,
	userID interface{}, // *uuid.UUID or uuid.UUID or nil
	action string,
	entityType string,
	entityID uuid.UUID,
	detail string,
) {
	// Resolve userID to *uuid.UUID
	var uid *uuid.UUID
	switch v := userID.(type) {
	case uuid.UUID:
		uid = &v
	case *uuid.UUID:
		uid = v
	}

	// Get actor name
	var actorName string
	if uid != nil {
		_ = db.QueryRow(ctx, `SELECT COALESCE(full_name, email::text, phone) FROM users WHERE id = $1`, uid).Scan(&actorName)
	}
	if actorName == "" {
		actorName = "System"
	}

	// Get previous hash for this org
	var prevHash *string
	_ = db.QueryRow(ctx,
		`SELECT entry_hash FROM audit_logs WHERE org_id = $1 ORDER BY created_at DESC LIMIT 1`,
		orgID,
	).Scan(&prevHash)

	// Compute this entry's hash: SHA256(prevHash + orgID + userID + action + entityType + entityID + detail + now)
	prev := ""
	if prevHash != nil {
		prev = *prevHash
	}
	raw := fmt.Sprintf("%s|%s|%v|%s|%s|%s|%s|%d",
		prev, orgID, uid, action, entityType, entityID, detail, time.Now().UnixNano())
	hash := sha256.Sum256([]byte(raw))
	entryHash := hex.EncodeToString(hash[:])[:16] // truncate to 16 chars for readability

	_, _ = db.Exec(ctx,
		`INSERT INTO audit_logs
		   (org_id, user_id, actor_name, action, entity_type, entity_id, detail, previous_hash, entry_hash)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		orgID, uid, actorName, action, entityType, entityID, detail, prevHash, entryHash,
	)
}

// ─── EXPORT ───────────────────────────────────────────────────────────────────

// GET /orgs/:orgId/audit/export — download as CSV
func (h *AuditHandler) ExportAuditCSV(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(chi.URLParam(r, "orgId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid org id")
		return
	}

	rows, err := h.db.Query(r.Context(),
		`SELECT created_at, actor_name, action, entity_type, detail, entry_hash
		 FROM audit_logs WHERE org_id = $1 ORDER BY created_at DESC`,
		orgID,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to export audit log")
		return
	}
	defer rows.Close()

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="audit_log.csv"`)

	_, _ = fmt.Fprintln(w, "Timestamp,Actor,Action,Entity Type,Detail,Hash")
	for rows.Next() {
		var ts time.Time
		var actor, action, entityType, detail, hash string
		if err := rows.Scan(&ts, &actor, &action, &entityType, &detail, &hash); err == nil {
			_, _ = fmt.Fprintf(w, "%s,%s,%s,%s,%q,%s\n",
				ts.Format(time.RFC3339), actor, action, entityType, detail, hash)
		}
	}
}
