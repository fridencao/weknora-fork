-- Migration 000112: OJK Fit & Proper checklist tables
--
-- Stores the machine-extracted checklist items from fp-rule-skill
-- and their human-review status. Two tables:
--   ojk_runs          — one row per skill execution
--   ojk_checklist_items — extracted requirements, auditable rows

BEGIN;

CREATE TABLE IF NOT EXISTS ojk_runs (
  run_id          TEXT PRIMARY KEY,
  tenant_id       INTEGER NOT NULL,
  skill_version   TEXT NOT NULL DEFAULT '1.0.0',
  status          TEXT NOT NULL DEFAULT 'pending',
  total_slices    INTEGER,
  total_items     INTEGER DEFAULT 0,
  flagged_items   INTEGER DEFAULT 0,
  error           TEXT,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE ojk_runs IS
  'One row per fp-rule-skill execution. Frontend polls status field.';

CREATE INDEX IF NOT EXISTS idx_ojk_runs_tenant_status
  ON ojk_runs (tenant_id, status) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS ojk_checklist_items (
  id               TEXT PRIMARY KEY,
  tenant_id        INTEGER NOT NULL,
  run_id           TEXT NOT NULL REFERENCES ojk_runs(run_id) ON DELETE CASCADE,
  regulation       TEXT NOT NULL,
  pasal            TEXT NOT NULL,
  pasal_text       TEXT NOT NULL,
  area             TEXT,
  requirement      TEXT NOT NULL,
  requirement_id   TEXT UNIQUE,
  evidence_type    TEXT,
  check_method     TEXT,
  applicable_roles TEXT[],
  severity         TEXT NOT NULL DEFAULT 'info',
  keywords         TEXT[],
  source           TEXT NOT NULL DEFAULT 'normal',
  _flag            TEXT,
  status           TEXT NOT NULL DEFAULT 'pending',
  reviewer_note    TEXT,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE ojk_checklist_items IS
  'Extracted checklist items awaiting human review. Only confirmed rows enter the published checklist.';

CREATE INDEX IF NOT EXISTS idx_ojk_items_tenant_status
  ON ojk_checklist_items (tenant_id, status);

CREATE INDEX IF NOT EXISTS idx_ojk_items_run
  ON ojk_checklist_items (run_id);

DO $$ BEGIN RAISE NOTICE '[Migration 000112] ojk_runs + ojk_checklist_items created.'; END $$;

COMMIT;
