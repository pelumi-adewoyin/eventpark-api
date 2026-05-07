package models

import (
	"time"

	"github.com/google/uuid"
)

// ─── ORGANISATION ─────────────────────────────────────────────────────────────

type Organisation struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	RCNumber  *string   `json:"rc_number,omitempty"`
	TIN       *string   `json:"tin,omitempty"`
	Industry  *string   `json:"industry,omitempty"`
	LogoURL   *string   `json:"logo_url,omitempty"`
	Address   *string   `json:"address,omitempty"`
	City      *string   `json:"city,omitempty"`
	State     *string   `json:"state,omitempty"`
	Website   *string   `json:"website,omitempty"`
	OwnerID   uuid.UUID `json:"owner_id"`
	KYBStatus string    `json:"kyb_status"`
	KYBTier   string    `json:"kyb_tier"`
	Plan      string    `json:"plan"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type KYBDocument struct {
	ID         uuid.UUID  `json:"id"`
	OrgID      uuid.UUID  `json:"org_id"`
	DocType    string     `json:"doc_type"`
	FileURL    string     `json:"file_url"`
	Status     string     `json:"status"`
	ReviewerID *uuid.UUID `json:"reviewer_id,omitempty"`
	ReviewedAt *time.Time `json:"reviewed_at,omitempty"`
	Notes      *string    `json:"notes,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// ─── DEPARTMENT ───────────────────────────────────────────────────────────────

type Department struct {
	ID        uuid.UUID  `json:"id"`
	OrgID     uuid.UUID  `json:"org_id"`
	Name      string     `json:"name"`
	HeadID    *uuid.UUID `json:"head_id,omitempty"`
	HeadName  *string    `json:"head_name,omitempty"` // enriched
	Budget    int64      `json:"budget"`
	Members   int        `json:"members,omitempty"` // enriched
	CreatedAt time.Time  `json:"created_at"`
}

// ─── ORG MEMBER ───────────────────────────────────────────────────────────────

type OrgMember struct {
	OrgID      uuid.UUID  `json:"org_id"`
	UserID     uuid.UUID  `json:"user_id"`
	Role       string     `json:"role"`
	DeptID     *uuid.UUID `json:"dept_id,omitempty"`
	Active     bool       `json:"active"`
	InvitedBy  *uuid.UUID `json:"invited_by,omitempty"`
	JoinedAt   time.Time  `json:"joined_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	// Enriched
	FullName   *string `json:"full_name,omitempty"`
	Email      *string `json:"email,omitempty"`
	AvatarURL  *string `json:"avatar_url,omitempty"`
	DeptName   *string `json:"dept_name,omitempty"`
}

// ─── CORPORATE VENDOR ─────────────────────────────────────────────────────────

type CorpVendor struct {
	ID           uuid.UUID  `json:"id"`
	OrgID        uuid.UUID  `json:"org_id"`
	VendorUserID *uuid.UUID `json:"vendor_user_id,omitempty"`
	Name         string     `json:"name"`
	Category     string     `json:"category"`
	Email        *string    `json:"email,omitempty"`
	Phone        *string    `json:"phone,omitempty"`
	Address      *string    `json:"address,omitempty"`
	Status       string     `json:"status"`
	TotalPaid    int64      `json:"total_paid"`
	Notes        *string    `json:"notes,omitempty"`
	AddedBy      *uuid.UUID `json:"added_by,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// ─── APPROVAL REQUEST ─────────────────────────────────────────────────────────

type ApprovalRequest struct {
	ID          uuid.UUID  `json:"id"`
	OrgID       uuid.UUID  `json:"org_id"`
	EventID     *uuid.UUID `json:"event_id,omitempty"`
	ItemType    string     `json:"item_type"`
	ItemID      uuid.UUID  `json:"item_id"`
	ItemRef     *string    `json:"item_ref,omitempty"`
	Title       string     `json:"title"`
	Amount      int64      `json:"amount"`
	Currency    string     `json:"currency"`
	RequestedBy uuid.UUID  `json:"requested_by"`
	Notes       *string    `json:"notes,omitempty"`
	Urgency     string     `json:"urgency"`
	Status      string     `json:"status"`
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	// Enriched
	RequesterName *string         `json:"requester_name,omitempty"`
	Actions       []ApprovalAction `json:"actions,omitempty"`
}

type ApprovalAction struct {
	ID          uuid.UUID  `json:"id"`
	RequestID   uuid.UUID  `json:"request_id"`
	ActorID     uuid.UUID  `json:"actor_id"`
	Action      string     `json:"action"`
	Note        *string    `json:"note,omitempty"`
	DelegatedTo *uuid.UUID `json:"delegated_to,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	// Enriched
	ActorName   *string `json:"actor_name,omitempty"`
}

// ─── RFQ ──────────────────────────────────────────────────────────────────────

type RFQ struct {
	ID            uuid.UUID  `json:"id"`
	OrgID         uuid.UUID  `json:"org_id"`
	EventID       *uuid.UUID `json:"event_id,omitempty"`
	Ref           string     `json:"ref"`
	Title         string     `json:"title"`
	Description   *string    `json:"description,omitempty"`
	Requirements  *string    `json:"requirements,omitempty"` // JSON string
	Deadline      *string    `json:"deadline,omitempty"`
	Status        string     `json:"status"`
	AwardedVendor *uuid.UUID `json:"awarded_vendor,omitempty"`
	CreatedBy     uuid.UUID  `json:"created_by"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	// Enriched
	ResponseCount   int          `json:"response_count,omitempty"`
	AwardedVendorName *string    `json:"awarded_vendor_name,omitempty"`
	Responses       []RFQResponse `json:"responses,omitempty"`
}

type RFQInvitation struct {
	ID        uuid.UUID `json:"id"`
	RFQId     uuid.UUID `json:"rfq_id"`
	VendorID  uuid.UUID `json:"vendor_id"`
	SentAt    time.Time `json:"sent_at"`
	Responded bool      `json:"responded"`
}

type RFQResponse struct {
	ID           uuid.UUID  `json:"id"`
	RFQId        uuid.UUID  `json:"rfq_id"`
	VendorID     uuid.UUID  `json:"vendor_id"`
	Amount       int64      `json:"amount"`
	DeliveryDays *int       `json:"delivery_days,omitempty"`
	Notes        *string    `json:"notes,omitempty"`
	Score        *int       `json:"score,omitempty"`
	Awarded      bool       `json:"awarded"`
	SubmittedAt  time.Time  `json:"submitted_at"`
	// Enriched
	VendorName *string `json:"vendor_name,omitempty"`
}

// ─── PURCHASE ORDER ───────────────────────────────────────────────────────────

type PurchaseOrder struct {
	ID           uuid.UUID  `json:"id"`
	OrgID        uuid.UUID  `json:"org_id"`
	EventID      *uuid.UUID `json:"event_id,omitempty"`
	BudgetLineID *uuid.UUID `json:"budget_line_id,omitempty"`
	VendorID     uuid.UUID  `json:"vendor_id"`
	Ref          string     `json:"ref"`
	Scope        *string    `json:"scope,omitempty"`
	Amount       int64      `json:"amount"`
	Currency     string     `json:"currency"`
	Status       string     `json:"status"`
	IssuedAt     *string    `json:"issued_at,omitempty"`
	DueAt        *string    `json:"due_at,omitempty"`
	CreatedBy    uuid.UUID  `json:"created_by"`
	ApprovedBy   *uuid.UUID `json:"approved_by,omitempty"`
	ApprovedAt   *time.Time `json:"approved_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	// Enriched
	VendorName  *string      `json:"vendor_name,omitempty"`
	GRDone      bool         `json:"gr_done,omitempty"`
	LineItems   []POLineItem `json:"line_items,omitempty"`
}

type POLineItem struct {
	ID          uuid.UUID `json:"id"`
	POID        uuid.UUID `json:"po_id"`
	Description string    `json:"description"`
	Quantity    float64   `json:"quantity"`
	Unit        *string   `json:"unit,omitempty"`
	UnitPrice   int64     `json:"unit_price"`
	Total       int64     `json:"total"`
}

type GoodsReceipt struct {
	ID         uuid.UUID `json:"id"`
	POID       uuid.UUID `json:"po_id"`
	ReceivedBy uuid.UUID `json:"received_by"`
	ReceivedAt string    `json:"received_at"`
	Notes      *string   `json:"notes,omitempty"`
	Partial    bool      `json:"partial"`
	CreatedAt  time.Time `json:"created_at"`
}

// ─── VENDOR INVOICE ───────────────────────────────────────────────────────────

type VendorInvoice struct {
	ID          uuid.UUID  `json:"id"`
	OrgID       uuid.UUID  `json:"org_id"`
	POID        *uuid.UUID `json:"po_id,omitempty"`
	VendorID    uuid.UUID  `json:"vendor_id"`
	Ref         string     `json:"ref"`
	Amount      int64      `json:"amount"`
	VATAmount   int64      `json:"vat_amount"`
	WHTAmount   int64      `json:"wht_amount"`
	NetPayable  int64      `json:"net_payable"`
	Currency    string     `json:"currency"`
	InvoiceDate string     `json:"invoice_date"`
	DueDate     *string    `json:"due_date,omitempty"`
	Status      string     `json:"status"`
	MatchStatus string     `json:"match_status"`
	PaidAt      *time.Time `json:"paid_at,omitempty"`
	UploadedBy  uuid.UUID  `json:"uploaded_by"`
	ApprovedBy  *uuid.UUID `json:"approved_by,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	// Enriched
	VendorName *string `json:"vendor_name,omitempty"`
	PORef      *string `json:"po_ref,omitempty"`
	GRExists   bool    `json:"gr_exists,omitempty"`
}

// ─── CORPORATE WALLET ─────────────────────────────────────────────────────────

type OrgWallet struct {
	ID         uuid.UUID `json:"id"`
	OrgID      uuid.UUID `json:"org_id"`
	Balance    int64     `json:"balance"`
	PendingOut int64     `json:"pending_out"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type OrgWalletTransaction struct {
	ID           uuid.UUID  `json:"id"`
	OrgWalletID  uuid.UUID  `json:"org_wallet_id"`
	Type         string     `json:"type"`
	Amount       int64      `json:"amount"`
	BalanceAfter int64      `json:"balance_after"`
	Reference    *string    `json:"reference,omitempty"`
	Description  *string    `json:"description,omitempty"`
	InvoiceID    *uuid.UUID `json:"invoice_id,omitempty"`
	InitiatedBy  *uuid.UUID `json:"initiated_by,omitempty"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
}

type OrgWithdrawalRequest struct {
	ID              uuid.UUID  `json:"id"`
	OrgID           uuid.UUID  `json:"org_id"`
	Amount          int64      `json:"amount"`
	Reason          *string    `json:"reason,omitempty"`
	RequestedBy     uuid.UUID  `json:"requested_by"`
	Status          string     `json:"status"`
	ApprovalsNeeded int        `json:"approvals_needed"`
	CreatedAt       time.Time  `json:"created_at"`
	ExecutedAt      *time.Time `json:"executed_at,omitempty"`
}

// ─── AUDIT LOG ────────────────────────────────────────────────────────────────

type AuditLog struct {
	ID           uuid.UUID  `json:"id"`
	OrgID        *uuid.UUID `json:"org_id,omitempty"`
	UserID       *uuid.UUID `json:"user_id,omitempty"`
	ActorName    *string    `json:"actor_name,omitempty"`
	Action       string     `json:"action"`
	EntityType   *string    `json:"entity_type,omitempty"`
	EntityID     *uuid.UUID `json:"entity_id,omitempty"`
	Detail       *string    `json:"detail,omitempty"`
	PreviousHash *string    `json:"previous_hash,omitempty"`
	EntryHash    string     `json:"entry_hash"`
	CreatedAt    time.Time  `json:"created_at"`
}

// ─── EVENT BUDGET LINE ────────────────────────────────────────────────────────

type EventBudgetLine struct {
	ID        uuid.UUID `json:"id"`
	EventID   uuid.UUID `json:"event_id"`
	Category  string    `json:"category"`
	Label     string    `json:"label"`
	Allocated int64     `json:"allocated"`
	Spent     int64     `json:"spent"`
	Notes     *string   `json:"notes,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ─── RSVP (guest extension) ───────────────────────────────────────────────────

type RSVPGuest struct {
	ID             uuid.UUID  `json:"id"`
	EventID        uuid.UUID  `json:"event_id"`
	FullName       string     `json:"full_name"`
	Email          *string    `json:"email,omitempty"`
	Phone          *string    `json:"phone,omitempty"`
	Status         string     `json:"status"`
	RSVPToken      *string    `json:"rsvp_token,omitempty"`
	RSVPStatus     string     `json:"rsvp_status"`
	RSVPNote       *string    `json:"rsvp_note,omitempty"`
	PlusOne        bool       `json:"plus_one"`
	PlusOneAllowed bool       `json:"plus_one_allowed"`
	PlusOneName    *string    `json:"plus_one_name,omitempty"`
	DietaryReq     *string    `json:"dietary_req,omitempty"`
	Tags           []string   `json:"tags,omitempty"`
	Relationship   *string    `json:"relationship,omitempty"`
	IVTemplate     *string    `json:"iv_template,omitempty"`
	RespondedAt    *time.Time `json:"responded_at,omitempty"`
	TicketCode     string     `json:"ticket_code"`
	CreatedAt      time.Time  `json:"created_at"`
	// Enriched: event info
	EventTitle    *string `json:"event_title,omitempty"`
	EventDate     *string `json:"event_date,omitempty"`
	EventVenue    *string `json:"event_venue,omitempty"`
	EventCoverURL *string `json:"event_cover_url,omitempty"`
	HostName      *string `json:"host_name,omitempty"`
}

// ─── INTEGRATION ─────────────────────────────────────────────────────────────

type Integration struct {
	ID          uuid.UUID  `json:"id"`
	OrgID       uuid.UUID  `json:"org_id"`
	Name        string     `json:"name"`
	Connected   bool       `json:"connected"`
	ConnectedBy *uuid.UUID `json:"connected_by,omitempty"`
	ConnectedAt *time.Time `json:"connected_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// ─── NOTIFICATION ─────────────────────────────────────────────────────────────

type Notification struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	Body      *string   `json:"body,omitempty"`
	Read      bool      `json:"read"`
	CreatedAt time.Time `json:"created_at"`
}

// ─── Personal Budget ──────────────────────────────────────────────────────────

type PersonalBudget struct {
	ID          string           `json:"id"`
	UserID      string           `json:"user_id"`
	Name        string           `json:"name"`
	Scope       string           `json:"scope"`
	EventID     *string          `json:"event_id"`
	TotalNGN    int64            `json:"total_ngn"`
	PeriodStart *string          `json:"period_start"`
	PeriodEnd   *string          `json:"period_end"`
	AlertAt70   bool             `json:"alert_at_70"`
	AlertAt90   bool             `json:"alert_at_90"`
	Categories  []BudgetCategory `json:"categories"`
	TotalSpent  int64            `json:"total_spent"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

type BudgetCategory struct {
	ID           string `json:"id"`
	BudgetID     string `json:"budget_id"`
	Name         string `json:"name"`
	AllocatedNGN int64  `json:"allocated_ngn"`
	SpentNGN     int64  `json:"spent_ngn"`
}

type BudgetExpense struct {
	ID            string    `json:"id"`
	BudgetID      string    `json:"budget_id"`
	CategoryID    *string   `json:"category_id"`
	Description   string    `json:"description"`
	AmountNGN     int64     `json:"amount_ngn"`
	ExpenseDate   string    `json:"expense_date"`
	PaymentMethod string    `json:"payment_method"`
	Notes         *string   `json:"notes"`
	CreatedAt     time.Time `json:"created_at"`
}
