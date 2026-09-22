-- Migration 000110: opt-in automatic LightRAG graph building after parsing (docs/09 A1/A3).
ALTER TABLE knowledge_bases ADD COLUMN IF NOT EXISTS graph_config JSONB;
