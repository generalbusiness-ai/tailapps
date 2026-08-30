# The installed-only MCP first-encounter experience

2026-08-30 · research and design note for the request "Research the
installed-only MCP first-encounter experience". Design only: nothing in this
note is implemented by the task that produced it.

## Scope and method

An installed-only user has the `tailapp` binary and nothing else: no
repository checkout, no source, no README. Everything they and their coding
agent can learn about the MCP surface arrives over the wire from
`tailapp mcp`. This note inventories that wire surface as it exists today,
compares it with what the current MCP specification and the late-2026 coding
harnesses require, expect, and reward, and designs the first-encounter
experience: server identity and instructions, self-sufficient tools, an
offline documentation resource catalog, build provenance, compatibility, and
the privacy and approval posture.

Evidence for the current surface is two captured stdio probe transcripts
against a binary built from exact revision
`70952f24f5901084f4eaefe9d2aa734e3cd3bf89` (the commit of the live
`internal/mcp/server.go` artifact) on 2026-08-30, together driving six
methods — `initialize`, `tools/list`, `resources/list`, `prompts/list`,
`resources/read`, and `tools/call` — against a fresh engine home; the exact
eleven input lines (six in probe A, five in probe B) are reproduced in
section 10. Protocol and harness claims cite their sources inline in
section 2.

## 1. The wire-visible surface today

What a first encounter actually sees, observed, not inferred from source:

**Identity.** `initialize` returns
`{"serverInfo": {"name": "tailapp", "version": "0.1.0"}}` with
`"protocolVersion": "2025-06-18"` and `"capabilities": {"tools": {}}`. There
is no `title`, no website or source linkage, no `instructions` string, and
the version is a constant: it does not change between builds, does not name a
revision, and cannot distinguish a release from a dirty development build.

**No version negotiation.** A client offering `protocolVersion` `2024-11-05`
receives `2025-06-18` back, take it or leave it. The server never echoes or
adapts to the requested revision.

**Tool catalog.** `tools/list` returns 14 tools. Descriptions run 53 to 185
characters, one sentence each, with no `title`, no annotations
(`readOnlyHint`, `destructiveHint`, `idempotentHint`), no `outputSchema`, and
no pointer to further documentation. The names carry an implicit workflow
(create → put_element → validate → activate; install as the one-shot
composite; list/get/status/metrics/ineffective/schema/query as the read
side) but nothing states it. A reader cannot tell from the wire that
`tailapp_query` is safe detective observation while `tailapp_delete` discards
a projection, except insofar as the one-line prose hints at it.

**Tool results.** Every result is the raw engine JSON re-encoded as one
`content[].text` blob; `structuredContent` is attached only when the engine
result is a JSON object (the earlier non-object `structuredContent` rejection
by Claude Code was fixed by omitting it for arrays). Two first-encounter
consequences observed on a fresh engine: `tailapps_list` with no apps returns
the literal text `"null"` — the very first call a curious agent makes answers
with four characters of nothing — and no tool declares an `outputSchema`, so
`structuredContent`, where present, is unannounced.

**Errors.** Engine errors come back as `isError: true` tool results carrying
the raw Go error string (socket dial failures included, absolute paths and
all). An unknown tool name is a protocol-level `-32601` `"unknown Tailapp
tool"`. `resources/list`, `resources/read`, `prompts/list`, and every other
unimplemented method are a bare `-32601` `"method not found"`.

**Absent surfaces.** No resources, no prompts, no logging capability, no
pagination, no `_meta`. The `capabilities` object says so honestly. There is
no way over the wire to read any documentation: the READMEs, the harness
setup guides, the query SQL reference, and the DDL/JSONata authoring model
all live in the repository an installed-only user does not have.

**The adapter underneath** (for the compatibility discussion only): a
140-line hand-written line-delimited JSON-RPC loop over stdio, 1 MiB line
cap, with a static method switch. It is deliberate, dependency-free, and easy
to hold to the project's "detective observation, not inline prevention"
stance; it is equally easy to outgrow.

## 2. Protocol requirements, vendor behavior, recommendations

This section separates three different kinds of claim, as the request
requires: what the protocol mandates (MUST/SHOULD with a spec citation),
what specific vendors observably do (with a version or snapshot date), and
what this note merely recommends. All spec fetches 2026-08-30.

### 2a. The specification landscape

The current MCP revision is **2026-07-28**
([spec versioning page](https://modelcontextprotocol.io/specification/versioning));
the chain is 2024-11-05 → 2025-03-26 → 2025-06-18 → 2025-11-25 →
2026-07-28. The server today speaks a fixed 2025-06-18, two revisions
behind. What matters from each boundary:

**2026-07-28 is a stateless rewrite**
([changelog](https://modelcontextprotocol.io/specification/2026-07-28/changelog)).
The `initialize`/`notifications/initialized` handshake is gone from the
modern era: every request carries its protocol version and client
capabilities in `_meta` (`io.modelcontextprotocol/protocolVersion`,
`…/clientCapabilities`), and a new **`server/discover`** RPC advertises
supported versions, capabilities, identity, and instructions up front.
Sessions are removed; list results may not vary per connection; list/read
results carry required cache metadata (`ttlMs`, `cacheScope`, SEP-2549);
`tools/list` ordering must be deterministic; roots, sampling, and logging
are deprecated (SEP-2577).

**Dual-era service is explicitly sanctioned**
([versioning & compatibility](https://modelcontextprotocol.io/specification/2026-07-28/basic/versioning)):
"A server that wishes to support both legacy clients … and modern clients
… MAY implement both behaviors", routing on how the client opens — an
`initialize` request selects legacy semantics for that stdio process, while
a request carrying modern per-request `_meta` is served statelessly. Stdio
clients probe with `server/discover` and fall back on error, so a
legacy-only server keeps working with modern clients that implement
fallback; the reverse is not true, which is why the plan (§13) treats
discovery support as required eventually, not immediately.

**Legacy negotiation was always negotiation.** The 2025-11-25 lifecycle
contract ([lifecycle](https://modelcontextprotocol.io/specification/2025-11-25/basic/lifecycle))
requires: if the server supports the client's requested version "it MUST
respond with the same version"; otherwise "another protocol version it
supports". Today's behavior — a constant `2025-06-18` regardless of
request — violates the MUST for any client asking for a supported older
revision, and is the cheapest spec bug to fix.

**Identity fields.** `Implementation` carries `name` (required), `version`
(required), and optional `title`, `description` (added 2025-11-25),
`websiteUrl` (`@format uri`), and `icons`
([schema.ts](https://github.com/modelcontextprotocol/modelcontextprotocol/blob/main/schema/2026-07-28/schema.ts)).
The spec adds a trust caveat: serverInfo is self-reported; clients SHOULD
NOT use it for security decisions — provenance (§7) is honesty, not proof.

**Instructions.** Still first-class in both eras (`InitializeResult` and
`DiscoverResult`): "guidance … to improve an LLM's understanding of
available tools … should not duplicate information already in tool
descriptions". **No length limit is stated in the spec**; the 512-character
core rule in §3 is a harness-driven constraint (§2b), not a protocol one.

**Tools.** Name rules (1–128 chars, `A-Za-z0-9_-.`), display precedence
`title` → `annotations.title` → `name`, and `ToolAnnotations` =
`readOnlyHint` (default false), `destructiveHint` (**default true**,
meaningful only when not read-only), `idempotentHint` (default false),
`openWorldHint` (default true)
([tools](https://modelcontextprotocol.io/specification/2026-07-28/server/tools)).
The default-true `destructiveHint` matters: an unannotated write tool is
presumed destructive, so the non-destructive write tools must say
`destructiveHint: false` explicitly (§4). There is **no documentation-link
field on Tool** — the `docs:` URI convention in descriptions is the right
mechanism, and `websiteUrl` lives only on `Implementation`. Annotations are
explicitly untrusted hints.

**structuredContent.** SEP-2106 (2026-07-28) loosened it: "can be any JSON
value … that conforms to the tool's `outputSchema` if one is defined", with
servers MUST conform to their declared schema, and a parallel text
serialization SHOULD accompany it. The object-only constraint this server
worked around in the earlier `properties:null`/array fixes is a *legacy-era
and client-side* constraint, not current protocol law — but the wrapping
design in §4 stands on contract-naming grounds and on the observed behavior
of shipping harnesses (§2b).

**Resources.** A server offering them MUST declare the `resources`
capability; entries carry `uri`/`name` (required), `title`, `description`,
`mimeType`, `annotations` (`audience`, `priority`, `lastModified`);
custom URI schemes MUST follow RFC 3986; and **an unknown URI MUST return
JSON-RPC `-32602`** with the URI echoed in `data` — clients SHOULD also
accept the older `-32002`
([resources](https://modelcontextprotocol.io/specification/2026-07-28/server/resources)).
Empty `contents` for a missing resource is forbidden. §5 and §10 use
`-32602` accordingly.

### 2b. The three harnesses, observed (snapshot 2026-08-30)

Versions examined: Claude Code **v2.1.251**
([docs](https://code.claude.com/docs/en/mcp)), OpenAI Codex CLI **0.151.0**
(2026-08-29, [docs](https://learn.chatgpt.com/docs/extend/mcp?surface=cli)),
OpenCode **v1.18.25** (2026-08-28, [docs](https://opencode.ai/docs/mcp-servers/)
plus source-level findings). Vendor behavior, not protocol law:

| Behavior | Claude Code v2.1.251 | Codex CLI 0.151.0 | OpenCode v1.18.25 |
| --- | --- | --- | --- |
| Protocol era for stdio | Legacy `initialize` (SDK 2.0 adds 2026-07-28, but stdio stays legacy unless `MCP_PROTOCOL_NEGOTIATION=auto`) | Both: 2026 flow with legacy retained (rmcp 3.1.3, [PR #35725](https://github.com/openai/codex/pull/35725)) | Legacy only (TS SDK 1.x, ≤2025-11-25); advertises no capabilities at initialize |
| `instructions` surfaced | Yes (since v1.0.52); **truncated at 2KB** ([docs](https://code.claude.com/docs/en/mcp), cap added v2.1.84) | Yes, documented, no limit found | Yes (current source; recent — earlier audits found it dropped); no limit found |
| Tool description limits | **2KB per description** (same cap) | None documented (8 MiB whole-response cap) | None documented |
| Annotations | Standard hints not documented; Anthropic `_meta` extensions instead | **`readOnlyHint: true` auto-runs in `writes` approval mode** — annotations directly gate approvals | Not consumed |
| Resources | Yes: `@server:uri` mentions, auto list/read tools (since v1.0.27) | Yes: internal list/read tools; known to over-route to `resources/read` on tool-only servers ([#27124](https://github.com/openai/codex/issues/27124)) and to hang on unanswered `resources/list` ([#25061](https://github.com/openai/codex/issues/25061)) | Yes (since [PR #33483](https://github.com/anomalyco/opencode/pull/33483), 2026-06-23): opaque URIs, list/read tools |
| `structuredContent` | Supported since v2.0.21; object-only rejection now **unconfirmed** either way | Upstream rmcp supports; client handling unconfirmed | **Fallback only**: used iff `content` is empty, then JSON-stringified; `outputSchema` stripped from tool definitions |
| `outputSchema` dialect | **Ajv, JSON Schema 2020-12 only — a draft-07 `$schema` makes every tool on the server unusable** ([#86142](https://github.com/anthropics/claude-code/issues/86142)) | Unconfirmed | Stripped, never validated |
| Skills | `.claude/skills/` (user, project, plugin, managed) | `.agents/skills/` (project→parents, home, /etc/codex, bundled); name+description budget ≈2% of context or 8,000 chars | Reads its own dirs **plus `.claude/skills/` and `.agents/skills/`** |
| Tool output caps | Warn 10k tokens, default max 25k (`MAX_MCP_OUTPUT_TOKENS`) | 8 MiB modern-protocol response cap | Documented only as "adds to your context" |

The design constraints this matrix fixes:

- **2KB is the hard ceiling** for the instructions string and for every
  individual tool description (Claude Code truncates at exactly that; the
  request's 512-character self-contained core is the belt to that ceiling's
  suspenders, and survives even the older reported ~4KB *shared* budget
  ([#43474](https://github.com/anthropics/claude-code/issues/43474))).
- **Always emit the `content` text block**: OpenCode ignores a lone
  `structuredContent`, and the spec's own SHOULD agrees (§2a).
- **`outputSchema` must be declared in JSON Schema 2020-12 with no foreign
  `$schema`**, or the server's entire catalog dies on Claude Code — the
  single sharpest interop landmine found in this research.
- **Annotations are worth stating precisely** even where ignored: Codex
  turns `readOnlyHint` into auto-approval policy, so a wrong `true` is a
  real safety hole and a missing `true` costs every read call a prompt.
- **Resources are progressive enhancement with a latency obligation**: all
  three harnesses can surface them, Codex punishes slow or absent answers
  to a declared capability, so the embedded catalog answers instantly or
  the capability stays undeclared.
- **A single skills directory serves all three** (§6): OpenCode reads
  Claude Code's and Codex's locations verbatim.

## 3. Server-wide instructions

The `instructions` field of the `initialize` result is the one server-wide
channel every harness generation understands (§2b: all three surface it
today). The binding limits from §2b: the full string stays under Claude
Code's 2KB truncation cap, and the critical orientation is complete within
the first 512 characters — the request's own rule, which also survives the
older reported ~4KB budget shared across all of a session's servers.

Proposed instructions text. The first paragraph is the self-contained core:
read as one space-normalized line it measures 478 characters, inside the
512 budget with margin; the rest is progressive detail a non-truncating
harness also gets:

> Tailapp turns local coding-agent OTLP telemetry into queryable SQLite
> analytics. Observation is detective, never inline prevention. Read first:
> tailapps_list, then tailapp_query (read-only SQL over a Tailapp's exported
> tables). tailapp_status shows engine readiness; tailapp_ineffective
> explains rejected records. Lifecycle tools
> (create/put_element/validate/activate, or install as one step) take an
> idempotency_key; delete and reset-mode activation discard materialized
> state.
>
> Each tool's description states its result contract. The resource catalog
> (resources/list) carries an overview and one Markdown page per tool; read
> tailapp://docs/overview before authoring. Query results are derived from
> telemetry and may contain session identifiers and file paths: treat them
> as sensitive, and prefer aggregate queries when sharing output.

Rules the text follows, which become conformance tests in section 11: name
the read-first path in the first two sentences; state the safety stance in
the first sentence that mentions state change; never promise a capability the
`capabilities` object does not declare; and keep the whole string under the
smallest non-truncating harness ceiling with the core under 512.

## 4. Self-sufficient tools

Every tool must be understandable from its own entry alone, because a client
that surfaces neither instructions nor resources still shows the tool list.
Per tool, the design requires:

- **`title`**: a human display name ("List Tailapps", "Run read-only SQL").
- **`description`** with a fixed internal shape, one to three sentences each:
  what it does and reads/changes; the result contract (what shape comes
  back, what `structuredContent` carries, what an empty result looks like);
  workflow position ("draft edits are not live until tailapp_activate");
  and a trailing stable documentation link
  (`docs: tailapp://docs/tools/<name>`) that degrades to an inert string on
  clients without resources.
- **Annotations**, honestly assigned, and *complete*, because the spec
  defaults `destructiveHint` to true for any non-read-only tool (§2a): every
  write tool states it explicitly. `readOnlyHint: true` on the read tools
  (list, get, validate, status, metrics, ineffective, schema, query);
  `destructiveHint: true` on `tailapp_delete` and on `tailapp_activate`
  (reset mode discards materialized state — the description must say the
  non-reset boundary mode is not destructive); `destructiveHint: false`,
  stated not defaulted, on the draft-only writers (`tailapp_create`,
  `tailapp_install`, `tailapp_put_element`, `tailapp_delete_element` — they
  create or edit drafts under optimistic revision control and never touch
  materialized state); `idempotentHint: true` on every mutation tool
  (create, install, put_element, delete_element, delete, activate) — all
  six take an idempotency key through the same idempotency ledger, so an
  exact replay lands once; `openWorldHint: false`
  on all fourteen — the server talks only to the local engine. Annotations
  are advisory hints for approval UIs, spec-flagged as untrusted, never a
  security boundary; the descriptions keep stating the facts in prose.
- **`outputSchema`** for the tools whose engine results are stable objects
  (status, metrics, query, schema, get, ineffective), declaring the
  `structuredContent` contract instead of leaving it unannounced — written
  in plain JSON Schema 2020-12 with **no `$schema` marker**, because Claude
  Code validates with a 2020-12-only Ajv and a foreign dialect disables the
  server's whole catalog (§2b). Every result keeps its serialized text
  `content` block alongside `structuredContent`: OpenCode reads only the
  former unless the latter is the sole content (§2b), and the spec's SHOULD
  agrees. Each description stays well under the 2KB per-tool cap; the shape
  in this design lands near 400 bytes.

Worked example, `tailapps_list` today versus designed:

> today: "List local Tailapps and their draft/active revisions."
>
> designed: title "List Tailapps"; description "List every local Tailapp
> with its draft and active revisions. Returns a JSON array in
> structuredContent.apps; an empty engine returns an empty array, not null.
> Start here, then tailapp_query to read a Tailapp's exported tables. docs:
> tailapp://docs/tools/tailapps_list"; readOnlyHint true.

The empty-list contract in that example is a behavior change the design
requires of the engine adapter (wrap top-level non-object results as
`{"apps": [...]}`-style named objects per tool, which also makes
`structuredContent` uniformly present and object-shaped): the first call an
agent makes must never answer `"null"`.

Error results also become self-explanatory: engine errors keep `isError:
true` but the text gains the tool's name and a next step ("engine not
reachable at $TAILAPP_HOME: is `tailapp serve` running?"), and socket paths
are reduced to the home directory. Unknown tool names move from `-32601` to
the argument-level error the spec expects for an unknown tool, with the
catalog's valid names in the message.

## 5. The documentation resource catalog

Design: a finite, offline, embedded catalog — no network, no filesystem
reads, byte-for-byte reproducible from the build.

- **URIs**: a stable custom scheme, `tailapp://docs/overview`,
  `tailapp://docs/authoring`, `tailapp://docs/query-sql`,
  `tailapp://docs/otlp-records`, `tailapp://docs/harnesses`, and
  `tailapp://docs/tools/<tool-name>` for each of the 14 tools. Scheme and
  paths are contract: they appear in tool descriptions and never change
  meaning; additions are allowed, renames are not.
- **`resources/list`** returns roughly 19 entries, each with `uri`, `name`,
  `title`, one-sentence `description`, `mimeType: "text/markdown"`, and
  `annotations` (`audience: ["assistant"]`, with `priority` marking
  overview and query-sql as read-first). The catalog fits one page;
  pagination cursors are accepted and answered with a complete first page
  and no `nextCursor`.
- **`resources/read`** returns the embedded Markdown as a single `text`
  content item with the same `mimeType`. Content is compiled from the
  repository documentation at build time (a trimmed, installed-only
  rendering: no repo-relative links, no contributor material — every page
  must stand alone, and repo paths appear only as provenance pointers).
- **Unknown URIs**: `resources/read` on anything else returns JSON-RPC
  `-32602` with the requested URI echoed in `data` and the overview URI as
  the safe next step in the message — the code the current spec mandates
  (§2a; older clients also accept the retired `-32002`). No other input is
  reflected. Never a crash, never a filesystem probe, never a bare
  `-32601`, never an empty `contents` array.
- **Capability**: `capabilities.resources` is declared (no `subscribe`, no
  `listChanged` — the catalog is immutable per build). When the modern era
  is served (§13 stage 4), list and read results carry the required
  `ttlMs`/`cacheScope` cache metadata with a long TTL, which an
  immutable-per-build catalog earns honestly; listing order is
  deterministic in both eras.
- **Degradation rule**: no tool behavior may depend on the client reading
  resources. Tool descriptions remain sufficient on their own (section 4);
  the catalog is depth, not a prerequisite. The `docs:` pointers cost a few
  tokens on resource-blind clients and pay for themselves on the rest.

## 6. A compact cross-harness SKILL.md, assessed

The §2b findings make this cheaper than expected: all three harnesses load
SKILL.md-style skills with progressive disclosure (name and description at
startup, body on use), and their discovery locations overlap — Claude Code
reads `.claude/skills/` (user, project, plugin, managed), Codex reads
`.agents/skills/` (project walking up to the repo root, home,
`/etc/codex/skills`), and OpenCode reads its own directories *plus both of
the others'* verbatim. One skill directory reachable as `.agents/skills/`
and `.claude/skills/` therefore covers all three. Codex budgets the startup
catalog (≈2% of context or 8,000 chars) and OpenCode bounds `description`
at 1,024 chars, so the skill stays tiny by rule, not restraint.

Position: ship it as optional packaging, never as the orientation
mechanism. The MCP surface must be complete without it (instructions +
tools + resources are the contract); a SKILL.md that restates the overview
resource in ~30 lines and points at `tailapp://docs/overview` gives
progressive disclosure on harnesses that index skills, and its absence
changes nothing. It must be generated from the same embedded content as the
overview resource so the two cannot drift. Because an installed-only binary
must not silently write into user directories, packaging is explicit: a
documented `tailapp mcp emit-skill DIRECTORY` (or equivalent) writes the
generated skill where the user chooses; nothing is installed as a side
effect.

## 7. Build provenance

Observed today: `serverInfo.version` is the constant `"0.1.0"` in two places
(the MCP adapter and the resident metadata), while the binary already
carries everything needed: Go build info stamps `vcs.revision`,
`vcs.time`, `vcs.modified`, and the module path and (pseudo-)version —
observed on the probe binary as module
`github.com/generalbusiness-ai/tailapps` at
`v0.0.0-20260830102107-70952f24f590` with the full revision and
`vcs.modified=false`.

Design, all via `runtime/debug.ReadBuildInfo()` at startup, computed once:

- **version**: the module version when it is a real tag; otherwise
  `0.1.0+<revision12>`, with a `.dirty` suffix when `vcs.modified` is true.
  Deterministic fallback when build info or VCS stamps are absent (e.g.
  `go run` outside a checkout): the bare base version `0.1.0`, never a
  guess.
- **identity linkage**, a two-step derivation satisfying the originating
  requirement to link the GitHub remote when available at build time.
  Primary: the release build stamps a sanitized remote via
  `-ldflags "-X ...buildinfo.SourceURL=$(sanitized remote)"`, where the
  sanitizer reads `git remote get-url origin`, accepts it only when it
  parses as a github.com remote (ssh or https form), and normalizes it to
  bare `https://github.com/OWNER/REPO` — user-info, credentials, ports, and
  `.git` suffixes stripped; any non-GitHub or unparsable remote stamps
  nothing. Fallback, when no stamp is present (a plain `go build` or
  `go install`): derive the same URL from the module path when its first
  element is a known forge host. Otherwise omit the links rather than
  fabricate. Both steps are deterministic for a given build input; the
  runtime never reads git state.
- **surfacing**: `serverInfo` gains `title: "Tailapp"`, a one-line
  `description`, and `websiteUrl` (the spec's identity fields, §2a); the
  same version string replaces the constant in the resident metadata; the
  overview resource repeats version, revision, and source link so an agent
  can quote its exact server provenance from either surface.
- **documentation links**: exact-version deep links only where the derived
  source identity exists; otherwise the links degrade to the overview
  resource, which is always present.

## 8. Compatibility and the protocol adapter

The adapter question is when, not whether, to stop extending the hand-written
loop. The facts (all fetched 2026-08-30):

The official Go SDK, `github.com/modelcontextprotocol/go-sdk`, is at
**v1.7.0** (tagged 2026-07-28), past its v1.0.0 API-stability promise, and
supports all five spec revisions with **transparent dual-era service**: it
registers `server/discover` automatically and still answers legacy
`initialize`, routing per client
([README](https://github.com/modelcontextprotocol/go-sdk),
[docs/protocol.md](https://github.com/modelcontextprotocol/go-sdk/blob/main/docs/protocol.md)).
Its server API covers everything this design needs: stdio transport,
`Implementation{Name, Version, Title, WebsiteURL}`, `ServerOptions`
instructions, typed `AddTool` with reflected input *and output* schemas,
`ToolAnnotations`, resources with templates, and pagination. Migration
deletes the framing/dispatch loop and turns 14 hand-written schema maps
into tagged Go structs; the SDK's generated schemas set
`additionalProperties: false`, which this server's schemas already do, so
that publicized strictness change
([#892](https://github.com/modelcontextprotocol/go-sdk/issues/892)) is
parity here, not a break.

Against adopting *now*: the SDK brings 8 direct dependencies (jwt, oauth2,
jsonschema-go, uritemplate, segmentio/encoding, x/time, x/tools, go-cmp)
into a binary that is deliberately dependency-light; the 2026-07-28 rewrite
landed weeks before this note, with `MCPGODEBUG` escape hatches slated for
removal in v1.9.0 and at least one open modern-era bug
([#1130](https://github.com/modelcontextprotocol/go-sdk/issues/1130),
`ping` poisoning stateless sessions); and this server needs none of the
deprecated surfaces the churn is about. The community alternative,
`mark3labs/mcp-go` v0.49.0 (2026-07-23), has no v1 stability promise and
no 2026-07-28 support — it is not a candidate.

**Verdict** (recommendation, not protocol requirement): stages 1–3 extend
the hand-written adapter — every §3–§7 behavior is a small, testable
addition to a 140-line loop, and none of it touches the parts the 2026
rewrite changed. Stage 4 adopts the official SDK when discovery-era support
is implemented, rather than hand-writing `server/discover`, per-request
`_meta` validation, `-32022` version errors, and cache metadata a
maintained SDK already ships; by then the SDK will be one or two point
releases past its rewrite. The trade — 8 dependencies for a
protocol-tracking machine this project does not want to own — is taken at
the stage where the protocol surface, not the product surface, is what
grows.

Independent of the SDK verdict, two compatibility behaviors are required:

- **Honest version negotiation**: respond with the client's requested
  protocol revision when the server supports it, and with the server's
  newest supported revision otherwise, per spec — never a fixed constant.
  The supported set, and how initialize-era clients and any 2026-07-28
  discovery-flow clients are both served, follows the section 2 findings.
- **Capability truthfulness stays absolute**: every advertised capability is
  implemented, every unimplemented method keeps a clean JSON-RPC error, and
  nothing is inferred from client identity.

## 9. Privacy and approval guidance

The engine's data is derived from coding-agent telemetry. Even though the
shipped normalizers are aggressively content-free, exported tables carry
session identifiers, harness names, day buckets, token and cost figures, and
(in guard findings) tool names and coverage reasons. The MCP surface treats
that as sensitive by default:

- `tailapp_query`, `tailapp_schema`, `tailapp_ineffective` descriptions say
  the results derive from local telemetry and may identify sessions;
  instructions (section 3) carry the aggregate-when-sharing guidance.
  `tailapp_ineffective` is the sharpest: rejected raw canonical records are
  exactly the material the normalizers refused to keep, and its description
  must say the buffer is bounded, memory-only, and for local diagnosis.
- Destructive and state-changing operations rely on three reinforcing
  layers: honest annotations (section 4) so approval UIs can gate them, an
  explicit acknowledgment argument for reset-mode activation (already in the
  engine contract), and idempotency keys so an approved retry is safe. The
  guidance for harness approval prompts: read tools are safe to allow
  permanently; lifecycle tools deserve per-call approval; `tailapp_delete`
  and reset-mode `tailapp_activate` deserve it always.
- Nothing in the MCP surface performs inline prevention, and no wording may
  imply it does: observation stays detective, and the instructions say so in
  the first paragraph.

## 10. Representative wire contract

The section 1 inventory comes from two stdio probes of `tailapp mcp`
(2026-08-30, revision `70952f24`, fresh engine home, resident started with
`--otlp-http 127.0.0.1:0`). These are the exact input lines, one JSON-RPC
message per line, reproducible verbatim:

```jsonc
// probe A — catalog and absent surfaces (6 lines)
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"probe","version":"0"}}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}
{"jsonrpc":"2.0","id":3,"method":"resources/list","params":{}}
{"jsonrpc":"2.0","id":4,"method":"prompts/list","params":{}}
{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"tailapps_list","arguments":{}}}

// probe B — negotiation, live results, unknown resource (5 lines)
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"probe","version":"0"}}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"tailapps_list","arguments":{}}}
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"tailapp_metrics","arguments":{}}}
{"jsonrpc":"2.0","id":4,"method":"resources/read","params":{"uri":"tailapp://docs/overview"}}
```

From those transcripts, abridged to the shapes: the "before", and the
designed "after" for the same three exchanges:

```jsonc
// today: initialize (client offered 2024-11-05)
{"protocolVersion":"2025-06-18","capabilities":{"tools":{}},
 "serverInfo":{"name":"tailapp","version":"0.1.0"}}

// designed
{"protocolVersion":"2024-11-05" /* echoes a supported request */,
 "capabilities":{"tools":{},"resources":{}},
 "serverInfo":{"name":"tailapp","title":"Tailapp",
   "version":"0.1.0+70952f24f590",
   "description":"Local OTLP telemetry analytics for coding agents",
   "websiteUrl":"https://github.com/generalbusiness-ai/tailapps"},
 "instructions":"Tailapp turns local coding-agent OTLP telemetry into ..."}

// today: first call on a fresh engine
{"content":[{"type":"text","text":"null"}]}

// designed
{"content":[{"type":"text","text":"{\"apps\":[]}"}],
 "structuredContent":{"apps":[]}}

// today: resources/read
{"error":{"code":-32601,"message":"method not found"}}

// designed: resources/read with an unknown URI
{"error":{"code":-32602,"message":"unknown resource uri;
  start at tailapp://docs/overview",
  "data":{"uri":"tailapp://docs/nope"}}}
```

## 11. Conformance and agent-orientation test matrix

Two test families, both runnable offline in `go test`:

**Protocol conformance** (table driven, one stdio session per row):

| Probe | Assert |
| --- | --- |
| initialize at each supported revision | echoed revision; identity fields present; instructions non-empty |
| initialize at an unsupported old revision | newest supported revision returned, no error |
| tools/list | 14 tools; every tool has title, annotations, description ending in its `docs:` URI; read/destructive annotation sets exactly match the fixed expected lists |
| tools/call per tool (fresh engine) | never text `"null"`; `structuredContent` object present iff `outputSchema` declared |
| tools/call unknown name | argument-level error naming valid tools |
| resources/list | complete catalog, every entry mimeType text/markdown, stable URI set equals the embedded manifest |
| resources/read each catalog URI | non-empty Markdown, no repo-relative links |
| resources/read unknown URI | resource-not-found error, echoed URI, overview pointer, no path probing |
| prompts/list | clean method-not-found (capability not declared) |
| oversized line, parse error | -32700, session survives |

**Agent orientation** (static, on the built catalog): instructions full
string < 2,048 bytes and its core ≤ 512 characters and self-contained
(names the read-first tools and the safety stance — asserted by substring
contract, reviewed by humans); every tool description < 2,048 bytes,
stating its result contract and empty-result shape; every `outputSchema`
free of `$schema` markers and valid JSON Schema 2020-12; every tool result
constructor emits a text `content` block whether or not `structuredContent`
is present; provenance version matches the build info of the test binary;
SKILL.md, when packaged, byte-derives from the overview resource and its
description fits OpenCode's 1,024-char bound.

Cross-harness verification stays manual but scripted: the section 2 harness
matrix becomes a checklist (`claude mcp add` / Codex config / OpenCode
config, then: instructions surfaced? tools approved with readable prompts?
resources visible where supported? structuredContent accepted?), run per
release against the harness versions named there.

## 12. Decisions, tradeoffs, open questions

Decided by this design:

- Tool descriptions are the orientation floor; resources are depth; SKILL.md
  is optional packaging. Three layers, strictly decreasing requiredness.
- Non-object engine results get wrapped in named objects per tool. This
  changes the text rendering of `tailapps_list` (an output-contract change
  visible to existing scripts that parsed the raw array text), accepted for
  the uniform `structuredContent` contract; the engine's own control API is
  untouched.
- Provenance derives from Go build info plus an optional build-time-stamped
  sanitized remote; no runtime git, no environment probing, omission over
  fabrication.
- Error text carries next steps but never absolute private paths.

Tradeoffs called out:

- Embedded documentation grows the binary (estimated tens of KiB — the
  installed-only rendering, not the full docs tree) in exchange for a
  self-documenting server; accepted, the binary already embeds four
  Tailapps.
- Richer descriptions cost tokens in every tools/list. The budget rule in
  section 4 (one to three sentences plus contract and link) keeps the whole
  catalog under roughly 4 KiB of description text; the harness truncation
  limits in section 2 bound the worst case.
- Wrapping list results is a compatibility break for text-parsing callers,
  taken now while the audience is small rather than after adoption.

Open questions for the requester or a follow-up decision:

0. Should the server answer `prompts/list` with an empty list (declaring an
   empty `prompts` capability — a truthful implementation with zero
   prompts) purely because at least one legacy client generation was
   reported to discard servers over a fatal `-32601` on optional list
   methods ([OpenCode #24985](https://github.com/anomalyco/opencode/issues/24985),
   closed, current behavior unconfirmed)? Leaning no until re-verified
   against a current OpenCode: capability minimalism is the cleaner
   default, and the report is stale.
1. Should the overview resource include a minimal end-to-end example
   (install signal-counts, send one OTLP record, query it) even though the
   example duplicates authoring docs? (Leaning yes: first-encounter value
   outweighs duplication, and both compile from one source.)
2. Does the resident metadata version string (`serve` status output) change
   in the same release, so the two surfaces never disagree? (Leaning yes,
   same helper.)
3. How long does the legacy `initialize` era stay supported after stage 4
   brings dual-era service? The spec sanctions dual-era indefinitely and
   deprecations carry a ≥12-month window; the practical answer depends on
   when the section 2b harnesses actually move to discovery, and should be
   revisited against their release notes, not decided now. (The
   instructions string itself poses no such question: both eras carry it —
   `InitializeResult` legacy, `DiscoverResult` modern — and the server
   serves one identical string to both.)

## 13. Staged implementation plan

Each stage independently shippable, reviewable, and gated; no stage depends
on a later one.

1. **Identity, negotiation, and honest results.** Build-info provenance
   (§7), version negotiation (§8), instructions (§3), object-wrapped
   results with `outputSchema` (§4's contract change), improved error text.
   Conformance rows for all of it. Smallest diff, largest first-encounter
   gain.
2. **Tool orientation.** Titles, annotations, restructured descriptions
   with `docs:` URIs (§4), the static orientation tests (§11). The URIs
   ship one stage before the resources that serve them; the strings are
   inert until stage 3 and the descriptions remain complete without them —
   ordering accepted to keep stage 2 free of embedded content.
3. **The resource catalog.** Embedded installed-only Markdown, list/read,
   unknown-URI errors, capability declaration (§5), full conformance rows.
4. **Discovery-flow compatibility and the adapter decision.** Adopt the §8
   verdict (extend the hand-written adapter or migrate to the official SDK)
   together with 2026-07-28 discovery support, since that is the stage
   whose surface area the verdict changes most.
5. **SKILL.md packaging** (§6), generated from the overview resource, with
   the per-harness placement notes from §2 — only if the section 2 findings
   show at least one harness rewards it.

Dependencies and dated sources are listed inline in section 2; the probe
transcripts backing section 1 and section 10 are reproducible with the two
probe scripts — eleven JSON-RPC input lines — shown verbatim in section 10
against this note's named revision.
