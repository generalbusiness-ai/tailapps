# Daily review

`daily-review` provides exact UTC-day aggregates for routine operational and
security reviews. It counts every timestamped OTLP record while retaining no
raw attribute values, prompts, responses, tool arguments, commands, paths,
account identifiers, or event names.

For each day and normalized harness it records signal volume, recognized tool
and API events, outcomes, observed duration and slow-event counts, token and
reported-cost totals, missing session identity, and presence-only indicators
for identity/auth metadata, raw prompt content, raw tool content, and the
reference guard's small risky-action vocabulary.

The presence indicators mean that a source record contained a sensitive field;
they do not copy its value. This is useful for detecting telemetry privacy
regressions even when the source records are otherwise outside the shipped
analytics vocabularies.

`daily-review` is not a built-in bundle; install and first-activate it from
its source directory in one request:

```sh
tailapp apps install \
  --idempotency-key install-daily-review-v1 \
  daily-review tailapps/daily-review
```

Then query the exported `daily_review` table:

```sql
SELECT day_utc, harness, record_count, tool_event_count, api_event_count,
       failure_count, duration_gt_5000ms_count, input_tokens,
       cached_input_tokens, output_tokens, reasoning_output_tokens,
       cost_microusd / 1000000.0 AS cost_usd,
       identity_metadata_count, auth_metadata_count,
       raw_prompt_content_count, raw_tool_content_count, risky_action_count
FROM daily_review
ORDER BY day_utc DESC, harness;
```

The Tailapp starts at its installation boundary because Tailapp does not retain
an event log for replay. Compare multiple complete days after installation;
the first day is necessarily partial.
