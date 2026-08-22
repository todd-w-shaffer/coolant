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

MONEY
-----
The dollar figure is computed locally from the transcript at public
list prices, per message, using that message's own model — a session
mixes models, since subagents and auxiliary calls run on cheaper ones.

It is priced per billing bucket, not from the ↓ figure above. Cache
reads bill at 0.1x the input rate and cache writes at 1.25x (5-minute
TTL) or 2x (1-hour). Because cache reads dominate a long session,
pricing the combined input total at the fresh-input rate overstates
cost by roughly 8x — on a real 5MB session here, $948 against an
actual $113.

The framing differs by provider because the number means different
things:

  ≈$113   off subscription: an estimate of real marginal spend. Public
          list rates — an organisation's negotiated Bedrock rate will
          differ, hence the approximation sign.

  +$113   on Pro/Max: value drawn against a flat fee, not a bill. The
          plan is flat-rate, so this is not a budget and cannot be
          overrun; the sesh/week bars are the constraint that binds.

Below $10 the figure carries cents; above it, whole dollars. On a
narrow terminal the money segment is dropped rather than wrapped —
the subscription layout already runs ~83 columns without it.

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
