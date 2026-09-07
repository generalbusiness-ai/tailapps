Checker evidence report for tailapp request #4205 (ce521a95…cb302b9f), filed 2026-09-07 against promise 58b10961.
Prepared from the resident's durable projection at depth 4205 and git at main 756ce43a; git facts (a498991 ancestry, 6aa38a36 parentage of merge 69a79729, byte-identity of docs/harnesses/claude-code.md lines 70-79, absence of the a498991 README strings at main), artifact retirement of every a498991 artifact, the absence of any later mention of a498991 or #1056, and the absence of a verdict field on #1096 were re-checked directly by checker before filing.

1. Timeline

All times UTC, all from the saved projection.

#980, request, builder, 2026-08-28 18:59:57, event ...8df0a85c. "Refresh all user documentation for compactness, readability, accuracy, and completeness, with README as the canonical first-time path." Rests on nothing. Self-request (builder to builder). Commitment status now `cancelled`; builder retired it by supersede at 19:57:48 ("Documentation-refresh commitment delivered through approved...").

#1047, request, builder, 19:17:08, event ...4990108f. Follow-up clarification: state the shipped analytics value more directly in README and accurately distinguish Claude tool-detail from tool-content telemetry. Self-request, rests on nothing. Commitment `cancelled`; retired by builder supersede at 19:57:14 ("Retire duplicate self-request #1047") and again 19:57:47.

#1056, request, builder to checker, 19:17:36, event ...b0654906. Review head a498991 against two named conditions; rests on artifacts #1046 (README.md@a498991) and #1048 (docs/harnesses/claude-code.md@a498991). Commitment status `satisfied`, `stale: true`, terminal `reported`, promise #1058, report #1096.

#1087, report, checker, 19:40:30, event ...bc7cf604, verdict approved, head 6aa38a36, artifact #1077. Rests on #1084, #1083, #1077, #1078. Ratified (projection `reviews` shows `ratified: true`). Its own closing words: "(Note: this commit is stacked on the clarification a498991 whose review #1056 is held pending builder retiring the duplicate self-request #1047; that does not affect the correctness of this graphic at its exact head.)"

#1089, assert (merge receipt), builder, 19:42:23, event ...f67369ad, rests on #1087. merge_candidate 6aa38a36, merge_head 69a79729, merge_changed_paths README.md, docs/assets/tailapp-architecture.svg, docs/harnesses/claude-code.md.

#1096, report, checker, 19:43:36, event ...3aec1d7c, rests on promise #1058. It carries no `body`, so it has no `verdict` field and does not appear in the projection `reviews` list at all. Ratified by builder at 19:57:47 (decision #1137, "requester declared satisfaction"). It ends: "When builder retires #1047 I will additionally file the formal guarded gs review verdict on this exact head."

#1139, assert, builder, 19:59:11, event ...2c7c2fd4, rests on #1056 and #1137, `dead_basis_override: true`. Live, never retired. "Formal guarded review for #1056 remains outstanding... Checker retains the independent next action."

Git. a498991db9ef1d36e1392e039109dfeec84f35d5 is an ancestor of main but is NOT a direct parent of any merge commit; its only child is 6aa38a36 (a single-parent, non-merge commit). 6aa38a36 IS a direct parent of merge 69a79729ead92881f6d1c150b076ba2296a37cf6, which is an ancestor of 756ce43a. So the clarification reached main only as the stacked parent of the graphic, under the graphic's approval.

2. Later coverage

Searching every statement after #1139 for "a498991", "#1056", "OTEL_LOG_TOOL", "guarded verdict": the only later hit for either "a498991" or "#1056" anywhere in the log is Hugh's request #4205 today. No later report, assert, review, ratification, supersession or merge receipt files a verdict on head a498991, and none adopts a disposition of the owed verdict. Nothing covers the obligation.

Two later independent approvals touched the same paths at different heads, without mentioning a498991: #2142 (checker, 2026-08-30 22:46, approved, exact head 4d7760ed, "one verdict over all 13 cited artifacts as a single change set... 13 files, +112/-143 vs main 4d859fcf", including README.md and docs/harnesses/claude-code.md), and #2277 (checker, 2026-08-31 08:35, approved head b874243, Claude Code 2.1.251 fixture and claude-code.md). Both are delta reviews of their own change sets, not of a498991.

3. Before receipt #1089 versus after

Before. Only #1087, which by its own words reviewed "the README architecture graphic... both cited exact-head artifacts (docs/assets/tailapp-architecture.svg and README.md)" at 6aa38a36, and explicitly bracketed the clarification as held.

After. #1096, filed one minute after the merge, states it "Independently reviewed the documentation clarification at exact head a498991... in a clean detached worktree", names both conditions as met and the full battery green, but was "delivered as a report because the guarded gs review of #1056 is blocked". It is a narrative report, not a formal verdict.

4. Current source

The two outcomes #1056 asked for are the README first-use table wording and the Claude guide privacy-gate separation.

README.md at 756ce43a: changed. Lines 15 to 23 are now "There are five built-in bundles" with a "Questions it answers" column. The a498991 strings "Privacy-preserving event, activity, token/cache, tool-frequency, and latency analytics across Claude Code, Codex, and OpenCode" and "Observed out-of-bounds evidence, explicit unknown coverage, repetition/no-progress signals, and stalled-session queries" were removed by 63a9232 (2026-08-30, "Lead the README with tangible Tailapp value"), the reviewed and approved work of #2142, merged as c43e7b3. The substance survives elsewhere: cross-harness at line 4, out-of-bounds, repeated failures, no progress and missing evidence at line 21, detective framing at line 158.

docs/harnesses/claude-code.md at 756ce43a: present, unchanged. `git diff a498991 756ce43a -- docs/harnesses/claude-code.md` shows no hunk covering lines 70 to 79; that paragraph is byte-identical to a498991.

I checked its factual claims today, 2026-09-07, against https://code.claude.com/docs/en/monitoring-usage (redirected from docs.claude.com). The page states OTEL_LOG_USER_PROMPTS, OTEL_LOG_ASSISTANT_RESPONSES and OTEL_LOG_TOOL_CONTENT all default to disabled; OTEL_LOG_TOOL_DETAILS "Enable logging of tool parameters and input arguments... Bash commands, MCP server and tool names, skill names... and tool input (default: disabled)"; OTEL_LOG_TOOL_CONTENT "Enable logging of tool input and output content in span events (default: disabled). Requires tracing". Our three sentences are accurate. One nuance the page adds and our text does not: OTEL_LOG_ASSISTANT_RESPONSES, when unset, falls back to the value of OTEL_LOG_USER_PROMPTS, so leaving it unset is only safe while prompt logging is also off.

5. Recommendation

Accept a bounded historical review exception, option (a).

Independently reviewed: the graphic and its README placement at 6aa38a36, by formal verdict #1087. Not independently reviewed by any formal verdict: the two-file clarification at a498991, whose content was examined and narrated by the checker in report #1096 but never projected as a review.

Why (a). No guarded verdict at a498991 is now obtainable: all four a498991 artifacts (#1042, #1043, #1046, #1048, #1054, #1055) are retired and stale, retired as merge-covered predecessors on 2026-08-30 11:24, so gs review at that head would refuse. The window in which it was possible closed then, after the #1047 blocker was cleared at 19:57 on 08-28. Meanwhile both outcomes are settled on current source: the README half was replaced by independently approved head 4d7760ed (#2142), and the claude-code.md half stands verbatim and I verified it today against the vendor's current documentation. There is no residual defect and no correction outcome, only an unfilled row. The exception should be recorded so that #1139 stops reading as owed work.