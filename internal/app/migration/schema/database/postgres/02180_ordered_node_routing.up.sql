-- Additive schema only; offline cutover writes v1 and preserves legacy material.
ALTER TABLE "server_config_overrides"
    ADD COLUMN "routing" TEXT NULL,
    ADD COLUMN "routing_revision" BIGINT NOT NULL DEFAULT 0;

INSERT INTO "system" ("category", "key", "value", "type", "desc", "created_at", "updated_at")
VALUES ('server', 'Routing', '', 'string', 'Ordered routing policy, empty means legacy', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
       ('server', 'RoutingRevision', '0', 'int64', 'Global routing revision', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
       ('server', 'RoutingCutover', 'pending', 'string', 'Offline ordered routing cutover status', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT DO NOTHING;
