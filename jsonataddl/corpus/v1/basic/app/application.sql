CREATE EVENT otel_event (
  key TEXT NOT NULL,
  count INTEGER NOT NULL,
  ratio REAL,
  flag BOOLEAN,
  payload BLOB,
  extra JSON,
  source_position INTEGER NOT NULL
);

CREATE TABLE totals (
  key TEXT NOT NULL,
  total INTEGER NOT NULL,
  last_ratio REAL,
  flag BOOLEAN,
  payload BLOB,
  extra JSON,
  PRIMARY KEY (key)
);

CREATE NORMALIZER normalize ON otlp_record
USING 'folds/normalize.jsonata'
EMITS otel_event;

CREATE FOLD accumulate ON otel_event
READ prior OPTIONAL ONE AS
  SELECT key, total, last_ratio, flag, payload, extra
  FROM totals
  WHERE key = :event.key
USING 'folds/accumulate.jsonata'
WRITES totals;

CREATE EXPORT totals AS
  SELECT key, total, last_ratio, flag, payload, extra FROM totals;
