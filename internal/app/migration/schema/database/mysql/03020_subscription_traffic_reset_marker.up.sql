-- Done-marker of the calendar traffic reset: the start of the day a
-- subscription was last reset. A retried reset run skips the subscriptions
-- already reset for the day instead of clearing their traffic again.
ALTER TABLE `user_subscribe`
ADD COLUMN `traffic_reset_at` DATETIME(3) NULL DEFAULT NULL
  COMMENT 'Last Calendar Traffic Reset Day';
