package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port             string
	DatabaseURL      string
	JWTSecret        string
	JWTRefreshSecret string
	PaystackSecretKey string
	PaystackPublicKey string
	DojahAppID       string
	DojahPrivateKey  string
	TwilioSID        string
	TwilioToken      string
	TwilioFrom       string
	TermiiAPIKey     string
	Env              string
}

func Load() *Config {
	_ = godotenv.Load()

	return &Config{
		Port:             getEnv("PORT", "8080"),
		DatabaseURL:      mustEnv("DATABASE_URL"),
		JWTSecret:        mustEnv("JWT_SECRET"),
		JWTRefreshSecret: getEnv("JWT_REFRESH_SECRET", mustEnv("JWT_SECRET")),
		PaystackSecretKey: getEnv("PAYSTACK_SECRET_KEY", ""),
		PaystackPublicKey: getEnv("PAYSTACK_PUBLIC_KEY", ""),
		DojahAppID:       getEnv("DOJAH_APP_ID", ""),
		DojahPrivateKey:  getEnv("DOJAH_PRIVATE_KEY", ""),
		TwilioSID:        getEnv("TWILIO_SID", ""),
		TwilioToken:      getEnv("TWILIO_TOKEN", ""),
		TwilioFrom:       getEnv("TWILIO_FROM", ""),
		TermiiAPIKey:     getEnv("TERMII_API_KEY", ""),
		Env:              getEnv("ENV", "development"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic("missing required env var: " + key)
	}
	return v
}
