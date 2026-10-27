ALTER TABLE "server_config_overrides"
    ADD COLUMN "block_ips" TEXT NULL,
    ADD COLUMN "block_protocols" TEXT NULL;

INSERT INTO "system" ("category", "key", "value", "type", "desc", "created_at", "updated_at")
VALUES ('server', 'BlockIPs', '[]', 'string', 'IP block rules', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
       ('server', 'BlockProtocols', '[]', 'string', 'Protocol block rules', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT DO NOTHING;
