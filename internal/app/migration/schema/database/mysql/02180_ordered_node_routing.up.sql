-- Additive schema only; offline cutover writes v1 and preserves legacy material.
ALTER TABLE `system`
    MODIFY COLUMN `value` MEDIUMTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT 'Key Value';

ALTER TABLE `server_config_overrides`
    ADD COLUMN `routing` MEDIUMTEXT NULL COMMENT 'Ordered routing policy, NULL means inherit',
    ADD COLUMN `routing_revision` BIGINT NOT NULL DEFAULT 0 COMMENT 'Local routing transition revision';

INSERT IGNORE INTO `system` (`category`, `key`, `value`, `type`, `desc`, `created_at`, `updated_at`)
VALUES ('server', 'Routing', '', 'string', 'Ordered routing policy, empty means legacy', CURRENT_TIMESTAMP(3), CURRENT_TIMESTAMP(3)),
       ('server', 'RoutingRevision', '0', 'int64', 'Global routing revision', CURRENT_TIMESTAMP(3), CURRENT_TIMESTAMP(3)),
       ('server', 'RoutingCutover', 'pending', 'string', 'Offline ordered routing cutover status', CURRENT_TIMESTAMP(3), CURRENT_TIMESTAMP(3));
