-- Migration 000109: KB 级图谱开关首版列（auto_graph_config）。
-- 已在开发环境执行过一次；后续被 ADR-008 定版的 graph_config（000110）取代。
-- 保留本迁移使已记录 version=109 的库可继续演进；两列并存无害。
ALTER TABLE knowledge_bases ADD COLUMN IF NOT EXISTS auto_graph_config JSONB;
