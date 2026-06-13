ALTER TABLE settings
  ADD COLUMN terms_content   TEXT NOT NULL DEFAULT '',
  ADD COLUMN privacy_content TEXT NOT NULL DEFAULT '';
