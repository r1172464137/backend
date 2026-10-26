-- DESTRUCTIVE MANUAL DOWN: export or clear all structured route rules first.
-- Production startup only migrates up; it never invokes this rollback automatically.
DELETE FROM `system`
WHERE `category` = 'server'
  AND `key` IN ('BlockIPs', 'BlockProtocols');

ALTER TABLE `server_config_overrides`
    DROP COLUMN `block_protocols`,
    DROP COLUMN `block_ips`;
