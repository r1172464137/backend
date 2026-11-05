-- The user subscription a renewal or traffic-reset order applies to, by its
-- id. The token the order also carries is rotated by the user or an
-- administrator, after which a token lookup no longer finds the subscription
-- and a paid order can never be fulfilled; the id survives the rotation.
-- Zero on orders created before it was recorded, which fall back to the token.
ALTER TABLE "order"
ADD COLUMN "user_subscribe_id" BIGINT NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS "idx_order_user_subscribe_id" ON "order" ("user_subscribe_id");
