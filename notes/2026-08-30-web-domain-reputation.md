---
date: 2026-08-30
status: discussion; revised the same day to the URL-level loopback design;
  no decision, no implementation
---

# Web reputation review, discussed

The operator's question: does Tailapp capture the URLs coding agents fetch
with web tools, and could the daily-review cron check what an agent
visited against a reputation service (Google Safe Browsing or VirusTotal)?
This note records the discussion. It was revised the same day: the first
design materialized only registrable domains, and the operator redirected
it to individual URLs with a loopback pipeline (see
[Design history](#design-history)). Nothing here is decided or tasked.

## What the pipeline does today, verified

URLs are absent from the analytics by two deliberate layers:

1. **The harness redacts them.** Claude Code's telemetry carries only
   `tool_name` and `success` on tool results. Identifying detail (file
   paths, commands, and URLs for web tools) flows only with
   `OTEL_LOG_TOOL_DETAILS=1`, raw content only with
   `OTEL_LOG_TOOL_CONTENT=1`; the documented recommendation leaves both
   unset. A default-configured WebFetch appears as "a tool named WebFetch
   ran, and succeeded" — agent-guard records `target_coverage: unknown`
   for exactly this case.
2. **Nothing retains them.** The shipped normalizers refuse to parse raw
   `tool_input` structures, the inbox does not keep canonical records
   after projections commit, and daily-review counts sensitive-content
   presence without retaining values. There is no URL inventory anywhere
   to scan.

## The loopback design

The operator's revised direction: domain-level reputation is not granular
enough for useful checks — Safe Browsing in particular verdicts canonical
URLs, and path-specific threats vanish at domain granularity — so the
pipeline tracks **individual URLs**. And the pieces the first design kept
outside the engine (the seen-URL cache, the exclusion configuration, the
verdicts) fold into the pipeline itself, because they are all just
OTLP-flavored inputs:

**One Tailapp, three input families, one loopback.**

1. **URL observations** from harness telemetry: web-tool events carrying
   the fetched URL (a dedicated attribute — reusing the guard's shared
   `target` attribute remains rejected, since the guard copies `target`
   into durable policy evidence and action fingerprints, and every
   concurrently installed consumer of a shared attribute must be accounted
   for).
2. **Exclusion records**: the exclusion list itself arrives as OTLP-
   flavored records (operator- or agent-emitted) and materializes into an
   exclusions table — suffixes or patterns for internal names whose very
   existence should not reach a third-party service, on top of
   no-configuration defaults (loopback and private-range literals,
   `localhost`, single-label names, `.local`, `.internal`, `.home.arpa`,
   `.test`).
3. **Reputation reports**: the assessing agent queries the reputation
   service and emits the verdicts back as OTLP-flavored `url-reputation`
   records. The pipeline materializes them next to the observations, so
   "which URLs need (re)checking" is an ordinary query — observed URLs
   minus exclusions minus fresh verdicts — and the materialized state
   stays consistent over long periods with **no side-cache** of checked
   URLs anywhere.

The daily-review cron instructions then: query for unchecked, non-excluded
URLs; call **either Google Safe Browsing or VirusTotal** through an
**example companion script** shipped alongside the instructions (the
script takes URLs, performs the lookup, and emits `url-reputation` OTLP
records to the resident); and report new hits. The scan layer remains
instructions-plus-script — no engine code, no fold logic for scanning.

**Trust model, stated plainly.** There is no spoofability, authority, or
trust boundary of interest inside this pipeline: every OTLP source is
local, and a harness log carrying a URL is not significantly distinct from
an agent script sending a `url-reputation` record. A local process that
could forge reputation records could as easily forge the observations —
or read the database. That is an accepted shape of this application under
the locally-deployed, single-user target model, and it matches the
engine's existing boundary statement (loopback-only receiver, no
authentication, owner-only control socket).

**Why this pipeline earns its complexity.** It is the most complex Tailapp
shape yet — a feedback loop where the pipeline's own reviewer feeds
derived records back in — and it deliberately demonstrates a pattern the
existing apps do not: **Tailapp inputs need not be agent-harness
instrumentation**. Exclusion configuration, enrichment feeds, and review
verdicts are all legitimate OTLP-flavored inputs, and the two-stage
topology materializes them like anything else.

## Privacy posture under URL-level tracking

Retaining full URLs is a real posture change from daily-review's
aggregate-only philosophy, accepted for the local target model with two
standing guards:

- **At-rest retention is local; transmission is not.** Full URLs (which
  routinely embed tokens, session identifiers, and signed parameters) are
  stored only in the local projection — but the design then sends the
  selected, non-excluded URLs to the chosen reputation service, so
  path-and-query secrets in those URLs **leave the machine** unless a
  separate redaction rule strips them first. The internal-name exclusions
  do not provide that redaction: they filter which hosts are checked at
  all, not what a checked URL carries. Whether a secret-stripping rule
  (drop query strings, or a deny-pattern for token-shaped parameters)
  belongs in the companion script is an open question below. The note
  keeps its warning: query results are sensitive; share aggregates.
- **Egress is checked twice.** The exclusions table filters what the
  review queries surface for checking, and the companion script applies
  the same exclusions again before any URL leaves the machine for the
  reputation service. The eligibility constraints stand: Safe Browsing's
  free API is non-commercial (commercial use goes to Web Risk;
  https://developers.google.com/safe-browsing/reference/Appropriate.Usage)
  and VirusTotal's public API prohibits routine commercial
  report-retrieval workflows
  (https://docs.virustotal.com/reference/public-vs-premium-api) — the
  licensing call is the operator's, recorded when the instructions are
  written. URL-level checking is exactly what Safe Browsing's canonical-
  URL and hash-prefix flows are built for
  (https://developers.google.com/safe-browsing/v4/urls-hashing), removing
  the first design's synthesized-root-URL loss.

## Open questions

1. Capture route: `OTEL_LOG_TOOL_DETAILS` (broad, with its data-policy
   cost) or a narrow adapter emitting the URL as a dedicated attribute?
   Operator policy; the adapter needs a home and a maintainer.
2. The private-event and table shapes for the three input families, and
   whether one normalizer discriminating three record kinds stays within
   the two-stage topology comfortably. Each table has exactly one writing
   program and sibling folds cannot read each other's tables, so either
   one owning program (the normalizer, or a single fold) materializes all
   three tables, or separate writer-owned tables compose only at
   review-query time through their exports.
3. Verdict freshness policy: how old a verdict may be before rechecking,
   and whether it lives in the review instructions (flexible) or a
   materialized column (queryable).
4. The companion script's home, its OTLP record schema for
   `url-reputation` and exclusion records, and its key handling for the
   chosen service.
5. Retention duration for URL rows, and whether activation-reset semantics
   suffice for pruning.
6. Whether the companion script strips path-and-query secrets before
   transmission (drop query strings entirely — at some verdict-precision
   cost — or deny token-shaped parameters), the separate redaction rule
   the at-rest-versus-transmission distinction requires.

## Design history

The first design (earlier the same day) materialized only registrable
domains — never full URLs — with public-suffix derivation in a
domain-minimizing adapter, a cron-side seen-domain cache, and exclusion
patterns at materialization and egress. Review hardened it (service
eligibility terms, the shared-attribute leak, Safe Browsing's URL-not-
domain semantics), and the operator then redirected: domain granularity is
too coarse for useful verdicts, and the cache, exclusions, and verdicts
belong inside the pipeline as OTLP inputs. The domain-only retention
posture and the side-cache are superseded by the loopback design above.
The shared-attribute rejection and the eligibility constraints carry
forward unchanged; the exclusion mechanism carries forward only as a
principle — two checks before egress — while its first layer **moved**:
the first design excluded internal names before materialization so they
were never retained, whereas the loopback design retains internal URLs
locally like any others and applies the exclusions at review-query time
and again in the companion script.

## Status

Discussion only. Implementation would follow the normal flow: an adopted
decision resolving the open questions, then staged, reviewed work — the
capture gate and service licensing being the operator's calls throughout.
