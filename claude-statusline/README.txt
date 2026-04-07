Claude Code Status Line — Braille Progress Bars
================================================

Shows context window, 5-hour session, and weekly rate limit usage
as braille progress bars with thermometer coloring (green → yellow → red).

Also displays: input/output token counts, reset countdown, and git branch.

INSTALL
-------
1. Copy statusline.sh to ~/.claude/statusline.sh
2. Add this to your ~/.claude/settings.json (create if needed):

   {
     "statusLine": {
       "type": "command",
       "command": "bash ~/.claude/statusline.sh"
     }
   }

3. Requires: jq, awk, git (all standard on macOS/Linux)
4. Restart Claude Code — the status line appears at the bottom.
