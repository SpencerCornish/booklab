ALTER TABLE bookings
  DROP COLUMN is_new_user;

ALTER TABLE settings
  DROP CONSTRAINT new_user_gate_schedule_is_array,
  DROP COLUMN new_user_gate_schedule,
  DROP COLUMN new_user_gate_question,
  DROP COLUMN new_user_gate_enabled;
