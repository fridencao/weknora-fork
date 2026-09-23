-- Migration 000111 down: 移除批一搬运的运维取值行。
--
-- 只删由本迁移写入、且此后未被管理员改动过的行（last_modified_by 仍为
-- 'migration-000111'）。若管理员在界面上改过这个键，说明它已是人工维护的
-- 配置，回滚迁移不应把它一起抹掉。
DELETE FROM system_settings
WHERE key = 'graph.channel.timeout_s'
  AND last_modified_by = 'migration-000111';

DO $$ BEGIN RAISE NOTICE '[Migration 000111] Rolled back.'; END $$;
