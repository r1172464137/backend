-- An identifier is unique within its auth type, not across types. A single
-- global key let any client register a device named after someone else's
-- email address or Telegram id and so block that owner from ever registering
-- or binding it. Existing rows are unique globally, hence also per type.
ALTER TABLE `user_auth_methods`
    DROP INDEX `idx_auth_identifier`,
    ADD UNIQUE KEY `idx_auth_type_identifier` (`auth_type`, `auth_identifier`);
