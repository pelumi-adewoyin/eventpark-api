-- Extend event_type ENUM with all types used by the frontend.
-- ALTER TYPE ... ADD VALUE is idempotent in PG 14+ via IF NOT EXISTS.
ALTER TYPE event_type ADD VALUE IF NOT EXISTS 'naming';
ALTER TYPE event_type ADD VALUE IF NOT EXISTS 'social';
ALTER TYPE event_type ADD VALUE IF NOT EXISTS 'workshop';
ALTER TYPE event_type ADD VALUE IF NOT EXISTS 'popup';
ALTER TYPE event_type ADD VALUE IF NOT EXISTS 'retreat';
ALTER TYPE event_type ADD VALUE IF NOT EXISTS 'teamparty';
ALTER TYPE event_type ADD VALUE IF NOT EXISTS 'festival';
ALTER TYPE event_type ADD VALUE IF NOT EXISTS 'dj_night';
ALTER TYPE event_type ADD VALUE IF NOT EXISTS 'meetup';
ALTER TYPE event_type ADD VALUE IF NOT EXISTS 'comedy_show';
ALTER TYPE event_type ADD VALUE IF NOT EXISTS 'theatre';
ALTER TYPE event_type ADD VALUE IF NOT EXISTS 'brand_activation';
