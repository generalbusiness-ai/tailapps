CREATE EVENT otel_event (
  key TEXT NOT NULL,
  source_position INTEGER NOT NULL
);

CREATE TABLE totals (
  key TEXT NOT NULL,
  PRIMARY KEY (key)
);

CREATE NORMALIZER normalize ON otlp_record
USING 'folds/normalize.jsonata'
EMITS otel_event;

CREATE FOLD first_writer ON otel_event
READ prior OPTIONAL ONE AS
  SELECT key FROM totals WHERE key = :event.key
USING 'folds/writer.jsonata'
WRITES totals;

CREATE FOLD second_writer ON otel_event
READ prior OPTIONAL ONE AS
  SELECT key FROM totals WHERE key = :event.key
USING 'folds/writer.jsonata'
WRITES totals;
