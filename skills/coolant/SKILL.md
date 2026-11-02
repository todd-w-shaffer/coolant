---
name: coolant
description: Show the machine-wide work queue — what heavy jobs are running, what is waiting, and estimates. Use when the user says /coolant, asks what is running or queued, or asks why a command is waiting.
---

# Coolant

Coolant keeps one machine-wide work queue for CPU- and memory-heavy jobs
(test suites, typechecks, lints, builds) across every repo and session.
Typed heavy commands are routed into it automatically by the PreToolUse
gate; scripts and hooks enter it through the door:

```bash
coolant run --class <suite|files|typecheck|lint|check|build> -- <command...>
```

## Usage

`/coolant` (or `/coolant status`) shows the horizon:

```bash
bash ${CLAUDE_SKILL_DIR}/../../scripts/coolant queue
```

`/coolant json` shows the same as JSON:

```bash
bash ${CLAUDE_SKILL_DIR}/../../scripts/coolant queue --json
```

Report what is running, what is waiting and in what order, and the
estimates. If a command of the user's is waiting, say what is ahead of it.

## What to know

- **Observe vs enforce.** In observe mode (the default) nothing waits;
  every job is recorded to `~/.coolant/runs.jsonl`. In enforce mode a job
  waits while the running total would exceed the core budget or memory
  ceiling in `~/.config/coolant/config.toml` (`[queue]`).
- **Interactive sessions go first.** A session without `COOLANT_LANE` is
  interactive and goes to the front of the waiting line. A running job is
  never stopped.
- **No timeouts.** A wait ends when the job ahead ends. Do not kill a
  waiting command to "unstick" it — check `coolant queue` for what is
  ahead.
- **Long waits and the Bash tool timeout.** A queued suite can wait past
  the Bash tool's limit. Run whole suites in the background
  (`run_in_background`) so a wait never costs the result.
