#!/bin/bash
# Run this once on your machine to download dependencies and create go.sum

set -e

echo "→ Downloading Go dependencies..."
go mod tidy

echo "→ Dependencies installed. go.sum created."
echo ""
echo "→ To run locally:"
echo "  cp .env.example .env"
echo "  # Edit .env with your values"
echo "  go run ./cmd/api"
echo ""
echo "→ To build binary:"
echo "  go build -o server ./cmd/api"
echo "  ./server"
echo ""
echo "→ To apply database schema (replace with your DATABASE_URL):"
echo "  psql \$DATABASE_URL -f migrations/001_init.sql"
