CREATE EVENT otel_event (
  harness TEXT NOT NULL,
  session_id TEXT NOT NULL,
  event_family TEXT NOT NULL,
  tool_bucket TEXT NOT NULL,
  endpoint_bucket TEXT NOT NULL,
  success_state TEXT NOT NULL CHECK (success_state IN ('success', 'failure', 'unknown')),
  reported_length INTEGER NOT NULL CHECK (reported_length >= 0),
  length_coverage TEXT NOT NULL CHECK (length_coverage IN ('observed', 'unknown', 'not-applicable')),
  duration_milliseconds INTEGER NOT NULL CHECK (duration_milliseconds >= 0),
  duration_coverage TEXT NOT NULL CHECK (duration_coverage IN ('observed', 'unknown', 'not-applicable')),
  input_tokens INTEGER NOT NULL CHECK (input_tokens >= 0),
  output_tokens INTEGER NOT NULL CHECK (output_tokens >= 0),
  cached_input_tokens INTEGER NOT NULL CHECK (cached_input_tokens >= 0),
  reasoning_output_tokens INTEGER NOT NULL CHECK (reasoning_output_tokens >= 0),
  cost_microusd INTEGER NOT NULL CHECK (cost_microusd >= 0),
  token_coverage TEXT NOT NULL CHECK (token_coverage IN ('observed', 'unknown', 'not-applicable')),
  session_coverage TEXT NOT NULL CHECK (session_coverage IN ('observed', 'unknown')),
  performance_event BOOLEAN NOT NULL,
  event_time_unix_nano TEXT NOT NULL,
  source_position INTEGER NOT NULL
);

CREATE TABLE event_inventory (
  harness TEXT NOT NULL,
  event_family TEXT NOT NULL,
  tool_bucket TEXT NOT NULL,
  endpoint_bucket TEXT NOT NULL,
  event_count INTEGER NOT NULL CHECK (event_count >= 1),
  first_seen_unix_nano TEXT NOT NULL,
  last_seen_unix_nano TEXT NOT NULL,
  reported_length_total INTEGER NOT NULL CHECK (reported_length_total >= 0),
  length_observed_count INTEGER NOT NULL CHECK (length_observed_count >= 0),
  length_unknown_count INTEGER NOT NULL CHECK (length_unknown_count >= 0),
  duration_milliseconds_total INTEGER NOT NULL CHECK (duration_milliseconds_total >= 0),
  duration_observed_count INTEGER NOT NULL CHECK (duration_observed_count >= 0),
  duration_unknown_count INTEGER NOT NULL CHECK (duration_unknown_count >= 0),
  success_count INTEGER NOT NULL CHECK (success_count >= 0),
  failure_count INTEGER NOT NULL CHECK (failure_count >= 0),
  outcome_unknown_count INTEGER NOT NULL CHECK (outcome_unknown_count >= 0),
  session_unknown_count INTEGER NOT NULL CHECK (session_unknown_count >= 0),
  token_observed_count INTEGER NOT NULL CHECK (token_observed_count >= 0),
  token_unknown_count INTEGER NOT NULL CHECK (token_unknown_count >= 0),
  last_source_position INTEGER NOT NULL,
  PRIMARY KEY (harness, event_family, tool_bucket, endpoint_bucket)
);

CREATE TABLE session_activity (
  harness TEXT NOT NULL,
  session_id TEXT NOT NULL,
  event_count INTEGER NOT NULL CHECK (event_count >= 1),
  first_seen_unix_nano TEXT NOT NULL,
  last_seen_unix_nano TEXT NOT NULL,
  tool_event_count INTEGER NOT NULL CHECK (tool_event_count >= 0),
  shell_command_count INTEGER NOT NULL CHECK (shell_command_count >= 0),
  prompt_count INTEGER NOT NULL CHECK (prompt_count >= 0),
  response_count INTEGER NOT NULL CHECK (response_count >= 0),
  request_count INTEGER NOT NULL CHECK (request_count >= 0),
  success_count INTEGER NOT NULL CHECK (success_count >= 0),
  failure_count INTEGER NOT NULL CHECK (failure_count >= 0),
  outcome_unknown_count INTEGER NOT NULL CHECK (outcome_unknown_count >= 0),
  reported_length_total INTEGER NOT NULL CHECK (reported_length_total >= 0),
  input_tokens INTEGER NOT NULL CHECK (input_tokens >= 0),
  output_tokens INTEGER NOT NULL CHECK (output_tokens >= 0),
  cached_input_tokens INTEGER NOT NULL CHECK (cached_input_tokens >= 0),
  reasoning_output_tokens INTEGER NOT NULL CHECK (reasoning_output_tokens >= 0),
  cost_microusd INTEGER NOT NULL CHECK (cost_microusd >= 0),
  unknown_field_observations INTEGER NOT NULL CHECK (unknown_field_observations >= 0),
  last_source_position INTEGER NOT NULL,
  PRIMARY KEY (harness, session_id)
);

CREATE TABLE request_performance (
  harness TEXT NOT NULL,
  session_id TEXT NOT NULL,
  event_family TEXT NOT NULL,
  endpoint_bucket TEXT NOT NULL,
  request_count INTEGER NOT NULL CHECK (request_count >= 1),
  first_seen_unix_nano TEXT NOT NULL,
  last_seen_unix_nano TEXT NOT NULL,
  success_count INTEGER NOT NULL CHECK (success_count >= 0),
  failure_count INTEGER NOT NULL CHECK (failure_count >= 0),
  outcome_unknown_count INTEGER NOT NULL CHECK (outcome_unknown_count >= 0),
  duration_observed_count INTEGER NOT NULL CHECK (duration_observed_count >= 0),
  duration_unknown_count INTEGER NOT NULL CHECK (duration_unknown_count >= 0),
  duration_milliseconds_total INTEGER NOT NULL CHECK (duration_milliseconds_total >= 0),
  duration_le_100ms INTEGER NOT NULL CHECK (duration_le_100ms >= 0),
  duration_le_500ms INTEGER NOT NULL CHECK (duration_le_500ms >= 0),
  duration_le_1000ms INTEGER NOT NULL CHECK (duration_le_1000ms >= 0),
  duration_le_5000ms INTEGER NOT NULL CHECK (duration_le_5000ms >= 0),
  duration_gt_5000ms INTEGER NOT NULL CHECK (duration_gt_5000ms >= 0),
  last_source_position INTEGER NOT NULL,
  PRIMARY KEY (harness, session_id, event_family, endpoint_bucket)
);

CREATE NORMALIZER normalize_activity ON otlp_record
USING 'folds/normalize.jsonata'
EMITS otel_event;

CREATE FOLD update_event_inventory ON otel_event
READ prior OPTIONAL ONE AS
  SELECT harness, event_family, tool_bucket, endpoint_bucket, event_count,
         first_seen_unix_nano, last_seen_unix_nano, reported_length_total,
         length_observed_count, length_unknown_count, duration_milliseconds_total,
         duration_observed_count, duration_unknown_count, success_count,
         failure_count, outcome_unknown_count, session_unknown_count,
         token_observed_count, token_unknown_count, last_source_position
  FROM event_inventory
  WHERE harness = :event.harness AND event_family = :event.event_family
    AND tool_bucket = :event.tool_bucket AND endpoint_bucket = :event.endpoint_bucket
USING 'folds/inventory.jsonata'
WRITES event_inventory;

CREATE FOLD update_session_activity ON otel_event
READ prior OPTIONAL ONE AS
  SELECT harness, session_id, event_count, first_seen_unix_nano,
         last_seen_unix_nano, tool_event_count, shell_command_count,
         prompt_count, response_count, request_count, success_count,
         failure_count, outcome_unknown_count, reported_length_total,
         input_tokens, output_tokens, cached_input_tokens,
         reasoning_output_tokens, cost_microusd, unknown_field_observations,
         last_source_position
  FROM session_activity
  WHERE harness = :event.harness AND session_id = :event.session_id
USING 'folds/session.jsonata'
WRITES session_activity;

CREATE FOLD update_request_performance ON otel_event
READ prior OPTIONAL ONE AS
  SELECT harness, session_id, event_family, endpoint_bucket, request_count,
         first_seen_unix_nano, last_seen_unix_nano, success_count,
         failure_count, outcome_unknown_count, duration_observed_count,
         duration_unknown_count, duration_milliseconds_total,
         duration_le_100ms, duration_le_500ms, duration_le_1000ms,
         duration_le_5000ms, duration_gt_5000ms, last_source_position
  FROM request_performance
  WHERE harness = :event.harness AND session_id = :event.session_id
    AND event_family = :event.event_family AND endpoint_bucket = :event.endpoint_bucket
USING 'folds/performance.jsonata'
WRITES request_performance;

CREATE EXPORT event_inventory AS
  SELECT harness, event_family, tool_bucket, endpoint_bucket, event_count,
         first_seen_unix_nano, last_seen_unix_nano, reported_length_total,
         length_observed_count, length_unknown_count, duration_milliseconds_total,
         duration_observed_count, duration_unknown_count, success_count,
         failure_count, outcome_unknown_count, session_unknown_count,
         token_observed_count, token_unknown_count, last_source_position
  FROM event_inventory;

CREATE EXPORT session_activity AS
  SELECT harness, session_id, event_count, first_seen_unix_nano,
         last_seen_unix_nano, tool_event_count, shell_command_count,
         prompt_count, response_count, request_count, success_count,
         failure_count, outcome_unknown_count, reported_length_total,
         input_tokens, output_tokens, cached_input_tokens,
         reasoning_output_tokens, cost_microusd, unknown_field_observations,
         last_source_position
  FROM session_activity;

CREATE EXPORT request_performance AS
  SELECT harness, session_id, event_family, endpoint_bucket, request_count,
         first_seen_unix_nano, last_seen_unix_nano, success_count,
         failure_count, outcome_unknown_count, duration_observed_count,
         duration_unknown_count, duration_milliseconds_total,
         duration_le_100ms, duration_le_500ms, duration_le_1000ms,
         duration_le_5000ms, duration_gt_5000ms, last_source_position
  FROM request_performance;
