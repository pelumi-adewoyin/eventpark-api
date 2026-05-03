-- EventPark Database Schema
-- Run this against your Railway PostgreSQL database

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ─── ENUMS ────────────────────────────────────────────────────────────────────

CREATE TYPE user_role AS ENUM ('diy', 'planner', 'corporate');
CREATE TYPE kyc_tier AS ENUM ('0', '1', '2');
CREATE TYPE kyc_status AS ENUM ('pending', 'processing', 'approved', 'rejected');
CREATE TYPE event_type AS ENUM (
  'wedding', 'birthday', 'corporate_retreat', 'conference', 'product_launch',
  'concert', 'fundraiser', 'baby_shower', 'graduation', 'house_party', 'other'
);
CREATE TYPE event_visibility AS ENUM ('private', 'public', 'corporate');
CREATE TYPE event_status AS ENUM ('draft', 'published', 'cancelled', 'completed');
CREATE TYPE booking_status AS ENUM ('pending', 'confirmed', 'cancelled', 'completed');
CREATE TYPE txn_type AS ENUM ('topup', 'withdraw', 'escrow_hold', 'escrow_release', 'payment', 'refund');
CREATE TYPE txn_status AS ENUM ('pending', 'success', 'failed');
CREATE TYPE guest_status AS ENUM ('invited', 'rsvp_yes', 'rsvp_no', 'checked_in');
CREATE TYPE approval_status AS ENUM ('pending', 'approved', 'rejected');

-- ─── USERS ────────────────────────────────────────────────────────────────────

CREATE TABLE users (
  id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  phone           VARCHAR(20) UNIQUE NOT NULL,
  email           VARCHAR(255) UNIQUE,
  full_name       VARCHAR(255),
  avatar_url      TEXT,
  role            user_role,
  kyc_tier        kyc_tier NOT NULL DEFAULT '0',
  onboarding_done BOOLEAN NOT NULL DEFAULT false,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_users_phone ON users(phone);

-- ─── OTP CODES ────────────────────────────────────────────────────────────────

CREATE TABLE otp_codes (
  id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  phone      VARCHAR(20) NOT NULL,
  code       VARCHAR(6) NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  used       BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_otp_phone ON otp_codes(phone, used, expires_at);

-- ─── REFRESH TOKENS ───────────────────────────────────────────────────────────

CREATE TABLE refresh_tokens (
  id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash VARCHAR(64) NOT NULL UNIQUE,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ─── ORGANISATIONS (Corporate) ────────────────────────────────────────────────

CREATE TABLE organisations (
  id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  name        VARCHAR(255) NOT NULL,
  rc_number   VARCHAR(50),
  industry    VARCHAR(100),
  logo_url    TEXT,
  owner_id    UUID NOT NULL REFERENCES users(id),
  kyb_status  kyc_status NOT NULL DEFAULT 'pending',
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE org_members (
  org_id     UUID NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
  user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role       VARCHAR(50) NOT NULL DEFAULT 'member',
  joined_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (org_id, user_id)
);

-- ─── EVENTS ───────────────────────────────────────────────────────────────────

CREATE TABLE events (
  id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  owner_id        UUID NOT NULL REFERENCES users(id),
  org_id          UUID REFERENCES organisations(id),
  title           VARCHAR(255) NOT NULL,
  description     TEXT,
  event_type      event_type NOT NULL DEFAULT 'other',
  visibility      event_visibility NOT NULL DEFAULT 'private',
  status          event_status NOT NULL DEFAULT 'draft',
  start_at        TIMESTAMPTZ,
  end_at          TIMESTAMPTZ,
  venue_name      VARCHAR(255),
  venue_address   TEXT,
  venue_city      VARCHAR(100),
  venue_state     VARCHAR(100),
  cover_url       TEXT,
  max_guests      INT,
  budget_total    BIGINT NOT NULL DEFAULT 0,   -- in kobo
  ticket_price    BIGINT NOT NULL DEFAULT 0,   -- in kobo (0 = free)
  approval_status approval_status NOT NULL DEFAULT 'approved',
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_events_owner ON events(owner_id);
CREATE INDEX idx_events_status ON events(status, visibility);

CREATE TABLE event_collaborators (
  event_id   UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
  user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role       VARCHAR(50) NOT NULL DEFAULT 'editor',
  invited_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (event_id, user_id)
);

-- ─── GUESTS ───────────────────────────────────────────────────────────────────

CREATE TABLE guests (
  id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  event_id      UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
  user_id       UUID REFERENCES users(id),
  full_name     VARCHAR(255) NOT NULL,
  email         VARCHAR(255),
  phone         VARCHAR(20),
  status        guest_status NOT NULL DEFAULT 'invited',
  ticket_code   VARCHAR(20) UNIQUE NOT NULL DEFAULT upper(substr(md5(random()::text), 1, 8)),
  seat_number   INT,
  checked_in_at TIMESTAMPTZ,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_guests_event ON guests(event_id);
CREATE INDEX idx_guests_ticket ON guests(ticket_code);

-- ─── VENDORS ──────────────────────────────────────────────────────────────────

CREATE TABLE vendors (
  id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  user_id      UUID NOT NULL REFERENCES users(id),
  business_name VARCHAR(255) NOT NULL,
  category     VARCHAR(100) NOT NULL,
  bio          TEXT,
  city         VARCHAR(100),
  state        VARCHAR(100),
  avatar_url   TEXT,
  cover_url    TEXT,
  rating       NUMERIC(3,2) NOT NULL DEFAULT 0,
  review_count INT NOT NULL DEFAULT 0,
  verified     BOOLEAN NOT NULL DEFAULT false,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_vendors_category ON vendors(category);

CREATE TABLE vendor_services (
  id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  vendor_id   UUID NOT NULL REFERENCES vendors(id) ON DELETE CASCADE,
  name        VARCHAR(255) NOT NULL,
  description TEXT,
  price_from  BIGINT NOT NULL DEFAULT 0,   -- in kobo
  price_to    BIGINT,
  unit        VARCHAR(50),
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE vendor_portfolio (
  id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  vendor_id   UUID NOT NULL REFERENCES vendors(id) ON DELETE CASCADE,
  image_url   TEXT NOT NULL,
  caption     TEXT,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ─── BOOKINGS ─────────────────────────────────────────────────────────────────

CREATE TABLE bookings (
  id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  event_id        UUID NOT NULL REFERENCES events(id),
  vendor_id       UUID NOT NULL REFERENCES vendors(id),
  service_id      UUID REFERENCES vendor_services(id),
  client_id       UUID NOT NULL REFERENCES users(id),
  status          booking_status NOT NULL DEFAULT 'pending',
  total_amount    BIGINT NOT NULL DEFAULT 0,   -- in kobo
  escrow_amount   BIGINT NOT NULL DEFAULT 0,   -- 50% held
  escrow_released BOOLEAN NOT NULL DEFAULT false,
  notes           TEXT,
  event_date      TIMESTAMPTZ,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_bookings_event ON bookings(event_id);
CREATE INDEX idx_bookings_vendor ON bookings(vendor_id);
CREATE INDEX idx_bookings_client ON bookings(client_id);

-- ─── WALLETS ──────────────────────────────────────────────────────────────────

CREATE TABLE wallets (
  id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  user_id     UUID NOT NULL UNIQUE REFERENCES users(id),
  balance     BIGINT NOT NULL DEFAULT 0,   -- in kobo
  escrow_held BIGINT NOT NULL DEFAULT 0,   -- in kobo
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE wallet_transactions (
  id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  wallet_id       UUID NOT NULL REFERENCES wallets(id),
  type            txn_type NOT NULL,
  amount          BIGINT NOT NULL,   -- in kobo
  status          txn_status NOT NULL DEFAULT 'pending',
  reference       VARCHAR(100) UNIQUE,
  paystack_ref    VARCHAR(100),
  description     TEXT,
  booking_id      UUID REFERENCES bookings(id),
  meta            JSONB,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_wallet_txn_wallet ON wallet_transactions(wallet_id);
CREATE INDEX idx_wallet_txn_ref ON wallet_transactions(reference);
CREATE INDEX idx_wallet_txn_paystack ON wallet_transactions(paystack_ref);

-- ─── KYC ──────────────────────────────────────────────────────────────────────

CREATE TABLE kyc_verifications (
  id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  user_id      UUID NOT NULL REFERENCES users(id),
  tier         kyc_tier NOT NULL,
  status       kyc_status NOT NULL DEFAULT 'pending',
  bvn_hash     VARCHAR(64),
  nin_hash     VARCHAR(64),
  dojah_ref    VARCHAR(100),
  failure_reason TEXT,
  verified_at  TIMESTAMPTZ,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_kyc_user ON kyc_verifications(user_id);

-- ─── PRODUCTS (Marketplace) ───────────────────────────────────────────────────

CREATE TABLE products (
  id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  vendor_id   UUID NOT NULL REFERENCES vendors(id),
  name        VARCHAR(255) NOT NULL,
  description TEXT,
  price       BIGINT NOT NULL DEFAULT 0,   -- in kobo
  category    VARCHAR(100),
  image_url   TEXT,
  stock       INT,
  active      BOOLEAN NOT NULL DEFAULT true,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_products_vendor ON products(vendor_id);
CREATE INDEX idx_products_category ON products(category, active);

-- ─── TRIGGERS: updated_at ─────────────────────────────────────────────────────

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_users_updated_at BEFORE UPDATE ON users
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_events_updated_at BEFORE UPDATE ON events
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_bookings_updated_at BEFORE UPDATE ON bookings
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_wallets_updated_at BEFORE UPDATE ON wallets
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ─── SEED: auto-create wallet on new user ─────────────────────────────────────

CREATE OR REPLACE FUNCTION create_wallet_for_user()
RETURNS TRIGGER AS $$
BEGIN
  INSERT INTO wallets (user_id) VALUES (NEW.id);
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_user_wallet AFTER INSERT ON users
  FOR EACH ROW EXECUTE FUNCTION create_wallet_for_user();
