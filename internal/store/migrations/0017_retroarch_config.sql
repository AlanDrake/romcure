-- The retroarch.cfg a RetroArch output writes its playlists for. Empty = none
-- chosen (every other kind of output, and RetroArch outputs made before this).
ALTER TABLE profiles ADD COLUMN retroarch_config TEXT NOT NULL DEFAULT '';
