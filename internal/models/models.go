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
	ID           uuid.UUID `json:"id"`
	UserID       uuid.UUID `json:"user_id"`
	BusinessName string    `json:"business_name"`
	Category     string    `json:"category"`
	Bio          *string   `json:"bio,omitempty"`
	City         *string   `json:"city,omitempty"`
	State        *string   `json:"state,omitempty"`
	AvatarURL    *string   `json:"avatar_url,omitempty"`
	CoverURL     *string   `json:"cover_url,omitempty"`
	Rating       float64   `json:"rating"`
	ReviewCount  int       `json:"review_count"`
	Verified     bool      `json:"verified"`
	CreatedAt    time.Time `json:"created_at"`
	// Enriched
	Services  []VendorService  `json:"services,omitempty"`
	Portfolio []VendorPortfolio `json:"portfolio,omitempty"`
}

type VendorService struct {
	ID          uuid.UUID `json:"id"`
	VendorID    uuid.UUID `json:"vendor_id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	PriceFrom   int64     `json:"price_from"`
	PriceTo     *int64    `json:"price_to,omitempty"`
	Unit        *string   `json:"unit,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
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
	ID          uuid.UUID `json:"id"`
	VendorID    uuid.UUID `json:"vendor_id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	Price       int64     `json:"price"`
	Category    *string   `json:"category,omitempty"`
	ImageURL    *string   `json:"image_url,omitempty"`
	Stock       *int      `json:"stock,omitempty"`
	Active      bool      `json:"active"`
	CreatedAt   time.Time `json:"created_at"`
}
