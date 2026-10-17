ALTER TABLE settings
  ADD COLUMN min_lead_hours INT NOT NULL DEFAULT 0 CHECK (min_lead_hours >= 0);
