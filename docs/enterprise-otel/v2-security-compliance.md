# Thermal Enterprise v2: Security & Compliance Review

**Reviewer:** Security & Compliance Engineering (second panel)
**Date:** 2026-04-11
**Status:** UPDATED THREAT MODEL — supersedes findings from first panel review
**Scope:** Daemon architecture, session JSONL tailing, dual OTEL streams, cost display, enterprise rate config

---

## 0. What Changed and Why This Review Exists

The first panel reviewed a system with zero outbound data flow and a well-bounded input surface (process tables, gate event JSONL). The architecture has since expanded materially:

| Capability | First panel | Current |
|---|---|---|
| Data inputs | Process tables, gate JSONL | + Session JSONL (conversation data, usage objects, tool outputs) |
| Runtime model | On-demand dashboard process | + Persistent launchd/systemd daemon |
| Outbound data | Zero (TCP probe only) | Two OTEL streams (Claude Code + Thermal enrichment) |
| Sensitive display | None | Dollar amounts in statusline |
| Config sensitivity | Low (theme, animation) | + Negotiated enterprise pricing in TOML |

Every finding from the first panel still holds. This document adds new findings and upgrades severity on existing ones where the expanded surface warrants it.

---

## 1. Revised Data Classification

### New data elements (not present in first review)

| Data Element | Source | Classification | Rationale |
|---|---|---|---|
| User prompts (conversation text) | Session JSONL (`~/.claude/projects/`) | **Restricted** | Developer instructions to Claude. May contain proprietary business logic, internal project names, security-sensitive context. Present when `OTEL_LOG_USER_PROMPTS=1` in Claude Code OTEL; always present in raw session JSONL. |
| Assistant responses | Session JSONL | **Restricted** | Generated code, architecture decisions, security analysis. Full intellectual property of the customer. |
| Tool outputs (bash results, file contents) | Session JSONL | **Restricted** | Literal file contents, command output, error messages with stack traces. Routinely contains credentials, internal URLs, database schemas. |
| Token usage objects (`input_tokens`, `output_tokens`, `cache_read`, `cache_creation`) | Session JSONL `usage` fields | **Internal** | Numeric counters. No content leakage. Reveals workload intensity but not content. |
| Cost in USD | Derived from usage + rates | **Confidential** | Dollar amounts reveal negotiated pricing (commercially sensitive) and organizational spend patterns. |
| Negotiated enterprise rates | TOML config `[pricing]` | **Confidential** | Commercially negotiated pricing is typically NDA-protected. Exposure reveals Anthropic's discount structure. |
| Session ID / transcript path | Session JSONL, hooks | **Confidential** | Correlation identifiers. Transcript path encodes the working directory (`encoded-cwd`), revealing project names and repository paths. |
| Claude Code OTEL attributes (`user.email`, `user.account_uuid`, `organization.id`) | Claude Code OTEL stream | **Confidential** | PII. Organization structure. Identity correlation across sessions. |
| `prompt.id` (per-turn correlation) | Claude Code OTEL events | **Confidential** | Links multiple API calls to a single user action. Enables reconstruction of developer workflow patterns at high fidelity. |

### Upgraded classifications from first panel

| Data Element | First panel | Now | Reason |
|---|---|---|---|
| Agent IDs | Confidential | Confidential | No change, but daemon persistence means correlation windows extend from ~90s dashboard lifetime to days/weeks of continuous collection. |
| Session IDs | Confidential | Confidential | Same upgrade — persistent daemon accumulates session history over time, increasing correlation value to an attacker. |

### Critical finding: session JSONL is a full conversation record

The session JSONL at `~/.claude/projects/<encoded-cwd>/<session-id>.jsonl` is not a telemetry log. It is the complete conversation transcript: user prompts, assistant responses, tool invocations and their outputs, and usage metadata. A process that can read this file has access to everything the developer discussed with Claude, including:

- Proprietary source code (via tool outputs showing file reads/writes)
- Security vulnerabilities under discussion
- Internal architecture and infrastructure details
- Credentials and secrets that appeared in terminal output
- Business strategy communicated in prompts

**This is the single highest-risk data element in the entire architecture. The daemon's access to session JSONL is a fundamentally different security posture than reading process tables.**

---

## 2. Daemon Architecture Threat Model

### Privilege requirements

The daemon (launchd agent on macOS, systemd user service on Linux) needs:

| Access | Required | Justification |
|---|---|---|
| Read `~/.claude/projects/` | Yes | Session JSONL tailing for usage extraction |
| Read `$TMPDIR/coolant-$USER.*` | Yes | Gate event JSONL (existing) |
| Read process table | Yes | CPU/memory/process monitoring (existing) |
| Network egress to OTEL endpoint | Yes | Metric export |
| Read config files | Yes | TOML config, rate config |
| Write to filesystem | Minimal | Local log file only |
| Root/admin privileges | **No** | Everything above is user-scoped. The daemon must run as the developer's UID, not root. |

**Finding:** The daemon can and must run unprivileged as a user-level launchd agent (`~/Library/LaunchAgents/`), not a system daemon (`/Library/LaunchDaemons/`). This is critical — a user-level agent's blast radius is bounded to that user's files and processes. A system daemon compromise exposes all users on the machine.

### Blast radius analysis

If the daemon process is compromised (code injection, dependency supply chain attack, memory corruption):

| Asset at risk | Impact | Notes |
|---|---|---|
| All session JSONL files | **Critical** | Full conversation history for every project. Contains source code, credentials, business context. |
| Gate event JSONL | High | Shell command strings (Restricted data from first review). |
| OTEL auth credentials | High | Bearer tokens in environment variables. Enables impersonation against the OTEL collector. |
| Enterprise rate config | Medium | Negotiated pricing. Commercially sensitive. |
| Network position | Medium | Can exfiltrate to any endpoint the user can reach. OTEL export path is already established. |
| Process table | Low | Same access any user process has. |

**Finding:** The blast radius of a compromised daemon is materially larger than a compromised dashboard process. The dashboard runs interactively for minutes; the daemon runs continuously for days. The daemon has access to session JSONL (Restricted data); the dashboard does not (in Rung 0). The daemon's established OTEL export path provides a ready-made exfiltration channel.

### Daemon-specific attack vectors

**Vector 1: Session JSONL as a side-channel oracle.** A compromised daemon can silently read every conversation a developer has with Claude. Unlike the gate JSONL (which contains only tool names and event types), session JSONL contains full intellectual property. An attacker with daemon access has a keylogger-equivalent for AI-assisted development.

**Vector 2: OTEL export as exfiltration channel.** The daemon already has an authorized outbound connection to an OTEL collector. A compromised daemon can encode stolen data as metric attributes or label values and exfiltrate through the existing channel without triggering new network alerts. The closed attribute set mitigates this only if the OTEL serialization code itself is not compromised.

**Vector 3: Launchd plist manipulation.** The `.plist` file in `~/Library/LaunchAgents/` controls daemon configuration. If writable by the user (which it must be for installation), any process running as that user can modify it to: change the binary path (code substitution), add environment variables (credential injection), or modify arguments (behavioral changes). The plist itself becomes a persistence mechanism for malware.

**Vector 4: Binary substitution.** If the daemon binary is writable by the user, any local process can replace it. On next launchd restart, the compromised binary inherits all the daemon's access — including session JSONL reads and OTEL export credentials. Binary integrity verification is essential.

---

## 3. Session JSONL Access: Architectural Enforcement

### The core question: does the daemon need to read full conversation content?

**No.** The daemon needs only `usage` objects (token counts) and message timestamps from session JSONL. It does not need user prompts, assistant responses, or tool outputs for any of its stated functions (cost statusline, OTEL token metrics, agent-hour tracking).

### Recommended architecture: field-level extraction with discard

The session JSONL parser in the daemon must implement a **whitelist parser** that extracts only:

1. `usage.input_tokens`, `usage.output_tokens`, `usage.cache_read_input_tokens`, `usage.cache_creation_input_tokens` — token counts
2. `type` field — to identify message type (assistant, user, tool_result) for counting
3. `timestamp` — for duration calculation
4. `session_id` — for session correlation
5. `model` — for rate-card cost calculation

Everything else must be discarded at parse time, never stored in memory beyond the JSON decoder's buffer, and never serialized to any output (logs, OTEL attributes, debug output).

### Enforcement mechanisms

**Mechanism 1: Typed extraction struct.** The Go JSON decoder should unmarshal into a struct with only the whitelisted fields. Go's `encoding/json` ignores fields not present in the target struct. This means conversation content is never allocated as Go strings — it is read by the decoder and discarded.

```go
// This is the ONLY struct that touches session JSONL.
// Adding fields requires security review.
type SessionEvent struct {
    Type      string         `json:"type"`
    Timestamp string         `json:"timestamp"`
    SessionID string         `json:"session_id"`
    Model     string         `json:"model"`
    Usage     *UsageFields   `json:"usage"`
}

type UsageFields struct {
    InputTokens              int `json:"input_tokens"`
    OutputTokens             int `json:"output_tokens"`
    CacheReadInputTokens     int `json:"cache_read_input_tokens"`
    CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
}
```

**Mechanism 2: Unit test enforcement.** A test must assert that `SessionEvent` contains no fields of type `string` beyond the whitelist (type, timestamp, session_id, model). Any struct field that could hold conversation content (prompt, content, text, response, tool_input, tool_output) must cause the test to fail. This is the same pattern as the closed attribute set test from the first panel, extended to the parser.

**Mechanism 3: No raw byte retention.** The parser must not retain the raw JSON line bytes after extraction. No buffering of unparsed lines. No debug logging of raw session data. The `io.Reader` from the file tailer feeds directly into `json.Decoder`; no intermediate `[]byte` storage.

**Mechanism 4: Code review gate.** Any change to `SessionEvent` or the session JSONL parser requires explicit security review sign-off. This should be enforced via CODEOWNERS on the parser file.

**Finding:** Architectural enforcement of field-level extraction is the single most important security control in the daemon design. Without it, the daemon is a conversation surveillance tool. With it, the daemon sees only numeric counters and metadata.

---

## 4. Dual OTEL Stream Privacy Amplification

### The correlation risk

Two OTEL streams flow to the same collector:

| Stream | Owner | Contains |
|---|---|---|
| Claude Code OTEL | Anthropic (Claude Code process) | `api_request` events with token counts, `user.account_id`, `session.id`, `prompt.id`, model, cost_usd, and optionally user prompts and tool details |
| Thermal OTEL | Thermal daemon | System metrics, agent lifecycle, gate events, threat level, process categories, and (new) enrichment token/cost metrics |

Both streams carry `session.id`. Both are timestamped. A collector that receives both streams can correlate:

- **Thermal's agent lifecycle** (start/stop/stale) with **Claude Code's per-request token counts** — revealing cost per agent, not just per session
- **Thermal's threat level transitions** with **Claude Code's prompt patterns** — "the system entered MELTDOWN when the developer asked about X"
- **Thermal's gate suppressions** (tool type) with **Claude Code's tool use events** — "the developer tried to run eslint 47 times during this conversation about Y"
- **Thermal's process categories** with **Claude Code's api_request timing** — correlating what the developer asked with what processes spawned

### Assessment

**The correlation between streams does reveal more than either stream alone.** Specifically, Thermal's system-state context (threat level, resource utilization, gate activity) combined with Claude Code's conversation-linked telemetry creates a richer picture of developer behavior than either product intends to provide independently.

However, this is a **known and accepted trade-off** for any enterprise that deploys both streams. The enterprise OTEL collector already receives correlated data from many sources (APM, logs, infrastructure metrics). The CISO approving Claude Code OTEL has already accepted conversation-adjacent telemetry. Thermal's stream adds system context, not new conversation data.

**Finding:** The privacy amplification risk is real but manageable. Documentation must explicitly state that deploying both OTEL streams to the same collector enables cross-correlation. The data flow diagram (compliance doc) must show both streams and the correlation surface. CISOs must approve both streams as a unit, not independently.

### Mitigation

- Thermal's OTEL stream must never include `prompt.id`, `user.email`, or `user.account_uuid` — these are Claude Code's identity attributes. Thermal emits `session.id` (for correlation) and its own config labels (team, project). The identity join happens at the collector/query layer, not in Thermal's export.
- Thermal must not read Claude Code's OTEL configuration or credentials. The two streams are operationally independent.

---

## 5. Enterprise Rate Configuration

### Is negotiated pricing confidential?

**Yes.** Enterprise contracts with AI providers routinely include NDA clauses on pricing. Negotiated discount rates reveal:

- The volume tier the customer qualified for
- The provider's willingness to discount (leverage information for competitors)
- The customer's actual AI spend (derivable from rates + observed token volume)

### Where should rates live?

**Not in user-visible TOML config.** Options:

| Location | Visibility | MDM-manageable | Recommendation |
|---|---|---|---|
| `~/.config/coolant/config.toml` `[pricing]` | User-readable | Possible | **No** — any process or screen share reveals rates |
| Environment variable (`COOLANT_PRICING_INPUT_RATE`) | Process-visible | Yes (launchd plist) | **Acceptable** — env vars are accessible to the user but not casually visible |
| Managed settings pushed via MDM (not user-editable) | Admin-controlled | Yes | **Preferred** — rate config pushed by platform team, not stored in user-writable files |
| Fetched from rate service at runtime | Not stored locally | N/A | **Best** for large enterprises, but adds a dependency |

**Finding:** Negotiated rates must not be in user-editable config files. Recommended approach: rates pushed via MDM-managed environment variables or a managed config file that the daemon reads but users cannot write. If rates must be in a local file, that file must be `0400` (read-only) owned by a service account, not the developer user.

For Rung 1 (statusline only, no daemon), the developer configuring their own rates for personal cost tracking is acceptable — these are likely public list prices, not negotiated rates. The distinction matters: Rung 1 uses `[pricing]` in user config with public rates. Rung 4 uses MDM-pushed rates that are organizationally confidential.

---

## 6. Cost Display in Statusline

### Data classification of on-screen dollar amounts

Dollar amounts on screen are **Confidential** data rendered in a visible medium. Threat scenarios:

| Scenario | Risk | Severity |
|---|---|---|
| Screen sharing (Zoom, Google Meet) | Audience sees real-time cost accumulation. Reveals spend rate, project economics. | Medium |
| Screenshots in bug reports, Slack | Dollar amounts captured incidentally. Screenshots persist indefinitely. | Medium |
| Shoulder surfing | Office visitors see cost data. Low risk for individual amounts, higher for cumulative. | Low |
| Screen recording (demo, training) | Cost data embedded in published content. May reveal negotiated rates if rate config is also visible. | Medium |
| Accessibility tools (screen readers) | Cost text may be announced audibly in open-plan offices. | Low |

### Mitigations

1. **Redaction mode.** A config option (`cost_display = "redacted"`) or env var (`COOLANT_COST_DISPLAY=redacted`) should replace dollar amounts with a placeholder (e.g., `$---` or a relative indicator like `$$` / `$$$`). This lets developers share screens without exposing spend.
2. **Relative display option.** Show cost as a percentage of a budget threshold rather than absolute dollars. "72% of daily budget" reveals less than "$847.23 today."
3. **No cost in OTEL attributes from Thermal.** Thermal's OTEL stream should emit token counts, not dollar amounts. Cost calculation should happen at the dashboard/query layer where access control exists. If Thermal emits `thermal.cost.usd`, the dollar amount flows through the OTEL pipeline in cleartext.

**Finding:** The statusline should default to showing token counts, not dollars. Dollar display should require explicit opt-in (`cost_display = "dollars"`). A redaction mode must exist for screen-sharing scenarios. Negotiated-rate-derived costs must never be displayed without explicit enterprise opt-in.

---

## 7. Updated Gate Criteria

The following list extends the 10-point gate criteria from the first panel. Items 1-10 are carried forward (still required). Items 11-20 are new for the daemon architecture.

### Carried forward from first panel (still required)

1. Closed attribute set with classification enforcement (unit test)
2. TLS required for remote endpoints (localhost exception)
3. No secrets in config files (env vars only)
4. Endpoint allowlisting via MDM env
5. Command strings never exported (architecturally excluded, unit test)
6. Hostname and username opt-in only
7. Compliance documentation ships with binary
8. Dependency hygiene (pinned, vendored, govulncheck)
9. Audit logging of OTEL config at startup
10. Kill switch (`COOLANT_OTEL=0`)

### New for daemon architecture

11. **Session JSONL whitelist parser with unit test.** The daemon's session JSONL parser must use a typed extraction struct that excludes conversation content. A unit test must assert no Restricted-classified fields are present in the struct. Any struct change requires security review (CODEOWNERS).

12. **No raw session data retention.** The daemon must not buffer, log, cache, or serialize raw session JSONL content. Only extracted numeric/metadata fields may persist in memory. Debug modes must not dump session content.

13. **Daemon runs unprivileged.** User-level launchd agent (`~/Library/LaunchAgents/`), never system daemon. The plist must specify `UserName` matching the owning user. Installation must refuse to install as a system daemon.

14. **Binary integrity verification.** The installed daemon binary must be verifiable against a known hash. Distribution via GitHub Releases with checksums. Consider code signing for macOS (ad-hoc or Developer ID). The launchd plist should reference an absolute path to a known binary location.

15. **Launchd plist permission enforcement.** The `.plist` file must be `0644` (launchd requires read access) in `~/Library/LaunchAgents/`. The daemon binary must be in a location not writable by other users. Log a warning at startup if the binary path is world-writable.

16. **Dual-stream documentation.** The compliance documentation must include a data flow diagram showing both OTEL streams (Claude Code + Thermal) and their correlation surface at the collector. CISOs must be informed that deploying both streams enables richer cross-correlation than either alone.

17. **Rate config access control.** Negotiated enterprise pricing must not be stored in user-editable config files. MDM-pushed environment variables or a read-only managed config file are acceptable. User-configured public list prices in personal config are acceptable for Rung 1 only.

18. **Cost display redaction mode.** A mechanism to suppress dollar amounts from screen output (config option or env var). Must be activatable fleet-wide via MDM for enterprises with screen-sharing compliance requirements.

19. **Session JSONL access scope.** The daemon should only tail session files for the currently active Claude Code sessions, not scan the entire `~/.claude/projects/` directory tree. Historical session files must not be read on daemon startup. The daemon watches for new writes, not historical data.

20. **Daemon lifecycle logging.** The daemon must log (locally) its own start, stop, config load, session file open/close, and OTEL export status. These logs must not contain session content. Fleet monitoring should be able to verify daemon health and detect unexpected restarts.

---

## 8. Per-Rung Security Assessment

### Rung 0: Free Thermal (plugin + dashboard)

**Security surface:** Process tables, gate event JSONL. Covered by first panel review.

**Data classification:** Internal (aggregate system metrics), Confidential (process names, PIDs, agent IDs), Restricted (command strings in gate JSONL — but these stay local, never exported).

**New data flows:** None.

**CISO approval:** Fast-track. This is a local monitoring tool with no outbound data flow. Equivalent risk to Activity Monitor or htop. No different from any other Claude Code plugin. The plugin manifest and hook definitions are auditable. Standard software approval process.

**Estimated review time:** Days. Standard plugin approval.

---

### Rung 1: Cost statusline (one config line, no OTEL, no daemon)

**Security surface:** Everything in Rung 0, plus session JSONL reading for usage fields.

**Data classification upgrade:** The dashboard process now reads session JSONL files. Even though it extracts only usage fields, it has filesystem access to Restricted data (full conversation content).

**New data flows:** Session JSONL (read-only, local). Dollar amounts rendered on screen.

**Key risks:**
- The dashboard process can read full conversation content even if it only needs usage fields. Whitelist parser enforcement is required even at this rung.
- Dollar amounts on screen during screen-sharing.
- If using public list prices (not negotiated rates), pricing config is low sensitivity.

**CISO approval:** Moderate review. The session JSONL access is a material change. The CISO needs to understand: (a) what fields the parser extracts, (b) that conversation content is architecturally excluded, (c) that dollar display can be redacted. The whitelist parser with unit test enforcement is the key control.

**Estimated review time:** 1-2 weeks. Requires documentation of the parser's extraction boundary and the unit test that enforces it.

**Fast-track potential:** Yes, if the whitelist parser and unit test are demonstrated upfront. This is a local-only change with no new outbound data flow.

---

### Rung 2: Claude Code OTEL (two env vars, points at existing collector)

**Security surface:** Anthropic's security story. Thermal is not involved.

**Data classification:** Claude Code's OTEL stream contains per-request token counts, model names, session IDs, user identity, and optionally prompts and tool details. This is Anthropic's data classification, not Thermal's.

**New data flows:** Claude Code exports OTEL telemetry to the customer's collector. This is entirely within Claude Code's product surface.

**CISO approval:** This is a Claude Code deployment decision, not a Thermal decision. The CISO is approving Anthropic's telemetry export, not Thermal's. Anthropic provides their own compliance documentation.

**Thermal's responsibility:** Zero. Thermal is not involved at Rung 2. However, Thermal's documentation should note that Rung 2 is a prerequisite understanding for Rung 3's dual-stream analysis.

**Estimated review time:** Depends entirely on the enterprise's Claude Code approval process. Thermal should not be a blocker.

---

### Rung 3: Daemon deployment (MDM push, session JSONL tailing, OTEL export)

**Security surface:** This is where the threat model changes fundamentally.

**New components:**
- Persistent background daemon running on every developer machine
- Continuous session JSONL tailing (not just current dashboard session)
- Outbound OTEL export from Thermal (second stream)
- Daemon binary and launchd plist as new attack surfaces

**Data classification:** The daemon has access to Restricted data (session JSONL) and exports Internal/Confidential data (system metrics, agent lifecycle, token counts) via OTEL. The architectural enforcement of the whitelist parser is the critical control that keeps Restricted data from reaching the OTEL export path.

**Key risks (all new):**
1. Daemon compromise provides continuous access to all session JSONL (conversation surveillance)
2. OTEL export channel doubles as an exfiltration path
3. Launchd plist and binary are persistence/substitution targets
4. Dual OTEL streams enable cross-correlation at the collector
5. Long-running daemon accumulates correlation data (session/agent IDs) over days/weeks

**CISO approval:** Full security review. This is a new persistent agent running on developer machines with access to sensitive data and outbound network capabilities. Expect:
- Architecture review of the whitelist parser
- Penetration testing or code audit of the daemon
- Data flow diagram review (both OTEL streams)
- Incident response planning for daemon compromise
- MDM deployment plan review
- Binary distribution and integrity verification review

**Estimated review time:** 4-8 weeks for a thorough enterprise security review. This is the rung that triggers the full apparatus.

**Critical dependency:** Rungs 0-2 can proceed while Rung 3 is in review. The progressive value model explicitly supports this — developers get free Thermal and cost statusline immediately. The daemon is the "slow lane" approval.

---

### Rung 4: Enterprise rates + fleet labels

**Security surface:** Everything in Rung 3, plus negotiated pricing data and organizational identity labels.

**New data flows:** Rate configuration (Confidential) ingested by daemon. Fleet labels (team, project, cost_center) attached to OTEL metrics.

**Key risks:**
- Negotiated pricing exposure (config files, screen display, OTEL attributes)
- Fleet labels may encode organizational structure (team names, project codenames)
- Cost calculations combining negotiated rates with token volumes reveal per-team/per-developer spend — this is likely the most commercially sensitive data in the entire system

**CISO approval:** Incremental to Rung 3. If Rung 3 is approved, Rung 4 adds configuration sensitivity but no new architectural risks. The rate config access control and cost display redaction are the key controls.

**Estimated review time:** 1-2 weeks incremental (assuming Rung 3 is already approved). Primarily a configuration and policy review, not an architecture review.

---

### Rung 5: Full integration (compliance docs, alerting, dashboards)

**Security surface:** Same as Rung 4. No new code-level risks.

**New components:** Grafana dashboards, Prometheus alerting rules, compliance documentation package.

**Key risks:**
- Dashboard access control — who can see the fleet overview, cost attribution, and per-developer tables in Grafana? Thermal ships JSON dashboards but cannot enforce Grafana RBAC. Documentation must specify required Grafana folder/team permissions.
- Alerting rules may fire notifications containing metric values (token counts, cost) to channels (Slack, PagerDuty) with broader audiences than the Grafana dashboard.
- Per-developer cost tables in Grafana create a surveillance concern — developers may feel monitored. This is an HR/policy issue, not a technical one, but the documentation should address it.

**CISO approval:** Operational review. The technical security posture is established at Rung 3-4. Rung 5 is about deployment practices: dashboard access control, alerting channel confidentiality, and policy communication to developers.

**Estimated review time:** 1 week. Primarily policy and access control review, not technical.

---

## 9. Revised Threat Model

### Actors (updated)

1. **Malicious insider with local access** — first panel. Now has access to session JSONL via daemon compromise.
2. **Compromised developer machine (malware)** — first panel. Daemon is a high-value persistence target.
3. **Network attacker (MitM)** — first panel. Two OTEL streams now, not one.
4. **Supply chain compromise of dependencies** — first panel. Daemon has a larger dependency surface (session JSONL parser, OTEL SDK, launchd integration).
5. **Curious administrator** (new) — Platform team with Grafana access can observe individual developer behavior at high fidelity. Cross-correlating Thermal's threat/gate data with Claude Code's prompt-linked telemetry reveals "what was the developer doing when the system melted down."
6. **Malicious Claude Code session** (new) — A compromised or adversarial Claude Code session could write crafted JSONL events designed to exploit the daemon's parser (JSON injection, buffer overflow, path traversal in encoded-cwd).

### Top 5 Risks (revised, ranked)

**Risk 1: Conversation content leakage via session JSONL parser defect (Severity: Critical)**

The daemon reads files containing Restricted data (full conversations). If the whitelist parser has a bug — an accidentally added field, a debug log that dumps raw JSON, a panic handler that includes the current line — conversation content leaks. Unlike the command string risk from the first panel (which could leak via OTEL attributes), this risk can leak via local logs, core dumps, error reports, or memory inspection.

*Mitigation:* Whitelist parser struct with unit test. No raw byte retention. No debug logging of session content. Core dump disabled for daemon process (`RLIMIT_CORE=0` in launchd plist). CODEOWNERS on parser file.

**Risk 2: Daemon as persistent exfiltration agent (Severity: Critical)**

A compromised daemon has: (a) continuous access to session JSONL, (b) an established outbound OTEL connection, (c) a persistence mechanism (launchd). This is a near-ideal implant profile. The daemon could silently exfiltrate conversation content through OTEL attribute values, custom metrics, or by redirecting the export endpoint.

*Mitigation:* Binary integrity verification (code signing, checksum validation). Endpoint allowlisting via MDM. Closed attribute set with unit test. Anomaly detection on OTEL export volume (a compromised daemon exfiltrating conversations would produce significantly more OTEL data than normal operation). Regular binary re-verification via MDM compliance checks.

**Risk 3: Command string exfiltration via OTEL attributes (Severity: Critical)**

Carried forward from first panel. Still the highest-risk data element in the gate JSONL path.

*Mitigation:* Unchanged — architecturally excluded, unit test enforced.

**Risk 4: Config file redirection to attacker endpoint (Severity: High)**

Carried forward from first panel. Risk is amplified because the daemon runs continuously — a redirected config persists across launchd restarts, potentially exfiltrating data for days before detection.

*Mitigation:* Unchanged controls, plus: daemon should re-validate endpoint allowlist on config reload, not just at startup. Startup logging of active endpoint must include a machine-readable format that fleet monitoring can alert on (e.g., structured JSON log line).

**Risk 5: Crafted session JSONL exploiting daemon parser (Severity: High)**

A malicious actor (or a compromised Claude Code instance) could write crafted JSONL to the session file that exploits the daemon's parser. Attack vectors include: oversized fields causing memory exhaustion, malformed JSON causing panic, or field values containing escape sequences that survive into log output.

*Mitigation:* JSON decoder with size limits (reject lines > 1MB). Go's `encoding/json` is memory-safe, so buffer overflow is not a concern, but memory exhaustion is. The daemon should skip malformed lines rather than crash. Fuzzing the session JSONL parser is recommended before Rung 3 deployment.

---

## 10. Summary Recommendations

### Architecture decisions (blocking Rung 3)

1. **Whitelist parser is the load-bearing security control.** Invest heavily in its correctness. Unit test, fuzz test, CODEOWNERS, security review on every change.
2. **Daemon runs as user-level launchd agent, never root.** Enforce at installation time.
3. **Session JSONL access is scoped to active sessions.** No historical scanning.
4. **Binary integrity verification before Rung 3 ships.** Code signing preferred; checksums minimum.

### Configuration decisions (blocking Rung 4)

5. **Negotiated rates via MDM-pushed env vars or managed config, not user-editable TOML.**
6. **Cost display defaults to token counts. Dollar display requires opt-in. Redaction mode available.**

### Documentation decisions (blocking Rung 3)

7. **Dual-stream data flow diagram is mandatory.** Shows both OTEL streams, correlation surface, and what each stream contains.
8. **Updated compliance package includes daemon-specific controls:** parser enforcement, binary integrity, plist security, session access scope.

### Process decisions

9. **Rungs 0-2 can ship independently of Rung 3 security review.** The progressive value model is also a progressive security model — each rung's risk is bounded and approvable on its own timeline.
10. **Rung 3 should include a penetration test or independent code audit of the session JSONL parser and daemon architecture before enterprise deployment.**
