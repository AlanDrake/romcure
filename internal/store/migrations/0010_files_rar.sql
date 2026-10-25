-- migrate: foreign_keys=off
-- files.kind gains 'rar'. SQLite cannot change a CHECK constraint, so the table
-- is rebuilt with its rows. Other tables reference files (id): the runner turns
-- foreign key enforcement off for this migration, so dropping the old table
-- does not cascade into them, and checks them again before committing.
CREATE TABLE files_new (
    id              INTEGER PRIMARY KEY,
    path            TEXT NOT NULL UNIQUE,
    size            INTEGER NOT NULL,
    mtime_unix      INTEGER NOT NULL,
    kind            TEXT NOT NULL CHECK (kind IN ('raw', 'zip', 'sevenzip', 'rar')),
    crc32           TEXT,
    sha1            TEXT,
    md5             TEXT,
    last_scanned_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

INSERT INTO files_new (id, path, size, mtime_unix, kind, crc32, sha1, md5, last_scanned_at)
SELECT id, path, size, mtime_unix, kind, crc32, sha1, md5, last_scanned_at FROM files;

DROP TABLE files;
ALTER TABLE files_new RENAME TO files;
