CREATE EVENT otel_event (
  source TEXT NOT NULL,
  signal TEXT NOT NULL,
  event_name TEXT NOT NULL,
  source_position INTEGER NOT NULL
);

CREATE TABLE signal_counts (
  source TEXT NOT NULL,
  signal TEXT NOT NULL,
  event_name TEXT NOT NULL,
  event_count INTEGER NOT NULL,
  last_source_position INTEGER NOT NULL,
  PRIMARY KEY (source, signal, event_name)
);

CREATE NORMALIZER normalize_signal ON otlp_record
USING 'folds/normalize.jsonata'
EMITS otel_event;

CREATE FOLD count_signal ON otel_event
READ prior OPTIONAL ONE AS
  SELECT source, signal, event_name, event_count, last_source_position
  FROM signal_counts
  WHERE source = :event.source AND signal = :event.signal AND event_name = :event.event_name
USING 'folds/count.jsonata'
WRITES signal_counts;

CREATE EXPORT signal_counts AS
  SELECT source, signal, event_name, event_count, last_source_position
  FROM signal_counts;
