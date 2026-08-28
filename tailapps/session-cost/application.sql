CREATE EVENT otel_event (
  harness TEXT NOT NULL,
  session_id TEXT NOT NULL,
  input_tokens INTEGER NOT NULL,
  output_tokens INTEGER NOT NULL,
  cost_microusd INTEGER NOT NULL,
  source_position INTEGER NOT NULL
);

CREATE TABLE session_cost (
  harness TEXT NOT NULL,
  session_id TEXT NOT NULL,
  input_tokens INTEGER NOT NULL,
  output_tokens INTEGER NOT NULL,
  cost_microusd INTEGER NOT NULL,
  last_source_position INTEGER NOT NULL,
  PRIMARY KEY (harness, session_id)
);

CREATE NORMALIZER normalize_usage ON otlp_record
USING 'folds/normalize.jsonata'
EMITS otel_event;

CREATE FOLD accumulate_cost ON otel_event
READ prior OPTIONAL ONE AS
  SELECT harness, session_id, input_tokens, output_tokens,
         cost_microusd, last_source_position
  FROM session_cost
  WHERE harness = :event.harness AND session_id = :event.session_id
USING 'folds/cost.jsonata'
WRITES session_cost;

CREATE EXPORT session_cost AS
  SELECT harness, session_id, input_tokens, output_tokens,
         cost_microusd, last_source_position
  FROM session_cost;

