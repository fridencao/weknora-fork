-- Migration 000109 down.
ALTER TABLE knowledge_bases DROP COLUMN IF EXISTS auto_graph_config;
