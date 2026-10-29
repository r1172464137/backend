-- DESTRUCTIVE OPERATOR-ONLY DOWN. Stop all writers before invoking this SQL.
-- Rollback restores the pre-upgrade database backup and original three binaries; do not use down for normal rollback.
-- PostgreSQL's system.value remains TEXT, so no value column is narrowed.
DO $guard$
BEGIN
    IF EXISTS (SELECT 1 FROM "system" WHERE "key" = 'Routing' AND "value" <> '')
       OR EXISTS (SELECT 1 FROM "server_config_overrides" WHERE "routing" IS NOT NULL) THEN
        RAISE EXCEPTION 'ordered routing down preconditions failed: v1 policy is still active';
    END IF;
END
$guard$;

ALTER TABLE "server_config_overrides"
    DROP COLUMN "routing_revision",
    DROP COLUMN "routing";
DELETE FROM "system" WHERE "category" = 'server' AND "key" IN ('Routing', 'RoutingRevision', 'RoutingCutover');
