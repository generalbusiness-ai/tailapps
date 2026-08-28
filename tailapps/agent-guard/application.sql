CREATE EVENT otel_event (
  harness TEXT NOT NULL,
  session_id TEXT NOT NULL,
  kind TEXT NOT NULL,
  operation_kind TEXT NOT NULL,
  tool TEXT,
  target TEXT,
  argument_digest TEXT NOT NULL,
  success BOOLEAN,
  event_time_unix_nano TEXT NOT NULL,
  progress_fingerprint TEXT NOT NULL,
  source_position INTEGER NOT NULL,
  tool_coverage TEXT NOT NULL,
  target_coverage TEXT NOT NULL,
  coverage_reason TEXT NOT NULL
);

CREATE TABLE telemetry_coverage (
  harness TEXT NOT NULL,
  capability TEXT NOT NULL,
  state TEXT NOT NULL CHECK (state IN ('observed', 'unknown')),
  reason TEXT NOT NULL,
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
         source_position, summary, evidence, coverage_state
  FROM policy_findings;

CREATE NORMALIZER normalize_harness_event ON otlp_record
USING 'folds/normalize.jsonata'
WRITES telemetry_coverage
EMITS otel_event;

CREATE FOLD update_guard_analytics ON otel_event
READ prior OPTIONAL ONE AS
  SELECT harness, session_id, first_activity_unix_nano,
         last_activity_unix_nano, last_distinct_progress_unix_nano,
         action_fingerprint, repeat_count, no_progress_count,
         consecutive_failures, total_actions, last_source_position
  FROM session_progress
  WHERE harness = :event.harness AND session_id = :event.session_id
USING 'folds/guard.jsonata'
WRITES session_progress, policy_findings, loop_findings;

CREATE EXPORT telemetry_coverage AS
  SELECT harness, capability, state, reason, last_source_position
  FROM telemetry_coverage;

CREATE EXPORT session_progress AS
  SELECT harness, session_id, first_activity_unix_nano,
         last_activity_unix_nano, last_distinct_progress_unix_nano,
         action_fingerprint, repeat_count, no_progress_count,
         consecutive_failures, total_actions, last_source_position
  FROM session_progress;

CREATE EXPORT policy_findings AS
  SELECT finding_id, rule_id, severity, harness, session_id,
         source_position, summary, evidence, coverage_state
  FROM policy_findings;

CREATE EXPORT loop_findings AS
  SELECT finding_id, finding_kind, harness, session_id, source_position,
         repeat_count, consecutive_failures, no_progress_count, evidence
  FROM loop_findings;
