-- Migration 000114 down: drop candidate registry

BEGIN;

DROP INDEX IF EXISTS idx_ojk_candidates_tenant;
DROP TABLE IF EXISTS ojk_candidates;

DO $$ BEGIN RAISE NOTICE '[Migration 000114] Rolled back.'; END $$;

COMMIT;
