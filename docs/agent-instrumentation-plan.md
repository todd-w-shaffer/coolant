# Agent Instrumentation Plan

Coolant has two layers of progressive disclosure:

1. **No plugin** — thermal polls `ps`/`sysctl`, shows system-level stats. Beautiful, but forensic. You see processes, not intent.
2. **With plugin** — hooks fire on agent lifecycle and tool use. You see *what Claude decided to do*, not just what the OS reports.

The `[i] install coolant plugin for agent-level insights` CTA in the thermal strip is the gate between these two worlds. This document defines what data flows in the plugin-enabled world and how it reaches the Go visualization layer.

---

## Three data layers

Each layer sees a different slice of reality. Merged, they give you the full picture.

### Layer 1: Hooks (intent + identity)

What Claude *decided* to do. Synchronous, per-event, agent-aware.

Hooks receive structured JSON on stdin. The current scripts (`agent-start.sh`, `agent-stop.sh`, `parallel-gate.sh`) **ignore stdin entirely** — they increment a counter and check a lockfile. The richest data source in the system is being discarded.

**SubagentStart stdin:**
```json
{
  "session_id": "abc-123",
  "agent_id": "def-456",
  "agent_type": "Explore",
  "transcript_path": "/path/to/parent/transcript.jsonl",
  "cwd": "/Users/todd/project",
  "hook_event_name": "SubagentStart"
}
```

**SubagentStop stdin:**
```json
{
  "session_id": "abc-123",
  "agent_id": "def-456",
  "agent_type": "Explore",
  "agent_transcript_path": "/path/to/agent/transcript.jsonl",
  "last_assistant_message": "I found 3 files matching...",
  "cwd": "/Users/todd/project",
  "hook_event_name": "SubagentStop"
}
```

**PostToolUse stdin (example: Bash tool):**
```json
{
  "session_id": "abc-123",
  "agent_id": "def-456",
  "tool_name": "Bash",
  "tool_input": {
    "command": "rg -l 'interface' src/",
    "description": "Search for interface definitions"
  },
  "tool_response": { "...": "tool-specific result" },
  "tool_use_id": "tool-789",
  "cwd": "/Users/todd/project",
  "hook_event_name": "PostToolUse"
}
```

**PostToolUse tool_input varies by tool:**
- **Bash:** `{"command": "...", "description": "...", "timeout": N}`
- **Edit:** `{"file_path": "...", "old_string": "...", "new_string": "..."}`
- **Write:** `{"file_path": "...", "content": "..."}`
- **Read:** `{"file_path": "...", "offset": N, "limit": N}`
- **Grep:** `{"pattern": "...", "path": "...", "glob": "..."}`
- **Glob:** `{"pattern": "...", "path": "..."}`

**Unique data only hooks provide:**
- `agent_id` — which specific agent (correlate everything an agent does)
- `agent_type` — Explore, Plan, general-purpose, custom
- `session_id` — ties agents to their parent session
- `transcript_path` / `agent_transcript_path` — the full conversation history
- `last_assistant_message` — what the agent concluded
- `tool_input` — the actual command, file path, search pattern
- `tool_response` — what came back from the tool
- `tool_use_id` — unique per tool invocation

### Layer 2: OpenTelemetry (economics + timing)

What Claude *cost*. Passive, async, no behavioral modification.

Claude Code has native oTel support (opt-in). It exports metrics and events, not traces/spans.

**Configuration:**
```bash
export CLAUDE_CODE_ENABLE_TELEMETRY=1
export OTEL_METRICS_EXPORTER=otlp
export OTEL_LOGS_EXPORTER=otlp
export OTEL_EXPORTER_OTLP_PROTOCOL=grpc
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317
```

**Metrics (8):**
| Metric | What it tells you |
|--------|-------------------|
| `claude_code.cost.usage` | USD cost per API call |
| `claude_code.token.usage` | Input/output/cache_read/cache_creation tokens |
| `claude_code.active_time.total` | Active time in seconds |
| `claude_code.session.count` | Sessions started |
| `claude_code.lines_of_code.count` | Lines added/removed |
| `claude_code.commit.count` | Commits created |
| `claude_code.pull_request.count` | PRs created |
| `claude_code.code_edit_tool.decision` | Edit permission decisions |

**Events (5):**
| Event | Key fields |
|-------|------------|
| `claude_code.user_prompt` | `prompt_length`, `prompt_id` (correlation ID) |
| `claude_code.tool_result` | `tool_name`, `success`, `duration_ms`, `decision_source` |
| `claude_code.api_request` | `model`, `cost_usd`, `duration_ms`, `input_tokens`, `output_tokens` |
| `claude_code.api_error` | `error`, `status_code`, `attempt` |
| `claude_code.tool_decision` | `tool_name`, `decision`, `source` |

**Unique data only oTel provides:**
- Cost in USD per API call
- Token counts (input, output, cache hit/miss)
- API latency (`duration_ms` per request)
- API errors and retry counts
- `prompt.id` — correlation ID linking all events from one user prompt

### Layer 3: ps polling (physics)

What the *machine* is doing. Already built, already great.

- CPU%, MEM%, SWAP% system-wide (`sysctl`, `vm_stat`)
- Per-process CPU, RSS, command name (`ps -Ao`)
- Process trees per Claude session (BFS descendant walk)
- Type classification (command → type code → category)
- Online/offline detection (API connectivity check)

This layer has no concept of agents, intent, or cost. It sees processes.

---

## What each layer answers

| Question | Hooks | oTel | ps |
|----------|-------|------|----|
| How many agents are running? | ✅ | | |
| What is each agent doing? | ✅ (agent_type, tool_input) | | |
| How long has each agent been alive? | ✅ (start/stop timestamps) | | |
| What tools is each agent using? | ✅ (PostToolUse) | | |
| What files is each agent touching? | ✅ (tool_input.file_path) | | |
| What commands is each agent running? | ✅ (tool_input.command) | | |
| What did an agent conclude? | ✅ (last_assistant_message) | | |
| Is an agent's context getting compacted? | ✅ (PreCompact/PostCompact) | | |
| How much did that agent cost? | | ✅ (cost.usage) | |
| How many tokens burned? | | ✅ (token.usage) | |
| Is the API slow right now? | | ✅ (api_request duration_ms) | |
| Are API calls failing? | | ✅ (api_error) | |
| Is the machine overloaded? | | | ✅ |
| Which process category is hottest? | | | ✅ |
| How much memory per session tree? | | | ✅ |
| What's the spawn/death rate? | | | ✅ |

The correlation story: an agent starts (hook) → spawns processes (ps) → makes API calls (oTel) → uses tools (hook) → its processes eat memory (ps) → it finishes (hook). One entity, three lenses.

---

## Available hook types

Claude Code supports 26 hook event types. Coolant currently wires 3.

### Currently wired

| Hook | Matcher | Script | What it does today |
|------|---------|--------|--------------------|
| `SubagentStart` | `.*` | `agent-start.sh` | Increment counter, auto-engage parallel mode |
| `SubagentStop` | `.*` | `agent-stop.sh` | Decrement counter, auto-disengage |
| `PostToolUse` | `Edit\|Write` | `parallel-gate.sh` | Suppress typecheck in parallel mode |

### Priority candidates to wire

| Hook | Why | Data it gives |
|------|-----|---------------|
| `PostToolUse` (all tools) | Real-time activity stream per agent | tool_name, tool_input, tool_response, duration |
| `Stop` | Know when Claude finishes a turn | session_id, combined with SubagentStop for full lifecycle |
| `TaskCreated` / `TaskCompleted` | Agent-internal task tracking | See an agent break work into steps |
| `PreCompact` / `PostCompact` | Context pressure signal | Agent has been working hard enough to compact |
| `PostToolUseFailure` | Tool failures per agent | Which agent hit errors, what failed |
| `UserPromptSubmit` | Human interaction events | When the human drives vs. agents working autonomously |

### Lower priority but available

| Hook | Use case |
|------|----------|
| `SessionStart` / `SessionEnd` | Session lifecycle |
| `Notification` | Claude sending notifications |
| `CwdChanged` | Working directory changes |
| `FileChanged` | External file modifications |
| `WorktreeCreate` / `WorktreeRemove` | Worktree isolation events |
| `PreToolUse` | What Claude wants to do before doing it |
| `PermissionRequest` / `PermissionDenied` | Permission flow |
| `ConfigChange` | Config changes mid-session |
| `InstructionsLoaded` | CLAUDE.md loading |

---

## Hook output protocol

Hooks communicate back to Claude Code via stdout JSON and exit codes.

**Exit codes:**
- `0` — success, stdout JSON processed
- `2` — blocking error, stderr shown to user, operation blocked
- Other — non-blocking error, stderr shown in verbose mode

**Output JSON schema:**
```json
{
  "continue": true,
  "systemMessage": "string shown to Claude as context",
  "decision": "block",
  "reason": "string",
  "hookSpecificOutput": {
    "hookEventName": "PreToolUse",
    "permissionDecision": "allow|deny|ask|defer",
    "updatedInput": {},
    "additionalContext": "string"
  }
}
```

**Environment variables available to all hook scripts:**
- `CLAUDE_PROJECT_DIR` — project root
- `CLAUDE_PLUGIN_ROOT` — plugin installation directory
- `CLAUDE_PLUGIN_DATA` — plugin persistent data directory
- `CLAUDE_CODE_REMOTE` — `"true"` if web environment

---

## The pipe: JSONL event stream

The boundary between bash hooks and Go visualization is a JSONL file. This follows the existing project convention: bash for hooks/plumbing, Go for visualization, structured data at the boundary.

**Path:** `/tmp/coolant-$USER.events.jsonl`

Each hook script reads stdin JSON, extracts relevant fields, writes one JSONL line.

### Event schema

All events share a common envelope:

```jsonl
{"ts":"...","event":"...","session_id":"...","agent_id":"...","agent_type":"..."}
```

**Agent lifecycle:**
```jsonl
{"ts":"2026-04-02T14:03:01Z","event":"agent.start","session_id":"abc","agent_id":"def","agent_type":"Explore","cwd":"/path"}
{"ts":"2026-04-02T14:03:32Z","event":"agent.stop","session_id":"abc","agent_id":"def","agent_type":"Explore","duration_s":31,"last_message_preview":"Found 3 files..."}
```

**Tool use:**
```jsonl
{"ts":"2026-04-02T14:03:02Z","event":"tool.use","session_id":"abc","agent_id":"def","tool":"Bash","input_preview":"rg -l 'interface' src/"}
{"ts":"2026-04-02T14:03:02Z","event":"tool.use","session_id":"abc","agent_id":"def","tool":"Read","input_preview":"src/types.ts"}
{"ts":"2026-04-02T14:03:05Z","event":"tool.done","session_id":"abc","agent_id":"def","tool":"Bash","success":true,"duration_ms":340}
```

**Task lifecycle:**
```jsonl
{"ts":"2026-04-02T14:03:03Z","event":"task.created","session_id":"abc","agent_id":"def","task_id":"ghi"}
{"ts":"2026-04-02T14:03:28Z","event":"task.completed","session_id":"abc","agent_id":"def","task_id":"ghi"}
```

**Context pressure:**
```jsonl
{"ts":"2026-04-02T14:05:00Z","event":"context.compact","session_id":"abc","agent_id":"def","phase":"pre"}
{"ts":"2026-04-02T14:05:02Z","event":"context.compact","session_id":"abc","agent_id":"def","phase":"post"}
```

**Parallel mode (existing behavior, now also in JSONL):**
```jsonl
{"ts":"2026-04-02T14:03:01Z","event":"parallel.engaged","agent_count":3,"threshold":3}
{"ts":"2026-04-02T14:03:32Z","event":"parallel.disengaged","agent_count":0}
```

**Tool suppression (existing behavior):**
```jsonl
{"ts":"2026-04-02T14:03:02Z","event":"tool.suppressed","tool":"Edit","reason":"parallel_mode"}
```

### Go consumption

The Go collector already runs a goroutine for ps polling. Add a second goroutine that tails `/tmp/coolant-$USER.events.jsonl`:

```
┌──────────────┐     ┌──────────────────┐
│  ps polling   │────▶│                  │
│  (1Hz)        │     │   bubbletea      │
└──────────────┘     │   Update loop    │
                      │                  │
┌──────────────┐     │   merges into    │
│  JSONL tail   │────▶│   AppState       │
│  (fsnotify)   │     │                  │
└──────────────┘     └──────────────────┘

                      (future)
┌──────────────┐
│  oTel OTLP    │────▶  same channel
│  receiver     │
└──────────────┘
```

The JSONL tailer sends `agentEventMsg` into bubbletea's program channel. The Update loop merges agent events with system snapshots in `AppState`. New state fields:

- `ActiveAgents map[string]*AgentState` — keyed by agent_id
- `AgentHistory []AgentEvent` — rolling event log
- Per-agent: type, start time, elapsed, tool activity, last tool, status

### oTel integration (future)

Two options:
1. **OTLP receiver in Go** — thermal listens on `localhost:4317`, receives metrics/events, merges into AppState. Most direct.
2. **oTel → JSONL bridge** — lightweight sidecar (bash or Go) receives OTLP, writes JSONL events to the same file. Simpler, reuses existing pipe.

Option 2 is more consistent with the project architecture. The JSONL file becomes the universal event bus.

---

## Implementation priority

Ordered by data richness per effort:

### Phase 1: Enrich existing hooks
- Read stdin JSON in `agent-start.sh` and `agent-stop.sh`
- Extract `agent_id`, `agent_type`, `session_id`
- Write JSONL events to `/tmp/coolant-$USER.events.jsonl`
- Keep existing counter/lockfile behavior (parallel mode still works)
- **Effort:** Small. Same scripts, add `jq` or awk JSON parsing.

### Phase 2: Wire PostToolUse for all tools
- Add new PostToolUse hook with `.*` matcher
- Extract `tool_name`, `tool_input` preview, `agent_id`
- Write JSONL tool events
- **Effort:** One new script, one new hook entry. Fire hose of data.

### Phase 3: Go JSONL consumer
- New goroutine in collector that tails the JSONL file
- New `AgentEvent` type in `collector/types.go`
- New agent state in `model/state.go`
- Feed into bubbletea Update loop
- **Effort:** Moderate. New collector, new state, but follows existing patterns.

### Phase 4: Wire additional hooks
- `TaskCreated` / `TaskCompleted` — task-level tracking
- `PreCompact` / `PostCompact` — context pressure
- `Stop` — turn completion
- `PostToolUseFailure` — error tracking
- **Effort:** Small per hook, uses same JSONL pipe.

### Phase 5: oTel sidecar
- Lightweight OTLP receiver → JSONL bridge
- Cost, tokens, API latency per request
- Correlation via `prompt.id`
- **Effort:** Larger. New component. But passive — doesn't affect existing hooks.

---

## What this enables (visualization possibilities)

Not building these yet — but the instrumentation should be rich enough to support all of them.

- **Agent lanes** — per-agent horizontal bars showing lifecycle, colored by type
- **Tool activity stream** — real-time feed of what each agent is doing right now
- **Agent-attributed process counts** — "agent-2 is responsible for 18 of those 41 test procs"
- **Category sparklines with agent correlation** — test count spiked *because* agent-3 started a test run
- **Cost ticker** — running USD cost, tokens burned, cost per agent
- **API latency sparkline** — see slow API responses in real time
- **Context pressure indicator** — agent is compacting, it's been working hard
- **Task progress** — agent broke work into 5 tasks, 3 done, 2 in progress
- **Agent waterfall** — scrolling log of agent starts/stops with duration and outcome
- **Done-state animation** — all agents finished, thermal bar cools, summary appears
