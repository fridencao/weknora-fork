-- Migration 000114: OJK 候选人登记表（阶段 2 材料摄入）
--
-- 每个候选人一个专属材料知识库（ojk_candidates.kb_id → knowledge_bases），
-- 材料文件走 WeKnora 现成的上传→解析→向量化管线（/knowledge-bases/:id/knowledge/file）。
-- 本表只做登记与聚合（文件数/解析态从 knowledges 派生）。

BEGIN;

CREATE TABLE IF NOT EXISTS ojk_candidates (
  id          TEXT PRIMARY KEY,
  tenant_id   INTEGER NOT NULL,
  name        TEXT NOT NULL,
  nik         TEXT,
  position    TEXT,
  institution TEXT,
  kb_id       TEXT NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_ojk_candidates_tenant
  ON ojk_candidates (tenant_id);

DO $$ BEGIN RAISE NOTICE '[Migration 000114] ojk_candidates created.'; END $$;

COMMIT;
