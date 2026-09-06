CREATE EVENT otel_event (
  day_utc INTEGER NOT NULL,
  harness TEXT NOT NULL,
  signal TEXT NOT NULL,
  tool_event INTEGER NOT NULL,
  api_event INTEGER NOT NULL,
  success_state TEXT NOT NULL,
  duration_observed INTEGER NOT NULL,
  duration_milliseconds INTEGER NOT NULL,
  input_tokens INTEGER NOT NULL,
  cached_input_tokens INTEGER NOT NULL,
  output_tokens INTEGER NOT NULL,
  reasoning_output_tokens INTEGER NOT NULL,
  cost_microusd INTEGER NOT NULL,
  session_unknown INTEGER NOT NULL,
  identity_metadata INTEGER NOT NULL,
  auth_metadata INTEGER NOT NULL,
  raw_prompt_content INTEGER NOT NULL,
  raw_tool_content INTEGER NOT NULL,
  risky_action INTEGER NOT NULL,
  event_time_unix_nano TEXT NOT NULL,
  source_position INTEGER NOT NULL
);

CREATE TABLE daily_review (
  day_utc INTEGER NOT NULL,
  harness TEXT NOT NULL,
  record_count INTEGER NOT NULL,
  log_count INTEGER NOT NULL,
  span_count INTEGER NOT NULL,
  metric_count INTEGER NOT NULL,
  tool_event_count INTEGER NOT NULL,
  api_event_count INTEGER NOT NULL,
  success_count INTEGER NOT NULL,
  failure_count INTEGER NOT NULL,
  outcome_unknown_count INTEGER NOT NULL,
  duration_observed_count INTEGER NOT NULL,
  duration_milliseconds_total INTEGER NOT NULL,
  duration_gt_5000ms_count INTEGER NOT NULL,
  input_tokens INTEGER NOT NULL,
  cached_input_tokens INTEGER NOT NULL,
  output_tokens INTEGER NOT NULL,
  reasoning_output_tokens INTEGER NOT NULL,
  cost_microusd INTEGER NOT NULL,
  session_unknown_count INTEGER NOT NULL,
  identity_metadata_count INTEGER NOT NULL,
  auth_metadata_count INTEGER NOT NULL,
  raw_prompt_content_count INTEGER NOT NULL,
  raw_tool_content_count INTEGER NOT NULL,
  risky_action_count INTEGER NOT NULL,
  first_event_time_unix_nano TEXT NOT NULL,
  last_event_time_unix_nano TEXT NOT NULL,
  last_source_position INTEGER NOT NULL,
  PRIMARY KEY (day_utc, harness)
);

CREATE NORMALIZER normalize_review_event ON otlp_record
USING 'folds/normalize.jsonata'
EMITS otel_event;

CREATE FOLD update_daily_review ON otel_event
READ prior OPTIONAL ONE AS
  SELECT day_utc, harness, record_count, log_count, span_count, metric_count,
         tool_event_count, api_event_count, success_count, failure_count,
         outcome_unknown_count, duration_observed_count,
         duration_milliseconds_total, duration_gt_5000ms_count,
         input_tokens, cached_input_tokens, output_tokens,
         reasoning_output_tokens, cost_microusd, session_unknown_count,
         identity_metadata_count, auth_metadata_count,
         raw_prompt_content_count, raw_tool_content_count, risky_action_count,
         first_event_time_unix_nano, last_event_time_unix_nano,
         last_source_position
  FROM daily_review
  WHERE day_utc = :event.day_utc AND harness = :event.harness
USING 'folds/daily.jsonata'
WRITES daily_review;

CREATE EXPORT daily_review AS
  SELECT day_utc, harness, record_count, log_count, span_count, metric_count,
         tool_event_count, api_event_count, success_count, failure_count,
         outcome_unknown_count, duration_observed_count,
         duration_milliseconds_total, duration_gt_5000ms_count,
         input_tokens, cached_input_tokens, output_tokens,
         reasoning_output_tokens, cost_microusd, session_unknown_count,
         identity_metadata_count, auth_metadata_count,
         raw_prompt_content_count, raw_tool_content_count, risky_action_count,
         first_event_time_unix_nano, last_event_time_unix_nano,
         last_source_position
  FROM daily_review;
