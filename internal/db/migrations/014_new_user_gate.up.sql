-- Optional pre-schedule question that routes self-reported new customers to
-- trainer-only hours. The schedule is a JSON array of
-- {"weekday":0-6,"start":"HH:MM","end":"HH:MM"} entries (0 = Sunday).
ALTER TABLE settings
  ADD COLUMN new_user_gate_enabled  BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN new_user_gate_question TEXT    NOT NULL DEFAULT 'Have you used this space before?',
  ADD COLUMN new_user_gate_schedule JSONB   NOT NULL DEFAULT '[]'::jsonb,
  ADD CONSTRAINT new_user_gate_schedule_is_array
    CHECK (jsonb_typeof(new_user_gate_schedule) = 'array');

-- NULL = the gate was disabled or not asked; true/false = the customer's answer.
ALTER TABLE bookings
  ADD COLUMN is_new_user BOOLEAN;
