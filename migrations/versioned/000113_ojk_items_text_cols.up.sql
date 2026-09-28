-- Migration 000113: OJK checklist items — array columns → TEXT, per-run requirement_id
--
-- Two defects made every checklist item insert fail silently while runs still
-- recorded total_items (phantom versions with an empty table):
--   1. pgx binds Go string params as text; a text[] column rejects them with
--      42804 ("column is of type text[] but expression is of type text").
--      applicable_roles / keywords were therefore unwritable. They are stored
--      as PG array literals ("{a,b}") in TEXT now — StrList scans both that
--      and JSON, so no read-side change.
--   2. requirement_id was globally UNIQUE, but every run re-extracts the same
--      regulations — versions are snapshots of the same requirement IDs. The
--      constraint moves to (run_id, requirement_id).
-- Table was empty at migration time, so type changes are lossless.

BEGIN;

ALTER TABLE ojk_checklist_items ALTER COLUMN applicable_roles TYPE TEXT;
ALTER TABLE ojk_checklist_items ALTER COLUMN keywords TYPE TEXT;

ALTER TABLE ojk_checklist_items
  DROP CONSTRAINT IF EXISTS ojk_checklist_items_requirement_id_key;

DROP INDEX IF EXISTS idx_ojk_items_run_req;
CREATE UNIQUE INDEX idx_ojk_items_run_req
  ON ojk_checklist_items (run_id, requirement_id);

DO $$ BEGIN RAISE NOTICE '[Migration 000113] ojk_checklist_items array cols -> TEXT, per-run requirement_id.'; END $$;

COMMIT;
