-- vendors.user_id needs a UNIQUE constraint so that
-- ON CONFLICT (user_id) DO UPDATE works in CreateVendor
ALTER TABLE vendors ADD CONSTRAINT vendors_user_id_unique UNIQUE (user_id);
