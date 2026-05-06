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

type OrganisationsHandler struct {
	db *pgxpool.Pool
}

func NewOrganisationsHandler(db *pgxpool.Pool) *OrganisationsHandler {
	return &OrganisationsHandler{db: db}
}

// ─── ORGANISATIONS ────────────────────────────────────────────────────────────

// POST /orgs — create organisation
func (h *OrganisationsHandler) CreateOrg(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var body struct {
		Name     string  `json:"name"`
		RCNumber *string `json:"rc_number"`
		TIN      *string `json:"tin"`
		Industry *string `json:"industry"`
		Address  *string `json:"address"`
		City     *string `json:"city"`
		State    *string `json:"state"`
		Website  *string `json:"website"`
	}
	if err := decode(r, &body); err != nil || body.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}

	var org models.Organisation
	err := h.db.QueryRow(r.Context(),
		`INSERT INTO organisations (name, rc_number, tin, industry, address, city, state, website, owner_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		 RETURNING id, name, rc_number, tin, industry, logo_url, address, city, state, website,
		   owner_id, kyb_status, kyb_tier, plan, created_at, updated_at`,
		body.Name, body.RCNumber, body.TIN, body.Industry,
		body.Address, body.City, body.State, body.Website, u.ID,
	).Scan(
		&org.ID, &org.Name, &org.RCNumber, &org.TIN, &org.Industry, &org.LogoURL,
		&org.Address, &org.City, &org.State, &org.Website,
		&org.OwnerID, &org.KYBStatus, &org.KYBTier, &org.Plan,
		&org.CreatedAt, &org.UpdatedAt,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Sprintf("failed to create organisation: %v", err))
		return
	}

	// Auto-add owner as org member with 'owner' role
	_, _ = h.db.Exec(r.Context(),
		`INSERT INTO org_members (org_id, user_id, role, role_enum, active)
		 VALUES ($1, $2, 'owner', 'owner', true) ON CONFLICT DO NOTHING`,
		org.ID, u.ID,
	)

	writeJSON(w, http.StatusCreated, org)
}

// GET /orgs/me — get my organisation
func (h *OrganisationsHandler) GetMyOrg(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var org models.Organisation
	err := h.db.QueryRow(r.Context(),
		`SELECT o.id, o.name, o.rc_number, o.tin, o.industry, o.logo_url,
		   o.address, o.city, o.state, o.website, o.owner_id,
		   o.kyb_status, o.kyb_tier, o.plan, o.created_at, o.updated_at
		 FROM organisations o
		 JOIN org_members m ON m.org_id = o.id
		 WHERE m.user_id = $1 AND m.active = true
		 LIMIT 1`,
		u.ID,
	).Scan(
		&org.ID, &org.Name, &org.RCNumber, &org.TIN, &org.Industry, &org.LogoURL,
		&org.Address, &org.City, &org.State, &org.Website, &org.OwnerID,
		&org.KYBStatus, &org.KYBTier, &org.Plan,
		&org.CreatedAt, &org.UpdatedAt,
	)
	if err != nil {
		writeErr(w, http.StatusNotFound, "no organisation found for this user")
		return
	}
	writeJSON(w, http.StatusOK, org)
}

// PATCH /orgs/:orgId — update org profile
func (h *OrganisationsHandler) UpdateOrg(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	orgID, err := uuid.Parse(chi.URLParam(r, "orgId"))
	if err != nil || u == nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}

	var body struct {
		Name     *string `json:"name"`
		RCNumber *string `json:"rc_number"`
		TIN      *string `json:"tin"`
		Industry *string `json:"industry"`
		Address  *string `json:"address"`
		City     *string `json:"city"`
		State    *string `json:"state"`
		Website  *string `json:"website"`
		LogoURL  *string `json:"logo_url"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}

	_, err = h.db.Exec(r.Context(),
		`UPDATE organisations SET
		   name      = COALESCE($3, name),
		   rc_number = COALESCE($4, rc_number),
		   tin       = COALESCE($5, tin),
		   industry  = COALESCE($6, industry),
		   address   = COALESCE($7, address),
		   city      = COALESCE($8, city),
		   state     = COALESCE($9, state),
		   website   = COALESCE($10, website),
		   logo_url  = COALESCE($11, logo_url),
		   updated_at = NOW()
		 WHERE id = $1 AND owner_id = $2`,
		orgID, u.ID,
		body.Name, body.RCNumber, body.TIN, body.Industry,
		body.Address, body.City, body.State, body.Website, body.LogoURL,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to update organisation")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "updated"})
}

// ─── KYB ──────────────────────────────────────────────────────────────────────

// POST /orgs/:orgId/kyb-documents — upload KYB document record
func (h *OrganisationsHandler) UploadKYBDocument(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	orgID, err := uuid.Parse(chi.URLParam(r, "orgId"))
	if err != nil || u == nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}

	var body struct {
		DocType string `json:"doc_type"`
		FileURL string `json:"file_url"`
	}
	if err := decode(r, &body); err != nil || body.DocType == "" || body.FileURL == "" {
		writeErr(w, http.StatusBadRequest, "doc_type and file_url are required")
		return
	}

	var doc models.KYBDocument
	err = h.db.QueryRow(r.Context(),
		`INSERT INTO kyb_documents (org_id, doc_type, file_url)
		 VALUES ($1, $2, $3)
		 RETURNING id, org_id, doc_type, file_url, status, reviewer_id, reviewed_at, notes, created_at`,
		orgID, body.DocType, body.FileURL,
	).Scan(
		&doc.ID, &doc.OrgID, &doc.DocType, &doc.FileURL,
		&doc.Status, &doc.ReviewerID, &doc.ReviewedAt, &doc.Notes, &doc.CreatedAt,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to upload document")
		return
	}
	writeJSON(w, http.StatusCreated, doc)
}

// GET /orgs/:orgId/kyb-documents
func (h *OrganisationsHandler) ListKYBDocuments(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(chi.URLParam(r, "orgId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid org id")
		return
	}

	rows, err := h.db.Query(r.Context(),
		`SELECT id, org_id, doc_type, file_url, status, reviewer_id, reviewed_at, notes, created_at
		 FROM kyb_documents WHERE org_id = $1 ORDER BY created_at DESC`,
		orgID,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to list documents")
		return
	}
	defer rows.Close()

	docs := []models.KYBDocument{}
	for rows.Next() {
		var d models.KYBDocument
		if err := rows.Scan(
			&d.ID, &d.OrgID, &d.DocType, &d.FileURL,
			&d.Status, &d.ReviewerID, &d.ReviewedAt, &d.Notes, &d.CreatedAt,
		); err == nil {
			docs = append(docs, d)
		}
	}
	writeJSON(w, http.StatusOK, docs)
}

// ─── DEPARTMENTS ──────────────────────────────────────────────────────────────

// GET /orgs/:orgId/departments
func (h *OrganisationsHandler) ListDepartments(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(chi.URLParam(r, "orgId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid org id")
		return
	}

	rows, err := h.db.Query(r.Context(),
		`SELECT d.id, d.org_id, d.name, d.head_id, u.full_name, d.budget, d.created_at,
		   COUNT(m.user_id) AS members
		 FROM departments d
		 LEFT JOIN users u ON u.id = d.head_id
		 LEFT JOIN org_members m ON m.dept_id = d.id AND m.active = true
		 WHERE d.org_id = $1
		 GROUP BY d.id, u.full_name
		 ORDER BY d.name`,
		orgID,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to list departments")
		return
	}
	defer rows.Close()

	depts := []models.Department{}
	for rows.Next() {
		var d models.Department
		if err := rows.Scan(
			&d.ID, &d.OrgID, &d.Name, &d.HeadID, &d.HeadName,
			&d.Budget, &d.CreatedAt, &d.Members,
		); err == nil {
			depts = append(depts, d)
		}
	}
	writeJSON(w, http.StatusOK, depts)
}

// POST /orgs/:orgId/departments
func (h *OrganisationsHandler) CreateDepartment(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	orgID, err := uuid.Parse(chi.URLParam(r, "orgId"))
	if err != nil || u == nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}

	var body struct {
		Name   string     `json:"name"`
		HeadID *uuid.UUID `json:"head_id"`
		Budget int64      `json:"budget"`
	}
	if err := decode(r, &body); err != nil || body.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}

	var dept models.Department
	err = h.db.QueryRow(r.Context(),
		`INSERT INTO departments (org_id, name, head_id, budget)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, org_id, name, head_id, budget, created_at`,
		orgID, body.Name, body.HeadID, body.Budget,
	).Scan(&dept.ID, &dept.OrgID, &dept.Name, &dept.HeadID, &dept.Budget, &dept.CreatedAt)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to create department")
		return
	}
	writeJSON(w, http.StatusCreated, dept)
}

// PATCH /orgs/:orgId/departments/:deptId
func (h *OrganisationsHandler) UpdateDepartment(w http.ResponseWriter, r *http.Request) {
	orgID, _ := uuid.Parse(chi.URLParam(r, "orgId"))
	deptID, err := uuid.Parse(chi.URLParam(r, "deptId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid dept id")
		return
	}

	var body struct {
		Name   *string    `json:"name"`
		HeadID *uuid.UUID `json:"head_id"`
		Budget *int64     `json:"budget"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}

	_, err = h.db.Exec(r.Context(),
		`UPDATE departments SET
		   name    = COALESCE($3, name),
		   head_id = COALESCE($4, head_id),
		   budget  = COALESCE($5, budget)
		 WHERE id = $1 AND org_id = $2`,
		deptID, orgID, body.Name, body.HeadID, body.Budget,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to update department")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "updated"})
}

// ─── MEMBERS / EMPLOYEES ──────────────────────────────────────────────────────

// GET /orgs/:orgId/members
func (h *OrganisationsHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(chi.URLParam(r, "orgId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid org id")
		return
	}

	rows, err := h.db.Query(r.Context(),
		`SELECT m.org_id, m.user_id, m.role, m.role_enum, m.dept_id, m.active,
		   m.invited_by, m.joined_at, m.updated_at,
		   u.full_name, u.email, u.avatar_url, d.name AS dept_name
		 FROM org_members m
		 JOIN users u ON u.id = m.user_id
		 LEFT JOIN departments d ON d.id = m.dept_id
		 WHERE m.org_id = $1
		 ORDER BY u.full_name`,
		orgID,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to list members")
		return
	}
	defer rows.Close()

	members := []models.OrgMember{}
	for rows.Next() {
		var m models.OrgMember
		var roleStr string
		if err := rows.Scan(
			&m.OrgID, &m.UserID, &roleStr, &m.Role, &m.DeptID, &m.Active,
			&m.InvitedBy, &m.JoinedAt, &m.UpdatedAt,
			&m.FullName, &m.Email, &m.AvatarURL, &m.DeptName,
		); err == nil {
			members = append(members, m)
		}
	}
	writeJSON(w, http.StatusOK, members)
}

// POST /orgs/:orgId/members — invite member
func (h *OrganisationsHandler) InviteMember(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	orgID, err := uuid.Parse(chi.URLParam(r, "orgId"))
	if err != nil || u == nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}

	var body struct {
		Email  string     `json:"email"`
		Role   string     `json:"role"`
		DeptID *uuid.UUID `json:"dept_id"`
	}
	if err := decode(r, &body); err != nil || body.Email == "" {
		writeErr(w, http.StatusBadRequest, "email is required")
		return
	}
	if body.Role == "" {
		body.Role = "member"
	}

	// Find or create user by email
	var userID uuid.UUID
	err = h.db.QueryRow(r.Context(),
		`SELECT id FROM users WHERE email = $1`, body.Email,
	).Scan(&userID)
	if err != nil {
		// Create a placeholder user
		err = h.db.QueryRow(r.Context(),
			`INSERT INTO users (phone, email, onboarding_done)
			 VALUES ('pending-' || substr(md5(random()::text),1,8), $1, false)
			 RETURNING id`,
			body.Email,
		).Scan(&userID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "failed to find/create user")
			return
		}
	}

	_, err = h.db.Exec(r.Context(),
		`INSERT INTO org_members (org_id, user_id, role, role_enum, dept_id, invited_by, active)
		 VALUES ($1, $2, $3, $4, $5, $6, true)
		 ON CONFLICT (org_id, user_id) DO UPDATE SET
		   role = EXCLUDED.role, role_enum = EXCLUDED.role_enum,
		   dept_id = EXCLUDED.dept_id, active = true, updated_at = NOW()`,
		orgID, userID, body.Role, body.Role, body.DeptID, u.ID,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to invite member")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"message": "member invited",
		"user_id": userID,
		"role":    body.Role,
	})
}

// PATCH /orgs/:orgId/members/:userId — update role or deactivate
func (h *OrganisationsHandler) UpdateMember(w http.ResponseWriter, r *http.Request) {
	orgID, _ := uuid.Parse(chi.URLParam(r, "orgId"))
	userID, err := uuid.Parse(chi.URLParam(r, "userId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid user id")
		return
	}

	var body struct {
		Role   *string `json:"role"`
		Active *bool   `json:"active"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}

	_, err = h.db.Exec(r.Context(),
		`UPDATE org_members SET
		   role      = COALESCE($3, role),
		   role_enum = COALESCE($3::org_member_role, role_enum),
		   active    = COALESCE($4, active),
		   updated_at = NOW()
		 WHERE org_id = $1 AND user_id = $2`,
		orgID, userID, body.Role, body.Active,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to update member")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "updated"})
}

// ─── CORP VENDORS ─────────────────────────────────────────────────────────────

// GET /orgs/:orgId/vendors
func (h *OrganisationsHandler) ListCorpVendors(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(chi.URLParam(r, "orgId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid org id")
		return
	}

	rows, err := h.db.Query(r.Context(),
		`SELECT id, org_id, vendor_user_id, name, category, email, phone,
		   address, status, total_paid, notes, added_by, created_at, updated_at
		 FROM corp_vendors WHERE org_id = $1 ORDER BY name`,
		orgID,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to list vendors")
		return
	}
	defer rows.Close()

	vendors := []models.CorpVendor{}
	for rows.Next() {
		var v models.CorpVendor
		if err := rows.Scan(
			&v.ID, &v.OrgID, &v.VendorUserID, &v.Name, &v.Category,
			&v.Email, &v.Phone, &v.Address, &v.Status, &v.TotalPaid,
			&v.Notes, &v.AddedBy, &v.CreatedAt, &v.UpdatedAt,
		); err == nil {
			vendors = append(vendors, v)
		}
	}
	writeJSON(w, http.StatusOK, vendors)
}

// POST /orgs/:orgId/vendors
func (h *OrganisationsHandler) AddCorpVendor(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	orgID, err := uuid.Parse(chi.URLParam(r, "orgId"))
	if err != nil || u == nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}

	var body struct {
		Name     string  `json:"name"`
		Category string  `json:"category"`
		Email    *string `json:"email"`
		Phone    *string `json:"phone"`
		Address  *string `json:"address"`
		Notes    *string `json:"notes"`
	}
	if err := decode(r, &body); err != nil || body.Name == "" || body.Category == "" {
		writeErr(w, http.StatusBadRequest, "name and category are required")
		return
	}

	var v models.CorpVendor
	err = h.db.QueryRow(r.Context(),
		`INSERT INTO corp_vendors (org_id, name, category, email, phone, address, notes, added_by)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		 RETURNING id, org_id, vendor_user_id, name, category, email, phone,
		   address, status, total_paid, notes, added_by, created_at, updated_at`,
		orgID, body.Name, body.Category, body.Email, body.Phone,
		body.Address, body.Notes, u.ID,
	).Scan(
		&v.ID, &v.OrgID, &v.VendorUserID, &v.Name, &v.Category,
		&v.Email, &v.Phone, &v.Address, &v.Status, &v.TotalPaid,
		&v.Notes, &v.AddedBy, &v.CreatedAt, &v.UpdatedAt,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to add vendor")
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

// PATCH /orgs/:orgId/vendors/:vendorId — update status (preferred/blacklisted/active)
func (h *OrganisationsHandler) UpdateCorpVendorStatus(w http.ResponseWriter, r *http.Request) {
	orgID, _ := uuid.Parse(chi.URLParam(r, "orgId"))
	vendorID, err := uuid.Parse(chi.URLParam(r, "vendorId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid vendor id")
		return
	}

	var body struct {
		Status string  `json:"status"`
		Notes  *string `json:"notes"`
	}
	if err := decode(r, &body); err != nil || body.Status == "" {
		writeErr(w, http.StatusBadRequest, "status is required")
		return
	}

	_, err = h.db.Exec(r.Context(),
		`UPDATE corp_vendors SET status = $3, notes = COALESCE($4, notes), updated_at = NOW()
		 WHERE id = $1 AND org_id = $2`,
		vendorID, orgID, body.Status, body.Notes,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to update vendor status")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "updated"})
}

// ─── INTEGRATIONS ─────────────────────────────────────────────────────────────

// GET /orgs/:orgId/integrations
func (h *OrganisationsHandler) ListIntegrations(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(chi.URLParam(r, "orgId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid org id")
		return
	}

	rows, err := h.db.Query(r.Context(),
		`SELECT id, org_id, name, connected, connected_by, connected_at, created_at
		 FROM integrations WHERE org_id = $1 ORDER BY name`,
		orgID,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to list integrations")
		return
	}
	defer rows.Close()

	intgs := []models.Integration{}
	for rows.Next() {
		var i models.Integration
		if err := rows.Scan(
			&i.ID, &i.OrgID, &i.Name, &i.Connected,
			&i.ConnectedBy, &i.ConnectedAt, &i.CreatedAt,
		); err == nil {
			intgs = append(intgs, i)
		}
	}
	writeJSON(w, http.StatusOK, intgs)
}

// POST /orgs/:orgId/integrations/:name/toggle
func (h *OrganisationsHandler) ToggleIntegration(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	orgID, _ := uuid.Parse(chi.URLParam(r, "orgId"))
	name := chi.URLParam(r, "name")
	if u == nil || name == "" {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}

	var body struct {
		Connected bool        `json:"connected"`
		Config    interface{} `json:"config"`
	}
	_ = decode(r, &body)

	_, err := h.db.Exec(r.Context(),
		`INSERT INTO integrations (org_id, name, connected, connected_by, connected_at)
		 VALUES ($1, $2, $3, $4, CASE WHEN $3 THEN NOW() ELSE NULL END)
		 ON CONFLICT (org_id, name) DO UPDATE SET
		   connected = $3,
		   connected_by = $4,
		   connected_at = CASE WHEN $3 THEN NOW() ELSE integrations.connected_at END,
		   updated_at = NOW()`,
		orgID, name, body.Connected, u.ID,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to update integration")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": name, "connected": body.Connected})
}
