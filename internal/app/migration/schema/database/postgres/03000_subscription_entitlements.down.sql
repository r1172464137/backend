-- Only roll back after provider-managed subscriptions have been retired.
DROP TABLE subscription_entitlement_revision;
DROP TABLE subscription_period;
DROP TABLE subscription_entitlement;
ALTER TABLE user_subscribe DROP COLUMN entitlement_source;
