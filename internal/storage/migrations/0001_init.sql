-- Times are stored fixed-width as 2006-01-02T15:04:05.000Z so they sort as text.

CREATE TABLE people (
  username   TEXT PRIMARY KEY,
  name       TEXT NOT NULL,
  email      TEXT,
  source     TEXT NOT NULL CHECK (source IN ('local','gateway')),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
) WITHOUT ROWID;

CREATE TABLE recipes (
  seq            INTEGER PRIMARY KEY,
  id             TEXT NOT NULL UNIQUE,
  title          TEXT NOT NULL CHECK (length(trim(title)) > 0),
  source_url     TEXT,
  description    TEXT,
  notes          TEXT,
  yield_text     TEXT,
  servings       INTEGER,
  prep_minutes   INTEGER CHECK (prep_minutes >= 0),
  cook_minutes   INTEGER CHECK (cook_minutes >= 0),
  total_minutes  INTEGER CHECK (total_minutes >= 0),
  time_text      TEXT,
  nutrition_text TEXT,
  version        INTEGER NOT NULL DEFAULT 1,
  deleted_at     TEXT,
  created_at     TEXT NOT NULL,
  created_by     TEXT NOT NULL REFERENCES people(username),
  updated_at     TEXT NOT NULL,
  updated_by     TEXT NOT NULL REFERENCES people(username)
);
CREATE INDEX recipes_by_title   ON recipes(title COLLATE NOCASE, id) WHERE deleted_at IS NULL;
CREATE INDEX recipes_by_updated ON recipes(updated_at, id) WHERE deleted_at IS NULL;
CREATE INDEX recipes_by_created ON recipes(created_at, id) WHERE deleted_at IS NULL;
CREATE INDEX recipes_trash      ON recipes(deleted_at, id) WHERE deleted_at IS NOT NULL;

CREATE TABLE recipe_ingredients (
  recipe_id      TEXT NOT NULL REFERENCES recipes(id) ON DELETE CASCADE,
  position       INTEGER NOT NULL,
  section        TEXT,
  raw            TEXT NOT NULL,
  quantity       REAL,
  quantity_max   REAL,
  unit           TEXT,
  item           TEXT NOT NULL,
  note           TEXT,
  parser_version INTEGER NOT NULL,
  PRIMARY KEY (recipe_id, position)
) WITHOUT ROWID;

CREATE TABLE recipe_steps (
  recipe_id TEXT NOT NULL REFERENCES recipes(id) ON DELETE CASCADE,
  position  INTEGER NOT NULL,
  section   TEXT,
  text      TEXT NOT NULL,
  PRIMARY KEY (recipe_id, position)
) WITHOUT ROWID;

CREATE TABLE images (
  hash         TEXT PRIMARY KEY CHECK (length(hash) = 64),
  content_type TEXT NOT NULL CHECK (content_type IN ('image/jpeg','image/png','image/webp','image/heic')),
  width        INTEGER,
  height       INTEGER,
  bytes        INTEGER NOT NULL,
  created_at   TEXT NOT NULL,
  created_by   TEXT NOT NULL REFERENCES people(username)
) WITHOUT ROWID;

CREATE TABLE recipe_images (
  recipe_id  TEXT NOT NULL REFERENCES recipes(id) ON DELETE CASCADE,
  position   INTEGER NOT NULL,
  image_hash TEXT NOT NULL REFERENCES images(hash),
  PRIMARY KEY (recipe_id, position),
  UNIQUE (recipe_id, image_hash)
) WITHOUT ROWID;

CREATE TABLE tags (
  id         INTEGER PRIMARY KEY,
  name       TEXT NOT NULL UNIQUE COLLATE NOCASE,
  created_at TEXT NOT NULL,
  created_by TEXT NOT NULL REFERENCES people(username)
);

CREATE TABLE recipe_tags (
  recipe_id TEXT NOT NULL REFERENCES recipes(id) ON DELETE CASCADE,
  tag_id    INTEGER NOT NULL REFERENCES tags(id),
  position  INTEGER NOT NULL,
  PRIMARY KEY (recipe_id, tag_id),
  UNIQUE (recipe_id, position)
) WITHOUT ROWID;
CREATE INDEX recipe_tags_by_tag ON recipe_tags(tag_id);

CREATE TABLE person_recipe (
  person_id    TEXT NOT NULL REFERENCES people(username),
  recipe_id    TEXT NOT NULL REFERENCES recipes(id) ON DELETE CASCADE,
  favorite     INTEGER NOT NULL DEFAULT 0 CHECK (favorite IN (0,1)),
  want_to_cook INTEGER NOT NULL DEFAULT 0 CHECK (want_to_cook IN (0,1)),
  updated_at   TEXT NOT NULL,
  PRIMARY KEY (person_id, recipe_id)
) WITHOUT ROWID;

CREATE TABLE changes (
  seq       INTEGER PRIMARY KEY,
  id        TEXT NOT NULL UNIQUE,
  at        TEXT NOT NULL,
  person_id TEXT REFERENCES people(username),
  via       TEXT NOT NULL CHECK (via IN ('ui','api','mcp')),
  entity    TEXT NOT NULL,
  entity_id TEXT NOT NULL,
  action    TEXT NOT NULL CHECK (action IN ('create','update','delete','restore')),
  summary   TEXT NOT NULL
);
CREATE INDEX changes_by_entity ON changes(entity_id, seq);
CREATE INDEX changes_by_person ON changes(person_id, seq);
CREATE INDEX changes_by_via    ON changes(via, seq);
CREATE TRIGGER changes_no_update BEFORE UPDATE ON changes BEGIN
  SELECT RAISE(ABORT, 'changes is append-only');
END;
CREATE TRIGGER changes_no_delete BEFORE DELETE ON changes BEGIN
  SELECT RAISE(ABORT, 'changes is append-only');
END;

-- rowid = recipes.seq. Deleted recipes stay in the index; the list join
-- filters them out.
CREATE VIRTUAL TABLE recipe_search USING fts5(
  title, description, notes, ingredients, tags,
  tokenize = 'unicode61 remove_diacritics 2'
);

CREATE TRIGGER recipes_search_insert AFTER INSERT ON recipes BEGIN
  INSERT INTO recipe_search (rowid, title, description, notes, ingredients, tags)
  VALUES (new.seq, new.title, coalesce(new.description, ''), coalesce(new.notes, ''), '', '');
END;

CREATE TRIGGER recipes_search_update AFTER UPDATE OF title, description, notes ON recipes BEGIN
  UPDATE recipe_search
     SET title = new.title,
         description = coalesce(new.description, ''),
         notes = coalesce(new.notes, '')
   WHERE rowid = new.seq;
END;

CREATE TRIGGER recipes_search_delete AFTER DELETE ON recipes BEGIN
  DELETE FROM recipe_search WHERE rowid = old.seq;
END;

CREATE TRIGGER recipe_ingredients_search_insert AFTER INSERT ON recipe_ingredients BEGIN
  UPDATE recipe_search
     SET ingredients = (SELECT coalesce(group_concat(item, ' '), '')
                          FROM (SELECT item FROM recipe_ingredients
                                 WHERE recipe_id = new.recipe_id ORDER BY position))
   WHERE rowid = (SELECT seq FROM recipes WHERE id = new.recipe_id);
END;

CREATE TRIGGER recipe_ingredients_search_update AFTER UPDATE OF item ON recipe_ingredients BEGIN
  UPDATE recipe_search
     SET ingredients = (SELECT coalesce(group_concat(item, ' '), '')
                          FROM (SELECT item FROM recipe_ingredients
                                 WHERE recipe_id = new.recipe_id ORDER BY position))
   WHERE rowid = (SELECT seq FROM recipes WHERE id = new.recipe_id);
END;

CREATE TRIGGER recipe_ingredients_search_delete AFTER DELETE ON recipe_ingredients BEGIN
  UPDATE recipe_search
     SET ingredients = (SELECT coalesce(group_concat(item, ' '), '')
                          FROM (SELECT item FROM recipe_ingredients
                                 WHERE recipe_id = old.recipe_id ORDER BY position))
   WHERE rowid = (SELECT seq FROM recipes WHERE id = old.recipe_id);
END;

CREATE TRIGGER recipe_tags_search_insert AFTER INSERT ON recipe_tags BEGIN
  UPDATE recipe_search
     SET tags = (SELECT coalesce(group_concat(name, ' '), '')
                   FROM (SELECT t.name FROM recipe_tags rt JOIN tags t ON t.id = rt.tag_id
                          WHERE rt.recipe_id = new.recipe_id ORDER BY rt.position))
   WHERE rowid = (SELECT seq FROM recipes WHERE id = new.recipe_id);
END;

CREATE TRIGGER recipe_tags_search_delete AFTER DELETE ON recipe_tags BEGIN
  UPDATE recipe_search
     SET tags = (SELECT coalesce(group_concat(name, ' '), '')
                   FROM (SELECT t.name FROM recipe_tags rt JOIN tags t ON t.id = rt.tag_id
                          WHERE rt.recipe_id = old.recipe_id ORDER BY rt.position))
   WHERE rowid = (SELECT seq FROM recipes WHERE id = old.recipe_id);
END;
