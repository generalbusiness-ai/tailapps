---
date: 2026-08-31
status: ratified stage-1 decisions; stage 2 may begin
basis: notes/2026-08-30-web-domain-reputation.md
---

# URL reputation loopback: stage 1 decisions

This note resolves the six design questions needed before implementation. It
keeps the feature entirely telemetry-based: the Tailapp consumes OTLP records,
the companion emits OTLP records, and neither reads an agent harness's local
session store.

The operator ratified the capture route, provider posture, and egress policy
on 2026-08-31 in assert `c9c7fa4c425f498668c976c026e5a8e1d3089db3`.
No engine or runtime-profile change is part of this feature.

## 1. Capture and provider choices

### Capture route

| Option | Consequence |
|---|---|
| Narrow adapter (recommended) | Emit `tailapp.url.observed` only for completed web-tool fetches, with the exact URL in the dedicated `tailapp.url.observed_full` attribute and the parsed host in `tailapp.url.host`. This avoids enabling unrelated tool detail and gives the Tailapp a stable input contract. |
| Harness detail gate | Enable the harness's broad tool-detail telemetry and normalize known WebFetch/WebSearch argument shapes. This is faster to bootstrap but exposes every tool's detailed arguments and couples the bundle to vendor payload shapes. |

The recommended route is the narrow adapter. The custom attribute is
intentional: generic `target` remains rejected because other installed
Tailapps consume it, while generic HTTP `url.full` records do not prove an
agent web tool actually fetched the URL. The adapter may also emit sanitized
standard URL attributes, but `tailapp.url.observed_full` is the authoritative
local value for this application.

The adapter accepts absolute `http` and `https` URLs only. It emits the URL
before redirect and one additional observation for every redirect target it
can observe. Session and harness identifiers are copied when telemetry supplies
them and marked unknown otherwise.

This differs intentionally from OpenTelemetry's portable `url.full` posture:
the custom attribute retains the exact locally observed URL for same-user local
analysis. The companion, not the stored observation, owns egress scrubbing.
OpenTelemetry's URL conventions remain the guide for parsed components and
sensitive-value handling: <https://opentelemetry.io/docs/specs/semconv/registry/attributes/url/>.

### Reputation provider

| Option | Eligibility and operational shape |
|---|---|
| Google Safe Browsing v5 (default) | The default for this separately run, user-instruction companion. It is explicitly non-commercial: the person running each installation must confirm that eligibility before it sends a URL. It accepts up to 50 URLs per search and returns `cacheDuration`; its warning/attribution requirements apply when results are presented to users. |
| Google Web Risk Lookup | Use when commercial use cannot be ruled out; one URL per Lookup request, the client need not canonicalize for provider matching, and the response carries threat types and cache expiry. Client-side exclusion and secret scrubbing remain mandatory before the request. Current pricing includes a monthly free Lookup allowance but pricing must not be baked into behavior. |
| VirusTotal | Use only with a Premium license that covers automated enrichment. Its Public API is not a default: it forbids commercial products/services and business report-retrieval workflows and has strict daily/rate limits. |

Safe Browsing v5 is the ratified default only after the runner confirms
non-commercial eligibility for that installation. When commercial use cannot
be ruled out, the runner must select Web Risk Lookup instead. VirusTotal is an
explicit Premium-only override, never an automatic fallback. The companion
requires exactly one configured provider and refuses to send anything when the
choice, eligibility confirmation, or credential is absent.

Current primary references, checked 2026-08-31:

- Safe Browsing permitted use: <https://developers.google.com/safe-browsing/reference/Appropriate.Usage>
- Safe Browsing v5 URL search and cache rules: <https://developers.google.com/safe-browsing/reference/rest/v5/urls/search>
- Web Risk Lookup: <https://cloud.google.com/web-risk/docs/lookup-api>
- Web Risk pricing: <https://cloud.google.com/web-risk/pricing>
- VirusTotal Public versus Premium API: <https://docs.virustotal.com/reference/public-vs-premium-api>

### Reference capture adapter

Stage 3 includes a project-maintained reference adapter under
`scripts/url-capture/`; capture is not left to an unspecified deployment. Its
shared executable accepts one documented harness hook record on standard input,
extracts absolute HTTP(S) URLs only from a fixture-backed tool shape, verifies
that the supplied host equals the host it parses from each URL, and posts one
`tailapp.url.observed` OTLP/HTTP log record per URL to the resident. It does not
read transcripts, session databases, or any other harness-local store.

The shipped harness integrations are deliberately explicit:

- **Claude Code:** a `PostToolUse` command hook matched to `WebFetch|WebSearch`.
  `WebFetch` uses `tool_input.url`; `WebSearch` result URLs are supported only
  for the exact scrubbed `tool_response` shape captured in the fixture. Claude
  Code documents that successful tool hooks receive `tool_name`, `tool_input`,
  and `tool_response`: <https://code.claude.com/docs/en/hooks>.
- **Codex:** a `PostToolUse` command hook for configured local function or MCP
  fetch tools, with one explicit argument mapping per fixture-backed tool name.
  Codex documents the hook input and also states that hosted tools such as
  `WebSearch` do not traverse the local tool-hook path, so the reference adapter
  does **not** claim native hosted-WebSearch coverage:
  <https://learn.chatgpt.com/docs/hooks>.
- **OpenCode:** a small v2 plugin registers `execute.after`, selects only the
  exact fixture-backed web-fetch tool identities, and forwards their completed
  input to the shared adapter. The v2 plugin API documents successful and
  failed after-execution hooks: <https://opencode.ai/v2/docs/build/plugins/>.

Each shim ships as reviewed source with installation instructions and a pinned
compatibility profile. The Tailapps project maintains these reference profiles;
unknown tool names or changed input shapes are ignored and surfaced as missing
capture coverage rather than guessed. The broad harness detail gate remains a
documented bootstrap alternative, not an implicit fallback.

## 2. Event, table, and writer shapes

One normalizer discriminates the three event families and is the sole writer
of all three materialized tables. It emits one compact private
`url_pipeline_event` for recognized inputs; one fold is the sole writer of a
small `url_pipeline_counts` table. This fits the existing two-stage topology,
keeps every table single-writer, and requires no sibling-fold reads.

### OTLP log families

`tailapp.url.observed`

- required: `tailapp.url.observed_full`, `tailapp.url.host`
- optional: `session.id`, `conversation.id`, `tool_name`, `project`, `cwd`
- envelope timestamp: observation time

`tailapp.url.exclusion`

- required: `tailapp.url.exclusion.id`, `tailapp.url.exclusion.kind`,
  `tailapp.url.exclusion.pattern`, `tailapp.url.exclusion.enabled`
- kinds: `host-exact`, `host-suffix`, `url-prefix`
- envelope timestamp: update time; the latest record for an ID wins

`tailapp.url.reputation`

- required: `tailapp.url.observed_full`, `tailapp.url.checked_full`,
  `tailapp.url.provider`, `tailapp.url.verdict`,
  `tailapp.url.checked_unix_nano`, `tailapp.url.valid_until_unix_nano`
- optional: `tailapp.url.threat_types` as a JSON array string,
  `tailapp.url.provider_reference`, `tailapp.url.error`
- verdicts: `clean`, `suspected`, `error`

The record schemas intentionally echo the original local URL in a reputation
record. That lets a scrubbed outbound URL attach its verdict to the exact local
observation without a new hash primitive or a side cache.

For every observation, the normalizer reparses `observed_full` and refuses the
record if `tailapp.url.host` differs from the parsed lowercase host. The adapter
cannot desynchronize host-based exclusions by supplying inconsistent fields.

### Tables and keys

`url_observations`

- primary key: `observed_full`
- retained: parsed host, first/last observation timestamps, count, latest
  harness/session/session-prefix/tool/project, and source position

`url_exclusions`

- primary key: `exclusion_id`
- retained: kind, pattern, enabled state, update timestamp, source position
- the normalizer seeds stable built-in IDs for loopback/private IP literals,
  `localhost`, single-label hosts, `.local`, `.internal`, `.home.arpa`, and
  `.test` only when each ID is absent; recognized events do not rewrite rows
  that are already present

`url_verdicts`

- primary key: `(observed_full, provider)`
- retained: checked URL, verdict, threat types, provider reference/error,
  checked/valid-until timestamps, source position

`url_pipeline_counts`

- primary key: `(day_utc, event_family)`
- retained: record count, errors, first/last timestamps, source position

All four tables have explicit exports. A shipped bounded query selects
observations whose host does not match an enabled exclusion and whose chosen
provider has no verdict with `valid_until_unix_nano > :now_unix_nano`. The
companion fetches enabled exclusions separately and repeats the same check
before egress. Full URLs remain available to local queries.

## 3. Verdict freshness

Freshness is stored per verdict, not hidden in cron instructions. The
companion emits both `checked_unix_nano` and `valid_until_unix_nano`.

- Safe Browsing uses the returned `cacheDuration`; a clean-result extension is
  capped at 24 hours as required by the v5 API.
- Web Risk uses its returned `expireTime`.
- Providers without an authoritative expiry use a configurable TTL whose
  default is 24 hours and maximum is 7 days.
- `error` verdicts use a five-minute retry expiry and retain the error text.

The review query receives `now_unix_nano` from its caller. Tailapp programs do
not read a wall clock, preserving deterministic interpretation.

## 4. Companion, schema, and credentials

The companion lives at `scripts/url-reputation/` with a small executable and a
README. Its responsibilities are narrow:

1. query due observations and current exclusions through the local Tailapp;
2. reapply built-in and materialized exclusions;
3. scrub each outbound URL;
4. deduplicate equal scrubbed `checked_full` values within the run, call the
   ratified provider once per distinct checked URL with bounded concurrency and
   provider-specific rate handling, then fan the result back out as one
   reputation record per exact local observation; and
5. POST OTLP/HTTP JSON log records to the resident's loopback `/v1/logs`
   endpoint.

OTLP/HTTP uses the standard `ExportLogsServiceRequest` JSON shape and
`application/json`; the protocol's default logs path is `/v1/logs`:
<https://opentelemetry.io/docs/specs/otlp/>.

Credentials come only from provider-specific environment variables:
`TAILAPP_WEBRISK_API_KEY`, `TAILAPP_SAFE_BROWSING_API_KEY`, or
`TAILAPP_VIRUSTOTAL_API_KEY`. They are never accepted on the command line,
written to config, logged, or included in OTLP. The script scrubs provider
request URLs before reporting failures so a query-string API key cannot enter
telemetry.

The companion README will name Safe Browsing v5 as the default and require the
runner to confirm its non-commercial eligibility for each installation before
the default provider can be used. It will direct any uncertain or commercial
installation to Web Risk Lookup, and will describe VirusTotal as Premium-only.

## 5. Retention and reset

Stage 2 retains one row per distinct exact URL until reset. This is explicit,
not an accidental absence of pruning. At the expected single-user scale it is
bounded by distinct URLs rather than event count, while observation counts and
timestamps update in place.

Activation reset is sufficient for the first release and is the only pruning
mechanism. It discards observations, verdicts, and operator exclusions
together; the companion README must instruct the operator to re-emit custom
exclusions after a reset. Continue activation preserves existing rows and
starts changed semantics at its boundary. A future retention feature should
use explicit telemetry tombstones, not ambient-clock deletion inside a fold.

Keeping operator exclusions in a separate mounted Tailapp would let an
operator reset URL history without losing policy, but it adds a second
lifecycle and cross-projection frontier to the first release. Stage 2 therefore
keeps exclusions together and makes the reset trade-off visible rather than
silently adding that complexity.

## 6. Egress scrubbing

The default is scrubbing on. Before provider lookup the companion:

- removes URL user information and fragments;
- removes the complete query string;
- replaces path segments matching high-confidence secret forms (JWTs, common
  token prefixes, or long hex/base64url values) with a fixed `REDACTED`
  segment; and
- refuses any URL that no longer parses as absolute HTTP(S).

This costs verdict precision: query-specific threats disappear, and a redacted
path may not match a path-specific threat. The local table still retains the
exact URL so the report can say what was observed. An operator may explicitly
choose `path-preserving` mode, which still removes user information, fragment,
and the complete query but does not redact path segments. There is no mode that
sends query strings in the first release.

Exclusions and scrubbing solve different problems. Exclusions prevent selected
hosts from leaving the machine at all; scrubbing limits secret disclosure in a
URL that is otherwise eligible. Both checks are mandatory immediately before
network egress.

## Staged implementation

### Stage 2: Tailapp source

- Add the `url-reputation` built-in bundle, its three-family normalizer,
  single-writer tables, counts fold, exports, and due-query documentation.
- Add representative observed, exclusion, clean, suspected, error, expiry,
  disabled-rule, built-in-exclusion, and missing-field fixtures.
- Add embed/compile, normalization, fold, query, continue-compatibility, and
  bounded-performance tests.
- Do not change RuntimeID, canonicalization, evaluator confinement, storage,
  transport, or any existing bundle.

### Stage 3: companion and scrubbed end-to-end path

- Add `scripts/url-capture/` with the shared hook-to-OTLP adapter, Claude Code
  and Codex command-hook configurations, the OpenCode v2 plugin, compatibility
  fixtures, installation docs, and explicit unsupported-coverage reporting.
- Add `scripts/url-reputation/` with provider adapters, strict egress scrubber,
  environment-only key handling, bounded batching/rate behavior, and OTLP JSON
  emission.
- Add adapter tests from each captured hook input through emitted OTLP and the
  normalizer to an exact `url_observations` row. Add a fake-provider test and a
  loopback end-to-end test proving real hook fixture -> observation -> due query
  -> deduplicated scrubbed lookup -> reputation OTLP -> fresh verdict.
- Assert that excluded URLs, credentials, fragments, queries, raw API keys,
  and recognized path secrets never reach the fake provider or emitted error
  telemetry.
- Keep the capture adapter separately deployable from the provider companion;
  document the broad harness detail gate only as a bootstrap alternative.

### Stage 4: review integration and catalog

- Add the bundle to the built-in catalog and install docs.
- Extend daily-review instructions with due-query, companion invocation,
  suspected-hit reporting, coverage, and reset caveats. They must name Safe
  Browsing v5 as the default, require the runner's per-install non-commercial
  eligibility confirmation, direct uncertain or commercial use to Web Risk,
  and describe VirusTotal as Premium-only.
- Add concise first-encounter MCP resource links for the bundle and companion.
- Verify all existing bundles and their exports remain unchanged.

## Ratification gate

The operator ratified all three gates on 2026-08-31 in assert
`c9c7fa4c425f498668c976c026e5a8e1d3089db3`:

1. the narrow dedicated adapter rather than the broad harness detail gate;
2. Safe Browsing v5 as the default with per-install non-commercial eligibility
   confirmation; Web Risk when commercial use cannot be ruled out; and
   VirusTotal only under a suitable Premium license; and
3. mandatory query removal plus high-confidence path-secret redaction before
   egress.

Stage 2 may proceed under these choices.
