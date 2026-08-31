CREATE EVENT otel_event (
  harness TEXT NOT NULL,
  session_id TEXT NOT NULL,
  session_id_prefix TEXT NOT NULL,
  model TEXT NOT NULL,
  project TEXT NOT NULL,
  model_coverage TEXT NOT NULL CHECK (model_coverage IN ('observed', 'unknown')),
  project_coverage TEXT NOT NULL CHECK (project_coverage IN ('observed', 'unknown')),
  input_tokens INTEGER NOT NULL,
  output_tokens INTEGER NOT NULL,
  cached_input_tokens INTEGER NOT NULL,
  reasoning_output_tokens INTEGER NOT NULL,
  cost_microusd INTEGER NOT NULL,
  event_time_unix_nano TEXT NOT NULL,
  source_position INTEGER NOT NULL
);

CREATE TABLE session_cost_detail (
  harness TEXT NOT NULL,
  session_id TEXT NOT NULL,
  session_id_prefix TEXT NOT NULL,
  model TEXT NOT NULL,
  project TEXT NOT NULL,
  model_coverage TEXT NOT NULL CHECK (model_coverage IN ('observed', 'unknown')),
  project_coverage TEXT NOT NULL CHECK (project_coverage IN ('observed', 'unknown')),
  input_tokens INTEGER NOT NULL,
  output_tokens INTEGER NOT NULL,
  cached_input_tokens INTEGER NOT NULL,
  reasoning_output_tokens INTEGER NOT NULL,
  cost_microusd INTEGER NOT NULL,
  first_event_time_unix_nano TEXT NOT NULL,
  last_event_time_unix_nano TEXT NOT NULL,
  last_source_position INTEGER NOT NULL,
  PRIMARY KEY (harness, session_id)
);

CREATE TABLE session_cost (
  harness TEXT NOT NULL,
  session_id TEXT NOT NULL,
  input_tokens INTEGER NOT NULL,
  output_tokens INTEGER NOT NULL,
  cached_input_tokens INTEGER NOT NULL,
  reasoning_output_tokens INTEGER NOT NULL,
  cost_microusd INTEGER NOT NULL,
  last_event_time_unix_nano TEXT NOT NULL,
  last_source_position INTEGER NOT NULL,
  PRIMARY KEY (harness, session_id)
);

CREATE NORMALIZER normalize_usage ON otlp_record
USING 'folds/normalize.jsonata'
EMITS otel_event;

CREATE FOLD accumulate_cost ON otel_event
READ prior OPTIONAL ONE AS
  SELECT harness, session_id, input_tokens, output_tokens,
         cached_input_tokens, reasoning_output_tokens,
         cost_microusd, last_event_time_unix_nano, last_source_position
  FROM session_cost
  WHERE harness = :event.harness AND session_id = :event.session_id
READ detail OPTIONAL ONE AS
  SELECT harness, session_id, session_id_prefix, model, project,
         model_coverage, project_coverage, input_tokens, output_tokens,
         cached_input_tokens, reasoning_output_tokens, cost_microusd,
         first_event_time_unix_nano, last_event_time_unix_nano,
         last_source_position
  FROM session_cost_detail
  WHERE harness = :event.harness AND session_id = :event.session_id
USING 'folds/cost.jsonata'
WRITES session_cost, session_cost_detail;

CREATE EXPORT session_cost AS
  SELECT harness, session_id, input_tokens, output_tokens,
         cached_input_tokens, reasoning_output_tokens,
         cost_microusd, last_event_time_unix_nano, last_source_position
  FROM session_cost;

CREATE EXPORT session_cost_detail AS
  SELECT harness, session_id, session_id_prefix, model, project,
         model_coverage, project_coverage, input_tokens, output_tokens,
         cached_input_tokens, reasoning_output_tokens, cost_microusd,
         first_event_time_unix_nano, last_event_time_unix_nano,
         last_source_position
  FROM session_cost_detail;
