Claude Code Status Line — Braille Progress Bars
================================================

Shows context window usage as a braille progress bar with thermometer
coloring (green → yellow → red), plus cumulative session token counts
and the git branch.

On a Claude.ai Pro/Max subscription it additionally shows 5-hour
("sesh") and weekly ("week") rate-limit bars and a countdown to the
next 5-hour reset.

PROVIDER BEHAVIOR
-----------------
The rate_limits data Claude Code sends is a Claude.ai subscription
signal — it is absent on Amazon Bedrock, Vertex AI, and plain API-key
auth. Rather than render two permanently-empty bars, the status line
detects this and drops the sesh/week/countdown segment entirely.

The token counts come from the session transcript, not from the
status line payload, because the payload has no session-cumulative
token field: context_window.total_input_tokens / total_output_tokens
describe the *current context window* from the most recent response,
so the output figure falls whenever a short reply follows a long one.
Reading the transcript gives true running totals and works on any
provider that returns usage data.

Note that ↓ counts cache reads and cache writes alongside fresh input,
matching how Claude Code defines total_input_tokens. On a long session
cache reads dominate, so ↓ will be far larger than ↑.

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
