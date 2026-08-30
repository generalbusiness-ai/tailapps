---
date: 2026-08-30
status: discussion; no decision, no implementation
---

# Web-domain reputation review, discussed

The operator's question: does Tailapp capture the URLs coding agents fetch
with web tools, and could the daily-review cron check the domains an agent
visited against a reputation service (VirusTotal, Cisco Umbrella)? This
note records the discussion, the verified current state, a candidate
design, and the privacy considerations — including the operator's follow-on
requirement that internal URLs be excluded so private service details are
not leaked. Nothing here is decided or tasked.

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
   `tool_input` structures (they only promote a pre-adapted safe `target`
   attribute), the inbox does not keep canonical records after projections
   commit, and daily-review counts sensitive-content presence without
   retaining values. There is no domain inventory anywhere to scan.

## Candidate design

The shape splits cleanly across the existing trust boundary:

**Capture** (operator policy). Either enable `OTEL_LOG_TOOL_DETAILS=1` —
which also exposes file paths and full shell commands — or a small adapter
that emits a **dedicated, already-minimized attribute** (the registrable
domain, post-exclusion) that only the domains Tailapp consumes. An earlier
draft of this note suggested promoting the full URL into the guard's
existing `target` attribute; review showed that route is not safe as
described: the shipped guard copies `target` verbatim into durable policy
evidence and into `session_progress.action_fingerprint`, so every
concurrently installed consumer of `target` would retain full URLs —
internal names, tokens and all — defeating the domain-only retention
promise. A dedicated minimized attribute keeps the guard's vocabulary
untouched; any route that reuses a shared attribute must first account for
every installed consumer of it.

**Materialize** (a Tailapp). A new Tailapp (or a daily-review companion)
whose normalizer recognizes web-tool events and upserts a per-UTC-day table
of **registrable domains with counts** — never the full URL, since URLs
routinely embed tokens, session identifiers, and signed parameters. This
matches daily-review's retention philosophy: aggregate facts, no raw
values. First-seen/last-seen day columns give the review its "new domain"
signal cheaply.

**Scan** (the daily-review cron agent, instructions only). The operator
scoped this layer explicitly: the reputation check lives entirely in the
cron agent's instructions — no engine code, no fold logic, no new tooling —
and the service is **either Google Safe Browsing or VirusTotal**. The
instructions query the domain table, diff against a local seen-domain
cache, apply the exclusion patterns, and submit only new external domains
to the chosen service, reporting hits in the daily review.

Two constraints on that choice, from the services' own terms and protocol:

- **Licensing is an explicit operator decision, not a default.** Safe
  Browsing's free API is restricted to non-commercial use; commercial use
  is directed to the paid Web Risk API
  (https://developers.google.com/safe-browsing/reference/Appropriate.Usage).
  VirusTotal's public API prohibits commercial use and business workflows
  that only retrieve reports — which is a fair description of a routine
  domain-enrichment cron
  (https://docs.virustotal.com/reference/public-vs-premium-api). Depending
  on the operator's context, the compliant options may be Web Risk or a VT
  premium entitlement; the eligibility call is recorded when the cron
  instructions are written.
- **Safe Browsing checks canonical URLs, not domain reputation.** Its
  Lookup API receives URLs, and the hash-prefix flow derives expressions
  from host plus path and query
  (https://developers.google.com/safe-browsing/v4/urls-hashing). A
  registrable-domain-only table can at most check a synthesized root URL
  (`https://<domain>/`), which misses path-specific threats — an accepted
  loss under the domain-only retention posture, and it should be stated in
  the review output. VirusTotal's domain reports match the domain-only
  representation directly (categories, resolutions, vendor verdicts) but
  carry the eligibility constraint above and free-tier rate limits.
  Retaining more than the domain to close the Safe Browsing gap would
  reopen the URL-retention privacy question and is not proposed here.

## Excluding internal URLs

The operator's added consideration: routine scanning must not leak private
service details (internal hostnames, single-label service names, corporate
domains) to a third-party reputation service. Directions discussed:

- **Exclude at the earliest layer, then again at egress.** The strongest
  posture excludes internal names *before materialization* — the domain
  table simply never holds them, so no later component can leak what was
  never retained — and the cron applies the same exclusion again before
  submission, as defense in depth against a stale or divergent table.
- **Default exclusions that need no configuration**: loopback and RFC 1918
  / RFC 4193 literals, `localhost`, single-label hostnames, `.local`,
  `.internal`, `.home.arpa`, `.test`, and anything that fails public-suffix
  registrable-domain derivation. These cover most private-service shapes
  before any operator setup.
- **Operator exclusion patterns** for the rest (the corporate domain, VPN
  suffixes): a small exclusion list — exact suffixes or a regex — but its
  *location* is a real design question. In the Tailapp source it becomes
  reviewable, versioned configuration but requires a source update to
  change; in the cron agent's config it is easy to change but only guards
  the egress layer. Both layers carrying it (source-versioned suffix list
  for materialization, agent config for egress) was the position the
  discussion leaned toward.
- An exclusion list is itself mildly sensitive (it names the private
  domains); keeping the materialization-layer list in the Tailapp source
  means it lands in the repository. If that is unacceptable, the
  materialization layer can carry only the no-configuration defaults and
  the operator patterns can live solely at the cron layer, trading some
  retention minimization for repository hygiene.

## Open questions

1. Capture route: `OTEL_LOG_TOOL_DETAILS` (broad, with its own data-policy
   cost) or the domain-minimizing adapter (narrow, the design above)?
   Operator policy; the adapter needs a home and a maintainer.
2. The domain-minimizing adapter owns public-suffix derivation: it emits an
   already-minimized registrable domain, applying PSL logic and the default
   exclusions *before* anything reaches telemetry or the materialized
   table, so neither the engine (dependency-light, no PSL) nor the cron
   layer derives from URLs. A cron-layer derivation alternative would mean
   full URLs flowing through telemetry and the table — the route the
   capture correction rejects — and is not on the table; the open question
   is only where the adapter itself lives and how its PSL data is updated.
3. Safe Browsing or VirusTotal (scoped to these two, instructions-only):
   which one, given the eligibility constraints above (non-commercial-only
   free tiers; Web Risk or VT premium for commercial contexts), how the API
   key is held for the cron agent, whether the synthesized-root-URL loss
   under Safe Browsing is acceptable, and whether its hashed-prefix flow is
   worth the extra instruction complexity over the plain Lookup API.
4. Retention: how many days of domain rows the review needs, and whether
   the table resets with the app's activation semantics suffices.
5. Where the exclusion configuration lives (both layers, or egress only),
   and whether it is suffix-list or regex shaped.

## Status

Discussion only. Implementation would follow the normal flow: an adopted
decision resolving the open questions, then staged, reviewed work — the
capture adapter and its policy being the operator's call throughout.
