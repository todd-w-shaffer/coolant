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
# Echoes "<input> <output> <cents>": cumulative tokens and cost for the
# whole session.
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
#
# Cost is public list price per MTok, priced per message from that
# message's own model — a session mixes models, since subagents and
# auxiliary calls run on cheaper ones. The cache multipliers are the
# reason this cannot be done from the displayed input total: cache reads
# bill at 0.1x and dominate a long session, so pricing the combined
# input figure at the fresh-input rate overstates by ~8x in practice.
# Cache writes bill 1.25x at 5-minute TTL and 2x at 1-hour.
sum_session_tokens() {
  local tp="$1" out
  [ -n "$tp" ] && [ -r "$tp" ] || { printf '0 0 0'; return; }
  out=$(jq -n -r '
    def rate:
      if test("haiku") then {i: 1, o: 5}
      elif test("sonnet") then {i: 3, o: 15}
      elif test("fable") or test("mythos") then {i: 10, o: 50}
      else {i: 5, o: 25} end;
    reduce (inputs | select(.message.usage)) as $m
      ({i: 0, o: 0, c: 0};
        ($m.message.usage) as $u
        | (($m.message.model // "opus") | rate) as $r
        | ($u.input_tokens // 0) as $fresh
        | ($u.cache_read_input_tokens // 0) as $read
        | ($u.output_tokens // 0) as $out
        # Older entries carry a flat cache_creation_input_tokens with no
        # TTL breakdown; treat those as the 5-minute rate.
        | ($u.cache_creation.ephemeral_5m_input_tokens
            // (if $u.cache_creation then 0
                else ($u.cache_creation_input_tokens // 0) end)) as $w5m
        | ($u.cache_creation.ephemeral_1h_input_tokens // 0) as $w1h
        | {i: (.i + $fresh + $w5m + $w1h + $read),
           o: (.o + $out),
           c: (.c + $fresh * $r.i
                  + $w5m * $r.i * 1.25
                  + $w1h * $r.i * 2
                  + $read * $r.i * 0.1
                  + $out * $r.o)})
    # $/MTok against raw token counts, expressed as whole cents.
    | "\(.i) \(.o) \((.c / 10000) | round)"' "$tp" 2>/dev/null)
  # A truncated or half-written transcript must not blank the display.
  case "$out" in
    [0-9]*\ [0-9]*\ [0-9]*) printf '%s' "$out" ;;
    *) printf '0 0 0' ;;
  esac
}

# fmt_money <cents>
# Cents below $10, whole dollars above — the status line is width-bound
# and the trailing cents on a three-figure sum are dead digits.
fmt_money() {
  local c="$1"
  if (( c < 1000 )); then
    printf '%d.%02d' $(( c / 100 )) $(( c % 100 ))
  else
    printf '%d' $(( (c + 50) / 100 ))
  fi
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
  read -r model
} <<EOF
$(echo "$input" | jq -r '
    (.context_window.used_percentage // 0),
    (.rate_limits.five_hour.used_percentage // 0),
    (.rate_limits.seven_day.used_percentage // 0),
    (.rate_limits.five_hour.resets_at // 0),
    (if .rate_limits then 1 else 0 end),
    (.transcript_path // ""),
    (.cwd // "."),
    (.model.display_name // .model.id // "")' 2>/dev/null)
EOF
: "${cwd:=.}"

read -r in_tok out_tok cents <<<"$(sum_session_tokens "$transcript")"

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

# Claude Code captures this script's stdout, so there is no controlling
# terminal to query; $COLUMNS is the documented source for the width.
# `tput cols` also honours $COLUMNS, so it is kept only as the fallback
# for a direct invocation outside Claude Code.
cols=${COLUMNS:-$(tput cols 2>/dev/null || echo 80)}

in_fmt=$(fmt_tok "$in_tok")
out_fmt=$(fmt_tok "$out_tok")
branch=$(git -C "$cwd" rev-parse --abbrev-ref HEAD 2>/dev/null || echo '—')

ctx_bar=$(build_thermo_bar "$ctx_pct" 10)

dim='\033[2m'
cap="${dim}⡇\033[0m"


grn_bold='\033[1;32m'
red_bold='\033[1;31m'
rst='\033[0m'

# Model leads the line as its subject. Deliberately placed away from the
# cost figure: the model is a right-now value while cost and tokens are
# session-cumulative, and cost is priced per message from each message's
# own model — a session mixes them, since subagents and auxiliary calls
# run on cheaper ones. Adjacency would imply the whole session ran on
# whatever model is current, which is false.
#
# Effort is not shown here; Claude Code already surfaces it in the
# terminal chrome outside the status line.
model_seg=""
model_cols=0
if [ -n "$model" ]; then
  model_seg=$(printf '%b%s %b│ ' "$dim" "$model" "$rst$dim")
  model_cols=$(( ${#model} + 3 ))
fi

# Money is framed by what it actually means on each provider. Off
# subscription it approximates a real marginal bill at public list rates
# (an org's negotiated Bedrock rate will differ), so it reads as an
# estimate. On Pro/Max nothing is being billed — the fee is flat — so the
# figure is value drawn against that fee, not a budget to stay under.
#
# Width-gated rather than wrapped: the subscription layout already runs
# ~83 columns before money, so on a narrow terminal the figure is
# dropped instead of pushing the line onto a second row.
money_seg=""
if [ "$has_limits" = "1" ]; then
  money_min=95
  money_glyph='+$'
else
  money_min=58
  money_glyph='≈$'
fi
# The model segment eats into the same budget, so the threshold has to
# move with it rather than being a bare constant.
money_min=$(( money_min + model_cols ))
if (( cols >= money_min )); then
  money_seg=$(printf '%b%s%s %b│ ' "$dim" "$money_glyph" "$(fmt_money "$cents")" "$rst$dim")
fi

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

printf -v sep '%*s' "$cols" ''
sep=${sep// /─}

# ↓/↑ are cumulative session totals from the transcript, not the current
# context window — they only ever climb.
printf '%scontext %s%b  %s%s%b%b↓%b %s │ %b↑%b %s │  %s%b\n\033[2m%s\033[0m' \
  "$model_seg" "$ctx_bar" "$cap" "$limits_seg" "$money_seg" \
  "$dim" "$grn_bold" "$rst$dim" "$in_fmt" "$red_bold" "$rst$dim" "$out_fmt" " $branch" "$rst" "$sep"
