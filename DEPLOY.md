# EventPark API — Deploy to Railway

## One-time setup (run from this folder)

### 1. Install Go (if not on your machine)
```
# Mac
brew install go
# or download from https://go.dev/dl/
```

### 2. Download dependencies & create go.sum
```bash
chmod +x setup.sh && ./setup.sh
```

### 3. Push to GitHub
```bash
git init
git add .
git commit -m "init: EventPark API"
git remote add origin https://github.com/YOUR_ORG/eventpark-api.git
git push -u origin main
```

---

## Deploy on Railway

### Step 1 — Create Railway project
1. Go to https://railway.app → New Project
2. Click **Deploy from GitHub repo** → select `eventpark-api`
3. Railway auto-detects the Dockerfile

### Step 2 — Add PostgreSQL
1. In your project: **+ New** → **Database** → **PostgreSQL**
2. Click the Postgres service → **Connect** tab → copy `DATABASE_URL`

### Step 3 — Run the database schema
```bash
psql "postgresql://..." -f migrations/001_init.sql
```
(Paste your Railway DATABASE_URL in the quotes)

### Step 4 — Set environment variables
In Railway → your API service → **Variables** tab, add:

| Variable | Value |
|---|---|
| `DATABASE_URL` | (from step 2, auto-linked) |
| `JWT_SECRET` | any 64-char random string |
| `JWT_REFRESH_SECRET` | another random string |
| `PAYSTACK_SECRET_KEY` | from paystack.com dashboard |
| `PAYSTACK_PUBLIC_KEY` | from paystack.com dashboard |
| `DOJAH_APP_ID` | from dojah.io dashboard |
| `DOJAH_PRIVATE_KEY` | from dojah.io dashboard |
| `TERMII_API_KEY` | from termii.com dashboard |
| `ENV` | `production` |

### Step 5 — Get your API URL
Railway will give you a URL like `https://eventpark-api-production.up.railway.app`

### Step 6 — Wire the frontend
In `eventpark/` (your Vercel project), add environment variable:
```
VITE_API_URL=https://eventpark-api-production.up.railway.app
```
Redeploy Vercel → done!

---

## API Endpoints Reference

### Public (no auth)
- `GET  /health`
- `POST /auth/request-otp` — sends SMS OTP
- `POST /auth/verify-otp` — returns JWT tokens + user
- `POST /auth/refresh` — exchange refresh token for new access token
- `GET  /discover/events?city=Lagos&type=wedding&q=summit`
- `GET  /discover/vendors?category=catering&city=Abuja`
- `GET  /discover/products?category=decor`
- `POST /wallet/topup/webhook` — Paystack webhook (verified by signature)

### Authenticated (Bearer token required)
- `POST /auth/logout`
- `GET  /users/me`
- `PATCH /users/me`
- `POST /users/onboarding` — sets role, creates org for corporate
- `POST /events` — create event
- `GET  /events` — list my events
- `GET  /events/:id`
- `PATCH /events/:id`
- `DELETE /events/:id`
- `POST /events/:id/publish` — KYC gated for public events
- `POST /events/:id/guests` — invite guests (bulk)
- `GET  /events/:id/guests`
- `POST /checkin/:eventId` — check-in by ticket_code or guest_id
- `GET  /checkin/:eventId/stats`
- `GET  /wallet`
- `GET  /wallet/transactions?type=topup`
- `POST /wallet/topup/initialize` — returns Paystack checkout URL
- `GET  /wallet/topup/verify?reference=...`
- `POST /wallet/withdraw` — KYC tier 1+ required
- `GET  /kyc/status`
- `POST /kyc/verify-bvn` — upgrades to tier 1
- `POST /kyc/verify-nin` — upgrades to tier 2
- `POST /vendors` — create/update vendor profile
- `GET  /vendors/:id`
- `POST /vendors/:id/services`
- `POST /bookings` — holds 50% escrow from wallet
- `POST /bookings/:id/release-escrow` — releases escrow to vendor
