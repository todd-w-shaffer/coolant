# Spec stub: guard against invisible distribution staleness

**Status:** shipped
**Seeded:** 2026-04-14
**Shipped:** 2026-04-15
**Origin:** Observation during v3 release cut — work-laptop install silently shipped a 3-week-old binary because `releases/latest/` was manually promoted while `install.sh` kept pointing at it.

## Problem

Anywhere coolant claims to ship "the latest" of something, but a human is the promotion mechanism, it will drift invisibly. Users see no error — they get a successful install of a stale artifact. The v2 → v3 gap (April 8 → April 14) was six days of shipped visual work invisible to anyone who ran the installer.

The auto-release workflow (`.github/workflows/auto-release.yml`) fixes the specific case for `thermo` binaries. But the *pattern* is broader than one workflow.

## Surfaces to audit

Things users pull by name, where "latest" is a moving target:

1. **`install.sh`** — fetched from `raw.githubusercontent.com/main/install.sh`. Live-edge, self-updating. Safe.
2. **Braille statusline** (`claude-statusline/statusline.sh`) — fetched from `raw.githubusercontent.com/main/claude-statusline/statusline.sh` during install. Live-edge. Safe, but **not re-pulled** after initial install — users who installed months ago run whatever version they got.
3. **Marketplace plugin manifest** — submodule in `todd-w-shaffer/marketplace` auto-updates via `repository_dispatch` on every push. Automated, safe.
4. **`thermo` binary** — now auto-released on `thermal/**` pushes. Fixed.
5. **VHS tapes / demo assets** — not distributed; checked into repo only. N/A.

The one real gap after today's fix: **statusline never self-updates**. Users who installed in March are running March's statusline indefinitely.

## Proposed approach

Three options in increasing order of intrusiveness:

1. **Passive: version-stamp the statusline.** Add a `# version: 2026-04-14` comment and have it self-report on a flag (`statusline.sh --version`). At least `/coolant` can tell you when a user is on a stale copy.
2. **Active: `coolant upgrade` command.** Add a subcommand (or standalone `upgrade.sh`) that re-runs the fetch portion of `install.sh` without the prompts. Users run it voluntarily.
3. **Automatic: nightly check in the statusline itself.** Statusline checks `raw.githubusercontent.com/main/claude-statusline/VERSION` once per day, toasts if newer. Invasive — risks noise, network dependency in a tight path.

**Recommendation:** option 2. Option 1 is free but passive (nobody looks at versions). Option 3 pollutes the statusline's hot path. Option 2 gives users an explicit "refresh me" with zero runtime cost.

## What actually shipped

Options 1, 2, and 3 — all three, with the hot-path concern mitigated by a daily TTL cache:

- **Option 1 (passive):** `VERSION` file at repo root, auto-release workflow stamps it and a `# VERSION: x.y.z` comment in the statusline. Thermo has `--version` flag; ldflags inject the build version.
- **Option 2 (active):** `scripts/upgrade.sh` re-fetches binary + statusline, prints before/after summary. Wired as `install.sh --upgrade`.
- **Option 3 (automatic):** Both statusline and thermo check `raw.githubusercontent.com/main/VERSION` once per day (cached in `$TMPDIR/coolant-$USER.latest-version`). Statusline shows a dim yellow ⬆; thermo's notification banner shows "update available · changelog → releases/latest". `[updates]` TOML section controls TTL + opt-out.

## Open questions

- Should `install.sh` itself be versioned and self-update-prompt on re-run? Currently piping into `bash` always fetches fresh, so running the install line again is already "upgrade." Maybe just document that.
- Are there other "pinned artifact" surfaces we'd add later (e.g., a bundled Grafana dashboard JSON delivered alongside the binary) that would need similar treatment?
- If we ever bundle a config file (`~/.config/coolant/config.toml`), does upgrade preserve user edits?

## References

- Auto-release workflow: `.github/workflows/auto-release.yml`
- Install script: `install.sh`
- Statusline: `claude-statusline/statusline.sh`
- Conversation context: v3 release cut on 2026-04-14 after work-laptop install delivered a v2 binary from 2026-04-08
