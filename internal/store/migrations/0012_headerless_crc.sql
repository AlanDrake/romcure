-- Index the columns of games that point at other tables. Without them every
-- deleted file, archive member or DAT entry made SQLite scan all games to clear
-- its reference: the DELETE below took a minute on a 14,000-game library (and
-- replacing a DAT was slow the same way).
CREATE INDEX IF NOT EXISTS idx_games_file_id ON games (file_id);
CREATE INDEX IF NOT EXISTS idx_games_archive_member_id ON games (archive_member_id);
CREATE INDEX IF NOT EXISTS idx_games_dat_entry_id ON games (dat_entry_id);

-- Cartridge dumps with a copier header (NES, FDS, Atari 7800, Lynx, SNES) are
-- listed in No-Intro DATs by the CRC of the data after the header. The scanner
-- now also records that CRC (NULL when the file has no header).
ALTER TABLE files ADD COLUMN alt_crc32 TEXT;
ALTER TABLE files ADD COLUMN alt_size INTEGER;
ALTER TABLE archive_members ADD COLUMN alt_crc32 TEXT;
ALTER TABLE archive_members ADD COLUMN alt_size INTEGER;

-- Forget the cached scan of files that may carry such a header (raw, or an
-- archive holding one), so the next scan reads them again. Their members cascade.
DELETE FROM files
WHERE lower(path) LIKE '%.nes' OR lower(path) LIKE '%.fds' OR lower(path) LIKE '%.a78'
   OR lower(path) LIKE '%.lnx' OR lower(path) LIKE '%.smc' OR lower(path) LIKE '%.sfc'
   OR lower(path) LIKE '%.swc' OR lower(path) LIKE '%.fig'
   OR id IN (SELECT file_id FROM archive_members
             WHERE lower(member_name) LIKE '%.nes' OR lower(member_name) LIKE '%.fds'
                OR lower(member_name) LIKE '%.a78' OR lower(member_name) LIKE '%.lnx'
                OR lower(member_name) LIKE '%.smc' OR lower(member_name) LIKE '%.sfc'
                OR lower(member_name) LIKE '%.swc' OR lower(member_name) LIKE '%.fig');
