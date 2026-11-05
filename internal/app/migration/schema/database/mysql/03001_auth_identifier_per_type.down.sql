-- Fails once the same identifier exists under two auth types (for example a
-- device named like an email address); remove the duplicates before rolling
-- back.
ALTER TABLE `user_auth_methods`
    DROP INDEX `idx_auth_type_identifier`,
    ADD UNIQUE KEY `idx_auth_identifier` (`auth_identifier`);
