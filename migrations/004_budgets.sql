-- Personal budgets
CREATE TABLE IF NOT EXISTS personal_budgets (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    scope       TEXT NOT NULL CHECK (scope IN ('event', 'period', 'lifetime')),
    event_id    UUID REFERENCES events(id) ON DELETE SET NULL,
    total_ngn   BIGINT NOT NULL CHECK (total_ngn > 0),
    period_start DATE,
    period_end   DATE,
    alert_at_70  BOOLEAN NOT NULL DEFAULT false,
    alert_at_90  BOOLEAN NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS budget_categories (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    budget_id    UUID NOT NULL REFERENCES personal_budgets(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    allocated_ngn BIGINT NOT NULL CHECK (allocated_ngn >= 0),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS budget_expenses (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    budget_id       UUID NOT NULL REFERENCES personal_budgets(id) ON DELETE CASCADE,
    category_id     UUID REFERENCES budget_categories(id) ON DELETE SET NULL,
    description     TEXT NOT NULL,
    amount_ngn      BIGINT NOT NULL CHECK (amount_ngn > 0),
    expense_date    DATE NOT NULL DEFAULT CURRENT_DATE,
    payment_method  TEXT NOT NULL DEFAULT 'other',
    notes           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Trigger to update personal_budgets.updated_at
DROP TRIGGER IF EXISTS trg_personal_budgets_updated_at ON personal_budgets;
CREATE TRIGGER trg_personal_budgets_updated_at
    BEFORE UPDATE ON personal_budgets
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
