-- DATs list a game code ("serial") for cartridge systems (GBA, NDS, N64), which
-- lets a dump no DAT matches by CRC (a scene release, a hacked header) be named
-- from the code in its own header. Kept per entry, looked up by system + serial.
ALTER TABLE dat_entries ADD COLUMN serial TEXT;
CREATE INDEX idx_dat_entries_serial ON dat_entries (system_id, serial);

-- DATs imported so far were stored without serials. Forget the version of the
-- downloaded No-Intro ones so the next "Update DATs" fetches and imports them
-- again (an unchanged remote version would skip the download).
UPDATE dats SET remote_version = NULL WHERE remote_source = 'no-intro';
