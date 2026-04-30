#!/usr/bin/env bash
# Patch ~/.claude/settings.json with the env-var block that points
# Claude Code's OTEL emitter at thermo's embedded receiver.
#
# Spec: docs/_drafts/cc-otel-beta-adapter.spec.md §3.3.
# Defaults to port 4318 unless COOLANT_CC_OTEL_PORT is set or the
# `--port=N` flag is passed. `--off` removes only the keys this script
# adds (other env keys are preserved).
#
# Bash 3.2 compatible (no mapfile, no associative arrays, no |&).

set -e

SETTINGS="${HOME}/.claude/settings.json"
PORT="${COOLANT_CC_OTEL_PORT:-4318}"
MODE="on"

while [ $# -gt 0 ]; do
  case "$1" in
    --off) MODE="off"; shift ;;
    --port) PORT="$2"; shift 2 ;;
    --port=*) PORT="${1#--port=}"; shift ;;
    -h|--help)
      cat <<'EOF'
Usage: enable-cc-otel.sh [--off] [--port=N]

Patches ~/.claude/settings.json so Claude Code pushes OTEL metrics to
thermo's embedded receiver. With --off, removes the env keys this
script previously added.

Env: COOLANT_CC_OTEL_PORT overrides the default 4318 receiver port.
EOF
      exit 0
      ;;
    *)
      echo "enable-cc-otel: unknown arg: $1" >&2
      exit 2
      ;;
  esac
done

# Sanity-check that thermo is running on the configured port — warn
# but do NOT refuse, so the user can configure CC OTEL ahead of
# starting thermo.
if [ "$MODE" = "on" ]; then
  if ! lsof -nPiTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
    echo "enable-cc-otel: warning — thermo doesn't appear to be listening on :${PORT}." >&2
    echo "                you can run thermo afterwards; CC OTEL will retry on next emission." >&2
  fi
fi

ENDPOINT="http://localhost:${PORT}/v1/metrics"

# Keys this script manages — listed once so --off can remove the same
# set it added.
KEYS="CLAUDE_CODE_ENABLE_TELEMETRY OTEL_METRICS_EXPORTER OTEL_EXPORTER_OTLP_PROTOCOL OTEL_EXPORTER_OTLP_METRICS_ENDPOINT OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE OTEL_METRIC_EXPORT_INTERVAL OTEL_METRICS_INCLUDE_SESSION_ID"

mkdir -p "${HOME}/.claude"
if [ ! -f "$SETTINGS" ]; then
  echo "{}" > "$SETTINGS"
fi

if command -v jq >/dev/null 2>&1; then
  # mktemp must land on the same filesystem as $SETTINGS so the
  # subsequent `mv` is rename(2)-atomic (mv falls back to copy+unlink
  # across filesystems, defeating the atomicity).
  tmp=$(mktemp "${SETTINGS}.XXXXXX")
  cp -p "$SETTINGS" "${SETTINGS}.bak"
  if [ "$MODE" = "on" ]; then
    jq --arg endpoint "$ENDPOINT" '
      .env = (.env // {}) +
        {
          "CLAUDE_CODE_ENABLE_TELEMETRY": "1",
          "OTEL_METRICS_EXPORTER": "otlp",
          "OTEL_EXPORTER_OTLP_PROTOCOL": "http/protobuf",
          "OTEL_EXPORTER_OTLP_METRICS_ENDPOINT": $endpoint,
          "OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE": "cumulative",
          "OTEL_METRIC_EXPORT_INTERVAL": "10000",
          "OTEL_METRICS_INCLUDE_SESSION_ID": "true"
        }
    ' "$SETTINGS" > "$tmp" && mv "$tmp" "$SETTINGS"
    echo "enable-cc-otel: patched ${SETTINGS} for receiver at ${ENDPOINT}"
  else
    jq '
      if (.env // {}) == {} then . else
        .env = (
          .env
          | del(.CLAUDE_CODE_ENABLE_TELEMETRY)
          | del(.OTEL_METRICS_EXPORTER)
          | del(.OTEL_EXPORTER_OTLP_PROTOCOL)
          | del(.OTEL_EXPORTER_OTLP_METRICS_ENDPOINT)
          | del(.OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE)
          | del(.OTEL_METRIC_EXPORT_INTERVAL)
          | del(.OTEL_METRICS_INCLUDE_SESSION_ID)
        )
      end
    ' "$SETTINGS" > "$tmp" && mv "$tmp" "$SETTINGS"
    echo "enable-cc-otel: removed CC OTEL env keys from ${SETTINGS}"
  fi
  exit 0
fi

# jq fallback — print the env block for the user to copy-paste.
if [ "$MODE" = "on" ]; then
  cat <<EOF
enable-cc-otel: jq not installed.

Add this to ${SETTINGS} under the top-level "env" key:

    "env": {
      "CLAUDE_CODE_ENABLE_TELEMETRY": "1",
      "OTEL_METRICS_EXPORTER": "otlp",
      "OTEL_EXPORTER_OTLP_PROTOCOL": "http/protobuf",
      "OTEL_EXPORTER_OTLP_METRICS_ENDPOINT": "${ENDPOINT}",
      "OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE": "cumulative",
      "OTEL_METRIC_EXPORT_INTERVAL": "10000",
      "OTEL_METRICS_INCLUDE_SESSION_ID": "true"
    }

Or install jq (brew install jq) and re-run this script.
EOF
else
  cat <<EOF
enable-cc-otel: jq not installed.

Open ${SETTINGS} and remove these keys from the "env" object:

  ${KEYS}

Or install jq (brew install jq) and re-run \`enable-cc-otel.sh --off\`.
EOF
fi
