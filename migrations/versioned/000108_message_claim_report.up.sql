-- Migration: 000108_message_claim_report
-- Description: M3 G4 (docs/07 WS4) — persist the answer-level claim audit
-- report on assistant messages (STARKB_CLAIM_GATE). NULL when the gate is off.

DO $$ BEGIN RAISE NOTICE '[Migration 000108] Adding messages.claim_report...'; END $$;

ALTER TABLE messages
    ADD COLUMN IF NOT EXISTS claim_report JSONB;

DO $$ BEGIN RAISE NOTICE '[Migration 000108] Message claim report ready'; END $$;
