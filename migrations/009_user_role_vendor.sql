-- Add 'vendor' to the user_role ENUM so vendor onboarding works
ALTER TYPE user_role ADD VALUE IF NOT EXISTS 'vendor';
