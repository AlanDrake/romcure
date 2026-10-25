-- The compression method of a zip's members that Go's zip reader (and RetroArch)
-- cannot decompress, recorded by the scan so linking a zipped game never has to
-- open the zip again. NULL = not looked at yet (the next scan reads just the
-- central directory, no rehash); '' = every member is stored or deflated;
-- otherwise the name of the first old method ("implode", "shrink"...).
ALTER TABLE files ADD COLUMN zip_method TEXT;
