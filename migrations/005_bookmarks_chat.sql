-- Migration 005: Bookmarks, vendor chat, wishlists

-- User bookmarks (vendors, events, products)
CREATE TABLE IF NOT EXISTS bookmarks (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    item_type  TEXT NOT NULL CHECK (item_type IN ('vendor', 'event', 'product')),
    item_id    UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, item_type, item_id)
);

-- Vendor chat threads (one per booking)
CREATE TABLE IF NOT EXISTS vendor_chats (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id  UUID NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (booking_id)
);

-- Chat messages
CREATE TABLE IF NOT EXISTS chat_messages (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    chat_id    UUID NOT NULL REFERENCES vendor_chats(id) ON DELETE CASCADE,
    sender_id  UUID NOT NULL REFERENCES users(id),
    body       TEXT NOT NULL,
    msg_type   TEXT NOT NULL DEFAULT 'text' CHECK (msg_type IN ('text', 'quote', 'counter_quote', 'system')),
    metadata   JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Booking quotes (structured quote inside chat)
CREATE TABLE IF NOT EXISTS booking_quotes (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id     UUID NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
    sent_by        UUID NOT NULL REFERENCES users(id),
    quote_type     TEXT NOT NULL DEFAULT 'initial' CHECK (quote_type IN ('initial','counter','accepted','rejected')),
    line_items     JSONB NOT NULL DEFAULT '[]',
    subtotal_ngn   BIGINT NOT NULL,
    vat_ngn        BIGINT NOT NULL DEFAULT 0,
    total_ngn      BIGINT NOT NULL,
    valid_until    TIMESTAMPTZ,
    notes          TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Extend bookings table (add missing columns if not present)
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS service_date DATE;
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS requirements_brief TEXT;
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS budget_hint BIGINT;
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS headcount INTEGER;
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS payment_status TEXT NOT NULL DEFAULT 'pending';
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS quote_status TEXT;

-- Wishlists (for personal budget planner integration)
CREATE TABLE IF NOT EXISTS wishlists (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    slug        TEXT UNIQUE,
    description TEXT,
    is_public   BOOLEAN NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS wishlist_items (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    wishlist_id  UUID NOT NULL REFERENCES wishlists(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    description  TEXT,
    price_ngn    BIGINT,
    url          TEXT,
    image_url    TEXT,
    is_reserved  BOOLEAN NOT NULL DEFAULT false,
    reserved_by  UUID REFERENCES users(id),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
