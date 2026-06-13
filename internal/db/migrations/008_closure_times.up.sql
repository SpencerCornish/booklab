ALTER TABLE closures
  ADD COLUMN all_day    BOOLEAN NOT NULL DEFAULT TRUE,
  ADD COLUMN start_time TIME,
  ADD COLUMN end_time   TIME,
  ADD CONSTRAINT closures_partial_day_check
    CHECK (all_day OR (start_time IS NOT NULL AND end_time IS NOT NULL AND end_time > start_time));
