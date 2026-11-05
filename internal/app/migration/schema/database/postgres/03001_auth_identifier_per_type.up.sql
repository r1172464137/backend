-- An identifier is unique within its auth type, not across types. A single
-- global key let any client register a device named after someone else's
-- email address or Telegram id and so block that owner from ever registering
-- or binding it. Existing rows are unique globally, hence also per type.
ALTER TABLE "user_auth_methods" DROP CONSTRAINT "idx_auth_identifier";
ALTER TABLE "user_auth_methods" ADD CONSTRAINT "idx_auth_type_identifier" UNIQUE ("auth_type", "auth_identifier");
