#!/usr/bin/env bash
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

input=$(cat)

ctx_pct=$(echo "$input" | jq -r '.context_window.used_percentage // 0')
five_pct=$(echo "$input" | jq -r '.rate_limits.five_hour.used_percentage // 0')
week_pct=$(echo "$input" | jq -r '.rate_limits.seven_day.used_percentage // 0')

in_tok=$(echo "$input" | jq -r '.context_window.total_input_tokens // 0')
out_tok=$(echo "$input" | jq -r '.context_window.total_output_tokens // 0')
resets_at=$(echo "$input" | jq -r '.rate_limits.five_hour.resets_at // 0')

# Format tokens as k/m
fmt_tok() {
  local t="$1"
  if (( t >= 1000000 )); then
    awk "BEGIN { printf \"%.1fm\", $t / 1000000 }"
  elif (( t >= 1000 )); then
    awk "BEGIN { printf \"%.0fk\", $t / 1000 }"
  else
    printf '%d' "$t"
  fi
}

# Countdown to reset
fmt_countdown() {
  local now resets_at diff
  now=$(date +%s)
  resets_at="$1"
  if (( resets_at <= 0 )); then printf '--:--'; return; fi
  diff=$(( resets_at - now ))
  if (( diff <= 0 )); then printf '0:00'; return; fi
  local hrs=$(( diff / 3600 ))
  local mins=$(( (diff % 3600) / 60 ))
  printf '%d:%02d' "$hrs" "$mins"
}

in_fmt=$(fmt_tok "$in_tok")
out_fmt=$(fmt_tok "$out_tok")
countdown=$(fmt_countdown "$resets_at")
branch=$(git -C "$(echo "$input" | jq -r '.cwd // "."')" rev-parse --abbrev-ref HEAD 2>/dev/null || echo '—')

ctx_bar=$(build_thermo_bar "$ctx_pct" 10)
sesh_bar=$(build_thermo_bar "$five_pct" 5)
week_bar=$(build_thermo_bar "$week_pct" 5)

dim='\033[2m'
cap="${dim}⡇\033[0m"

grn_bold='\033[1;32m'
red_bold='\033[1;31m'
rst='\033[0m'

cols=$(tput cols 2>/dev/null || echo 80)
sep=$(printf '─%.0s' $(seq 1 "$cols"))

printf 'context %s%b  sesh %s%b  week %s%b  %b%b↓%b %s │ %b↑%b %s │ ⟳  %s │  %s%b\n\033[2m%s\033[0m' \
  "$ctx_bar" "$cap" "$sesh_bar" "$cap" "$week_bar" "$cap" \
  "$dim" "$grn_bold" "$rst$dim" "$in_fmt" "$red_bold" "$rst$dim" "$out_fmt" "$countdown" " $branch" "$rst" "$sep"
