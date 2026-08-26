ALTER TABLE settings
  ADD COLUMN booking_screening JSONB DEFAULT NULL;

CREATE TABLE interest_submissions (
  id SERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  email TEXT NOT NULL,
  phone TEXT,
  message TEXT,
  selected_option TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
