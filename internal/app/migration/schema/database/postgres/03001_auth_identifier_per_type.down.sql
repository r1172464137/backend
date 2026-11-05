-- Fails once the same identifier exists under two auth types (for example a
-- device named like an email address); remove the duplicates before rolling
-- back.
ALTER TABLE "user_auth_methods" DROP CONSTRAINT "idx_auth_type_identifier";
ALTER TABLE "user_auth_methods" ADD CONSTRAINT "idx_auth_identifier" UNIQUE ("auth_identifier");
