# Phase 1 Launch Prompts

Three self-contained prompts for executing Phase 1 in fresh Claude Code sessions. Each is designed to be paste-and-go — no context from the brainstorm/design session required. Run them in dependency order: **1a → 1b → 1c**.

## State at handoff

- **Head commit:** `24cca9f` (plan gap patches)
- **Spec:** `docs/superpowers/specs/2026-04-11-coolant-oss-bsl-foundation-design.md`
- **Plans:** `docs/superpowers/plans/{README, 2026-04-11-phase1a-foundation, 2026-04-11-phase1b-telemetry, 2026-04-11-phase1c-presentation}.md`
- **Unresolved:** `.claude/`, untracked binaries in `thermal/`, a few untracked docs — none block Phase 1 execution.

## Before starting any phase

Run these four checks in a fresh session to confirm the local environment is ready:

```bash
# 1. Confirm head commit
git -C /Users/toddwshaffer/Desktop/apps/coolant log --oneline -1
# Expected: 24cca9f or later

# 2. Confirm baseline tests green
cd /Users/toddwshaffer/Desktop/apps/coolant
bats tests/ 2>&1 | tail -3
cd thermal && go test ./... 2>&1 | tail -5 && cd ..

# 3. Confirm dev/otel stack is running (needed for 1b smoke tests and 1c dashboards)
curl -sf http://localhost:9090/-/ready && echo "Prometheus OK"
curl -sf http://localhost:3000/api/health && echo "Grafana OK"
# If down: cd dev/otel && ./start.sh

# 4. Confirm cclaude env sourced (for OTEL_RESOURCE_ATTRIBUTES=repo=coolant)
echo "$OTEL_RESOURCE_ATTRIBUTES"
# Expected: repo=coolant
# If empty: you're not running from a cclaude-launched session — restart via `cclaude`
```

If any check fails, fix before starting the plan. The plans assume a green baseline.

---

## Launch Prompt — Plan 1a (Foundation)

**Execution mode:** inline, sequential. No subagents. Roughly 30–60 minutes of wall time.

**Paste this into a fresh Claude Code session launched from the coolant repo via `cclaude`:**

```
I'm executing Plan 1a of the Coolant Phase 1 roadmap. The plan is at:

  docs/superpowers/plans/2026-04-11-phase1a-foundation.md

Context you need to know up front:
- This is mechanical work: licensing files, repo reorganization, pkg/collector lift.
- Three tasks, all sequential. No TDD on the reorg itself; the "test" is that existing go test ./... and bats tests/ stay green.
- Commit via the /commit skill, never raw git commit. My CLAUDE.md requires this.
- ABSOLUTE RULE from my CLAUDE.md: never delete files or directories without explicit permission. Task A2 Step 12 in particular — the plan already guards against rm -rf, but re-read that step carefully before acting on it.

Use the superpowers:executing-plans skill to work through the three tasks in order. Pause at each commit step so I can review before the skill continues. When all three tasks are committed and the exit criteria at the bottom of the plan are met, stop and report status — do NOT proceed to Plan 1b from within the same session.
```

**Expected outcome when 1a completes:** three new commits on main, `thermal/` directory gone, `cmd/thermo`/`pkg/collector`/`internal/*` layout established, `LICENSE`/`NOTICE`/`enterprise/LICENSE` in place, all tests still green.

---

## Launch Prompt — Plan 1b (Telemetry)

**Execution mode:** subagent-driven-development. One serial bootstrap (Task B0), then seven parallel subagents (B1–B8), then one serial merge (B9). Roughly 60–90 minutes of wall time, most of which is subagent parallelism.

**Prerequisite:** Plan 1a fully committed and green.

**Paste this into a fresh Claude Code session:**

```
I'm executing Plan 1b of the Coolant Phase 1 roadmap. The plan is at:

  docs/superpowers/plans/2026-04-11-phase1b-telemetry.md

Context you need to know up front:
- Phase 1a must already be committed. Verify with `git log --oneline | head -10` — you should see commits landing Apache 2.0 LICENSE, the enterprise/ BSL subtree, and the thermal/ → root reorganization.
- This plan is parallel-native. Task B0 is SERIAL (builds the coolant-emit CLI that all hooks depend on). Tasks B1–B8 are PARALLEL (seven independent hook/skill tasks). Task B9 is SERIAL (final merge into hooks/hooks.json).
- Task B8 is cross-repo — it modifies /commit skill at ~/.claude/plugins/local/personal-plugins/plugins/commit-skill/skills/commit/SKILL.md, NOT this repo. Read that task's header carefully; there's a safety check to confirm the plugin directory is git-tracked before touching it.
- Strict TDD throughout: red → green → refactor. No implementation commits without tests.
- Commit via /commit skill per project convention.
- ABSOLUTE RULE: never delete files without explicit permission.

Use superpowers:subagent-driven-development. Dispatch workflow:

1. Execute Task B0 inline (you do it, not a subagent). This extracts test_helper.bash helpers and builds coolant-emit. Commit.
2. Verify B0 is green: `bin/coolant-emit counter coolant_smoketest_total marker=launch` should push a metric that appears in `curl 'http://localhost:9090/api/v1/query?query=coolant_smoketest_total'`.
3. Dispatch Tasks B1–B8 in parallel — one fresh subagent per task. The "Parallel execution guide for subagents" table near the top of the plan lists each task's file-isolation boundary; brief each subagent with ONLY its own task's section from the plan plus the top-of-file context block.
4. Review each subagent's commit as it returns. If any fails, debrief and redispatch.
5. Once all parallel tasks are committed, execute Task B9 inline (hooks/hooks.json merge). Commit.
6. Run the integration smoke test in B9 Step 4. Confirm metrics populate in Prometheus.
7. Report status with all commit SHAs; stop, do NOT proceed to Plan 1c from the same session.

One thing to watch: Task B8 may surface that the commit-skill plugin directory isn't git-tracked. If that happens, don't make any destructive edits — surface it to me and we'll decide (either `git init` the plugin or back up SKILL.md first).
```

**Expected outcome when 1b completes:** `bin/coolant-emit` built and installed, six new/extended hook scripts with bats tests, `hooks/hooks.json` wired for all Phase 1 hook points, Prometheus showing `coolant_*` metrics populating on every Claude Code action, /commit skill emitting `Coolant-Session-V1:` trailers.

---

## Launch Prompt — Plan 1c (Presentation)

**Execution mode:** inline with optional light parallelism. Roughly 20–30 minutes.

**Prerequisite:** Plan 1b fully committed, metrics flowing, at least one Claude Code session run end-to-end so Prometheus has data to visualize.

**Paste this into a fresh Claude Code session:**

```
I'm executing Plan 1c of the Coolant Phase 1 roadmap. The plan is at:

  docs/superpowers/plans/2026-04-11-phase1c-presentation.md

Context you need to know up front:
- Plans 1a and 1b must be complete. Verify: `bats tests/` all green, `bin/coolant-emit` exists, `curl 'http://localhost:9090/api/v1/query?query=coolant_prompt_total'` returns at least one series.
- This plan has three tasks: two Grafana dashboards (C1, C2) and a docs sweep (C3). Tasks C1 and C2 touch independent JSON files and can run as parallel subagents if you want; C3 can parallel with either.
- Every panel's PromQL must be live-verified against Prometheus before the dashboard is committed. Do not ship dashboards with untested queries.
- Commit via /commit skill per project convention.

Use superpowers:executing-plans for inline execution OR superpowers:subagent-driven-development with two parallel subagents (one for C1, one for C2) plus C3 inline. Either approach is fine — pick based on whether you want to watch the queries land or run it unattended.

When all three tasks are committed, verify the full Phase 1 exit criteria at the bottom of the plan:
- Both dashboards render in Grafana at http://localhost:3000/d/coolant-friction and /d/coolant-funnel
- A live Claude Code session via `cclaude` produces metrics visible on both dashboards within one refresh cycle
- CLAUDE.md reflects the new repo layout
- LICENSING.md present at repo root
- Last commit has a Coolant-Session-V1: trailer

Then stop and report — Phase 1 is complete. Do NOT tag a release or push anything without explicit approval.
```

**Expected outcome when 1c completes:** Phase 1 shipped. Two new dashboards live. Docs aligned. Ready for release tagging (which happens in a separate, deliberate step).

---

## If something goes sideways mid-execution

The plans have TDD + incremental commits as their safety net. Any task that fails can be reverted with a single `git revert <sha>` without affecting completed tasks.

**Most likely friction points and their fixes:**

| Symptom | Likely cause | Fix |
|---|---|---|
| Plan 1a Task A2 import-path sweep leaves stragglers | macOS BSD `sed -i ''` behaving unexpectedly | Grep for remaining `toddwshaffer/coolant/thermal` references and fix by hand |
| Plan 1b Task B0 smoke test metric never appears in Prometheus | `dev/otel` stack not running, or OTLP endpoint env var not sourced | Restart stack via `cd dev/otel && ./start.sh`; `source dev/otel/env.sh` in the shell running `coolant-emit` |
| Plan 1b parallel subagent edits `hooks/hooks.json` | Subagent violated isolation rule | Revert that commit, rebrief the subagent with explicit "do NOT touch hooks.json", redispatch |
| Plan 1b Task B8 finds commit-skill plugin isn't a git repo | Known — flagged in task's Step 1 | Surface to user, don't proceed without version control safety |
| Plan 1c dashboard queries return empty | Not enough session data accumulated yet | Run a few `cclaude` sessions, wait ~15 minutes for metric buildup, re-verify |

---

## Post-Phase-1 checklist (do NOT execute tonight)

After all three plans complete, the outstanding follow-ups are:

- [ ] Tag `v0.1.0` (coordinate with user)
- [ ] Tag `pkg/collector/v0.1.0` (prepared in Plan 1a Task A3)
- [ ] Push to origin (explicit approval required — never force)
- [ ] Begin Plan Phase 2 spec (daemon extraction, BSL territory)
- [ ] Legal entity formation decision (LLC for BSL copyright holder)
- [ ] Product name disambiguation (Coolant vs Thermal vs Thermal Enterprise)
- [ ] Trademark filings for "Coolant" and "Thermal"
- [ ] `security@coolant.dev` and `licensing@coolant.dev` forwarder setup

None of these are urgent. Sleep well.
