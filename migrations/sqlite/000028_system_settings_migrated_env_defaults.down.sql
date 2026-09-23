-- Migration 000028 down: 移除批一搬运的运维取值行（SQLite）。
-- 只删由本迁移写入、且此后未被管理员改动过的行。
DELETE FROM system_settings
WHERE key = 'graph.channel.timeout_s'
  AND last_modified_by = 'migration-000028';
