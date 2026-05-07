-- Migration 011: Allow direct vendor bookings (no event required) + service category

-- bookings.event_id is NOT NULL but direct VendorDetail bookings have no event yet
ALTER TABLE bookings ALTER COLUMN event_id DROP NOT NULL;

-- Add category to vendor_services so service listings can be categorised
ALTER TABLE vendor_services ADD COLUMN IF NOT EXISTS category TEXT;

-- Add updated_at trigger for orders (if missing)
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_trigger WHERE tgname = 'trg_orders_updated_at'
  ) THEN
    CREATE TRIGGER trg_orders_updated_at
      BEFORE UPDATE ON orders
      FOR EACH ROW EXECUTE FUNCTION set_updated_at();
  END IF;
END $$;
