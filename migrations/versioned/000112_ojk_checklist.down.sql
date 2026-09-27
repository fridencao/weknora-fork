-- Migration 000112 down: drop OJK tables in reverse dependency order

BEGIN;

DROP INDEX IF EXISTS idx_ojk_items_run;
DROP INDEX IF EXISTS idx_ojk_items_tenant_status;
DROP INDEX IF EXISTS idx_ojk_runs_tenant_status;

DROP TABLE IF EXISTS ojk_checklist_items;
DROP TABLE IF EXISTS ojk_runs;

DO $$ BEGIN RAISE NOTICE '[Migration 000112] Rolled back.'; END $$;

COMMIT;
