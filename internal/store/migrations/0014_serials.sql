-- Serial -> game name lists (libretro-database), downloaded with the DATs, for
-- naming PlayStation / PlayStation 2 discs that no DAT matched.
CREATE TABLE serials (
    system_id TEXT NOT NULL,
    serial    TEXT NOT NULL,
    name      TEXT NOT NULL,
    PRIMARY KEY (system_id, serial)
);

CREATE TABLE serial_lists (
    system_id  TEXT PRIMARY KEY,
    version    TEXT NOT NULL,
    fetched_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
