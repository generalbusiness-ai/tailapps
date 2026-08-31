---
date: 2026-08-31
status: evidence stage; no normalizer or capture profile claimed yet
basis: official Kiro and Visual Studio Code documentation, checked 2026-08-31
---

# Harness coverage: Kiro and VS Code evidence stage

This note separates documented, user-routable telemetry from vendor telemetry.
It makes no event-name or attribute claim not supported by an official source,
and makes no Tailapp compatibility claim until a scrubbed live fixture proves
the receiver shape.

## Findings

| Surface | User-routable local OTLP | Decision |
|---|---|---|
| Kiro CLI v3 | Not documented | Negative finding; do not add a profile or guide that implies delivery to Tailapp. |
| Kiro IDE | Not documented | Negative finding; do not add a profile or guide that implies delivery to Tailapp. |
| VS Code Copilot Chat | Yes | Stage the capture and exact compatibility profile before normalizer work. |

### Kiro CLI v3

Kiro's current v3 material confirms the v3 unified harness and its web tools,
but does not document an OTLP/OTel exporter, local collector endpoint, or
configurable telemetry destination. Its CLI settings reference exposes only
`telemetry.enabled` and `telemetryClientId`; the firewall guide identifies
Kiro telemetry endpoints owned by the service; and enterprise monitoring
delivers daily CSV activity reports to the organization's S3 bucket. Those are
useful vendor or administrator facilities, not a same-user OTLP stream that an
operator can point at Tailapp.

Sources: [Kiro CLI 3.0](https://kiro.dev/docs/cli/v3/),
[CLI settings](https://kiro.dev/docs/cli/reference/settings/),
[network destinations](https://kiro.dev/docs/cli/privacy-and-security/firewalls/),
and [enterprise activity reports](https://kiro.dev/docs/cli/enterprise/monitor-and-track/user-activity/).

### Kiro IDE

The IDE shares the Kiro agent engine but its documented telemetry and data-
sharing controls are vendor-directed: the firewall guide names Kiro telemetry
destinations, while the data-protection guide describes opt-out rather than an
operator-selected OTLP exporter. No official IDE document found in this pass
offers an OTLP/HTTP endpoint, OTLP transport choice, or event contract.

Sources: [Kiro IDE overview](https://kiro.dev/docs/ide/),
[network destinations](https://kiro.dev/docs/privacy-and-security/firewalls/),
and [IDE data protection](https://kiro.dev/docs/privacy-and-security/data-protection/).

### VS Code Copilot Chat

VS Code's Copilot Chat documentation describes an OTLP-compatible exporter for
traces, metrics, and events. It is off by default; `github.copilot.chat.otel`
can enable `otlp-http` and point it at `http://localhost:4318`. The documented
resource identity is `service.name = copilot-chat` (configurable), and default
capture omits prompt, response, and tool-argument content. It documents
GenAI-convention spans named `invoke_agent`, `chat`, and `execute_tool`, plus
events including `gen_ai.client.inference.operation.details`,
`copilot_chat.session.start`, `copilot_chat.tool.call`, and
`copilot_chat.agent.turn`.

This is a routable source, but existing Tailapp normalizers do not yet name a
`copilot-chat` compatibility profile. A Stage 1 capture must use a current VS
Code build, preserve default content-off settings, route each signal through
Tailapp's OTLP/HTTP receiver, and check the resulting canonical records before
recognition is added. In particular, the capture must establish whether the
receiver represents the documented span names and span events as the expected
canonical record names; vendor documentation alone does not settle that local
translation boundary.

Source: [Monitor agent usage with OpenTelemetry](https://code.visualstudio.com/docs/agents/guides/monitoring-agents).

## Next stages

1. Keep the Kiro negatives in the harness index until Kiro publishes a
   user-routable local OTLP contract.
2. Capture one content-off Copilot Chat interaction with a tool call against a
   loopback Tailapp receiver; scrub it, record its VS Code/Copilot version, and
   compare the receiver's canonical log/span/metric records with the official
   table above.
3. Only then add a `copilot-chat` compatibility profile consistently to every
   applicable shipped normalizer, a harness guide, fixture tests, README and
   index links. Unknown target/command detail remains unknown when content
   capture is off.
