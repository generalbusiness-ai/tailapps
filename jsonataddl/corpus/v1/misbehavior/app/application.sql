CREATE EVENT otel_event (
  key TEXT NOT NULL,
  count INTEGER NOT NULL,
  source_position INTEGER NOT NULL
);

CREATE TABLE totals (
  key TEXT NOT NULL,
  total INTEGER NOT NULL,
  PRIMARY KEY (key)
);

CREATE TABLE t_emits (
  key TEXT NOT NULL,
  total INTEGER NOT NULL,
  PRIMARY KEY (key)
);

CREATE TABLE t_undeclared (
  key TEXT NOT NULL,
  total INTEGER NOT NULL,
  PRIMARY KEY (key)
);

CREATE TABLE t_ineffective (
  key TEXT NOT NULL,
  total INTEGER NOT NULL,
  PRIMARY KEY (key)
);

CREATE TABLE t_facts (
  key TEXT NOT NULL,
  total INTEGER NOT NULL,
  PRIMARY KEY (key)
);

CREATE TABLE t_unknown (
  key TEXT NOT NULL,
  total INTEGER NOT NULL,
  PRIMARY KEY (key)
);

CREATE NORMALIZER normalize_wrong_event ON otlp_record
USING 'folds/normalize_wrong_event.jsonata'
EMITS otel_event;

CREATE FOLD fold_emits_event ON otel_event
READ prior OPTIONAL ONE AS
  SELECT key, total FROM t_emits WHERE key = :event.key
USING 'folds/fold_emits_event.jsonata'
WRITES t_emits;

CREATE FOLD fold_undeclared_table ON otel_event
READ prior OPTIONAL ONE AS
  SELECT key, total FROM t_undeclared WHERE key = :event.key
USING 'folds/fold_undeclared_table.jsonata'
WRITES t_undeclared;

CREATE FOLD fold_ineffective_outputs ON otel_event
READ prior OPTIONAL ONE AS
  SELECT key, total FROM t_ineffective WHERE key = :event.key
USING 'folds/fold_ineffective_outputs.jsonata'
WRITES t_ineffective;

CREATE FOLD fold_too_many_facts ON otel_event
READ prior OPTIONAL ONE AS
  SELECT key, total FROM t_facts WHERE key = :event.key
USING 'folds/fold_too_many_facts.jsonata'
WRITES t_facts;

CREATE FOLD fold_unknown_field ON otel_event
READ prior OPTIONAL ONE AS
  SELECT key, total FROM t_unknown WHERE key = :event.key
USING 'folds/fold_unknown_field.jsonata'
WRITES t_unknown;

CREATE FOLD fold_row_shapes ON otel_event
READ prior OPTIONAL ONE AS
  SELECT key, total FROM totals WHERE key = :event.key
USING 'folds/fold_row_shapes.jsonata'
WRITES totals;

CREATE EXPORT totals AS
  SELECT key, total FROM totals;
