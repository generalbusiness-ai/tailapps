CREATE EVENT otel_event (
  event_family TEXT NOT NULL CHECK (event_family IN ('observation', 'exclusion', 'reputation')),
  day_utc INTEGER NOT NULL,
  event_time_unix_nano TEXT NOT NULL,
  source_position INTEGER NOT NULL,
  observed_full TEXT,
  host TEXT,
  harness TEXT,
  session_id TEXT,
  session_id_prefix TEXT,
  tool_name TEXT,
  project TEXT,
  exclusion_id TEXT,
  exclusion_kind TEXT CHECK (exclusion_kind IN ('host-exact', 'host-suffix', 'url-prefix')),
  exclusion_pattern TEXT,
  exclusion_enabled INTEGER CHECK (exclusion_enabled IN (0, 1)),
  provider TEXT,
  checked_full TEXT,
  verdict TEXT CHECK (verdict IN ('clean', 'suspected', 'error')),
  threat_types TEXT,
  provider_reference TEXT,
  error TEXT,
  checked_unix_nano TEXT,
  valid_until_unix_nano TEXT,
  builtin_ip_literal_id TEXT NOT NULL,
  builtin_localhost_id TEXT NOT NULL,
  builtin_single_label_id TEXT NOT NULL,
  builtin_local_id TEXT NOT NULL,
  builtin_internal_id TEXT NOT NULL,
  builtin_home_arpa_id TEXT NOT NULL,
  builtin_test_id TEXT NOT NULL
);

CREATE TABLE url_observations (
  observed_full TEXT PRIMARY KEY,
  host TEXT NOT NULL,
  first_observed_unix_nano TEXT NOT NULL,
  last_observed_unix_nano TEXT NOT NULL,
  observation_count INTEGER NOT NULL CHECK (observation_count >= 1),
  latest_harness TEXT,
  latest_session_id TEXT,
  latest_session_id_prefix TEXT,
  latest_tool_name TEXT,
  latest_project TEXT,
  last_source_position INTEGER NOT NULL
);

CREATE TABLE url_exclusions (
  exclusion_id TEXT PRIMARY KEY,
  kind TEXT NOT NULL CHECK (kind IN ('host-exact', 'host-suffix', 'url-prefix')),
  pattern TEXT NOT NULL,
  enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
  updated_unix_nano TEXT NOT NULL,
  last_source_position INTEGER NOT NULL
);

CREATE TABLE url_verdicts (
  observed_full TEXT NOT NULL,
  provider TEXT NOT NULL,
  checked_full TEXT NOT NULL,
  verdict TEXT NOT NULL CHECK (verdict IN ('clean', 'suspected', 'error')),
  threat_types TEXT,
  provider_reference TEXT,
  error TEXT,
  checked_unix_nano TEXT NOT NULL,
  valid_until_unix_nano TEXT NOT NULL,
  last_source_position INTEGER NOT NULL,
  PRIMARY KEY (observed_full, provider)
);

CREATE TABLE url_pipeline_counts (
  day_utc INTEGER NOT NULL,
  event_family TEXT NOT NULL CHECK (event_family IN ('observation', 'exclusion', 'reputation')),
  record_count INTEGER NOT NULL CHECK (record_count >= 1),
  error_count INTEGER NOT NULL CHECK (error_count >= 0),
  first_event_time_unix_nano TEXT NOT NULL,
  last_event_time_unix_nano TEXT NOT NULL,
  last_source_position INTEGER NOT NULL,
  PRIMARY KEY (day_utc, event_family)
);

CREATE INDEX url_observations_due_order
  ON url_observations(last_observed_unix_nano, observed_full);

CREATE INDEX url_verdicts_provider_freshness
  ON url_verdicts(provider, valid_until_unix_nano, observed_full);

CREATE NORMALIZER normalize_url_pipeline ON otlp_record
USING 'folds/normalize.jsonata'
EMITS otel_event;

CREATE FOLD update_url_pipeline ON otel_event
READ observation_prior OPTIONAL ONE AS
  SELECT observed_full, host, first_observed_unix_nano,
         last_observed_unix_nano, observation_count, latest_harness,
         latest_session_id, latest_session_id_prefix, latest_tool_name,
         latest_project, last_source_position
  FROM url_observations
  WHERE observed_full = :event.observed_full
READ exclusion_prior OPTIONAL ONE AS
  SELECT exclusion_id, kind, pattern, enabled, updated_unix_nano,
         last_source_position
  FROM url_exclusions
  WHERE exclusion_id = :event.exclusion_id
READ verdict_prior OPTIONAL ONE AS
  SELECT observed_full, provider, checked_full, verdict, threat_types,
         provider_reference, error, checked_unix_nano,
         valid_until_unix_nano, last_source_position
  FROM url_verdicts
  WHERE observed_full = :event.observed_full AND provider = :event.provider
READ count_prior OPTIONAL ONE AS
  SELECT day_utc, event_family, record_count, error_count,
         first_event_time_unix_nano, last_event_time_unix_nano,
         last_source_position
  FROM url_pipeline_counts
  WHERE day_utc = :event.day_utc AND event_family = :event.event_family
READ builtin_ip_literal_prior OPTIONAL ONE AS
  SELECT exclusion_id, kind, pattern, enabled, updated_unix_nano,
         last_source_position
  FROM url_exclusions
  WHERE exclusion_id = :event.builtin_ip_literal_id
READ builtin_localhost_prior OPTIONAL ONE AS
  SELECT exclusion_id, kind, pattern, enabled, updated_unix_nano,
         last_source_position
  FROM url_exclusions
  WHERE exclusion_id = :event.builtin_localhost_id
READ builtin_single_label_prior OPTIONAL ONE AS
  SELECT exclusion_id, kind, pattern, enabled, updated_unix_nano,
         last_source_position
  FROM url_exclusions
  WHERE exclusion_id = :event.builtin_single_label_id
READ builtin_local_prior OPTIONAL ONE AS
  SELECT exclusion_id, kind, pattern, enabled, updated_unix_nano,
         last_source_position
  FROM url_exclusions
  WHERE exclusion_id = :event.builtin_local_id
READ builtin_internal_prior OPTIONAL ONE AS
  SELECT exclusion_id, kind, pattern, enabled, updated_unix_nano,
         last_source_position
  FROM url_exclusions
  WHERE exclusion_id = :event.builtin_internal_id
READ builtin_home_arpa_prior OPTIONAL ONE AS
  SELECT exclusion_id, kind, pattern, enabled, updated_unix_nano,
         last_source_position
  FROM url_exclusions
  WHERE exclusion_id = :event.builtin_home_arpa_id
READ builtin_test_prior OPTIONAL ONE AS
  SELECT exclusion_id, kind, pattern, enabled, updated_unix_nano,
         last_source_position
  FROM url_exclusions
  WHERE exclusion_id = :event.builtin_test_id
USING 'folds/pipeline.jsonata'
WRITES url_observations, url_exclusions, url_verdicts, url_pipeline_counts;

CREATE EXPORT url_observations AS
  SELECT observed_full, host, first_observed_unix_nano,
         last_observed_unix_nano, observation_count, latest_harness,
         latest_session_id, latest_session_id_prefix, latest_tool_name,
         latest_project, last_source_position
  FROM url_observations;

CREATE EXPORT url_exclusions AS
  SELECT exclusion_id, kind, pattern, enabled, updated_unix_nano,
         last_source_position
  FROM url_exclusions;

CREATE EXPORT url_verdicts AS
  SELECT observed_full, provider, checked_full, verdict, threat_types,
         provider_reference, error, checked_unix_nano,
         valid_until_unix_nano, last_source_position
  FROM url_verdicts;

CREATE EXPORT url_pipeline_counts AS
  SELECT day_utc, event_family, record_count, error_count,
         first_event_time_unix_nano, last_event_time_unix_nano,
         last_source_position
  FROM url_pipeline_counts;
