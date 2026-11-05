-- Provider state and absolute entitlement periods. Existing rows remain local.
ALTER TABLE user_subscribe ADD COLUMN entitlement_source VARCHAR(32) NOT NULL DEFAULT '';
CREATE TABLE subscription_entitlement (
    id CHAR(64) PRIMARY KEY,
    source VARCHAR(32) NOT NULL,
    scope VARCHAR(255) NOT NULL,
    subscription_key VARCHAR(255) NOT NULL,
    user_id BIGINT NOT NULL,
    user_subscribe_id BIGINT NOT NULL UNIQUE,
    revision BIGINT NOT NULL,
    fingerprint CHAR(64) NOT NULL,
    period_id CHAR(64) NOT NULL,
    mode VARCHAR(32) NOT NULL,
    status VARCHAR(32) NOT NULL,
    auto_renew BOOLEAN NOT NULL DEFAULT FALSE,
    grace_until TIMESTAMPTZ NULL,
    revoked_at TIMESTAMPTZ NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE subscription_period (
    id CHAR(64) PRIMARY KEY,
    entitlement_id CHAR(64) NOT NULL,
    transaction_key VARCHAR(255) NOT NULL,
    user_subscribe_id BIGINT NOT NULL,
    order_id BIGINT NOT NULL,
    plan_id BIGINT NOT NULL,
    billing_interval VARCHAR(16) NOT NULL DEFAULT '',
    traffic_limit BIGINT NOT NULL,
    start_at TIMESTAMPTZ NOT NULL,
    end_at TIMESTAMPTZ NOT NULL,
    traffic_reset BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_subscription_period_entitlement_id ON subscription_period (entitlement_id);
CREATE INDEX idx_subscription_period_user_subscribe_id ON subscription_period (user_subscribe_id);
CREATE TABLE subscription_entitlement_revision (
    id VARCHAR(96) PRIMARY KEY,
    entitlement_id CHAR(64) NOT NULL,
    payload TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_entitlement_revision_entitlement_id ON subscription_entitlement_revision (entitlement_id);
