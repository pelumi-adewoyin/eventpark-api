-- Migration 003: Phase 4 — Full Corporate Schema
-- Departments, KYB docs, corporate vendor directory, approvals,
-- RFQs, POs, goods receipts, invoices, corporate wallet, audit log, integrations

-- ─── ENUMS ────────────────────────────────────────────────────────────────────

DO $$ BEGIN
  CREATE TYPE org_member_role AS ENUM (
    'member', 'event_organizer', 'approver', 'finance',
    'procurement', 'comms', 'admin', 'owner'
  );
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
  CREATE TYPE kyb_tier AS ENUM ('0', '1', '2', '3');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
  CREATE TYPE kyb_doc_type AS ENUM (
    'cac_certificate', 'memorandum', 'director_id', 'utility_bill', 'tax_clearance', 'other'
  );
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
  CREATE TYPE approval_item_type AS ENUM ('po', 'invoice', 'expense', 'rfq_award', 'withdrawal', 'event');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
  CREATE TYPE approval_action AS ENUM ('approved', 'rejected', 'changes_requested', 'delegated');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
  CREATE TYPE rfq_status AS ENUM ('draft', 'open', 'evaluating', 'awarded', 'cancelled');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
  CREATE TYPE po_status AS ENUM ('draft', 'pending_approval', 'approved', 'rejected', 'closed', 'cancelled');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
  CREATE TYPE invoice_status AS ENUM ('pending_approval', 'approved', 'paid', 'disputed', 'cancelled');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
  CREATE TYPE match_status AS ENUM ('unmatched', 'partial', 'matched');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
  CREATE TYPE corp_vendor_status AS ENUM ('active', 'preferred', 'blacklisted');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- ─── ORGANISATIONS — extend existing table ────────────────────────────────────

ALTER TABLE organisations
  ADD COLUMN IF NOT EXISTS tin            VARCHAR(50),
  ADD COLUMN IF NOT EXISTS address        TEXT,
  ADD COLUMN IF NOT EXISTS city           VARCHAR(100),
  ADD COLUMN IF NOT EXISTS state          VARCHAR(100),
  ADD COLUMN IF NOT EXISTS website        TEXT,
  ADD COLUMN IF NOT EXISTS kyb_tier       kyb_tier NOT NULL DEFAULT '0',
  ADD COLUMN IF NOT EXISTS plan           VARCHAR(50) NOT NULL DEFAULT 'starter',
  ADD COLUMN IF NOT EXISTS updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW();

DROP TRIGGER IF EXISTS trg_orgs_updated_at ON organisations;
CREATE TRIGGER trg_orgs_updated_at BEFORE UPDATE ON organisations
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ─── KYB DOCUMENTS ────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS kyb_documents (
  id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  org_id      UUID NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
  doc_type    kyb_doc_type NOT NULL,
  file_url    TEXT NOT NULL,
  status      VARCHAR(20) NOT NULL DEFAULT 'pending',   -- pending | approved | rejected
  reviewer_id UUID REFERENCES users(id),
  reviewed_at TIMESTAMPTZ,
  notes       TEXT,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kyb_docs_org ON kyb_documents(org_id);

-- ─── DEPARTMENTS ──────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS departments (
  id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  org_id      UUID NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
  name        VARCHAR(255) NOT NULL,
  head_id     UUID REFERENCES users(id),
  budget      BIGINT NOT NULL DEFAULT 0,   -- in kobo, annual budget
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (org_id, name)
);

CREATE INDEX IF NOT EXISTS idx_departments_org ON departments(org_id);

-- ─── ORG MEMBERS — extend join table ─────────────────────────────────────────

ALTER TABLE org_members
  ADD COLUMN IF NOT EXISTS role_enum   org_member_role NOT NULL DEFAULT 'member',
  ADD COLUMN IF NOT EXISTS dept_id     UUID REFERENCES departments(id),
  ADD COLUMN IF NOT EXISTS active      BOOLEAN NOT NULL DEFAULT true,
  ADD COLUMN IF NOT EXISTS invited_by  UUID REFERENCES users(id),
  ADD COLUMN IF NOT EXISTS updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- ─── CORPORATE VENDOR DIRECTORY ───────────────────────────────────────────────
-- Separate from the public vendor marketplace; org-scoped preferred/blacklist

CREATE TABLE IF NOT EXISTS corp_vendors (
  id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  org_id          UUID NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
  vendor_user_id  UUID REFERENCES users(id),            -- if vendor is on platform
  name            VARCHAR(255) NOT NULL,
  category        VARCHAR(100) NOT NULL,
  email           VARCHAR(255),
  phone           VARCHAR(30),
  address         TEXT,
  status          corp_vendor_status NOT NULL DEFAULT 'active',
  total_paid      BIGINT NOT NULL DEFAULT 0,
  notes           TEXT,
  added_by        UUID REFERENCES users(id),
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_corp_vendors_org ON corp_vendors(org_id);

CREATE TRIGGER trg_corp_vendors_updated_at BEFORE UPDATE ON corp_vendors
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ─── APPROVAL REQUESTS ────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS approval_requests (
  id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  org_id          UUID NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
  event_id        UUID REFERENCES events(id),
  item_type       approval_item_type NOT NULL,
  item_id         UUID NOT NULL,              -- ref to po_id / invoice_id / etc.
  item_ref        VARCHAR(50),                -- human-readable ref e.g. PO-2026-041
  title           VARCHAR(255) NOT NULL,
  amount          BIGINT NOT NULL DEFAULT 0,
  currency        CHAR(3) NOT NULL DEFAULT 'NGN',
  requested_by    UUID NOT NULL REFERENCES users(id),
  notes           TEXT,
  urgency         VARCHAR(10) NOT NULL DEFAULT 'medium', -- low | medium | high
  status          VARCHAR(30) NOT NULL DEFAULT 'pending', -- pending | approved | rejected | changes_requested
  resolved_at     TIMESTAMPTZ,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_approvals_org ON approval_requests(org_id, status);
CREATE INDEX IF NOT EXISTS idx_approvals_item ON approval_requests(item_id);

CREATE TRIGGER trg_approvals_updated_at BEFORE UPDATE ON approval_requests
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ─── APPROVAL ACTIONS (the audit chain of steps) ──────────────────────────────

CREATE TABLE IF NOT EXISTS approval_actions (
  id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  request_id      UUID NOT NULL REFERENCES approval_requests(id) ON DELETE CASCADE,
  actor_id        UUID NOT NULL REFERENCES users(id),
  action          approval_action NOT NULL,
  note            TEXT,
  delegated_to    UUID REFERENCES users(id),
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_approval_actions_request ON approval_actions(request_id);

-- ─── RFQs ─────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS rfqs (
  id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  org_id          UUID NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
  event_id        UUID REFERENCES events(id),
  ref             VARCHAR(30) UNIQUE NOT NULL,
  title           VARCHAR(255) NOT NULL,
  description     TEXT,
  requirements    JSONB,          -- custom form fields defined by organizer
  deadline        DATE,
  status          rfq_status NOT NULL DEFAULT 'draft',
  awarded_vendor  UUID REFERENCES corp_vendors(id),
  created_by      UUID NOT NULL REFERENCES users(id),
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_rfqs_org ON rfqs(org_id);

CREATE TRIGGER trg_rfqs_updated_at BEFORE UPDATE ON rfqs
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- RFQ invitations (which vendors were invited)
CREATE TABLE IF NOT EXISTS rfq_invitations (
  id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  rfq_id      UUID NOT NULL REFERENCES rfqs(id) ON DELETE CASCADE,
  vendor_id   UUID NOT NULL REFERENCES corp_vendors(id),
  sent_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  responded   BOOLEAN NOT NULL DEFAULT false
);

CREATE INDEX IF NOT EXISTS idx_rfq_invitations_rfq ON rfq_invitations(rfq_id);

-- RFQ responses from vendors
CREATE TABLE IF NOT EXISTS rfq_responses (
  id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  rfq_id          UUID NOT NULL REFERENCES rfqs(id) ON DELETE CASCADE,
  vendor_id       UUID NOT NULL REFERENCES corp_vendors(id),
  amount          BIGINT NOT NULL DEFAULT 0,
  delivery_days   INT,
  notes           TEXT,
  attachments     TEXT[],
  score           SMALLINT,        -- evaluator score 1-10
  awarded         BOOLEAN NOT NULL DEFAULT false,
  submitted_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_rfq_responses_rfq ON rfq_responses(rfq_id);

-- ─── PURCHASE ORDERS ──────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS purchase_orders (
  id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  org_id          UUID NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
  event_id        UUID REFERENCES events(id),
  budget_line_id  UUID REFERENCES event_budget_lines(id),
  vendor_id       UUID NOT NULL REFERENCES corp_vendors(id),
  ref             VARCHAR(30) UNIQUE NOT NULL,
  scope           TEXT,
  amount          BIGINT NOT NULL DEFAULT 0,
  currency        CHAR(3) NOT NULL DEFAULT 'NGN',
  status          po_status NOT NULL DEFAULT 'draft',
  issued_at       DATE,
  due_at          DATE,
  created_by      UUID NOT NULL REFERENCES users(id),
  approved_by     UUID REFERENCES users(id),
  approved_at     TIMESTAMPTZ,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_pos_org ON purchase_orders(org_id);
CREATE INDEX IF NOT EXISTS idx_pos_vendor ON purchase_orders(vendor_id);

CREATE TRIGGER trg_pos_updated_at BEFORE UPDATE ON purchase_orders
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- PO line items
CREATE TABLE IF NOT EXISTS po_line_items (
  id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  po_id       UUID NOT NULL REFERENCES purchase_orders(id) ON DELETE CASCADE,
  description VARCHAR(255) NOT NULL,
  quantity    NUMERIC(10,2) NOT NULL DEFAULT 1,
  unit        VARCHAR(50),
  unit_price  BIGINT NOT NULL DEFAULT 0,
  total       BIGINT NOT NULL DEFAULT 0
);

-- ─── GOODS RECEIPTS ───────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS goods_receipts (
  id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  po_id           UUID NOT NULL REFERENCES purchase_orders(id) ON DELETE CASCADE,
  received_by     UUID NOT NULL REFERENCES users(id),
  received_at     DATE NOT NULL DEFAULT CURRENT_DATE,
  notes           TEXT,
  partial         BOOLEAN NOT NULL DEFAULT false,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_gr_po ON goods_receipts(po_id);

-- ─── VENDOR INVOICES ──────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS vendor_invoices (
  id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  org_id          UUID NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
  po_id           UUID REFERENCES purchase_orders(id),
  vendor_id       UUID NOT NULL REFERENCES corp_vendors(id),
  ref             VARCHAR(50) NOT NULL,
  amount          BIGINT NOT NULL DEFAULT 0,   -- gross
  vat_amount      BIGINT NOT NULL DEFAULT 0,   -- 7.5% of amount
  wht_amount      BIGINT NOT NULL DEFAULT 0,   -- 5% or 10% of amount
  net_payable     BIGINT NOT NULL DEFAULT 0,   -- amount + vat - wht
  currency        CHAR(3) NOT NULL DEFAULT 'NGN',
  invoice_date    DATE NOT NULL DEFAULT CURRENT_DATE,
  due_date        DATE,
  status          invoice_status NOT NULL DEFAULT 'pending_approval',
  match_status    match_status NOT NULL DEFAULT 'unmatched',
  paid_at         TIMESTAMPTZ,
  uploaded_by     UUID NOT NULL REFERENCES users(id),
  approved_by     UUID REFERENCES users(id),
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_invoices_org ON vendor_invoices(org_id);
CREATE INDEX IF NOT EXISTS idx_invoices_po ON vendor_invoices(po_id);

CREATE TRIGGER trg_invoices_updated_at BEFORE UPDATE ON vendor_invoices
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ─── CORPORATE WALLET ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS org_wallets (
  id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  org_id          UUID NOT NULL UNIQUE REFERENCES organisations(id) ON DELETE CASCADE,
  balance         BIGINT NOT NULL DEFAULT 0,
  pending_out     BIGINT NOT NULL DEFAULT 0,
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER trg_org_wallets_updated_at BEFORE UPDATE ON org_wallets
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Multi-signatory config
CREATE TABLE IF NOT EXISTS org_wallet_signatories (
  id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  org_id      UUID NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
  user_id     UUID NOT NULL REFERENCES users(id),
  threshold   BIGINT NOT NULL DEFAULT 5000000_00,  -- above this amount, multi-sig required (kobo)
  added_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (org_id, user_id)
);

-- Corporate wallet transactions
CREATE TABLE IF NOT EXISTS org_wallet_transactions (
  id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  org_wallet_id   UUID NOT NULL REFERENCES org_wallets(id),
  type            VARCHAR(20) NOT NULL,   -- credit | debit
  amount          BIGINT NOT NULL,
  balance_after   BIGINT NOT NULL,
  reference       VARCHAR(100),
  description     TEXT,
  invoice_id      UUID REFERENCES vendor_invoices(id),
  initiated_by    UUID REFERENCES users(id),
  status          VARCHAR(20) NOT NULL DEFAULT 'success',
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_org_wallet_txns ON org_wallet_transactions(org_wallet_id, created_at DESC);

-- Withdrawal requests (multi-sig)
CREATE TABLE IF NOT EXISTS org_withdrawal_requests (
  id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  org_id          UUID NOT NULL REFERENCES organisations(id),
  amount          BIGINT NOT NULL,
  reason          TEXT,
  requested_by    UUID NOT NULL REFERENCES users(id),
  status          VARCHAR(20) NOT NULL DEFAULT 'pending',  -- pending | approved | rejected | executed
  approvals_needed INT NOT NULL DEFAULT 2,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  executed_at     TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS org_withdrawal_approvals (
  request_id  UUID NOT NULL REFERENCES org_withdrawal_requests(id) ON DELETE CASCADE,
  signer_id   UUID NOT NULL REFERENCES users(id),
  action      VARCHAR(10) NOT NULL,   -- approved | rejected
  signed_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (request_id, signer_id)
);

-- Auto-create org wallet when organisation is created
CREATE OR REPLACE FUNCTION create_org_wallet()
RETURNS TRIGGER AS $$
BEGIN
  INSERT INTO org_wallets (org_id) VALUES (NEW.id) ON CONFLICT DO NOTHING;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_org_wallet ON organisations;
CREATE TRIGGER trg_org_wallet AFTER INSERT ON organisations
  FOR EACH ROW EXECUTE FUNCTION create_org_wallet();

-- ─── AUDIT LOG ────────────────────────────────────────────────────────────────
-- Append-only. Each row stores hash of (previous_hash || payload) for tamper evidence.

CREATE TABLE IF NOT EXISTS audit_logs (
  id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  org_id          UUID REFERENCES organisations(id),
  user_id         UUID REFERENCES users(id),
  actor_name      VARCHAR(255),
  action          VARCHAR(100) NOT NULL,
  entity_type     VARCHAR(100),
  entity_id       UUID,
  detail          TEXT,
  meta            JSONB,
  previous_hash   VARCHAR(64),
  entry_hash      VARCHAR(64) NOT NULL,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_org ON audit_logs(org_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_user ON audit_logs(user_id);

-- Audit log is append-only — prevent update and delete
CREATE OR REPLACE RULE audit_no_update AS ON UPDATE TO audit_logs DO INSTEAD NOTHING;
CREATE OR REPLACE RULE audit_no_delete AS ON DELETE TO audit_logs DO INSTEAD NOTHING;

-- ─── INTEGRATIONS ─────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS integrations (
  id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  org_id      UUID NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
  name        VARCHAR(100) NOT NULL,   -- slack | google_calendar | quickbooks | paystack | zoom | hubspot | zapier | webhook
  connected   BOOLEAN NOT NULL DEFAULT false,
  config      JSONB,                   -- encrypted tokens, webhook URL, etc.
  connected_by UUID REFERENCES users(id),
  connected_at TIMESTAMPTZ,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (org_id, name)
);

CREATE INDEX IF NOT EXISTS idx_integrations_org ON integrations(org_id);

-- ─── SEQUENCE for human-readable refs ─────────────────────────────────────────

CREATE SEQUENCE IF NOT EXISTS rfq_ref_seq START 1;
CREATE SEQUENCE IF NOT EXISTS po_ref_seq  START 1;
