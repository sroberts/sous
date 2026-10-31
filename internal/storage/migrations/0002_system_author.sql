-- Sous itself becomes an author: people.source and changes.via gain
-- 'system', for the sample recipes a new library starts with. SQLite can't
-- alter a CHECK, so both tables are rebuilt (sqlite.org/lang_altertable.html,
-- "making other kinds of table schema changes"). The runner turns foreign
-- keys off around this and checks them before committing. Columns, rows and
-- changes.seq are kept exactly; the Sous person itself is added by the
-- seeding, not here.

CREATE TABLE people_new (
  username   TEXT PRIMARY KEY,
  name       TEXT NOT NULL,
  email      TEXT,
  source     TEXT NOT NULL CHECK (source IN ('local','gateway','system')),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
) WITHOUT ROWID;
INSERT INTO people_new (username, name, email, source, created_at, updated_at)
  SELECT username, name, email, source, created_at, updated_at FROM people;
DROP TABLE people;
ALTER TABLE people_new RENAME TO people;

CREATE TABLE changes_new (
  seq       INTEGER PRIMARY KEY,
  id        TEXT NOT NULL UNIQUE,
  at        TEXT NOT NULL,
  person_id TEXT REFERENCES people(username),
  via       TEXT NOT NULL CHECK (via IN ('ui','api','mcp','system')),
  entity    TEXT NOT NULL,
  entity_id TEXT NOT NULL,
  action    TEXT NOT NULL CHECK (action IN ('create','update','delete','restore')),
  summary   TEXT NOT NULL
);
INSERT INTO changes_new (seq, id, at, person_id, via, entity, entity_id, action, summary)
  SELECT seq, id, at, person_id, via, entity, entity_id, action, summary FROM changes;
DROP TABLE changes;
ALTER TABLE changes_new RENAME TO changes;
CREATE INDEX changes_by_entity ON changes(entity_id, seq);
CREATE INDEX changes_by_person ON changes(person_id, seq);
CREATE INDEX changes_by_via    ON changes(via, seq);
CREATE TRIGGER changes_no_update BEFORE UPDATE ON changes BEGIN
  SELECT RAISE(ABORT, 'changes is append-only');
END;
CREATE TRIGGER changes_no_delete BEFORE DELETE ON changes BEGIN
  SELECT RAISE(ABORT, 'changes is append-only');
END;
