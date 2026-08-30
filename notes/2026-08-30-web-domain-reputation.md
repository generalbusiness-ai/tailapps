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
which also exposes file paths and full shell commands — or, preferably, a
small adapter that promotes only the web-tool URL into the safe `target`
attribute the guard vocabulary already understands. The narrower adapter
avoids widening the telemetry surface for one use case.

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
to the chosen service, reporting hits in the daily review. Between the
two: Safe Browsing's Lookup API is free with an API key and purpose-built
for exactly this question, and its Update API variant works over hashed
prefixes (more private, more instruction complexity); VirusTotal's domain
reports are richer (categories, resolutions, vendor verdicts) but
rate-limited on the free tier and send the bare domain as a query. Either
fits an instructions-only implementation; the choice is an operator call
recorded when the cron instructions are written.

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

1. Capture route: `OTEL_LOG_TOOL_DETAILS` (broad) or a URL-only adapter
   (narrow)? Operator policy; the adapter needs a home and a maintainer.
2. Registrable-domain derivation needs public-suffix logic; shipping a PSL
   dependency in the engine conflicts with its dependency-light stance —
   derivation may belong in the adapter or the cron layer instead of the
   fold.
3. Safe Browsing or VirusTotal (scoped to these two, instructions-only):
   which one, how the API key is held for the cron agent, and whether Safe
   Browsing's hashed-prefix Update API is worth the extra instruction
   complexity over the plain Lookup API.
4. Retention: how many days of domain rows the review needs, and whether
   the table resets with the app's activation semantics suffices.
5. Where the exclusion configuration lives (both layers, or egress only),
   and whether it is suffix-list or regex shaped.

## Status

Discussion only. Implementation would follow the normal flow: an adopted
decision resolving the open questions, then staged, reviewed work — the
capture adapter and its policy being the operator's call throughout.
