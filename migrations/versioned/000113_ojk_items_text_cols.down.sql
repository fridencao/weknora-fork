-- Migration 000113 down: restore text[] columns and global requirement_id uniqueness
--
-- Reverting the type change requires the column to hold valid array literals;
-- JSON-shaped values (if any) would fail the cast, hence the USING guard.

BEGIN;

DROP INDEX IF EXISTS idx_ojk_items_run_req;

ALTER TABLE ojk_checklist_items
  ALTER COLUMN applicable_roles TYPE TEXT[]
  USING CASE WHEN applicable_roles IS NULL OR applicable_roles = ''
             THEN '{}' ELSE applicable_roles::text[] END;

ALTER TABLE ojk_checklist_items
  ALTER COLUMN keywords TYPE TEXT[]
  USING CASE WHEN keywords IS NULL OR keywords = ''
             THEN '{}' ELSE keywords::text[] END;

ALTER TABLE ojk_checklist_items
  ADD CONSTRAINT ojk_checklist_items_requirement_id_key UNIQUE (requirement_id);

DO $$ BEGIN RAISE NOTICE '[Migration 000113] Rolled back.'; END $$;

COMMIT;
