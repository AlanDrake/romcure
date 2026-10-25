-- GameCube/Wii discs carry their game ID and title in the disc header; the
-- scanner records them so a disc no DAT knows can be named properly. NULL = not
-- looked at yet (the next scan reads just the header, no rehash); '' = no such
-- header (another console's ISO).
ALTER TABLE files ADD COLUMN disc_id TEXT;
ALTER TABLE files ADD COLUMN disc_title TEXT;
ALTER TABLE archive_members ADD COLUMN disc_id TEXT;
ALTER TABLE archive_members ADD COLUMN disc_title TEXT;
-- The disc number (1 for the first disc), so the discs of one game are told apart.
ALTER TABLE files ADD COLUMN disc_number INTEGER;
ALTER TABLE archive_members ADD COLUMN disc_number INTEGER;
