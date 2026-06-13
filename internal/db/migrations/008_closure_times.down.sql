ALTER TABLE closures
  DROP CONSTRAINT closures_partial_day_check,
  DROP COLUMN end_time,
  DROP COLUMN start_time,
  DROP COLUMN all_day;
