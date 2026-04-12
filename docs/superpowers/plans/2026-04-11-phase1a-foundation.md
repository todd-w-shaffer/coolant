# Phase 1a — Foundation: Licensing & Repo Reorganization

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax for tracking. This plan is serial — do NOT parallelize across tasks here.

**Goal:** Land Apache 2.0 at the root, establish the BSL 1.1 `enterprise/` subtree, collapse `thermal/` into the new repo layout with module path `github.com/toddwshaffer/coolant`, and finalize `pkg/collector` as a public package.

**Architecture:** Single repo, two Go modules (root OSS + `enterprise/` BSL stub). Thermal's existing code redistributes into top-level `cmd/`, `pkg/`, and `internal/`. No functional changes — all existing tests must remain green after the move.

**Tech Stack:** Go 1.25, bash 3.2, Apache License 2.0, Business Source License 1.1.

---

## Task A1 — Land licensing files at root and in `enterprise/` stub

**Files:**
- Create: `LICENSE`
- Create: `NOTICE`
- Create: `CONTRIBUTING.md`
- Create: `SECURITY.md`
- Create: `CODE_OF_CONDUCT.md`
- Create: `enterprise/LICENSE`
- Create: `enterprise/README.md`
- Create: `enterprise/go.mod`

- [ ] **Step 1: Fetch the canonical Apache 2.0 license text**

Run:
```bash
curl -sL https://www.apache.org/licenses/LICENSE-2.0.txt -o LICENSE
```
Verify: `head -5 LICENSE` should show "Apache License\n                           Version 2.0, January 2004".

- [ ] **Step 2: Write `NOTICE`**

Create `NOTICE` with exactly this content:
```
Coolant
Copyright 2026 Todd Shaffer

This product includes software developed by Todd Shaffer.
Licensed under the Apache License, Version 2.0 (the "License").
```

- [ ] **Step 3: Write `CONTRIBUTING.md`**

Create `CONTRIBUTING.md` with:
```markdown
# Contributing to Coolant

Thank you for your interest in contributing! A few ground rules keep this project healthy.

## Licensing by directory

- Root-level code (`cmd/`, `pkg/`, `internal/`, `scripts/`, `skills/`, `hooks/`, `tests/`, `dashboards/`) is Apache 2.0. Contributions here are welcome from anyone.
- Code under `enterprise/` is under the Business Source License 1.1. External contributions to this subtree are not accepted without a signed commercial contributor agreement — please open an issue first to discuss.

## Developer Certificate of Origin (DCO)

Every commit must be signed off under the [Developer Certificate of Origin](https://developercertificate.org). Use `git commit -s` to append the required `Signed-off-by:` line automatically. The project's CI will reject PRs without sign-off.

## Coding conventions

See `CLAUDE.md` for the full project conventions. In short:
- Bash 3.2 compatible (macOS system bash)
- Go 1.25+, direct `*testing.T` assertions, no test framework
- Strict TDD: red → green → refactor, one behavior per cycle
- Commit via the `/commit` skill, which generates a narrative commit message

## Where changes go

- Adding a new hook? `scripts/<name>.sh` + `tests/<name>.bats` + manifest entry in `hooks/hooks.json`
- Adding a new metric? Wire it through `coolant-emit` and document it in `CLAUDE.md`
- Adding a new dashboard? `dashboards/<name>.json`; verify PromQL against a live Prometheus before PR
- Changing the OTEL schema or the `Coolant-Session-V1:` trailer? Open a discussion first — these are protocol commitments

## Reporting issues

Security issues: see `SECURITY.md`. Everything else: GitHub issues.
```

- [ ] **Step 4: Write `SECURITY.md`**

Create `SECURITY.md` with:
```markdown
# Security Policy

## Supported versions

Coolant is pre-1.0 and ships only the latest release. Security fixes are applied to `main` and released as patch versions.

## Reporting a vulnerability

Please email security reports to **security@coolant.dev** (TODO: set up forwarder) or open a private security advisory on GitHub. Do not open a public issue for security-sensitive findings.

We aim to acknowledge reports within 72 hours and provide a fix or mitigation within 14 days for critical issues.

## Threat model

### Bash hooks

Coolant's hooks run as bash scripts in the user's Claude Code shell. They have the permissions of the logged-in user. The hooks:

- Read Claude Code's hook payload from stdin (JSON)
- Parse JSON using `bash` regex (no `jq` dependency), with defensive escaping in `_json_escape`
- Write to files under `$TMPDIR/coolant-$USER.*` (per-user, not `/tmp`, avoiding symlink attacks on macOS)
- Invoke `coolant-emit` (OSS Go binary) to push OTLP metrics

Hooks MUST:
- Never block Claude Code for telemetry failure (wrap emissions with `|| true`)
- Never execute code derived from hook-payload fields without escaping
- Respect a 5-second timeout configured in `hooks/hooks.json`

### Go binaries

`coolant-emit` and `thermo` are Go binaries with no network listeners. They only make outbound OTLP/HTTP requests to the endpoint configured in `OTEL_EXPORTER_OTLP_METRICS_ENDPOINT`. No elevated privileges are required.

### JSONL event log

The event log at `$TMPDIR/coolant-$USER.events.jsonl` is per-user and should be considered sensitive — it may contain session IDs, repo names, and prompt-length metadata. It is not intended to contain prompt contents. If you discover prompt contents being written to this log, report it as a vulnerability.

## Disclosure

Coordinated disclosure is preferred. We will credit reporters in release notes unless they prefer anonymity.
```

- [ ] **Step 5: Write `CODE_OF_CONDUCT.md`**

Fetch the Contributor Covenant v2.1:
```bash
curl -sL https://www.contributor-covenant.org/version/2/1/code_of_conduct.md -o CODE_OF_CONDUCT.md
```
Then open the file and replace the `[INSERT CONTACT METHOD]` placeholder with `conduct@coolant.dev (TODO: set up forwarder)`.

- [ ] **Step 6: Create `enterprise/` subtree**

Run:
```bash
mkdir -p enterprise
```

- [ ] **Step 7: Write the BSL 1.1 `enterprise/LICENSE`**

The BSL 1.1 template is published by MariaDB and has been stable since 2017. Rather than `curl`-fetch at runtime (risking 404s), source the canonical text from one of these known-good copies and save it locally:

- **Primary:** `https://raw.githubusercontent.com/mariadb-corporation/MaxScale/master/LICENSE.TXT` (MariaDB's own canonical copy)
- **Secondary:** `https://raw.githubusercontent.com/hashicorp/terraform/main/LICENSE` (HashiCorp's BSL 1.1 instance — parameters differ from ours, use only for the template body, not the parameters)

Strategy:
1. Try the primary URL with `curl -sf --max-time 10 <url> -o /tmp/bsl11.txt`. If it succeeds, verify the file starts with "Business Source License 1.1" via `head -1 /tmp/bsl11.txt`.
2. If the primary fails, try the secondary.
3. If both fail, ask the user to paste the canonical BSL 1.1 text into `enterprise/LICENSE` directly. The legal text is publicly available and the user may already have it in a reference repo.

Once the canonical template is in place as `enterprise/LICENSE`, edit the **header parameters block only** at the top of the file. These are the parameters specific to this project and must appear exactly as written:

```
Business Source License 1.1

Licensor:             Todd Shaffer
Licensed Work:        Coolant Enterprise
                      The Licensed Work is (c) 2026 Todd Shaffer.

Additional Use Grant: You may use the Licensed Work for internal
                      business purposes, personal use, evaluation,
                      and development. You may not offer the Licensed
                      Work or a derivative work to third parties as a
                      commercial service that substantially provides
                      the observability, AI attribution, or engineering
                      intelligence functionality of the Licensed Work.

Change Date:          Four years from the date the Licensed Work is
                      published.

Change License:       Apache License, Version 2.0
```

Below this, include the full BSL 1.1 legal text verbatim from the template. Do not modify the legal text — only the header parameters above.

- [ ] **Step 8: Write `enterprise/README.md`**

Create `enterprise/README.md` with:
```markdown
# Coolant Enterprise

Code in this subtree is licensed under the Business Source License 1.1 (see `LICENSE`). It converts to the Apache License 2.0 four years after each release.

## What lives here

- `cmd/coolant-daemon/` — headless process-aware collector (Phase 2, not yet implemented)
- `cmd/coolant-correlator/` — GitHub App + webhook consumer for SHA → PR attribution (Phase 3, not yet implemented)
- `internal/` — enterprise-only packages

## Why BSL?

We want the community to use, fork, study, and learn from this code — including running it inside a company. We don't want a third party to host it as a competing commercial service that would undercut our ability to fund continued development. The BSL's **Additional Use Grant** (in `LICENSE`) spells out exactly what's allowed. If your intended use falls outside the grant, email licensing@coolant.dev (TODO: set up forwarder) for a commercial license.

## Why a separate Go module?

Each Go module boundary physically prevents OSS code at the repo root from importing BSL code here. Go's module resolver is the enforcement; no convention to remember. The enterprise module imports OSS via `github.com/toddwshaffer/coolant`; local development uses a `replace ../ => ../` directive.
```

- [ ] **Step 9: Write `enterprise/go.mod`**

Create `enterprise/go.mod`:
```go
module github.com/toddwshaffer/coolant/enterprise

go 1.25

require github.com/toddwshaffer/coolant v0.0.0

replace github.com/toddwshaffer/coolant => ../
```

(The `replace` directive lets local development build against the working-tree OSS code. Published releases will use a pinned version.)

- [ ] **Step 10: Verify Apache 2.0 and BSL files render correctly**

Run:
```bash
head -3 LICENSE
head -20 enterprise/LICENSE
```
Expected: first file begins with "Apache License\n...Version 2.0"; second begins with "Business Source License 1.1" and shows the filled parameters.

- [ ] **Step 11: Commit via `/commit` skill**

Invoke `/commit`. The skill will generate a narrative commit message. Before invoking, ensure `git status` shows only the new license-related files (no accidental additions).

---

## Task A2 — Collapse `thermal/` into root layout and update module path

**Files:**
- Move: `thermal/cmd/thermal/` → `cmd/thermo/`
- Move: `thermal/cmd/brailletext/` → `cmd/brailletext/`
- Move: `thermal/cmd/swatch/` → `cmd/swatch/`
- Move: `thermal/internal/collector/` → `pkg/collector/` (will finalize API in Task A3)
- Move: `thermal/internal/{anim,theme,widgets,layout,config,ui,model,demo}/` → `internal/{anim,theme,widgets,layout,config,ui,model,demo}/`
- Move: `thermal/go.mod` content into root `go.mod`
- Move: `thermal/go.sum` → `go.sum`
- Move: `dev/otel/dashboards/` → `dashboards/`
- Modify: `dev/otel/provisioning/dashboards/default.yml` (update path reference)
- Modify: `install.sh` (update build paths)
- Modify: `.claude-plugin/plugin.json` (update any thermal/ path references)
- Modify: `CLAUDE.md` (update project-structure section)
- Delete: `thermal/` directory entirely after successful move

- [ ] **Step 1: Record baseline test state**

Run:
```bash
cd /Users/toddwshaffer/Desktop/apps/coolant
bats tests/ 2>&1 | tail -5
cd thermal && go test ./... 2>&1 | tail -20
cd ..
```
Expected: both suites fully green. Record the pass count. **If either fails, STOP and fix before proceeding.** This is the safety baseline for detecting regressions from the move.

- [ ] **Step 2: Create new top-level directories**

Run:
```bash
mkdir -p cmd pkg internal dashboards
```

- [ ] **Step 3: Move command directories**

Run:
```bash
git mv thermal/cmd/thermal cmd/thermo
git mv thermal/cmd/brailletext cmd/brailletext
git mv thermal/cmd/swatch cmd/swatch
```

Also handle archived experiments — they move intact:
```bash
git mv thermal/cmd/archive cmd/archive 2>/dev/null || true
```

- [ ] **Step 4: Move `collector` to `pkg/`**

Run:
```bash
git mv thermal/internal/collector pkg/collector
```

- [ ] **Step 5: Move remaining internal packages**

Run:
```bash
for pkg in anim theme widgets layout config ui model demo; do
  git mv "thermal/internal/$pkg" "internal/$pkg"
done
```

- [ ] **Step 6: Move `go.mod` and `go.sum`, update module path**

Run:
```bash
git mv thermal/go.mod go.mod
git mv thermal/go.sum go.sum
```

Edit `go.mod` — change the first line from:
```
module github.com/toddwshaffer/coolant/thermal
```
to:
```
module github.com/toddwshaffer/coolant
```

- [ ] **Step 7: Rewrite all import paths**

Run:
```bash
grep -rln 'github.com/toddwshaffer/coolant/thermal' --include='*.go' | \
  xargs sed -i '' 's|github.com/toddwshaffer/coolant/thermal/internal/collector|github.com/toddwshaffer/coolant/pkg/collector|g'

grep -rln 'github.com/toddwshaffer/coolant/thermal/internal' --include='*.go' | \
  xargs sed -i '' 's|github.com/toddwshaffer/coolant/thermal/internal|github.com/toddwshaffer/coolant/internal|g'

grep -rln 'github.com/toddwshaffer/coolant/thermal' --include='*.go' | \
  xargs sed -i '' 's|github.com/toddwshaffer/coolant/thermal|github.com/toddwshaffer/coolant|g'
```

Verify no stale references remain:
```bash
grep -rn 'toddwshaffer/coolant/thermal' --include='*.go' .
```
Expected: zero results.

- [ ] **Step 8: Move dashboards to product location**

Run:
```bash
git mv dev/otel/dashboards dashboards
```

Then edit `dev/otel/provisioning/dashboards/default.yml`. Find the `path:` line and update from `/var/lib/grafana/dashboards` (or similar container path) — the key is the host-mount reference in `docker-compose.yml` or `start.sh`. Open `dev/otel/start.sh` (or equivalent) and change any `./dashboards` or `dev/otel/dashboards` reference to `../../dashboards`. Confirm with:

```bash
grep -rn 'otel/dashboards\|dev/otel/dashboards' dev/otel/
```
Expected: zero results after edits.

- [ ] **Step 9: Update `install.sh`**

Open `install.sh`. Find any reference to `thermal/` in build commands or paths. Update:
- `cd thermal && go build -o ../bin/thermo ./cmd/thermal/` → `go build -o bin/thermo ./cmd/thermo/`
- Any `GOARCH=...` build commands similarly

Verify:
```bash
grep -n 'thermal/' install.sh
```
Expected: zero results.

- [ ] **Step 10: Update `.claude-plugin/plugin.json`**

Open `.claude-plugin/plugin.json`. If any path references `thermal/`, update it. Most likely untouched — plugin manifest points at `scripts/` hooks, not Go source. Confirm:
```bash
grep -n 'thermal' .claude-plugin/plugin.json
```
Expected: zero results (or a display name containing the word "thermal", which is fine).

- [ ] **Step 11: Update `CLAUDE.md` project-structure section**

Open `CLAUDE.md`. In the project-structure tree, change references:
- `thermal/` → appropriate root paths (`cmd/thermo/`, `pkg/collector/`, `internal/...`)
- `thermal/cmd/thermal/` → `cmd/thermo/`
- Build command `cd thermal && go build -o ../bin/thermo ./cmd/thermal/` → `go build -o bin/thermo ./cmd/thermo/`

Leave the Go-conventions and testing sections intact.

- [ ] **Step 12: Inspect `thermal/` contents and remove only tracked remnants — NEVER `rm -rf`**

**Hard rule (per CLAUDE.md):** never delete files without explicit user permission. The user has untracked binaries in `thermal/` (build artifacts like `thermal/brailletext`, `thermal/sparkdebug`, `thermal/swatch`) that must be preserved.

Run this inspection:
```bash
# List what's left (tracked + untracked)
find thermal/ -type f 2>/dev/null
# Separate tracked from untracked
git -C thermal/ ls-files 2>/dev/null
git status --porcelain thermal/ 2>/dev/null
```

Action rules:
- Any file listed by `git -C thermal/ ls-files` that still shows a result AFTER the moves above: something didn't move correctly — STOP and debug.
- Any file shown as `??` (untracked) in `git status --porcelain thermal/`: **leave it alone**. Ask the user what to do with each one.
- If `find thermal/ -type f` returns only untracked binaries the user wants to keep: offer to move them to `bin/archive/` (which `CLAUDE.md` already documents as the home for compiled binaries from archived experiments). Do not move without explicit user approval per file.
- If `find thermal/ -type f` returns empty AND `git -C thermal/ ls-files` returns empty: the directory contains only empty subdirectories from the moves. Safe to remove with `find thermal/ -type d -empty -delete`. Run that instead of `rm -rf`.

Do NOT run `rm -rf thermal/` under any circumstance.

- [ ] **Step 13: Run the full test suite**

Run:
```bash
go build ./...
go test ./...
bats tests/
```
Expected: all green. Pass counts match the baseline from Step 1. **If any test fails, debug before committing.** Common causes: missed import path, forgotten directory move.

- [ ] **Step 14: Rebuild the `thermo` binary and verify it runs**

Run:
```bash
go build -o bin/thermo ./cmd/thermo/
./bin/thermo --demo
```
Expected: TUI launches in demo mode. Kill with `q` or `Ctrl+C`.

- [ ] **Step 15: Commit via `/commit` skill**

Invoke `/commit`. The skill will generate the commit message from conversation context and the staged diff.

---

## Task A3 — Finalize `pkg/collector` public API (SPDX + Linux stub)

**Files:**
- Modify: every `.go` file in `pkg/collector/` (add SPDX header)
- Create: `pkg/collector/cpu_linux.go`
- Create: `pkg/collector/doc.go`
- Modify: every `.go` file in `cmd/` and `internal/` (add SPDX header)
- Modify: every `.sh` file in `scripts/` (add SPDX comment line)

- [ ] **Step 1: Add SPDX headers to `pkg/collector/*.go`**

For each `.go` file in `pkg/collector/` (including `_test.go` files), ensure the first line of the file is exactly:
```go
// SPDX-License-Identifier: Apache-2.0
```

Followed by a blank line, then the existing `package collector` declaration.

Run the following to add it to any file missing it:
```bash
for f in pkg/collector/*.go; do
  if ! head -1 "$f" | grep -q 'SPDX-License-Identifier'; then
    { echo '// SPDX-License-Identifier: Apache-2.0'; echo ''; cat "$f"; } > "$f.new" && mv "$f.new" "$f"
  fi
done
```

Verify:
```bash
head -1 pkg/collector/*.go | grep -v '==>' | sort -u
```
Expected: only `// SPDX-License-Identifier: Apache-2.0`.

- [ ] **Step 2: Add SPDX headers to `cmd/` and `internal/` Go files**

Run:
```bash
for f in $(find cmd internal -name '*.go'); do
  if ! head -1 "$f" | grep -q 'SPDX-License-Identifier'; then
    { echo '// SPDX-License-Identifier: Apache-2.0'; echo ''; cat "$f"; } > "$f.new" && mv "$f.new" "$f"
  fi
done
```

- [ ] **Step 3: Add SPDX comments to bash scripts**

For each `.sh` file in `scripts/`, ensure the second line (after the shebang) is:
```bash
# SPDX-License-Identifier: Apache-2.0
```

Run:
```bash
for f in scripts/*.sh; do
  if ! sed -n '2p' "$f" | grep -q 'SPDX-License-Identifier'; then
    awk 'NR==1{print; print "# SPDX-License-Identifier: Apache-2.0"; next} {print}' "$f" > "$f.new" && mv "$f.new" "$f"
    chmod +x "$f"
  fi
done
```

Verify:
```bash
for f in scripts/*.sh; do sed -n '1,2p' "$f"; echo '---'; done
```
Expected: each file begins with `#!/usr/bin/env bash` then the SPDX line.

- [ ] **Step 4: Write the Linux CPU stub**

Create `pkg/collector/cpu_linux.go`:
```go
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package collector

// cpuTicks returns zero-valued CPU tick deltas on Linux.
//
// A real Linux implementation using /proc/stat will land with the
// Phase 2 coolant-daemon. Until then, the collector library
// compiles on Linux but reports no CPU data.
//
// TODO(phase2): implement /proc/stat-based collection.
func cpuTicks() (userTicks, sysTicks, idleTicks uint64, err error) {
	return 0, 0, 0, nil
}
```

**Note:** the function name and signature must match what `cpu_darwin.go` exports for internal use. Before committing, open `pkg/collector/cpu_darwin.go` and confirm the function name/signature. If they differ from `cpuTicks() (userTicks, sysTicks, idleTicks uint64, err error)`, update the stub in this step to match exactly.

- [ ] **Step 5: Write `pkg/collector/doc.go`**

Create `pkg/collector/doc.go`:
```go
// SPDX-License-Identifier: Apache-2.0

// Package collector observes the Claude Code process tree and system
// state (CPU, memory, network, subagent events) and exposes a snapshot
// API. It is designed to be embedded in headless and interactive tools:
// the Coolant thermal TUI uses it for live dashboards, and the
// Coolant enterprise daemon embeds the same collector for OTLP
// emission.
//
// # Stability
//
// This package is pre-1.0. The API may change without notice in v0.x.
// Tagged v0.1.0 at the foundation lift. Expect backward-incompatible
// changes until v1.0.
//
// # Platform support
//
// CPU collection via cgo mach host_statistics on darwin. Linux has a
// no-op stub pending the Phase 2 daemon. Other platforms are not
// currently supported.
package collector
```

- [ ] **Step 6: Verify the collector builds on both platforms**

Run:
```bash
go build ./pkg/collector/
GOOS=linux go build ./pkg/collector/
```
Expected: both succeed. The second invocation tests that the Linux stub picks up cleanly via build tags.

- [ ] **Step 7: Run the full test suite one more time**

Run:
```bash
go test ./...
bats tests/
```
Expected: all green.

- [ ] **Step 8: Tag the collector library**

Run:
```bash
git tag pkg/collector/v0.1.0
```

(Per Go's semver-for-subpackages convention, the tag `pkg/collector/v0.1.0` makes the package independently versionable. We do not push the tag yet — that happens at Plan 1c's completion when the full foundation is ready to publish.)

- [ ] **Step 9: Commit via `/commit` skill**

Invoke `/commit`.

---

## Exit criteria for Plan 1a

Before starting Plan 1b, verify:

- [ ] `LICENSE`, `NOTICE`, `CONTRIBUTING.md`, `SECURITY.md`, `CODE_OF_CONDUCT.md` all present at repo root
- [ ] `enterprise/LICENSE`, `enterprise/README.md`, `enterprise/go.mod` all present
- [ ] `thermal/` directory does not exist
- [ ] `go.mod` module path is `github.com/toddwshaffer/coolant`
- [ ] `go test ./...` returns all green
- [ ] `bats tests/` returns all green
- [ ] `bin/thermo --demo` launches successfully
- [ ] Grafana (via `dev/otel/start.sh`) still picks up dashboards from the new `dashboards/` location
- [ ] Three commits landed via `/commit` skill, all with narrative commit messages

If any of the above fails, do not proceed to Plan 1b.
