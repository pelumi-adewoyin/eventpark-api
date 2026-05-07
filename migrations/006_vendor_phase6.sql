-- Migration 006: Vendor Phase 6 — extended schema for vendor marketplace

-- ─── Extend vendors table ───────────────────────────────────────────────────

ALTER TABLE vendors ADD COLUMN IF NOT EXISTS vendor_type         TEXT NOT NULL DEFAULT 'service' CHECK (vendor_type IN ('service', 'product'));
ALTER TABLE vendors ADD COLUMN IF NOT EXISTS verification_status TEXT NOT NULL DEFAULT 'tier_1'
    CHECK (verification_status IN ('tier_1', 'tier_2_pending', 'tier_2_approved', 'tier_3_pending', 'tier_3_approved'));
ALTER TABLE vendors ADD COLUMN IF NOT EXISTS verification_tier   INT  NOT NULL DEFAULT 0;
ALTER TABLE vendors ADD COLUMN IF NOT EXISTS is_registered       BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE vendors ADD COLUMN IF NOT EXISTS cac_rc_number       TEXT;
ALTER TABLE vendors ADD COLUMN IF NOT EXISTS address             TEXT;
ALTER TABLE vendors ADD COLUMN IF NOT EXISTS postal_code         TEXT;
ALTER TABLE vendors ADD COLUMN IF NOT EXISTS tagline             TEXT;
ALTER TABLE vendors ADD COLUMN IF NOT EXISTS highlight_1         TEXT;
ALTER TABLE vendors ADD COLUMN IF NOT EXISTS highlight_2         TEXT;
ALTER TABLE vendors ADD COLUMN IF NOT EXISTS highlight_3         TEXT;
ALTER TABLE vendors ADD COLUMN IF NOT EXISTS years_experience    INT;
ALTER TABLE vendors ADD COLUMN IF NOT EXISTS events_completed    INT;
ALTER TABLE vendors ADD COLUMN IF NOT EXISTS website             TEXT;
ALTER TABLE vendors ADD COLUMN IF NOT EXISTS instagram           TEXT;
ALTER TABLE vendors ADD COLUMN IF NOT EXISTS twitter             TEXT;
ALTER TABLE vendors ADD COLUMN IF NOT EXISTS whatsapp            TEXT;
ALTER TABLE vendors ADD COLUMN IF NOT EXISTS updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- ─── Extend vendor_services ─────────────────────────────────────────────────

ALTER TABLE vendor_services ADD COLUMN IF NOT EXISTS pricing_model      TEXT NOT NULL DEFAULT 'fixed'
    CHECK (pricing_model IN ('fixed','starting_from','per_hour','per_day','negotiable'));
ALTER TABLE vendor_services ADD COLUMN IF NOT EXISTS is_active          BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE vendor_services ADD COLUMN IF NOT EXISTS min_notice_hours   INT NOT NULL DEFAULT 24;
ALTER TABLE vendor_services ADD COLUMN IF NOT EXISTS max_advance_days   INT NOT NULL DEFAULT 90;
ALTER TABLE vendor_services ADD COLUMN IF NOT EXISTS response_time_hrs  INT NOT NULL DEFAULT 2;
ALTER TABLE vendor_services ADD COLUMN IF NOT EXISTS updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- ─── Extend products table ──────────────────────────────────────────────────

ALTER TABLE products ADD COLUMN IF NOT EXISTS min_order_qty      INT NOT NULL DEFAULT 1;
ALTER TABLE products ADD COLUMN IF NOT EXISTS lead_time_days     INT NOT NULL DEFAULT 3;
ALTER TABLE products ADD COLUMN IF NOT EXISTS options            JSONB NOT NULL DEFAULT '[]';
ALTER TABLE products ADD COLUMN IF NOT EXISTS delivery_zones     TEXT[] NOT NULL DEFAULT '{}';
ALTER TABLE products ADD COLUMN IF NOT EXISTS free_delivery_above BIGINT;  -- kobo; NULL = no free delivery
ALTER TABLE products ADD COLUMN IF NOT EXISTS updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- ─── Orders (product vendor flow) ───────────────────────────────────────────

CREATE TABLE IF NOT EXISTS orders (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    vendor_id        UUID        NOT NULL REFERENCES vendors(id) ON DELETE RESTRICT,
    customer_id      UUID        NOT NULL REFERENCES users(id),
    status           TEXT        NOT NULL DEFAULT 'new'
        CHECK (status IN ('new','processing','ready','out_for_delivery','delivered','confirmed','cancelled')),
    total_amount     BIGINT      NOT NULL,          -- kobo
    escrow_amount    BIGINT      NOT NULL DEFAULT 0,
    escrow_released  BOOLEAN     NOT NULL DEFAULT false,
    delivery_address TEXT,
    delivery_zone    TEXT,
    notes            TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS order_items (
    id           UUID    PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id     UUID    NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    product_id   UUID    REFERENCES products(id),
    name         TEXT    NOT NULL,
    qty          INT     NOT NULL DEFAULT 1,
    unit_price   BIGINT  NOT NULL,   -- kobo
    options      JSONB   NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_orders_vendor ON orders(vendor_id);
CREATE INDEX IF NOT EXISTS idx_orders_customer ON orders(customer_id);
CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status);

-- ─── Vendor availability ────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS vendor_availability (
    id            UUID     PRIMARY KEY DEFAULT gen_random_uuid(),
    vendor_id     UUID     NOT NULL UNIQUE REFERENCES vendors(id) ON DELETE CASCADE,
    working_days  INT[]    NOT NULL DEFAULT '{1,2,3,4,5}',  -- 0=Sun … 6=Sat
    blocked_dates DATE[]   NOT NULL DEFAULT '{}',
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ─── Vendor bank accounts ───────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS vendor_bank_accounts (
    id             UUID     PRIMARY KEY DEFAULT gen_random_uuid(),
    vendor_id      UUID     NOT NULL REFERENCES vendors(id) ON DELETE CASCADE,
    bank_name      TEXT     NOT NULL,
    account_number TEXT     NOT NULL,
    account_name   TEXT     NOT NULL,
    is_default     BOOLEAN  NOT NULL DEFAULT false,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_vendor_bank_vendor ON vendor_bank_accounts(vendor_id);

-- ─── Vendor verification submissions ────────────────────────────────────────

CREATE TABLE IF NOT EXISTS vendor_verifications (
    id              UUID     PRIMARY KEY DEFAULT gen_random_uuid(),
    vendor_id       UUID     NOT NULL REFERENCES vendors(id) ON DELETE CASCADE,
    target_tier     INT      NOT NULL,           -- 2 or 3
    cac_rc_number   TEXT,
    cac_doc_url     TEXT,
    id_type         TEXT,
    id_doc_url      TEXT,
    bank_stmt_url   TEXT,
    notes           TEXT,
    status          TEXT     NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected')),
    reviewer_notes  TEXT,
    submitted_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reviewed_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_vendor_verif_vendor ON vendor_verifications(vendor_id, status);

-- ─── Triggers ───────────────────────────────────────────────────────────────

CREATE TRIGGER trg_vendors_updated_at BEFORE UPDATE ON vendors
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_vendor_services_updated_at BEFORE UPDATE ON vendor_services
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_products_updated_at BEFORE UPDATE ON products
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_orders_updated_at BEFORE UPDATE ON orders
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
