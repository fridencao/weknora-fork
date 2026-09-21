-- Migration: 000108_message_claim_report (rollback)

ALTER TABLE messages
    DROP COLUMN IF EXISTS claim_report;
