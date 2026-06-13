ALTER TABLE settings
  ADD COLUMN referral_sources TEXT[] NOT NULL DEFAULT '{}';
