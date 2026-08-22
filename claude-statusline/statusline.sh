#!/usr/bin/env bash
# VERSION: 0.32.0
# Braille progress bar status line for Claude Code
# Thermometer coloring on sesh: green < 50%, yellow 50-70%, red >= 70%
# Format: context ⣿⣿⣿⣿⣦⠀⠀⠀⠀⠀⡇  sesh ⣿⣿⣦⠀⠀⡇  week ⣿⣤⠀⠀⠀⡇

BRAILLE=(⠀ ⣀ ⣄ ⣆ ⣇ ⣧ ⣷ ⣿)

# build_bar <percentage> <num_chars>
build_bar() {
  local pct="${1:-0}" n="${2:-5}"
  pct=$(awk "BEGIN { p=$pct; if(p<0)p=0; if(p>100)p=100; print p }")
  local total_steps=$((n * 7))
  local filled
  filled=$(awk "BEGIN { printf \"%d\", ($pct / 100.0 * $total_steps) + 0.5 }")
  local bar="" i
  for (( i = 0; i < n; i++ )); do
    local lvl=$(( filled - i * 7 ))
    if (( lvl >= 7 )); then bar+="${BRAILLE[7]}"
    elif (( lvl <= 0 )); then bar+="${BRAILLE[0]}"
    else bar+="${BRAILLE[$lvl]}"
    fi
  done
  printf '%s' "$bar"
}

# build_thermo_bar <percentage> <num_chars>
# Each filled character colored by which zone it falls in
build_thermo_bar() {
  local pct="${1:-0}" n="${2:-5}"
  pct=$(awk "BEGIN { p=$pct; if(p<0)p=0; if(p>100)p=100; print p }")
  local total_steps=$((n * 7))
  local filled
  filled=$(awk "BEGIN { printf \"%d\", ($pct / 100.0 * $total_steps) + 0.5 }")
  local green='\033[32m' yellow='\033[33m' red='\033[31m' reset='\033[0m'
  local bar="" i
  for (( i = 0; i < n; i++ )); do
    local lvl=$(( filled - i * 7 ))
    if (( lvl <= 0 )); then
      bar+="${BRAILLE[0]}"
    else
      (( lvl > 7 )) && lvl=7
      local mid_pct
      mid_pct=$(awk "BEGIN { printf \"%d\", (($i + 0.5) / $n) * 100 }")
      if (( mid_pct < 50 )); then bar+="${green}${BRAILLE[$lvl]}${reset}"
      elif (( mid_pct < 70 )); then bar+="${yellow}${BRAILLE[$lvl]}${reset}"
      else bar+="${red}${BRAILLE[$lvl]}${reset}"
      fi
    fi
  done
  printf '%b' "$bar"
}

# sum_session_tokens <transcript_path>
# Echoes "<input> <output>": cumulative tokens for the whole session.
#
# The statusline payload has no session-cumulative token field —
# context_window.total_* describe the CURRENT context window from the
# most recent response, so the output figure falls whenever a short
# reply follows a long one. The transcript is the only local source of
# true totals, and it is provider-agnostic: it records whatever usage
# the API returned, which is what makes this work on Bedrock where
# rate_limits and cost are both absent.
#
# Input counts cache reads and cache writes, matching how
# context_window.total_input_tokens is defined.
#
# Streams with `inputs` rather than slurping with -s: a transcript grows
# without bound, and -s buffers the whole parsed array before summing
# (~144MB resident on a 28MB transcript, against a flat ~2.5MB here).
sum_session_tokens() {
  local tp="$1" out
  [ -n "$tp" ] && [ -r "$tp" ] || { printf '0 0'; return; }
  out=$(jq -n -r '
    reduce (inputs | select(.message.usage) | .message.usage) as $u
      ({i: 0, o: 0};
        {i: (.i + ($u.input_tokens // 0)
                + ($u.cache_creation_input_tokens // 0)
                + ($u.cache_read_input_tokens // 0)),
         o: (.o + ($u.output_tokens // 0))})
    | "\(.i) \(.o)"' "$tp" 2>/dev/null)
  # A truncated or half-written transcript must not blank the display.
  case "$out" in
    [0-9]*\ [0-9]*) printf '%s' "$out" ;;
    *) printf '0 0' ;;
  esac
}

input=$(cat)

# One jq pass for the whole payload — the status line re-renders on
# every keystroke, so each interpreter spawn per frame is real cost.
#
# has_limits: rate_limits is a Claude.ai Pro/Max subscription signal. It
# never arrives on Bedrock, Vertex, or plain API-key auth, so the two
# bars and the reset countdown that describe it are dropped wholesale
# rather than rendered permanently empty.
#
# One field per line, one `read` each, rather than a single split. A
# delimited line cannot survive an empty field here: transcript_path is
# absent off-subscription, and bash collapses runs of any IFS character
# that is also whitespace — tab included, even when IFS is set to tab
# alone — which would slide cwd into transcript and leave cwd empty.
# Line-per-field sidesteps that, and keeps paths containing spaces
# intact. Line reads are used rather than mapfile for bash 3.2.
{
  read -r ctx_pct
  read -r five_pct
  read -r week_pct
  read -r resets_at
  read -r has_limits
  read -r transcript
  read -r cwd
} <<EOF
$(echo "$input" | jq -r '
    (.context_window.used_percentage // 0),
    (.rate_limits.five_hour.used_percentage // 0),
    (.rate_limits.seven_day.used_percentage // 0),
    (.rate_limits.five_hour.resets_at // 0),
    (if .rate_limits then 1 else 0 end),
    (.transcript_path // ""),
    (.cwd // ".")' 2>/dev/null)
EOF
: "${cwd:=.}"

read -r in_tok out_tok <<<"$(sum_session_tokens "$transcript")"

# Format tokens as k/m. Integer arithmetic rather than awk: this runs
# twice per render and a fork costs more than the formatting.
fmt_tok() {
  local t="$1"
  if (( t >= 1000000 )); then
    printf '%d.%dm' $(( t / 1000000 )) $(( (t / 100000) % 10 ))
  elif (( t >= 1000 )); then
    printf '%dk' $(( (t + 500) / 1000 ))
  else
    printf '%d' "$t"
  fi
}

# Countdown to reset
fmt_countdown() {
  local now resets_at diff
  now=$(date +%s)
  resets_at="$1"
  if (( resets_at <= 0 )); then printf '%s' '--:--'; return; fi
  diff=$(( resets_at - now ))
  if (( diff <= 0 )); then printf '%s' '0:00'; return; fi
  local hrs=$(( diff / 3600 ))
  local mins=$(( (diff % 3600) / 60 ))
  printf '%d:%02d' "$hrs" "$mins"
}

in_fmt=$(fmt_tok "$in_tok")
out_fmt=$(fmt_tok "$out_tok")
branch=$(git -C "$cwd" rev-parse --abbrev-ref HEAD 2>/dev/null || echo '—')

ctx_bar=$(build_thermo_bar "$ctx_pct" 10)

dim='\033[2m'
cap="${dim}⡇\033[0m"

grn_bold='\033[1;32m'
red_bold='\033[1;31m'
rst='\033[0m'

# Subscription segment: rendered only when the payload actually carries
# rate_limits. Both bars and the reset countdown live or die together,
# since all three describe the same Claude.ai window.
limits_seg=""
if [ "$has_limits" = "1" ]; then
  sesh_bar=$(build_thermo_bar "$five_pct" 5)
  week_bar=$(build_thermo_bar "$week_pct" 5)
  countdown=$(fmt_countdown "$resets_at")
  limits_seg=$(printf 'sesh %s%b  week %s%b  %b⟳  %s %b│ ' \
    "$sesh_bar" "$cap" "$week_bar" "$cap" "$dim" "$countdown" "$rst$dim")
fi

cols=$(tput cols 2>/dev/null || echo 80)
printf -v sep '%*s' "$cols" ''
sep=${sep// /─}

# ↓/↑ are cumulative session totals from the transcript, not the current
# context window — they only ever climb.
printf 'context %s%b  %s%b%b↓%b %s │ %b↑%b %s │  %s%b\n\033[2m%s\033[0m' \
  "$ctx_bar" "$cap" "$limits_seg" \
  "$dim" "$grn_bold" "$rst$dim" "$in_fmt" "$red_bold" "$rst$dim" "$out_fmt" " $branch" "$rst" "$sep"
