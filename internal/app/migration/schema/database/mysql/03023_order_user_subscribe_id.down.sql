ALTER TABLE `order`
DROP INDEX `idx_order_user_subscribe_id`,
DROP COLUMN `user_subscribe_id`;
