-- Migration 002: Phase 2 gaps
-- RSVP tokens, budget lines, guest enhancements, vendor reviews, IV templates

-- ─── ENUMS (new) ──────────────────────────────────────────────────────────────

DO $$ BEGIN
  CREATE TYPE rsvp_status AS ENUM ('pending', 'accepted', 'declined');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
  CREATE TYPE budget_category AS ENUM (
    'venue', 'catering', 'av_tech', 'transport', 'accommodation',
    'photography', 'entertainment', 'decor', 'printing', 'security',
    'staff', 'marketing', 'miscellaneous'
  );
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- ─── RSVP TOKENS ──────────────────────────────────────────────────────────────
-- Each guest gets a unique short token for their RSVP link

ALTER TABLE guests
  ADD COLUMN IF NOT EXISTS rsvp_token   VARCHAR(16) UNIQUE DEFAULT upper(substr(md5(random()::text), 1, 8)),
  ADD COLUMN IF NOT EXISTS rsvp_status  rsvp_status NOT NULL DEFAULT 'pending',
  ADD COLUMN IF NOT EXISTS rsvp_note    TEXT,
  ADD COLUMN IF NOT EXISTS plus_one     BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS plus_one_allowed BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS plus_one_name    VARCHAR(255),
  ADD COLUMN IF NOT EXISTS dietary_req  TEXT,
  ADD COLUMN IF NOT EXISTS tags         TEXT[],
  ADD COLUMN IF NOT EXISTS relationship VARCHAR(100),
  ADD COLUMN IF NOT EXISTS iv_template  VARCHAR(50),
  ADD COLUMN IF NOT EXISTS responded_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_guests_rsvp_token ON guests(rsvp_token);

-- ─── EVENT BUDGET LINES ───────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS event_budget_lines (
  id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  event_id    UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
  category    budget_category NOT NULL DEFAULT 'miscellaneous',
  label       VARCHAR(255) NOT NULL,
  allocated   BIGINT NOT NULL DEFAULT 0,   -- in kobo
  spent       BIGINT NOT NULL DEFAULT 0,   -- in kobo (updated as POs/bookings are paid)
  notes       TEXT,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_budget_lines_event ON event_budget_lines(event_id);

CREATE TRIGGER trg_budget_lines_updated_at BEFORE UPDATE ON event_budget_lines
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ─── VENDOR REVIEWS ───────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS vendor_reviews (
  id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  vendor_id   UUID NOT NULL REFERENCES vendors(id) ON DELETE CASCADE,
  booking_id  UUID REFERENCES bookings(id),
  reviewer_id UUID NOT NULL REFERENCES users(id),
  rating      SMALLINT NOT NULL CHECK (rating BETWEEN 1 AND 5),
  body        TEXT,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_vendor_reviews_vendor ON vendor_reviews(vendor_id);

-- Auto-update vendor rating + review_count after insert/delete
CREATE OR REPLACE FUNCTION refresh_vendor_rating()
RETURNS TRIGGER AS $$
BEGIN
  UPDATE vendors
  SET
    rating       = (SELECT COALESCE(AVG(rating), 0) FROM vendor_reviews WHERE vendor_id = COALESCE(NEW.vendor_id, OLD.vendor_id)),
    review_count = (SELECT COUNT(*) FROM vendor_reviews WHERE vendor_id = COALESCE(NEW.vendor_id, OLD.vendor_id))
  WHERE id = COALESCE(NEW.vendor_id, OLD.vendor_id);
  RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_vendor_rating_ins ON vendor_reviews;
CREATE TRIGGER trg_vendor_rating_ins AFTER INSERT OR DELETE ON vendor_reviews
  FOR EACH ROW EXECUTE FUNCTION refresh_vendor_rating();

-- ─── EVENT COLLABORATORS (ensure unique constraint exists) ────────────────────

-- Already defined in migration 001 but ensure index exists
CREATE INDEX IF NOT EXISTS idx_event_collaborators_user ON event_collaborators(user_id);

-- ─── NOTIFICATIONS ────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS notifications (
  id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  type        VARCHAR(100) NOT NULL,   -- rsvp_received, approval_needed, payment_done, etc.
  title       VARCHAR(255) NOT NULL,
  body        TEXT,
  read        BOOLEAN NOT NULL DEFAULT false,
  meta        JSONB,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_notifications_user ON notifications(user_id, read, created_at DESC);

-- ─── REMINDER SCHEDULES ───────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS reminder_schedules (
  id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  event_id    UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
  send_at     TIMESTAMPTZ NOT NULL,
  subject     VARCHAR(255),
  body        TEXT,
  recipient_filter VARCHAR(50) NOT NULL DEFAULT 'pending',  -- pending | all | rsvp_yes
  sent        BOOLEAN NOT NULL DEFAULT false,
  sent_at     TIMESTAMPTZ,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_reminders_event ON reminder_schedules(event_id);
CREATE INDEX IF NOT EXISTS idx_reminders_send_at ON reminder_schedules(send_at) WHERE sent = false;
