ALTER TABLE settings
  ADD COLUMN new_customer_block_start   DATE,
  ADD COLUMN new_customer_block_end     DATE,
  ADD COLUMN new_customer_block_message TEXT NOT NULL DEFAULT '';
