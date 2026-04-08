---
name: coolant
description: Toggle build suppression for multi-agent work. Use when the user says /coolant, or when a threshold warning suggests it.
---

# Coolant

Toggle build suppression on or off to manage system resources during multi-agent work.

## Usage

The user will invoke this with `/coolant` (engage), `/coolant off` (disengage), or `/coolant status`.

Default action (no argument) is `on`.

Run the toggle script:

```bash
bash ${CLAUDE_SKILL_DIR}/../../scripts/toggle.sh ${ARGUMENTS:-on}
```

## When build suppression is ON

Follow these rules for the remainder of the session:

1. **Do not run validation inside parallel agents** (`tsc`, `vitest`, project-specific build/check commands). Let agents write code only.
2. **After all agents return**, run validation sequentially in the main context:
   ```bash
   npm run check    # typecheck + tests
   npm run build    # bundle validation
   ```
3. **After validation passes**, turn suppression off:
   ```bash
   bash ${CLAUDE_SKILL_DIR}/../../scripts/toggle.sh off
   ```

## When build suppression is OFF

Normal operation. Per-edit typecheck hooks fire as usual.
