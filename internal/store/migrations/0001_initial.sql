-- Schema v1 (spec M0): files, archive_members, dat_entries, games,
-- game_groups, rules, overrides, link_state, transcode_cache.
-- `dats` is a supporting table giving dat_entries a provenance parent
-- (which DAT file/version each entry came from).

CREATE TABLE dats (
    id               INTEGER PRIMARY KEY,
    system_id        TEXT NOT NULL,
    format           TEXT NOT NULL CHECK (format IN ('logiqx_xml', 'clrmamepro')),
    source_path      TEXT NOT NULL,
    has_parent_clone INTEGER NOT NULL DEFAULT 0,
    imported_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- Scanned source files (raw ROMs or archives). Hash cache keyed by
-- (path, size, mtime); crc32/sha1/md5 are filled in as they're computed.
CREATE TABLE files (
    id              INTEGER PRIMARY KEY,
    path            TEXT NOT NULL UNIQUE,
    size            INTEGER NOT NULL,
    mtime_unix      INTEGER NOT NULL,
    kind            TEXT NOT NULL CHECK (kind IN ('raw', 'zip', 'sevenzip')),
    crc32           TEXT,
    sha1            TEXT,
    md5             TEXT,
    last_scanned_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- Archive member headers read without decompression (spec §2.8).
CREATE TABLE archive_members (
    id             INTEGER PRIMARY KEY,
    file_id        INTEGER NOT NULL REFERENCES files (id) ON DELETE CASCADE,
    member_name    TEXT NOT NULL,
    size           INTEGER NOT NULL,
    crc32          TEXT NOT NULL,
    sha1           TEXT,
    is_only_member INTEGER NOT NULL DEFAULT 0,
    UNIQUE (file_id, member_name)
);

-- Parsed DAT catalog entries (Logiqx XML / ClrMamePro).
CREATE TABLE dat_entries (
    id        INTEGER PRIMARY KEY,
    dat_id    INTEGER NOT NULL REFERENCES dats (id) ON DELETE CASCADE,
    system_id TEXT NOT NULL,
    game_name TEXT NOT NULL,
    rom_name  TEXT NOT NULL,
    size      INTEGER NOT NULL,
    crc32     TEXT,
    sha1      TEXT,
    md5       TEXT,
    clone_of  TEXT,
    status    TEXT
);

CREATE INDEX idx_dat_entries_crc32 ON dat_entries (crc32);
CREATE INDEX idx_dat_entries_sha1 ON dat_entries (sha1);

-- 1G1R groups: one row per logical game, independent of which variant is
-- picked. grouping_source records which of the three methods in spec §2.9
-- established the group.
CREATE TABLE game_groups (
    id                  INTEGER PRIMARY KEY,
    system_id           TEXT NOT NULL,
    group_key           TEXT NOT NULL,
    grouping_source     TEXT NOT NULL CHECK (grouping_source IN ('dat_pc', 'clone_list', 'title_normalization', 'none')),
    display_name        TEXT NOT NULL,
    is_manual_override  INTEGER NOT NULL DEFAULT 0,
    excluded            INTEGER NOT NULL DEFAULT 0,
    exclusion_reason    TEXT,
    created_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE (system_id, group_key)
);

-- Canonical games: candidate (and selected) variants within a group.
-- ultimate_source_* mirrors link_state's source columns (spec §2.1, §2.7)
-- so a profile can link straight to it. game_key is the stable identity
-- from spec §2.11 and must survive DAT renames.
CREATE TABLE games (
    id                   INTEGER PRIMARY KEY,
    group_id             INTEGER NOT NULL REFERENCES game_groups (id) ON DELETE CASCADE,
    system_id            TEXT NOT NULL,
    game_key             TEXT NOT NULL,
    -- SET NULL, not the default RESTRICT: identity (game_key) is
    -- content-hash based, not tied to any of these rows, so losing the
    -- link when a DAT is re-imported (dat_entries cascades from dats) or
    -- an archive is re-scanned (archive_members is replaced wholesale by
    -- ReplaceArchiveMembers) must not block that deletion.
    dat_entry_id         INTEGER REFERENCES dat_entries (id) ON DELETE SET NULL,
    file_id              INTEGER REFERENCES files (id) ON DELETE SET NULL,
    archive_member_id    INTEGER REFERENCES archive_members (id) ON DELETE SET NULL,
    confidence           TEXT NOT NULL CHECK (confidence IN ('dat_verified', 'heuristic', 'unidentified')),
    region               TEXT,
    language             TEXT,
    revision             TEXT,
    is_verified_dump     INTEGER NOT NULL DEFAULT 0,
    is_beta_proto_demo   INTEGER NOT NULL DEFAULT 0,
    is_selected          INTEGER NOT NULL DEFAULT 0,
    ultimate_source_kind TEXT NOT NULL CHECK (ultimate_source_kind IN ('raw_file', 'archive', 'archive_member_extracted', 'transcode_cache')),
    ultimate_source_ref  TEXT NOT NULL,
    created_at           TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE (system_id, game_key)
);

CREATE INDEX idx_games_group_id ON games (group_id);

-- User-defined 1G1R preference rules (region/language priority order,
-- exclusion toggles). NULL system_id means the rule applies globally.
CREATE TABLE rules (
    id        INTEGER PRIMARY KEY,
    system_id TEXT,
    kind      TEXT NOT NULL CHECK (kind IN ('region_priority', 'language_priority', 'exclude_beta', 'exclude_proto', 'exclude_demo', 'require_verified')),
    value     TEXT,
    priority  INTEGER NOT NULL DEFAULT 0
);

-- Manual overrides on canonical game records (spec §2.1, §2.9). Keyed by
-- group_id + the overriding game_key so the override applies to all
-- profiles at once and survives DAT updates.
CREATE TABLE overrides (
    id                INTEGER PRIMARY KEY,
    group_id          INTEGER NOT NULL REFERENCES game_groups (id) ON DELETE CASCADE,
    override_game_key TEXT NOT NULL,
    reason            TEXT,
    created_at        TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- Per-profile link state (spec §2.7 — schema reproduced verbatim).
CREATE TABLE link_state (
    id                  INTEGER PRIMARY KEY,
    profile_id          TEXT NOT NULL,
    system_id           TEXT NOT NULL,
    game_key            TEXT NOT NULL,
    target_path         TEXT NOT NULL,
    source_kind         TEXT NOT NULL CHECK (source_kind IN ('raw_file', 'archive', 'archive_member_extracted', 'transcode_cache')),
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

CREATE INDEX idx_link_state_profile ON link_state (profile_id);
CREATE INDEX idx_link_state_game_key ON link_state (game_key);

-- Content-addressed transcode cache (spec §2.10). cache_key is derived from
-- content_hash + format + tool_version + codec_flags so a re-zipped source
-- or a codec change never collide on the same cache entry.
CREATE TABLE transcode_cache (
    id           INTEGER PRIMARY KEY,
    cache_key    TEXT NOT NULL UNIQUE,
    content_hash TEXT NOT NULL,
    format       TEXT NOT NULL CHECK (format IN ('chd', 'rvz', 'cso')),
    tool_name    TEXT NOT NULL,
    tool_version TEXT NOT NULL,
    codec_flags  TEXT NOT NULL,
    cache_path   TEXT NOT NULL,
    verified     INTEGER NOT NULL DEFAULT 0,
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
