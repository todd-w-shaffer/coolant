---
name: parallel
description: Toggle parallel mode to suppress per-edit typecheck hooks and stagger build validation when running multiple concurrent agents. Use when launching 3+ agents or when the user says to use parallel mode.
---

# Parallel Mode

Toggle parallel mode on or off to manage system resources during multi-agent work.

## Usage

The user will invoke this with `/coolant:parallel on`, `/coolant:parallel off`, or `/coolant:parallel status`.

Run the toggle script:

```bash
bash ${CLAUDE_SKILL_DIR}/../../scripts/toggle.sh $ARGUMENTS
```

## When parallel mode is ON

Follow these rules for the remainder of the session:

1. **Cap concurrent agents at 4.** Do not launch more than 4 agents at once. If more work remains, wait for running agents to finish before launching the next batch.
2. **Do not run `npm run check`, `npm run build`, `tsc`, or `vitest` inside parallel agents.** Let agents write code only.
3. **After all agents return**, run validation sequentially in the main context:
   ```bash
   npm run check    # typecheck + tests
   npm run build    # bundle validation
   ```
4. **After validation passes**, turn parallel mode off:
   ```bash
   bash ${CLAUDE_SKILL_DIR}/../../scripts/toggle.sh off
   ```

## When parallel mode is OFF

Normal operation. Per-edit typecheck hooks fire as usual. No agent cap enforced by coolant.
