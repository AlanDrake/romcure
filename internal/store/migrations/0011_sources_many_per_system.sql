-- A system may have several source folders: drop the UNIQUE on system_id. SQLite
-- cannot drop a constraint, so the table is rebuilt with its rows (nothing
-- references it).
CREATE TABLE sources_new (
    id        INTEGER PRIMARY KEY,
    path      TEXT NOT NULL UNIQUE,
    system_id TEXT NOT NULL
);

INSERT INTO sources_new (id, path, system_id) SELECT id, path, system_id FROM sources;

DROP TABLE sources;
ALTER TABLE sources_new RENAME TO sources;
