-- Which archive member was matched, for games whose ultimate_source_kind
-- is 'archive' — ultimate_source_ref is the archive's own path, which
-- doesn't say which entry inside it this game is. NULL for raw files.
ALTER TABLE games ADD COLUMN member_name TEXT;
