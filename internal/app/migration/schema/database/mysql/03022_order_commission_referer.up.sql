-- The referrer whose wallet received the order's commission when it settled.
-- A refund takes the commission back from that referrer, not from whoever
-- refers the buyer by then; zero on orders settled before it was recorded.
ALTER TABLE `order`
ADD COLUMN `commission_referer_id` BIGINT NOT NULL DEFAULT 0
  COMMENT 'Referrer credited with the commission' AFTER `commission`;
