# Agent guard query recipes

`agent-guard` records detective evidence only; it is not inline enforcement.
Missing or redacted tool/target fields create `insufficient-telemetry` findings
with `coverage_state = 'unknown'`, never a compliance claim.

Recent policy evidence:

```sql
SELECT harness, session_id, rule_id, severity, summary, evidence, coverage_state
FROM policy_findings
ORDER BY source_position DESC;
```

Loop evidence:

```sql
SELECT harness, session_id, finding_kind, repeat_count,
       consecutive_failures, no_progress_count, evidence
FROM loop_findings
ORDER BY source_position DESC;
```

A truly idle process emits no event, so stalled detection is a query-time
check. Compute a nanosecond cutoff in the caller and bind it:

```sql
SELECT harness, session_id, last_activity_unix_nano,
       last_distinct_progress_unix_nano, total_actions
FROM session_progress
WHERE last_distinct_progress_unix_nano < ?
ORDER BY last_distinct_progress_unix_nano, harness, session_id;
```

```sh
tailapp query --sql 'SELECT harness, session_id, last_distinct_progress_unix_nano FROM session_progress WHERE last_distinct_progress_unix_nano < ? ORDER BY last_distinct_progress_unix_nano' --param '"1787900000009999999"' agent-guard
```

`loop_findings` retains the latest row per session and finding kind. Policy
violations retain per-position evidence, while insufficient telemetry uses a
stable per-session ID to avoid a missing-field firehose.
