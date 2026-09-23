-- Migration 000028: 配置治理批一 —— 把运维在环境变量里配置过的取值搬进 system_settings。
-- 与 versioned/000111 同一迁移的 SQLite 版本（value 为 TEXT，存 JSON 字面量）。
--
-- 搬运清单（详见 versioned/000111 的完整说明）：
--   graph.channel.timeout_s = 45   （.env 的 GRAPH_CHANNEL_TIMEOUT_S=45，内置默认 8）
-- 其余 16 项批一键的运维取值与内置默认一致，无需 seed。
INSERT INTO system_settings (key, value, value_type, category, description, last_modified_by)
VALUES (
    'graph.channel.timeout_s',
    '45',
    'int',
    'retrieval',
    '图谱通道软超时（秒），取值 1–120。检索主链路不应被图谱抖动拖死；思考型 LLM 做关键词抽取时可放大。',
    'migration-000028'
)
ON CONFLICT (key) DO NOTHING;
