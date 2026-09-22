-- Migration 000110 down: drop the automatic graph build configuration column.
ALTER TABLE knowledge_bases DROP COLUMN IF EXISTS graph_config;
