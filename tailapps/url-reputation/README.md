# URL reputation

`url-reputation` retains exact agent-fetched URLs, operator exclusions, and
provider verdicts from three explicit OTLP log families. It makes the local
state queryable; this Stage 2 Tailapp performs no network requests and sends no
data anywhere.

Install it explicitly. Installation first-activates the new Tailapp with reset
semantics:

```sh
tailapp apps install --bundle url-reputation \
  --idempotency-key install-url-reputation-v1 url-reputation
```

## Inputs

Only logs named `tailapp.url.observed`, `tailapp.url.exclusion`, or
`tailapp.url.reputation` are effective. Nanosecond timestamps are decimal
strings.

`tailapp.url.observed` requires:

- `tailapp.url.observed_full`: an absolute HTTP(S) URL retained exactly;
- `tailapp.url.host`: the parsed host. Its lowercase value must immediately
  follow the lowercase scheme and `://`, followed by end-of-string, `/`, `:`,
  `?`, or `#`.

It may also carry `session.id` or `conversation.id`, `tool_name`, `project`,
and `cwd`. The OTLP envelope timestamp is the observation time.

`tailapp.url.exclusion` requires:

- `tailapp.url.exclusion.id`;
- `tailapp.url.exclusion.kind`: `host-exact`, `host-suffix`, or `url-prefix`;
- `tailapp.url.exclusion.pattern`;
- `tailapp.url.exclusion.enabled`: a boolean.

The latest event for an exclusion ID wins. Host patterns are lowercased; URL
prefixes retain their spelling.

`tailapp.url.reputation` requires:

- `tailapp.url.observed_full`, `tailapp.url.checked_full`, and
  `tailapp.url.provider`;
- `tailapp.url.verdict`: `clean`, `suspected`, or `error`;
- `tailapp.url.checked_unix_nano` and
  `tailapp.url.valid_until_unix_nano`.

It may carry `tailapp.url.threat_types` as a JSON-array string,
`tailapp.url.provider_reference`, and `tailapp.url.error`.

## Results

- `url_observations` has one row per exact URL, with its host, first and last
  observation time, count, latest harness/session/tool/project context, and
  source position.
- `url_exclusions` has the latest enabled state, kind, pattern, update time,
  and source position for each operator or built-in rule.
- `url_verdicts` has the latest verdict for each exact URL and provider,
  including the checked URL, expiry, threats, reference, error, and source
  position.
- `url_pipeline_counts` counts observations, exclusion updates, reputation
  records, and error verdicts by UTC day.

The fold seeds stable enabled built-in policy rows for IP literals,
`localhost`, single-label hosts, `.local`, `.internal`, `.home.arpa`, and
`.test`. An operator update to one of those IDs is preserved; later events do
not silently restore it.

## Find work due for a provider

The following bounded query returns observations with no fresh verdict from
the selected provider. Pass the provider and current Unix time in nanoseconds:

```sh
tailapp query --sql '
  SELECT o.observed_full, o.host, o.last_observed_unix_nano,
         o.latest_harness, o.latest_session_id_prefix, o.latest_tool_name,
         o.latest_project
  FROM url_observations AS o
  LEFT JOIN url_verdicts AS v
    ON v.observed_full = o.observed_full AND v.provider = ?
  WHERE v.observed_full IS NULL
     OR CAST(v.valid_until_unix_nano AS INTEGER) <= CAST(? AS INTEGER)
  ORDER BY o.last_observed_unix_nano, o.observed_full
  LIMIT 200' \
  --param '"safe-browsing-v5"' --param '1788192000000000000' \
  url-reputation
```

This list is intentionally **pre-exclusion**. Query enabled policy separately:

```sh
tailapp query --sql '
  SELECT exclusion_id, kind, pattern, updated_unix_nano
  FROM url_exclusions
  WHERE enabled = 1
  ORDER BY kind, pattern, exclusion_id
  LIMIT 200' url-reputation
```

The Stage 3 companion is the mandatory enforcement point. It must apply every
enabled exact-host, host-suffix, URL-prefix, and built-in rule immediately
before provider egress, even if it checked the same URL earlier in a run. The
due query makes no claim that an included URL is eligible to leave the machine.

## Retention, privacy, and reset

Full local URLs and project/session context are retained intentionally for
same-user investigation. Stage 2 has no egress. The later companion owns
provider eligibility, exclusion enforcement, credential handling, and URL
scrubbing.

History is compacted by key but is not time-pruned. Reset activation discards
observations, verdicts, counts, and custom exclusions together; re-emit custom
exclusions afterward. Continue activation preserves existing rows and begins
new semantics at the activation boundary.
