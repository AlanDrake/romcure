-- link_state.source_kind gains 'generated' (a file the app wrote itself, such as
-- a multi-disc .m3u, with no source file) and 'sbi' (LibCrypt subchannel files,
-- which were recorded as 'raw_file'). SQLite cannot change a CHECK constraint,
-- so the table is rebuilt with its rows.
CREATE TABLE link_state_new (
    id                  INTEGER PRIMARY KEY,
    profile_id          TEXT NOT NULL,
    system_id           TEXT NOT NULL,
    game_key            TEXT NOT NULL,
    target_path         TEXT NOT NULL,
    source_kind         TEXT NOT NULL CHECK (source_kind IN ('raw_file', 'archive', 'archive_member_extracted', 'transcode_cache', 'sbi', 'generated')),
    source_path         TEXT NOT NULL,
    source_hash         TEXT NOT NULL,
    link_type           TEXT NOT NULL CHECK (link_type IN ('symlink_rel', 'symlink_abs', 'reflink', 'hardlink', 'copy')),
    is_manual_override  INTEGER NOT NULL DEFAULT 0,
    disc_index          INTEGER,
    link_status         TEXT NOT NULL CHECK (link_status IN ('ok', 'broken', 'stale', 'pending')),
    created_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    last_verified_at    TEXT,
    UNIQUE (profile_id, target_path)
);

INSERT INTO link_state_new (id, profile_id, system_id, game_key, target_path, source_kind, source_path, source_hash,
                            link_type, is_manual_override, disc_index, link_status, created_at, last_verified_at)
SELECT id, profile_id, system_id, game_key, target_path, source_kind, source_path, source_hash,
       link_type, is_manual_override, disc_index, link_status, created_at, last_verified_at
FROM link_state;

DROP TABLE link_state;
ALTER TABLE link_state_new RENAME TO link_state;

CREATE INDEX idx_link_state_profile ON link_state (profile_id);
CREATE INDEX idx_link_state_game_key ON link_state (game_key);
