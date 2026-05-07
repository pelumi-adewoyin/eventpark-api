package models

import (
	"time"

	"github.com/google/uuid"
)

// ─── USER ─────────────────────────────────────────────────────────────────────

type User struct {
	ID             uuid.UUID  `json:"id"`
	Phone          string     `json:"phone"`
	Email          *string    `json:"email,omitempty"`
	FullName       *string    `json:"full_name,omitempty"`
	AvatarURL      *string    `json:"avatar_url,omitempty"`
	Role           *string    `json:"role,omitempty"`
	KYCTier        string     `json:"kyc_tier"`
	OnboardingDone bool       `json:"onboarding_done"`
	OrgID          *uuid.UUID `json:"org_id,omitempty"`
	OrgName        *string    `json:"org_name,omitempty"`
	// Vendor fields (populated when role='vendor')
	VendorID           *uuid.UUID `json:"vendor_id,omitempty"`
	VendorType         *string    `json:"vendor_type,omitempty"`
	BusinessName       *string    `json:"business_name,omitempty"`
	VerificationStatus *string    `json:"verification_status,omitempty"`
	VerificationTier   *int       `json:"verification_tier,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// ─── EVENT ────────────────────────────────────────────────────────────────────

type Event struct {
	ID             uuid.UUID  `json:"id"`
	OwnerID        uuid.UUID  `json:"owner_id"`
	OrgID          *uuid.UUID `json:"org_id,omitempty"`
	Title          string     `json:"title"`
	Description    *string    `json:"description,omitempty"`
	EventType      string     `json:"event_type"`
	Visibility     string     `json:"visibility"`
	Status         string     `json:"status"`
	StartAt        *time.Time `json:"start_at,omitempty"`
	EndAt          *time.Time `json:"end_at,omitempty"`
	VenueName      *string    `json:"venue_name,omitempty"`
	VenueAddress   *string    `json:"venue_address,omitempty"`
	VenueCity      *string    `json:"venue_city,omitempty"`
	VenueState     *string    `json:"venue_state,omitempty"`
	CoverURL       *string    `json:"cover_url,omitempty"`
	MaxGuests      *int       `json:"max_guests,omitempty"`
	BudgetTotal    int64      `json:"budget_total"`
	TicketPrice    int64      `json:"ticket_price"`
	ApprovalStatus string     `json:"approval_status"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	// Enriched fields (not in table)
	GuestCount  int `json:"guest_count,omitempty"`
	CheckedIn   int `json:"checked_in,omitempty"`
}

// ─── GUEST ────────────────────────────────────────────────────────────────────

type Guest struct {
	ID          uuid.UUID  `json:"id"`
	EventID     uuid.UUID  `json:"event_id"`
	UserID      *uuid.UUID `json:"user_id,omitempty"`
	FullName    string     `json:"full_name"`
	Email       *string    `json:"email,omitempty"`
	Phone       *string    `json:"phone,omitempty"`
	Status      string     `json:"status"`
	TicketCode  string     `json:"ticket_code"`
	SeatNumber  *int       `json:"seat_number,omitempty"`
	CheckedInAt *time.Time `json:"checked_in_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// ─── VENDOR ───────────────────────────────────────────────────────────────────

type Vendor struct {
	ID                 uuid.UUID `json:"id"`
	UserID             uuid.UUID `json:"user_id"`
	BusinessName       string    `json:"business_name"`
	Category           string    `json:"category"`
	Bio                *string   `json:"bio,omitempty"`
	City               *string   `json:"city,omitempty"`
	State              *string   `json:"state,omitempty"`
	AvatarURL          *string   `json:"avatar_url,omitempty"`
	CoverURL           *string   `json:"cover_url,omitempty"`
	Rating             float64   `json:"rating"`
	ReviewCount        int       `json:"review_count"`
	Verified           bool      `json:"verified"`
	// Phase 6 extended fields
	VendorType         string    `json:"vendor_type"`
	VerificationStatus string    `json:"verification_status"`
	VerificationTier   int       `json:"verification_tier"`
	IsRegistered       bool      `json:"is_registered"`
	CACRCNumber        *string   `json:"cac_rc_number,omitempty"`
	Address            *string   `json:"address,omitempty"`
	PostalCode         *string   `json:"postal_code,omitempty"`
	Tagline            *string   `json:"tagline,omitempty"`
	Highlight1         *string   `json:"highlight_1,omitempty"`
	Highlight2         *string   `json:"highlight_2,omitempty"`
	Highlight3         *string   `json:"highlight_3,omitempty"`
	YearsExperience    *int      `json:"years_experience,omitempty"`
	EventsCompleted    *int      `json:"events_completed,omitempty"`
	Website            *string   `json:"website,omitempty"`
	Instagram          *string   `json:"instagram,omitempty"`
	Twitter            *string   `json:"twitter,omitempty"`
	WhatsApp           *string   `json:"whatsapp,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	// Enriched
	Services  []VendorService   `json:"services,omitempty"`
	Portfolio []VendorPortfolio `json:"portfolio,omitempty"`
}

type VendorService struct {
	ID              uuid.UUID `json:"id"`
	VendorID        uuid.UUID `json:"vendor_id"`
	Name            string    `json:"name"`
	Description     *string   `json:"description,omitempty"`
	PriceFrom       int64     `json:"price_from"`
	PriceTo         *int64    `json:"price_to,omitempty"`
	Unit            *string   `json:"unit,omitempty"`
	// Phase 6 extended fields
	PricingModel    string    `json:"pricing_model"`
	IsActive        bool      `json:"is_active"`
	MinNoticeHours  int       `json:"min_notice_hours"`
	MaxAdvanceDays  int       `json:"max_advance_days"`
	ResponseTimeHrs int       `json:"response_time_hrs"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// ─── ORDER (product vendor flow) ──────────────────────────────────────────────

type Order struct {
	ID              uuid.UUID  `json:"id"`
	VendorID        uuid.UUID  `json:"vendor_id"`
	CustomerID      uuid.UUID  `json:"customer_id"`
	Status          string     `json:"status"`
	TotalAmount     int64      `json:"total_amount"`
	EscrowAmount    int64      `json:"escrow_amount"`
	EscrowReleased  bool       `json:"escrow_released"`
	DeliveryAddress *string    `json:"delivery_address,omitempty"`
	DeliveryZone    *string    `json:"delivery_zone,omitempty"`
	Notes           *string    `json:"notes,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	// Enriched
	Items           []OrderItem `json:"items,omitempty"`
	CustomerName    *string     `json:"customer_name,omitempty"`
	CustomerPhone   *string     `json:"customer_phone,omitempty"`
}

type OrderItem struct {
	ID        uuid.UUID  `json:"id"`
	OrderID   uuid.UUID  `json:"order_id"`
	ProductID *uuid.UUID `json:"product_id,omitempty"`
	Name      string     `json:"name"`
	Qty       int        `json:"qty"`
	UnitPrice int64      `json:"unit_price"`
}

// ─── VENDOR AVAILABILITY ──────────────────────────────────────────────────────

type VendorAvailability struct {
	ID           uuid.UUID   `json:"id"`
	VendorID     uuid.UUID   `json:"vendor_id"`
	WorkingDays  []int       `json:"working_days"`
	BlockedDates []time.Time `json:"blocked_dates"`
	UpdatedAt    time.Time   `json:"updated_at"`
}

// ─── VENDOR BANK ACCOUNT ──────────────────────────────────────────────────────

type VendorBankAccount struct {
	ID            uuid.UUID `json:"id"`
	VendorID      uuid.UUID `json:"vendor_id"`
	BankName      string    `json:"bank_name"`
	AccountNumber string    `json:"account_number"`
	AccountName   string    `json:"account_name"`
	IsDefault     bool      `json:"is_default"`
	CreatedAt     time.Time `json:"created_at"`
}

// ─── VENDOR VERIFICATION SUBMISSION ──────────────────────────────────────────

type VendorVerification struct {
	ID            uuid.UUID  `json:"id"`
	VendorID      uuid.UUID  `json:"vendor_id"`
	TargetTier    int        `json:"target_tier"`
	CACRCNumber   *string    `json:"cac_rc_number,omitempty"`
	CACDocURL     *string    `json:"cac_doc_url,omitempty"`
	IDType        *string    `json:"id_type,omitempty"`
	IDDocURL      *string    `json:"id_doc_url,omitempty"`
	BankStmtURL   *string    `json:"bank_stmt_url,omitempty"`
	Notes         *string    `json:"notes,omitempty"`
	Status        string     `json:"status"`
	ReviewerNotes *string    `json:"reviewer_notes,omitempty"`
	SubmittedAt   time.Time  `json:"submitted_at"`
	ReviewedAt    *time.Time `json:"reviewed_at,omitempty"`
}

type VendorPortfolio struct {
	ID        uuid.UUID `json:"id"`
	VendorID  uuid.UUID `json:"vendor_id"`
	ImageURL  string    `json:"image_url"`
	Caption   *string   `json:"caption,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// ─── BOOKING ──────────────────────────────────────────────────────────────────

type Booking struct {
	ID             uuid.UUID  `json:"id"`
	EventID        uuid.UUID  `json:"event_id"`
	VendorID       uuid.UUID  `json:"vendor_id"`
	ServiceID      *uuid.UUID `json:"service_id,omitempty"`
	ClientID       uuid.UUID  `json:"client_id"`
	Status         string     `json:"status"`
	TotalAmount    int64      `json:"total_amount"`
	EscrowAmount   int64      `json:"escrow_amount"`
	EscrowReleased bool       `json:"escrow_released"`
	Notes          *string    `json:"notes,omitempty"`
	EventDate      *time.Time `json:"event_date,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// ─── WALLET ───────────────────────────────────────────────────────────────────

type Wallet struct {
	ID         uuid.UUID `json:"id"`
	UserID     uuid.UUID `json:"user_id"`
	Balance    int64     `json:"balance"`
	EscrowHeld int64     `json:"escrow_held"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type WalletTransaction struct {
	ID          uuid.UUID  `json:"id"`
	WalletID    uuid.UUID  `json:"wallet_id"`
	Type        string     `json:"type"`
	Amount      int64      `json:"amount"`
	Status      string     `json:"status"`
	Reference   *string    `json:"reference,omitempty"`
	PaystackRef *string    `json:"paystack_ref,omitempty"`
	Description *string    `json:"description,omitempty"`
	BookingID   *uuid.UUID `json:"booking_id,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// ─── KYC ──────────────────────────────────────────────────────────────────────

type KYCVerification struct {
	ID            uuid.UUID  `json:"id"`
	UserID        uuid.UUID  `json:"user_id"`
	Tier          string     `json:"tier"`
	Status        string     `json:"status"`
	DojahRef      *string    `json:"dojah_ref,omitempty"`
	FailureReason *string    `json:"failure_reason,omitempty"`
	VerifiedAt    *time.Time `json:"verified_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

// ─── PRODUCT ──────────────────────────────────────────────────────────────────

type Product struct {
	ID                 uuid.UUID `json:"id"`
	VendorID           uuid.UUID `json:"vendor_id"`
	Name               string    `json:"name"`
	Description        *string   `json:"description,omitempty"`
	Price              int64     `json:"price"`
	Category           *string   `json:"category,omitempty"`
	ImageURL           *string   `json:"image_url,omitempty"`
	Stock              *int      `json:"stock,omitempty"`
	Active             bool      `json:"active"`
	// Phase 6 extended fields
	MinOrderQty        int       `json:"min_order_qty"`
	LeadTimeDays       int       `json:"lead_time_days"`
	FreeDeliveryAbove  *int64    `json:"free_delivery_above,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}
