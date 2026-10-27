ALTER TABLE `server_config_overrides`
    ADD COLUMN `block_ips` TEXT NULL COMMENT 'IP block override, NULL means inherit' AFTER `block`,
    ADD COLUMN `block_protocols` TEXT NULL COMMENT 'Protocol block override, NULL means inherit' AFTER `block_ips`;

INSERT IGNORE INTO `system` (`category`, `key`, `value`, `type`, `desc`, `created_at`, `updated_at`)
VALUES ('server', 'BlockIPs', '[]', 'string', 'IP block rules', CURRENT_TIMESTAMP(3), CURRENT_TIMESTAMP(3)),
       ('server', 'BlockProtocols', '[]', 'string', 'Protocol block rules', CURRENT_TIMESTAMP(3), CURRENT_TIMESTAMP(3));
