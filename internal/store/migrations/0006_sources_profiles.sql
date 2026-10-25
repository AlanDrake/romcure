-- User configuration that the UI edits: where the ROM collections are
-- (sources) and which outputs to generate from them (profiles).
CREATE TABLE sources (
    id        INTEGER PRIMARY KEY,
    path      TEXT NOT NULL UNIQUE,
    system_id TEXT NOT NULL UNIQUE
);

CREATE TABLE profiles (
    id          INTEGER PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    kind        TEXT NOT NULL CHECK (kind IN ('retroarch', 'esde', 'retrodeck', 'batocera', 'retrobat', 'raw', 'device')),
    output_root TEXT NOT NULL,
    -- comma-separated system ids; "" means every system that has a source
    systems     TEXT NOT NULL DEFAULT ''
);
