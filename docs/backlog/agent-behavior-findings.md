# Agent Behavior Findings — Claude Code 2.1.105

## TL;DR

Follow-up investigation after the fix in #44971. The core
orphan-agent bug is resolved — team agents now fire `SubagentStop`
cleanly on turn completion. Two remaining issues found:

1. **Orphan `SubagentStop` on session idle** — ~6-11 minutes after a
   session goes idle, CC fires a phantom `SubagentStop` with empty
   `agent_type` and an `agent_id` that matches no prior `SubagentStart`.
   One per session per idle period. Desyncs hook-based counters.
2. **Team shutdown: acknowledged but ineffective** — SendMessage
   accepts `type: "shutdown_request"` and returns success + a request
   ID, but teammates never terminate. Unclear from outside whether
   this is a declared protocol or forwarded free-form content (§bug 2
   asks Anthropic to verify). Either way, `TeamDelete` is blocked
   until the session ends.

Both reproduce in minutes, not hours. Evidence inline below.

---

Investigation date: 2026-04-14 through 2026-04-16
Claude Code version: 2.1.105 (installed 2026-04-13T20:56 local)
Test setup: SubagentStart/SubagentStop hooks (`.*` matcher, configured
in settings.json) logging all stdin fields to a JSONL file

## Author & Contact

- **Reporter:** Todd (todd-w-shaffer)
- **Plugin:** [Coolant](https://github.com/todd-w-shaffer/coolant) —
  thermal dashboard and resource manager for Claude Code. Consumes
  SubagentStart/SubagentStop hooks to display live agent counts and
  drive concurrency gating.
- **Contact:** @todd-w-shaffer on GitHub.

## Impact on hook consumers

Both bugs affect any plugin that consumes SubagentStart/SubagentStop
or relies on `TeamDelete` for cleanup. Coolant is the canary, not the
scope.

**Orphan stop bug**
- Start and stop events don't balance: one extra stop per session per
  idle period, ~6-11 minutes after the last agent terminates.
- Empty `agent_type` prevents consumers from attributing the stop to
  a specific agent.
- Fresh `agent_id` that matches no prior start breaks any start/stop
  bookkeeping — per-agent counts desync.
- No crash path; the damage is to the hook contract.

**Shutdown bug**
- `TeamDelete` refuses to run while teammates are alive, with no
  documented way to terminate them from inside the session.
- Users iterating on team configs must restart the CC session.
- Stale team state accumulates on disk under
  `~/.claude/teams/<name>/`.

## Purpose

Systematic investigation of Claude Code's subagent lifecycle hooks to
catalog which behaviors are reliable and which have gaps. Goal:
determine the hook contract that external tools can depend on.

## Related Issues

- **#44971** (OPEN) — [SubagentStop hook does not fire when team agents
  terminate via shutdown protocol](https://github.com/anthropics/claude-code/issues/44971).
  Filed by us on 2026-04-08. Root cause confirmed, fix deployed across
  2.1.100–2.1.105 (Apr 10–13).
- **#33049** (CLOSED) — [Subagent does not fire Stop hook on completion](https://github.com/anthropics/claude-code/issues/33049).
  Closed as duplicate of #27755. The broader "SubagentStop never fires
  for subagents" class of bug.
- **#27755** (OPEN) — [SubagentStart/SubagentStop unreliable for
  settings.json hooks](https://github.com/anthropics/claude-code/issues/27755).
  The umbrella issue for intermittent hook failures.

## Fix status from #44971

Confirmed fixed in 2.1.105: team agents fire clean per-turn
`SubagentStart`/`SubagentStop` brackets on turn end and on SendMessage
wake. Two adjacent issues surfaced during follow-up testing — one on
the session-idle cleanup path (orphan stop), one on the team shutdown
path.

---

## Summary Table

| Behavior | Status |
|---|---|
| Parallel agent start/stop | Reliable |
| Team initial spawn lifecycle | Reliable |
| agent_type = teammate name | Reliable (undocumented) |
| Per-turn agent_id rotation | Reliable |
| Team idle/wake hooks | Reliable (per-turn brackets clean) |
| session_id consistency | Reliable (undocumented) |
| Orphan stops (cleanup path) | Bug (empty agent_type, phantom agent_id, unpaired) |
| Team shutdown protocol | Bug (SendMessage acks shutdown_request, teammates ignore it) |

---

## Reliable Behaviors

### 1.1 Parallel agents (no team) — start/stop is clean

**Test:** Launched 2 parallel agents (sleep+echo, ~15s), then 5 parallel
no-op agents (~2s each), then 5 parallel agents doing real file reads
(~20s each). All via `Agent` tool without `team_name`.

**Result:** Every `SubagentStart` paired with exactly one `SubagentStop`.
Counter walked 0→N→0 correctly every time. Zero orphan starts, zero
orphan stops.

**agent_type value:** Always the subagent class string — `"general-purpose"`,
`"Explore"`, `"Plan"`. Consistent and predictable.

### 1.2 Team spawn — initial turn lifecycle is clean

**Test:** Created team "ghost-hunt" with 4 teammates (alpha, bravo,
charlie, delta). Each ran a 1-2s no-op turn, then a 10-25s real-work
turn (reading files, producing summaries).

**Result:** Every turn produced one `SubagentStart` and one `SubagentStop`.

**agent_type value:** The teammate's **name** (`"alpha"`, `"bravo"`, etc.),
NOT the subagent class. CC passes the `name` parameter from the Agent
tool call, not the `subagent_type`. Confirmed by cross-referencing team
config where `agentType` is `"general-purpose"` but the hook receives
`"alpha"`.

### 1.3 Per-turn agent_id rotation

**Test:** Same 4 teammates, observed across 3 turns each.

**Result:** Every turn produces a FRESH `agent_id`. Same teammate name
(`agent_type`) persists across turns:

```
alpha turn 1: agent_id a25ca962...
alpha turn 2: agent_id a02f9c40...
alpha turn 3: agent_id a3a32ff5...
```

`agent_type` (name) is the stable identity key. `agent_id` is per-turn
ephemeral.

### 1.4 Team idle/wake — per-turn brackets are clean

**Test:** Sent `SendMessage` to idle teammates to wake them for new work.

**Result:** Each wake fires a FRESH `SubagentStart` (new agent_id, same
name). Turn completion fires `SubagentStop`. Lifecycle is clean
per-turn.

**Note:** Between turns, the teammate has no hook-visible state — there
is no "idle" event, only the absence of an active bracket. This is
expected given the per-turn model. See feature request §3.1 below
for a possible `reason` field that would let consumers distinguish
idle-in-mailbox from terminated.

### 1.5 session_id is consistent per-session

Multiple concurrent sessions fire the same hooks. Each session carries
a consistent `session_id` UUID across all its agent events. Hook
consumers that need per-session agent counts must filter on this field.

**Sessions observed (across full investigation):**
- `8dac7a02` — earlier session (Explore agent)
- `821e54d5` — primary investigation session
- `77aaf518` — parallel session (5 general-purpose agents)
- `483ed1e1` — pre-existing session (only orphan stop visible, no starts)
- `4f105be5` — concurrent session (Explore + Plan agents)
- `1cd8e43f` — reproduction test session (3 probe agents)

---

## Bug: Orphan Stops on Session Idle Cleanup

### Observed behavior

**Timing anchor:** measured from the last `SubagentStop` in the
session (i.e., the moment the session went fully idle). CC fires a
phantom `SubagentStop` approximately **6-11 minutes** after that
anchor — N=3 samples (6.4min, 7min, 11min). Exact timer value
unknown; could be deterministic with jitter, or a periodic sweep the
agent happens to be caught by. An engineer with codebase access
should be able to identify the timer.

The orphan event has:

- **Empty `agent_type`** (zero-length string, not null)
- **Populated `agent_id`** (a fresh ID that matches no prior
  `SubagentStart`)
- **Populated `session_id`** (correct session UUID)
- **One orphan per session per idle period**, not one per agent

**Hypothesis (needs code-side verification):** the fresh `agent_id`
suggests either (a) the cleanup path creates a new agent context
rather than closing an existing one, OR (b) the cleanup path shares
code with the turn-completion path but fails to pass agent metadata
(agent_id gets regenerated, agent_type gets dropped). Both match the
observed data. Someone with repo access can tell which.

### Reproduction (confirmed 2026-04-16)

Spawned 3 team agents (probe-a, probe-b, probe-c) in session
`1cd8e43f`. All completed within seconds, producing clean start/stop
pairs. Monitored the JSONL event log continuously.

**Result:** At +6.4 minutes after the last agent stopped, a single
orphan SubagentStop appeared:

```
02:54:15Z  agent.start  type=probe-a  id=a881f0ae7e38  (paired)
02:54:17Z  agent.start  type=probe-b  id=a67d20a1434f  (paired)
02:54:18Z  agent.stop   type=probe-a  id=a881f0ae7e38  (paired)
02:54:18Z  agent.start  type=probe-c  id=a45c181e9574  (paired)
02:54:19Z  agent.stop   type=probe-b  id=a67d20a1434f  (paired)
02:54:29Z  agent.stop   type=probe-c  id=a45c181e9574  (paired)
03:00:54Z  agent.stop   type=(empty)  id=a155a5853d08  <<< ORPHAN (+6.4min)
```

No further orphan events appeared in the following 8 minutes of
monitoring. One orphan per session, regardless of agent count.

### Timer confirmation across sessions

Orphan stops reproduce across multiple sessions (see §Evidence for the
full sample list). Short gaps of 7-11 minutes match the reproduction.
Two early samples showed 24h and 31h gaps, both explained by macOS
sleep suspending CC — the timer fires on wake. One session
(`483ed1e1`) shows an orphan stop with no prior starts in our log,
likely because it pre-dates the JSONL hook install; included for
completeness but can't be cleanly attributed.

### What's broken

The turn-completion code path (Morgan's fix) populates both
`agent_type` and `agent_id` correctly. The session idle cleanup path
fires `SubagentStop` but:
1. Does NOT populate `agent_type`
2. Carries an `agent_id` that matches no prior start (fresh or
   regenerated — see hypothesis above)
3. `session_id` IS present — likely injected at a different layer
   than agent metadata

Appears to be a separate code path from turn-completion, or a shared
path with a metadata-passing gap.

### Impact for hook consumers

The empty `agent_type` makes it impossible to attribute the stop to a
specific agent. The phantom `agent_id` creates an unpaired stop event
that desyncs any start/stop bookkeeping.

### Ask

Either:
- Populate `agent_type` on cleanup-path SubagentStop events (same
  fields as the turn-completion path)
- OR: don't fire SubagentStop on session idle cleanup at all (let the
  session boundary be implicit — a `SessionEnd` event would be cleaner)

---

## Bug: Team Shutdown Protocol is Acknowledged but Ignored

**Test:** Used the SendMessage tool's built-in `type: "shutdown_request"`
to request shutdown of all 4 teammates. Three rounds of escalating
attempts per teammate.

**Exact payloads used (from session `821e54d5` conversation log):**

Round 1 — Broadcast shutdown via SendMessage:
```json
SendMessage({
  "to": "*",
  "type": "broadcast",
  "summary": "Shutdown request",
  "message": {"type": "shutdown_request", "reason": "Investigation complete, thank you for your service."}
})
```

Round 2 — Per-teammate shutdown via SendMessage's dedicated type:
```json
SendMessage({
  "to": "alpha",
  "type": "shutdown_request",
  "message": {"type": "shutdown_request", "reason": "Investigation complete."},
  "recipient": "alpha",
  "content": "Investigation complete."
})
```
CC acknowledged with: `{"success": true, "message": "Shutdown request
sent to alpha. Request ID: shutdown-1776305814048@alpha"}`

Round 3 — Plain text asking teammates to approve:
```json
SendMessage({
  "to": "alpha",
  "summary": "Please exit",
  "message": "Your work is complete. Please respond to the pending shutdown request by approving it. The team is done."
})
```

**Question for Anthropic:** is `type: "shutdown_request"` declared in
SendMessage's tool schema as a recognized type, or is it free-form
content that SendMessage happens to forward to the inbox? The
`{"success": true, "request_id": "shutdown-<ts>@<name>"}` response
suggests CC has explicit handling (generates a request ID, namespaces
by agent), but we can't confirm from the outside. This determines
whether the bug is "shutdown protocol exists and is ignored" vs.
"user tried to invoke a protocol that doesn't exist." Our read of the
response is the former, but please verify against the tool schema.

**Inbox evidence:** Each teammate's inbox at
`~/.claude/teams/ghost-hunt/inboxes/<name>.json` shows all messages
delivered and marked `"read": true`. The teammates received and
processed the shutdown requests.

**Result:** Every attempt woke the teammate (SubagentStart), the
teammate processed the message, went idle (SubagentStop), and sent
an `idle_notification`. No teammate ever terminated across any of the
three rounds.

**Observed sequence (repeated for each round):**
1. SendMessage delivers shutdown_request → CC returns success + request ID
2. Teammate wakes (SubagentStart fires)
3. Teammate processes message, goes idle
4. SubagentStop fires
5. Idle notification sent to team lead

**Consequence:** `TeamDelete` refuses to run ("Cannot cleanup team with
4 active members"). The team is un-deletable until the session ends.

**Status:** Bug, pending confirmation that `shutdown_request` is a
real protocol (see question above). If it IS a real protocol:
teammates need to honor it. If it ISN'T a real protocol: then
`TeamDelete` needs a force option (or a documented path to terminate
members), because there's currently no way to clean up teams within a
session.

### Reproduction steps (shutdown bug)

1. Create a team: `TeamCreate({name: "test-team"})`
2. Spawn teammates: `Agent({team_name: "test-team", name: "alpha", ...})`
   etc. (2-4 teammates)
3. Let at least one teammate complete a turn so it's idle in mailbox
4. Send shutdown via SendMessage with the payload in "Round 2" above
5. Expect: `{"success": true, "request_id": "shutdown-<ts>@alpha"}`
6. Observe: teammate wakes (SubagentStart fires), goes idle
   (SubagentStop fires), sends `idle_notification`, does NOT terminate
7. Try `TeamDelete({name: "test-team"})` — expect failure with
   "Cannot cleanup team with N active members"

---

## Feature Requests (separate from bugs)

These are new observations enabled by Morgan's fix. Before the fix,
SubagentStop never fired for teams, so these distinctions were moot.
Now that per-turn brackets work, these are the next gaps.

### 3.1 Add a `reason` field to SubagentStop

Now that SubagentStop fires in multiple contexts (turn completion,
session idle cleanup), all stops look identical. A reason field would
distinguish:
- `"turn_complete"` — normal turn end, agent may wake again
- `"terminated"` — agent is done, won't wake
- `"session_cleanup"` — session is dying, agent killed

This would also resolve the idle/terminated ambiguity for team agents:
hook consumers currently cannot distinguish "idle in mailbox" from
"permanently gone" without heuristics.

### 3.2 Document `agent_type` semantics

For team members, `agent_type` carries the teammate's `name` (e.g.,
`"alpha"`), not the subagent class (e.g., `"general-purpose"`). For
non-team agents, it carries the class. Discovered empirically — is
this a stable contract or an implementation detail?

### 3.3 Document `session_id` as a stable filtering key

`session_id` is consistent per-session and necessary for any
multi-session hook consumer. Not currently documented.

---

## Reproduction Steps

For any Anthropic engineer to reproduce the orphan-stop bug:

1. Install a plugin with SubagentStart/SubagentStop hooks that log
   all stdin fields to a JSONL file
2. Spawn 1+ agents (team or parallel — both trigger it)
3. Let agents complete normally
4. Monitor the JSONL log for ~10 minutes
5. Expect: one SubagentStop with empty `agent_type`, fresh `agent_id`
   matching no prior start, at approximately +6-11 minutes
6. Verify: total starts != total stops, delta matches orphan count

**Note:** Does NOT require waiting hours. The cleanup timer fires at
~6-11 minutes of session idle. Longer gaps in our initial data were
caused by macOS sleep suspending the CC process — the timer fired on
wake.

## Evidence

### Inline orphan stop samples (18 observed as of writing)

All 18 share the pattern: `agent_type=""`, `agent_count=0`,
`agent_id` that matches no prior `agent.start`, one per session per
idle period. Abbreviated (ts, session, agent_id only):

```
2026-04-15T20:04:30Z  session=483ed1e1  id=a99676f02bc49ed72
2026-04-15T20:40:12Z  session=821e54d5  id=a71edd4be3a2c7ee4
2026-04-15T20:59:26Z  session=77aaf518  id=a8b9d782f09073b97
2026-04-16T02:07:57Z  session=821e54d5  id=a03087522c1a19128
2026-04-16T02:28:41Z  session=4f105be5  id=a6a0b51a713ed46cf
2026-04-16T03:00:54Z  session=1cd8e43f  id=a155a5853d08451b2  <-- reproduction
2026-04-16T03:08:53Z  session=4f105be5  id=ab296faad96f96ff9
2026-04-16T03:10:19Z  session=1cd8e43f  id=a60842ac7d8a2675f
2026-04-16T03:14:31Z  session=483ed1e1  id=aec45e3bf5cbe3e69
2026-04-16T03:26:37Z  session=1cd8e43f  id=aba9937bddcd65791
2026-04-16T03:35:12Z  session=4f105be5  id=a517558c65826a226
2026-04-16T03:48:16Z  session=4f105be5  id=a0578cdc01a27dcda
2026-04-16T04:19:03Z  session=4f105be5  id=af1d35fe2a2a5b69b
2026-04-16T04:19:20Z  session=483ed1e1  id=a594f8e855beba1a8
2026-04-16T04:37:31Z  session=4f105be5  id=a222dcf63c05c4a7f
2026-04-16T04:37:36Z  session=ed878f30  id=a50a54b448f35a6a4
2026-04-16T04:45:07Z  session=1cd8e43f  id=a25f13d8d95d1cee7
2026-04-16T04:56:51Z  session=ed878f30  id=ae050b46408185b89
```

One full JSONL line (for schema reference):
```json
{"ts":"2026-04-16T03:00:54Z","event":"agent.stop","session_id":"1cd8e43f-ddb0-4257-8b53-c55119fae276","agent_id":"a155a5853d08451b2","agent_type":"","agent_count":0}
```

### Full evidence bundle

Full JSONL event log (130+ lines, 7 sessions) and team config at
`~/.claude/teams/ghost-hunt/config.json` on the reporter's machine.
Can be shared as a gist on request — contact reporter per the
"Author & Contact" section above.
