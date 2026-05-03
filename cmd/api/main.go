package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/eventpark/api/internal/config"
	"github.com/eventpark/api/internal/db"
	"github.com/eventpark/api/internal/handlers"
	"github.com/eventpark/api/internal/middleware"
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

	// Handlers
	authH    := handlers.NewAuthHandler(pool, cfg.JWTSecret, cfg.JWTRefreshSecret)
	usersH   := handlers.NewUsersHandler(pool)
	eventsH  := handlers.NewEventsHandler(pool)
	guestsH  := handlers.NewGuestsHandler(pool)
	walletH  := handlers.NewWalletHandler(pool, cfg.PaystackSecretKey)
	kycH     := handlers.NewKYCHandler(pool, cfg.DojahAppID, cfg.DojahPrivateKey)
	vendorsH := handlers.NewVendorsHandler(pool)
	discoverH := handlers.NewDiscoverHandler(pool)

	r := chi.NewRouter()

	// Global middleware
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.RealIP)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"https://*.vercel.app", "http://localhost:5173", "http://localhost:3000"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Dev-Mode"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// ── Auth (public) ──────────────────────────────────────────────────────────
	r.Post("/auth/request-otp", authH.RequestOTP)
	r.Post("/auth/verify-otp", authH.VerifyOTP)
	r.Post("/auth/refresh", authH.Refresh)

	// ── Discover (public) ──────────────────────────────────────────────────────
	r.Get("/discover/events", discoverH.Events)
	r.Get("/discover/vendors", discoverH.Vendors)
	r.Get("/discover/products", discoverH.Products)

	// ── Paystack webhook (public, verified by sig) ─────────────────────────────
	r.Post("/wallet/topup/webhook", walletH.PaystackWebhook)

	// ── Authenticated routes ───────────────────────────────────────────────────
	r.Group(func(r chi.Router) {
		r.Use(middleware.Authenticate(cfg.JWTSecret))

		// Auth
		r.Post("/auth/logout", authH.Logout)

		// Users
		r.Get("/users/me", usersH.GetMe)
		r.Patch("/users/me", usersH.UpdateMe)
		r.Post("/users/onboarding", usersH.CompleteOnboarding)

		// Events
		r.Post("/events", eventsH.CreateEvent)
		r.Get("/events", eventsH.ListMyEvents)
		r.Get("/events/{id}", eventsH.GetEvent)
		r.Patch("/events/{id}", eventsH.UpdateEvent)
		r.Delete("/events/{id}", eventsH.DeleteEvent)
		r.Post("/events/{id}/publish", eventsH.PublishEvent)

		// Guests / Check-in
		r.Post("/events/{id}/guests", guestsH.InviteGuests)
		r.Get("/events/{id}/guests", guestsH.ListGuests)
		r.Post("/checkin/{eventId}", guestsH.CheckIn)
		r.Get("/checkin/{eventId}/stats", guestsH.CheckInStats)

		// Wallet
		r.Get("/wallet", walletH.GetWallet)
		r.Get("/wallet/transactions", walletH.ListTransactions)
		r.Post("/wallet/topup/initialize", walletH.InitializeTopUp)
		r.Get("/wallet/topup/verify", walletH.VerifyTopUp)
		r.Post("/wallet/withdraw", walletH.Withdraw)

		// KYC
		r.Get("/kyc/status", kycH.GetStatus)
		r.Post("/kyc/verify-bvn", kycH.VerifyBVN)
		r.Post("/kyc/verify-nin", kycH.VerifyNIN)

		// Vendors
		r.Post("/vendors", vendorsH.CreateVendor)
		r.Get("/vendors/{id}", vendorsH.GetVendor)
		r.Post("/vendors/{id}/services", vendorsH.AddService)

		// Bookings
		r.Post("/bookings", vendorsH.CreateBooking)
		r.Post("/bookings/{id}/release-escrow", vendorsH.ReleaseEscrow)
	})

	addr := fmt.Sprintf(":%s", cfg.Port)
	log.Printf("EventPark API listening on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
