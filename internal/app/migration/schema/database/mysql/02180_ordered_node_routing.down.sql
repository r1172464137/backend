-- DESTRUCTIVE OPERATOR-ONLY DOWN. Stop all writers before invoking this SQL.
-- Rollback restores the pre-upgrade database backup and original three binaries; do not use down for normal rollback.
-- MySQL DDL commits implicitly. Guard every precondition before the FIRST DDL;
-- the intentionally invalid statement causes PREPARE to fail when unsafe.
SET @ppanel_routing_down_guard = IF(
    EXISTS(SELECT 1 FROM `system` WHERE `key` = 'Routing' AND OCTET_LENGTH(`value`) <> 0)
    OR EXISTS(SELECT 1 FROM `server_config_overrides` WHERE `routing` IS NOT NULL)
    OR EXISTS(SELECT 1 FROM `system` WHERE OCTET_LENGTH(`value`) > 65535),
    'ORDERED ROUTING DOWN PRECONDITIONS FAILED',
    'SELECT 1'
);
PREPARE ppanel_routing_down_check FROM @ppanel_routing_down_guard;
EXECUTE ppanel_routing_down_check;
DEALLOCATE PREPARE ppanel_routing_down_check;

ALTER TABLE `system`
    MODIFY COLUMN `value` TEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT 'Key Value';
ALTER TABLE `server_config_overrides`
    DROP COLUMN `routing_revision`,
    DROP COLUMN `routing`;
DELETE FROM `system` WHERE `category` = 'server' AND `key` IN ('Routing', 'RoutingRevision', 'RoutingCutover');
