-- Track hashes of a CHD, computed by extracting it (chdman extractcd/dvd)
-- because a CHD header only holds whole-disc hashes, which Redump doesn't
-- use. size/mtime record which version of the CHD the hashes describe, so a
-- changed file is treated as unverified until re-checked.
CREATE TABLE chd_checked (
    file_id    INTEGER PRIMARY KEY REFERENCES files (id) ON DELETE CASCADE,
    size       INTEGER NOT NULL,
    mtime_unix INTEGER NOT NULL,
    checked_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE chd_tracks (
    file_id INTEGER NOT NULL REFERENCES files (id) ON DELETE CASCADE,
    ordinal INTEGER NOT NULL,
    name    TEXT NOT NULL,
    size    INTEGER NOT NULL,
    crc32   TEXT NOT NULL,
    sha1    TEXT NOT NULL,
    md5     TEXT NOT NULL,
    PRIMARY KEY (file_id, ordinal)
);
