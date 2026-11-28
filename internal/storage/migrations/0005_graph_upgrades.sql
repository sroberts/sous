-- Upgrading recipes to step graphs with local models: the answers a model
-- weighed beside the one it picked, what people corrected in review, and
-- bulk upgrade jobs run in the background.

-- The next answers a model weighed, as JSON [{value, probability}], for
-- review to offer beside the value. NULL when there were none.
ALTER TABLE graph_labels ADD COLUMN alternatives TEXT CHECK (alternatives IS NULL OR json_valid(alternatives));

-- One correction in review: what a model said and what a person chose, for
-- examples to learn from and to re-test the thresholds against.
CREATE TABLE review_corrections (
  id           TEXT PRIMARY KEY,
  at           TEXT NOT NULL,
  person       TEXT NOT NULL REFERENCES people(username),
  recipe_id    TEXT NOT NULL REFERENCES recipes(id),
  node_id      TEXT NOT NULL,
  node_text    TEXT NOT NULL,       -- the action's words, so it reads on its own
  field        TEXT NOT NULL,
  model_value  TEXT NOT NULL CHECK (json_valid(model_value)),
  model_source TEXT NOT NULL,
  confidence   REAL NOT NULL,
  user_value   TEXT NOT NULL CHECK (json_valid(user_value))
);
CREATE INDEX review_corrections_by_field ON review_corrections(field, at);

-- A bulk upgrade: the recipes chosen when it was asked for, worked through
-- one at a time by the background worker.
CREATE TABLE upgrade_jobs (
  id          TEXT PRIMARY KEY,
  status      TEXT NOT NULL CHECK (status IN ('queued', 'running', 'done', 'cancelled')),
  mode        TEXT NOT NULL CHECK (mode IN ('draft', 'apply')),
  force       INTEGER NOT NULL DEFAULT 0,  -- redo recipes that already have a graph
  via         TEXT NOT NULL,
  created_at  TEXT NOT NULL,
  created_by  TEXT NOT NULL REFERENCES people(username),
  finished_at TEXT
);

CREATE TABLE upgrade_job_items (
  job_id       TEXT NOT NULL REFERENCES upgrade_jobs(id) ON DELETE CASCADE,
  position     INTEGER NOT NULL,
  recipe_id    TEXT NOT NULL REFERENCES recipes(id),
  status       TEXT NOT NULL CHECK (status IN ('pending', 'running', 'done', 'needs_review', 'failed', 'skipped')),
  review_count INTEGER,
  error        TEXT,
  finished_at  TEXT,
  PRIMARY KEY (job_id, position)
) WITHOUT ROWID;
CREATE INDEX upgrade_job_items_pending ON upgrade_job_items(status, job_id, position) WHERE status IN ('pending', 'running');
