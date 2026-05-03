FROM golang:1.22 AS builder

WORKDIR /app

COPY go.mod ./
RUN go mod download -mod=mod || true

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=mod -ldflags="-s -w" -o /app/server ./cmd/api

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y ca-certificates tzdata && rm -rf /var/lib/apt/lists/*
WORKDIR /app

COPY --from=builder /app/server .
COPY migrations/ ./migrations/

EXPOSE 8080

CMD ["./server"]
