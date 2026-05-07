-- Ticket tiers per event (replaces the single ticket_price column for public events)
CREATE TABLE IF NOT EXISTS ticket_tiers (
  id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  event_id      UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
  name          VARCHAR(100) NOT NULL,
  description   TEXT,
  price         BIGINT NOT NULL DEFAULT 0,   -- kobo; 0 = free
  quantity      INT    NOT NULL DEFAULT 0,
  quantity_sold INT    NOT NULL DEFAULT 0,
  kind          VARCHAR(20) NOT NULL DEFAULT 'paid',  -- paid | free | donation
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ticket_tiers_event ON ticket_tiers(event_id);
