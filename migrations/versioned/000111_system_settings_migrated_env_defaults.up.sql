-- Migration 000111: 配置治理批一 —— 把运维在环境变量里配置过的取值搬进 system_settings。
--
-- 背景：批一把 17 个业务可调项从环境变量迁到系统设置（DB > ENV > 内置默认），
-- 并从 docker-compose.yml / .env 移除对应变量。移除 ENV 来源后，若不同步搬运
-- 运维取值，行为会静默回退到内置默认——所以这里必须 seed。
--
-- 为什么这次要 seed，而 000053 当时刻意不 seed：
--   000053 的顾虑是「插入内置默认值会盖住运维已有的 ENV 配置」（因为 DB 行优先
--   级高于 ENV）。本次方向正好相反：我们要删掉 ENV 来源，DB 行是运维配置的
--   唯一去处。因此这里**只搬运运维实际配置过、且与内置默认不同的键**，
--   不是内置默认的副本——内置默认仍由 service.registry 提供。
--
-- 搬运清单（逐项核对过 compose 默认值、.env 与运行容器实际 env）：
--   graph.channel.timeout_s = 45
--     .env 里 GRAPH_CHANNEL_TIMEOUT_S=45（内置默认 8）。这是唯一一项运维取值
--     与内置默认不同的键；其余 16 项的运维取值与内置默认一致，无需 seed。
--     注：该 .env 行是在运行中的 app 容器创建之后才加的，所以当时容器内并无此
--     变量、实际跑的是 8。此处按运维的**配置意图**搬运 45，同时把 .env 行清掉，
--     避免"环境变量看着配了、实际没生效"这种更糟的状态。
--
-- 幂等：ON CONFLICT DO NOTHING，重复执行（或运维已手工设过该键）都不会覆盖。
INSERT INTO system_settings (key, value, value_type, category, description, last_modified_by)
VALUES (
    'graph.channel.timeout_s',
    '45'::jsonb,
    'int',
    'retrieval',
    '图谱通道软超时（秒），取值 1–120。检索主链路不应被图谱抖动拖死；思考型 LLM 做关键词抽取时可放大。',
    'migration-000111'
)
ON CONFLICT (key) DO NOTHING;

DO $$ BEGIN RAISE NOTICE '[Migration 000111] Done.'; END $$;
