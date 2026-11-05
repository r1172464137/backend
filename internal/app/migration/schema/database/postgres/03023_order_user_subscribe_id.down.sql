DROP INDEX IF EXISTS "idx_order_user_subscribe_id";
ALTER TABLE "order"
DROP COLUMN "user_subscribe_id";
