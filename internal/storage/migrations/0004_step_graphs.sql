-- Step graphs (ontology 2.0): a recipe's method as atomic actions joined by
-- the items they make and use, each classified field labelled with where it
-- came from and how sure that is. One graph per recipe version; the newest
-- not deleted is the recipe's graph. Step timing (recipe_steps.timing) is
-- left as it is: a graph sits beside it.

CREATE TABLE recipe_graphs (
  recipe_id        TEXT NOT NULL REFERENCES recipes(id),
  recipe_version   INTEGER NOT NULL,      -- the recipe's version it was saved against
  version          INTEGER NOT NULL,      -- the graph's own, rising across the recipe's graphs
  ontology_version TEXT NOT NULL,
  status           TEXT NOT NULL CHECK (status IN ('draft', 'needs_review', 'verified')),
  serve_item_id    TEXT NOT NULL,
  deleted_at       TEXT,
  created_at       TEXT NOT NULL,
  created_by       TEXT NOT NULL REFERENCES people(username),
  updated_at       TEXT NOT NULL,
  updated_by       TEXT NOT NULL REFERENCES people(username),
  PRIMARY KEY (recipe_id, recipe_version)
) WITHOUT ROWID;
CREATE INDEX recipe_graphs_current ON recipe_graphs(recipe_id, version) WHERE deleted_at IS NULL;

-- Anything a node uses or makes. ingredient is the 1-based line of the
-- recipe's ingredient list, for kind ingredient.
CREATE TABLE graph_items (
  recipe_id      TEXT NOT NULL,
  recipe_version INTEGER NOT NULL,
  id             TEXT NOT NULL,
  position       INTEGER NOT NULL,
  kind           TEXT NOT NULL CHECK (kind IN ('ingredient', 'intermediate', 'final')),
  label          TEXT NOT NULL,
  ingredient     INTEGER,
  quantity       TEXT,
  PRIMARY KEY (recipe_id, recipe_version, id),
  FOREIGN KEY (recipe_id, recipe_version) REFERENCES recipe_graphs(recipe_id, recipe_version) ON DELETE CASCADE
) WITHOUT ROWID;

-- One atomic action. step, span_start and span_end point at the words it
-- comes from (step from 1, characters from 0); quote is those words when
-- the graph was saved, so an edit to them shows. inputs and outputs are
-- JSON arrays of item ids, in order.
CREATE TABLE graph_nodes (
  recipe_id      TEXT NOT NULL,
  recipe_version INTEGER NOT NULL,
  id             TEXT NOT NULL,
  position       INTEGER NOT NULL,
  step           INTEGER NOT NULL,
  span_start     INTEGER NOT NULL,
  span_end       INTEGER NOT NULL,
  quote          TEXT NOT NULL,
  text           TEXT NOT NULL,
  technique      TEXT,
  inputs         TEXT NOT NULL CHECK (json_valid(inputs)),
  outputs        TEXT NOT NULL CHECK (json_valid(outputs)),
  implied        INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (recipe_id, recipe_version, id),
  FOREIGN KEY (recipe_id, recipe_version) REFERENCES recipe_graphs(recipe_id, recipe_version) ON DELETE CASCADE
) WITHOUT ROWID;

-- One classified field of a node, with its provenance: the rows a review
-- queue reads. value is JSON in the field's own shape.
CREATE TABLE graph_labels (
  recipe_id      TEXT NOT NULL,
  recipe_version INTEGER NOT NULL,
  node_id        TEXT NOT NULL,
  field          TEXT NOT NULL CHECK (field IN ('action', 'duration', 'attention', 'check_interval_min',
                   'interruptible', 'equipment', 'hold', 'timing_anchor')),
  value          TEXT NOT NULL CHECK (json_valid(value)),
  source         TEXT NOT NULL CHECK (source IN ('author', 'recipe_text', 'model', 'jev', 'user')),
  confidence     REAL NOT NULL CHECK (confidence BETWEEN 0 AND 1),
  reviewed_by    TEXT REFERENCES people(username),
  reviewed_at    TEXT,
  PRIMARY KEY (recipe_id, recipe_version, node_id, field),
  FOREIGN KEY (recipe_id, recipe_version, node_id) REFERENCES graph_nodes(recipe_id, recipe_version, id) ON DELETE CASCADE
) WITHOUT ROWID;
CREATE INDEX graph_labels_unsure ON graph_labels(field, confidence) WHERE reviewed_at IS NULL;
