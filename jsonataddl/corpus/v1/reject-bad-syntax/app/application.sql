CREATE EVENT otel_event (
  key TEXT NOT NULL,
  source_position INTEGER NOT NULL
);

CREATE NORMALIZER normalize ON otlp_record
USING 'folds/normalize.jsonata'
EMITS otel_event;
