-- Meal planning: timing on recipe steps, the household's kitchen, and meals
-- (several recipes cooked for one sitting) with their progress on the day.

-- A step's timing for meal planning, as JSON (recipes.StepTiming): minutes,
-- hands_on, equipment, oven_temp, space, hold_minutes, hold_in,
-- reheat_minutes, reheat_temp. NULL when the step has none.
ALTER TABLE recipe_steps ADD COLUMN timing TEXT CHECK (timing IS NULL OR json_valid(timing));

-- The one kitchen the household cooks in (recipes.KitchenProfile as JSON):
-- ovens and their racks, burners, other appliances, serving dishes and the
-- hours of the day hands-on work is planned in. It starts as a plain
-- kitchen, so a meal can be planned before anyone describes theirs.
CREATE TABLE kitchen (
  id         INTEGER PRIMARY KEY CHECK (id = 1),
  profile    TEXT NOT NULL CHECK (json_valid(profile)),
  version    INTEGER NOT NULL DEFAULT 1,
  updated_at TEXT NOT NULL,
  updated_by TEXT REFERENCES people(username)
);
INSERT INTO kitchen (id, profile, version, updated_at) VALUES (1,
  '{"ovens":[{"name":"Oven","racks":2}],"burners":4,"appliances":[],"dishes":[],"day_start":"08:00","day_end":"22:00"}',
  1, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));

CREATE TABLE meals (
  seq        INTEGER PRIMARY KEY,
  id         TEXT NOT NULL UNIQUE,
  title      TEXT NOT NULL CHECK (length(trim(title)) > 0),
  serve_at   TEXT NOT NULL,            -- UTC, when the first course is served
  timezone   TEXT NOT NULL,            -- IANA name: where the kitchen's day begins and ends
  cooks      INTEGER NOT NULL DEFAULT 1 CHECK (cooks BETWEEN 1 AND 8),
  prep_days  INTEGER NOT NULL DEFAULT 1 CHECK (prep_days BETWEEN 0 AND 7),
  notes      TEXT,
  version    INTEGER NOT NULL DEFAULT 1,
  deleted_at TEXT,
  created_at TEXT NOT NULL,
  created_by TEXT NOT NULL REFERENCES people(username),
  updated_at TEXT NOT NULL,
  updated_by TEXT NOT NULL REFERENCES people(username)
);
CREATE INDEX meals_by_serve ON meals(serve_at, id) WHERE deleted_at IS NULL;

-- One dish on the menu. id is kept across edits, so progress on the day
-- stays with the dish when the menu is reordered.
CREATE TABLE meal_items (
  meal_id              TEXT NOT NULL REFERENCES meals(id) ON DELETE CASCADE,
  id                   TEXT NOT NULL,
  position             INTEGER NOT NULL,
  recipe_id            TEXT NOT NULL REFERENCES recipes(id),
  serve_offset_minutes INTEGER NOT NULL DEFAULT 0,
  dish                 TEXT,
  PRIMARY KEY (meal_id, id),
  UNIQUE (meal_id, position)
) WITHOUT ROWID;
CREATE INDEX meal_items_by_recipe ON meal_items(recipe_id);

-- What has happened to a task on the day, and where someone pinned it.
-- task is "<item id>:<step index>" or "<item id>:reheat".
CREATE TABLE meal_tasks (
  meal_id      TEXT NOT NULL REFERENCES meals(id) ON DELETE CASCADE,
  task         TEXT NOT NULL,
  pinned_start TEXT,
  started_at   TEXT,
  done_at      TEXT,
  done_by      TEXT REFERENCES people(username),
  updated_at   TEXT NOT NULL,
  PRIMARY KEY (meal_id, task)
) WITHOUT ROWID;
