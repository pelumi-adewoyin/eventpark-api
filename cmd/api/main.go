package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/eventpark/api/internal/config"
	"github.com/eventpark/api/internal/db"
	"github.com/eventpark/api/internal/handlers"
	"github.com/eventpark/api/internal/middleware"
	"github.com/eventpark/api/migrations"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

func main() {
	cfg := config.Load()

	pool, err := db.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	defer pool.Close()
	log.Println("connected to database")

	// Run any pending SQL migrations before accepting traffic.
	// migrations.FS is the embed.FS exported by the migrations package —
	// all *.sql files are compiled into the binary so no file I/O at runtime.
	if err := db.RunMigrations(pool, migrations.FS); err != nil {
		log.Fatalf("migrations failed: %v", err)
	}

	// ── Handlers ──────────────────────────────────────────────────────────────
	authH    := handlers.NewAuthHandler(pool, cfg.JWTSecret, cfg.JWTRefreshSecret, cfg.TermiiAPIKey, cfg.Env)
	usersH   := handlers.NewUsersHandler(pool)
	eventsH  := handlers.NewEventsHandler(pool)
	guestsH  := handlers.NewGuestsHandler(pool)
	walletH  := handlers.NewWalletHandler(pool, cfg.PaystackSecretKey)
	kycH     := handlers.NewKYCHandler(pool, cfg.DojahAppID, cfg.DojahPrivateKey)
	vendorsH := handlers.NewVendorsHandler(pool)
	discoverH := handlers.NewDiscoverHandler(pool)

	// Phase 2 / 4 handlers
	rsvpH    := handlers.NewRSVPHandler(pool)
	budgetH  := handlers.NewBudgetHandler(pool)
	orgsH    := handlers.NewOrganisationsHandler(pool)
	approvH  := handlers.NewApprovalsHandler(pool)
	rfqsH    := handlers.NewRFQsHandler(pool)
	procH    := handlers.NewProcurementHandler(pool)
	corpWalH := handlers.NewCorpWalletHandler(pool)
	auditH   := handlers.NewAuditHandler(pool)

	r := chi.NewRouter()

	// Global middleware
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.RealIP)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"https://*.vercel.app", "http://localhost:5173", "http://localhost:3000"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Dev-Mode", "X-User-ID"},
		ExposedHeaders:   []string{"Link", "Content-Disposition"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// ── Health ────────────────────────────────────────────────────────────────
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","version":"2.0.0"}`))
	})

	// ── Auth (public) ──────────────────────────────────────────────────────────
	r.Post("/auth/request-otp", authH.RequestOTP)
	r.Post("/auth/verify-otp", authH.VerifyOTP)
	r.Post("/auth/refresh", authH.Refresh)

	// ── Discover (public) ──────────────────────────────────────────────────────
	r.Get("/discover/events", discoverH.Events)
	r.Get("/discover/vendors", discoverH.Vendors)
	r.Get("/discover/products", discoverH.Products)

	// ── RSVP (public — guest follows their link) ──────────────────────────────
	r.Get("/rsvp/{token}", rsvpH.GetRSVP)
	r.Post("/rsvp/{token}/respond", rsvpH.Respond)

	// ── Paystack webhook (public, sig-verified) ────────────────────────────────
	r.Post("/wallet/topup/webhook", walletH.PaystackWebhook)

	// ── Authenticated routes ──────────────────────────────────────────────────
	r.Group(func(r chi.Router) {
		r.Use(middleware.Authenticate(cfg.JWTSecret))

		// Auth
		r.Post("/auth/logout", authH.Logout)

		// Users
		r.Get("/users/me", usersH.GetMe)
		r.Patch("/users/me", usersH.UpdateMe)
		r.Post("/users/onboarding", usersH.CompleteOnboarding)

		// ── Events ───────────────────────────────────────────────────────────
		r.Post("/events", eventsH.CreateEvent)
		r.Get("/events", eventsH.ListMyEvents)
		r.Get("/events/{id}", eventsH.GetEvent)
		r.Patch("/events/{id}", eventsH.UpdateEvent)
		r.Delete("/events/{id}", eventsH.DeleteEvent)
		r.Post("/events/{id}/publish", eventsH.PublishEvent)

		// ── Budget Lines ──────────────────────────────────────────────────────
		r.Get("/events/{id}/budget", budgetH.ListBudgetLines)
		r.Get("/events/{id}/budget/summary", budgetH.BudgetSummary)
		r.Post("/events/{id}/budget", budgetH.CreateBudgetLine)
		r.Patch("/events/{id}/budget/{lineId}", budgetH.UpdateBudgetLine)
		r.Delete("/events/{id}/budget/{lineId}", budgetH.DeleteBudgetLine)

		// ── Guests / Check-in ─────────────────────────────────────────────────
		r.Post("/events/{id}/guests", guestsH.InviteGuests)
		r.Get("/events/{id}/guests", guestsH.ListGuests)
		r.Post("/checkin/{eventId}", guestsH.CheckIn)
		r.Get("/checkin/{eventId}/stats", guestsH.CheckInStats)

		// ── Personal Wallet ───────────────────────────────────────────────────
		r.Get("/wallet", walletH.GetWallet)
		r.Get("/wallet/transactions", walletH.ListTransactions)
		r.Post("/wallet/topup/initialize", walletH.InitializeTopUp)
		r.Get("/wallet/topup/verify", walletH.VerifyTopUp)
		r.Post("/wallet/withdraw", walletH.Withdraw)

		// ── KYC (personal) ────────────────────────────────────────────────────
		r.Get("/kyc/status", kycH.GetStatus)
		r.Post("/kyc/verify-bvn", kycH.VerifyBVN)
		r.Post("/kyc/verify-nin", kycH.VerifyNIN)

		// ── Vendors (marketplace) ─────────────────────────────────────────────
		r.Post("/vendors", vendorsH.CreateVendor)
		r.Get("/vendors/{id}", vendorsH.GetVendor)
		r.Post("/vendors/{id}/services", vendorsH.AddService)

		// ── Bookings ──────────────────────────────────────────────────────────
		r.Post("/bookings", vendorsH.CreateBooking)
		r.Post("/bookings/{id}/release-escrow", vendorsH.ReleaseEscrow)

		// ── Notifications ─────────────────────────────────────────────────────
		r.Get("/notifications", rsvpH.ListNotifications)
		r.Post("/notifications/mark-read", rsvpH.MarkAllRead)

		// ── Organisations ─────────────────────────────────────────────────────
		r.Post("/orgs", orgsH.CreateOrg)
		r.Get("/orgs/me", orgsH.GetMyOrg)

		r.Route("/orgs/{orgId}", func(r chi.Router) {
			r.Patch("/", orgsH.UpdateOrg)

			// KYB documents
			r.Get("/kyb-documents", orgsH.ListKYBDocuments)
			r.Post("/kyb-documents", orgsH.UploadKYBDocument)

			// Departments
			r.Get("/departments", orgsH.ListDepartments)
			r.Post("/departments", orgsH.CreateDepartment)
			r.Patch("/departments/{deptId}", orgsH.UpdateDepartment)

			// Members / Employees
			r.Get("/members", orgsH.ListMembers)
			r.Post("/members", orgsH.InviteMember)
			r.Patch("/members/{userId}", orgsH.UpdateMember)

			// Corporate vendor directory
			r.Get("/vendors", orgsH.ListCorpVendors)
			r.Post("/vendors", orgsH.AddCorpVendor)
			r.Patch("/vendors/{vendorId}", orgsH.UpdateCorpVendorStatus)

			// Integrations
			r.Get("/integrations", orgsH.ListIntegrations)
			r.Post("/integrations/{name}/toggle", orgsH.ToggleIntegration)

			// ── Approvals ─────────────────────────────────────────────────────
			r.Get("/approvals", approvH.ListApprovals)
			r.Post("/approvals", approvH.SubmitApproval)
			r.Get("/approvals/{id}", approvH.GetApproval)
			r.Post("/approvals/{id}/approve", approvH.Approve)
			r.Post("/approvals/{id}/reject", approvH.Reject)
			r.Post("/approvals/{id}/request-changes", approvH.RequestChanges)
			r.Post("/approvals/{id}/delegate", approvH.Delegate)

			// ── RFQs ──────────────────────────────────────────────────────────
			r.Get("/rfqs", rfqsH.ListRFQs)
			r.Post("/rfqs", rfqsH.CreateRFQ)
			r.Get("/rfqs/{id}", rfqsH.GetRFQ)
			r.Post("/rfqs/{id}/send", rfqsH.SendRFQ)
			r.Post("/rfqs/{id}/respond", rfqsH.SubmitResponse)
			r.Patch("/rfqs/{id}/responses/{responseId}/score", rfqsH.ScoreResponse)
			r.Post("/rfqs/{id}/award", rfqsH.AwardRFQ)
			r.Post("/rfqs/{id}/cancel", rfqsH.CancelRFQ)

			// ── Purchase Orders ───────────────────────────────────────────────
			r.Get("/pos", procH.ListPOs)
			r.Post("/pos", procH.CreatePO)
			r.Get("/pos/{id}", procH.GetPO)
			r.Post("/pos/{id}/goods-receipt", procH.RecordGoodsReceipt)

			// ── Invoices ──────────────────────────────────────────────────────
			r.Get("/invoices", procH.ListInvoices)
			r.Post("/invoices", procH.CreateInvoice)
			r.Get("/invoices/{id}/match", procH.GetMatchStatus)
			r.Post("/invoices/{id}/pay", procH.PayInvoice)

			// ── Corporate Wallet ──────────────────────────────────────────────
			r.Get("/wallet", corpWalH.GetWallet)
			r.Get("/wallet/transactions", corpWalH.ListTransactions)
			r.Post("/wallet/topup", corpWalH.TopUp)
			r.Get("/wallet/signatories", corpWalH.ListSignatories)
			r.Post("/wallet/signatories", corpWalH.AddSignatory)
			r.Get("/wallet/withdrawal-requests", corpWalH.ListWithdrawalRequests)
			r.Post("/wallet/withdrawal-requests", corpWalH.RequestWithdrawal)
			r.Post("/wallet/withdrawal-requests/{id}/sign", corpWalH.SignWithdrawal)

			// ── Audit Log ─────────────────────────────────────────────────────
			r.Get("/audit", auditH.ListAuditLogs)
			r.Get("/audit/verify", auditH.VerifyChain)
			r.Get("/audit/export", auditH.ExportAuditCSV)
		})
	})

	addr := fmt.Sprintf(":%s", cfg.Port)
	log.Printf("EventPark API v2 listening on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
