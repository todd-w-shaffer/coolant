# .githooks/

Git hooks that block private/strategy content from leaking into this public
repo. Activated by `core.hooksPath = .githooks`.

**One-time setup:**

```bash
./scripts/install-hooks.sh
```

## Files

- `pre-push` — blocks pushes containing paths flagged by classification rules.
- `blocklist.txt` — path prefixes and keywords that mark content as private.
- `allowlist.txt` — exact paths and prefixes that are explicitly public
  (wins over the blocklist). Read from `HEAD:`, so allowlist extensions
  must land in a prior commit before they take effect.

A companion Claude Code PreToolUse hook at `.claude/hooks/classify-staged.sh`
runs the same classification on `git commit` calls so the failure shows up
before the commit is created.

## Bypassing

Two documented escape hatches, both deliberate:

```
git push --no-verify                       # skips pre-push
git -c core.hooksPath=/dev/null push       # also skips, equivalent
```

Use sparingly. The hook is here because honor-system rules drift.

## Editing the lists

- Adding a path to `allowlist.txt` requires its own prior commit (see above).
- Adding a path to `blocklist.txt` takes effect immediately.

## Full classification guide

See `thermal-enterprise/docs/repo-split.md` (private repo) for the rationale
and the public-vs-private decision tree.
