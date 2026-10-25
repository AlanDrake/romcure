-- Per-output game selection (docs/game-selection-plan.md, ADR 0001).
-- A game's state on an output is the first that exists of: the user's tick on
-- the game, the user's tick on its system, the output's default.
ALTER TABLE profiles ADD COLUMN include_by_default INTEGER NOT NULL DEFAULT 1;

CREATE TABLE output_systems (
    profile_id INTEGER NOT NULL REFERENCES profiles (id) ON DELETE CASCADE,
    system_id  TEXT NOT NULL,
    included   INTEGER NOT NULL,
    PRIMARY KEY (profile_id, system_id)
);

-- Stored per variant game_key (content based, spec §2.11), so a tick survives
-- DAT updates that rename or regroup games.
CREATE TABLE output_games (
    profile_id INTEGER NOT NULL REFERENCES profiles (id) ON DELETE CASCADE,
    system_id  TEXT NOT NULL,
    game_key   TEXT NOT NULL,
    included   INTEGER NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    PRIMARY KEY (profile_id, system_id, game_key)
);

-- An output that listed systems took only those: default exclude plus one
-- included row per listed system. profiles.systems is no longer read.
UPDATE profiles SET include_by_default = 0 WHERE systems != '';
WITH RECURSIVE split(pid, item, rest) AS (
    SELECT id, '', systems || ',' FROM profiles WHERE systems != ''
    UNION ALL
    SELECT pid, substr(rest, 1, instr(rest, ',') - 1), substr(rest, instr(rest, ',') + 1) FROM split WHERE rest != ''
)
INSERT OR IGNORE INTO output_systems (profile_id, system_id, included)
SELECT pid, item, 1 FROM split WHERE item != '';
