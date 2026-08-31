CREATE EVENT otel_event (
  harness TEXT NOT NULL,
  session_id TEXT NOT NULL,
  session_id_prefix TEXT NOT NULL,
  project TEXT NOT NULL,
  model TEXT NOT NULL,
  kind TEXT NOT NULL,
  operation_kind TEXT NOT NULL,
  tool TEXT,
  target TEXT,
  command TEXT NOT NULL,
  tool_arguments TEXT NOT NULL,
  failure_detail TEXT NOT NULL,
  argument_digest TEXT NOT NULL,
  success BOOLEAN,
  event_time_unix_nano TEXT NOT NULL,
  action_fingerprint TEXT NOT NULL,
  progress_fingerprint TEXT NOT NULL,
  progress_coverage TEXT NOT NULL,
  source_position INTEGER NOT NULL,
  tool_coverage TEXT NOT NULL,
  target_coverage TEXT NOT NULL,
  coverage_reason TEXT NOT NULL,
  tool_capability TEXT NOT NULL,
  target_capability TEXT NOT NULL,
  progress_capability TEXT NOT NULL,
  tool_coverage_reason TEXT NOT NULL,
  target_coverage_reason TEXT NOT NULL,
  progress_coverage_reason TEXT NOT NULL
);

CREATE TABLE tool_failure_detail (
  source_position INTEGER PRIMARY KEY,
  event_time_unix_nano TEXT NOT NULL,
  harness TEXT NOT NULL,
  session_id TEXT NOT NULL,
  session_id_prefix TEXT NOT NULL,
  project TEXT NOT NULL,
  model TEXT NOT NULL,
  tool TEXT NOT NULL,
  command TEXT NOT NULL,
  tool_arguments TEXT NOT NULL,
  target TEXT NOT NULL,
  failure_detail TEXT NOT NULL,
  detail_coverage TEXT NOT NULL CHECK (detail_coverage IN ('observed', 'partial', 'unknown'))
);

CREATE TABLE telemetry_coverage (
  harness TEXT NOT NULL,
  capability TEXT NOT NULL,
  state TEXT NOT NULL CHECK (state IN ('observed', 'unknown')),
  reason TEXT NOT NULL,
  first_seen_unix_nano TEXT NOT NULL,
  last_seen_unix_nano TEXT NOT NULL,
  last_source_position INTEGER NOT NULL,
  PRIMARY KEY (harness, capability)
);

CREATE TABLE session_progress (
  harness TEXT NOT NULL,
  session_id TEXT NOT NULL,
  first_activity_unix_nano TEXT NOT NULL,
  last_activity_unix_nano TEXT NOT NULL,
  last_distinct_progress_unix_nano TEXT NOT NULL,
  action_fingerprint TEXT NOT NULL,
  progress_fingerprint TEXT NOT NULL,
  repeat_count INTEGER NOT NULL CHECK (repeat_count >= 1),
  no_progress_count INTEGER NOT NULL CHECK (no_progress_count >= 0),
  consecutive_failures INTEGER NOT NULL CHECK (consecutive_failures >= 0),
  total_actions INTEGER NOT NULL CHECK (total_actions >= 1),
  last_source_position INTEGER NOT NULL,
  PRIMARY KEY (harness, session_id)
);

CREATE TABLE policy_findings (
  finding_id TEXT PRIMARY KEY,
  rule_id TEXT NOT NULL,
  severity TEXT NOT NULL,
  harness TEXT NOT NULL,
  session_id TEXT NOT NULL,
  source_position INTEGER NOT NULL,
  observed_unix_nano TEXT NOT NULL,
  summary TEXT NOT NULL,
  evidence JSON NOT NULL,
  coverage_state TEXT NOT NULL
);

CREATE TABLE loop_findings (
  finding_id TEXT PRIMARY KEY,
  finding_kind TEXT NOT NULL,
  harness TEXT NOT NULL,
  session_id TEXT NOT NULL,
  source_position INTEGER NOT NULL,
  first_observed_unix_nano TEXT NOT NULL,
  last_observed_unix_nano TEXT NOT NULL,
  repeat_count INTEGER NOT NULL,
  consecutive_failures INTEGER NOT NULL,
  no_progress_count INTEGER NOT NULL,
  evidence JSON NOT NULL
);

CREATE INDEX policy_findings_session
  ON policy_findings(harness, session_id, source_position);

CREATE INDEX loop_findings_session
  ON loop_findings(harness, session_id, source_position);

CREATE VIEW open_guard_findings AS
  SELECT finding_id, rule_id, severity, harness, session_id,
         source_position, observed_unix_nano, summary, evidence, coverage_state
  FROM policy_findings;

CREATE NORMALIZER normalize_harness_event ON otlp_record
USING 'folds/normalize.jsonata'
EMITS otel_event;

CREATE FOLD update_guard_analytics ON otel_event
READ prior OPTIONAL ONE AS
  SELECT harness, session_id, first_activity_unix_nano,
         last_activity_unix_nano, last_distinct_progress_unix_nano,
         action_fingerprint, progress_fingerprint, repeat_count, no_progress_count,
         consecutive_failures, total_actions, last_source_position
  FROM session_progress
  WHERE harness = :event.harness AND session_id = :event.session_id
READ tool_coverage_prior OPTIONAL ONE AS
  SELECT harness, capability, state, reason, first_seen_unix_nano,
         last_seen_unix_nano, last_source_position
  FROM telemetry_coverage
  WHERE harness = :event.harness AND capability = :event.tool_capability
READ target_coverage_prior OPTIONAL ONE AS
  SELECT harness, capability, state, reason, first_seen_unix_nano,
         last_seen_unix_nano, last_source_position
  FROM telemetry_coverage
  WHERE harness = :event.harness AND capability = :event.target_capability
READ progress_coverage_prior OPTIONAL ONE AS
  SELECT harness, capability, state, reason, first_seen_unix_nano,
         last_seen_unix_nano, last_source_position
  FROM telemetry_coverage
  WHERE harness = :event.harness AND capability = :event.progress_capability
USING 'folds/guard.jsonata'
WRITES telemetry_coverage, session_progress, policy_findings, loop_findings, tool_failure_detail;

CREATE EXPORT telemetry_coverage AS
  SELECT harness, capability, state, reason, first_seen_unix_nano,
         last_seen_unix_nano, last_source_position
  FROM telemetry_coverage;

CREATE EXPORT session_progress AS
  SELECT harness, session_id, first_activity_unix_nano,
         last_activity_unix_nano, last_distinct_progress_unix_nano,
         action_fingerprint, progress_fingerprint, repeat_count, no_progress_count,
         consecutive_failures, total_actions, last_source_position
  FROM session_progress;

CREATE EXPORT policy_findings AS
  SELECT finding_id, rule_id, severity, harness, session_id, source_position,
         observed_unix_nano, summary, evidence, coverage_state
  FROM policy_findings;

CREATE EXPORT loop_findings AS
  SELECT finding_id, finding_kind, harness, session_id, source_position,
         first_observed_unix_nano, last_observed_unix_nano, repeat_count,
         consecutive_failures, no_progress_count, evidence
  FROM loop_findings;

CREATE EXPORT tool_failure_detail AS
  SELECT source_position, event_time_unix_nano, harness, session_id,
         session_id_prefix, project, model, tool, command, tool_arguments,
         target, failure_detail, detail_coverage
  FROM tool_failure_detail;
