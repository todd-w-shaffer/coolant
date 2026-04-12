#!/bin/bash
# Start local Prometheus + Grafana for Claude Code OTEL dogfooding.
# Usage: ./dev/otel/start.sh
# Stop:  Ctrl-C (kills both)

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

# Preflight
command -v prometheus >/dev/null 2>&1 || { echo "brew install prometheus"; exit 1; }
command -v grafana    >/dev/null 2>&1 || { echo "brew install grafana"; exit 1; }

# Port-in-use guard — prevents a second start.sh from clobbering a stack
# already running in another terminal.
for port_check in "3000:Grafana" "9090:Prometheus" "9091:Lookups"; do
    port="${port_check%%:*}"
    service="${port_check##*:}"
    if lsof -i ":$port" >/dev/null 2>&1; then
        echo "⚠  Port $port is already in use — $service is likely running in another terminal."
        echo "   Ctrl-C the other terminal first if you want to restart here."
        exit 1
    fi
done

# Version drift check — last validated against this Grafana.
# Bump GRAFANA_VALIDATED_VERSION when we've sanity-checked dashboards
# on a newer release. Warns but does not block.
GRAFANA_VALIDATED_VERSION="12.4.2"
GRAFANA_CURRENT_VERSION="$(grafana --version 2>/dev/null | awk '{print $NF}')"
if [ -n "$GRAFANA_CURRENT_VERSION" ] && [ "$GRAFANA_CURRENT_VERSION" != "$GRAFANA_VALIDATED_VERSION" ]; then
    echo ""
    echo "⚠  Grafana $GRAFANA_CURRENT_VERSION installed — dashboards last validated on $GRAFANA_VALIDATED_VERSION."
    echo "   If theming looks off, eyeball dev/otel/dashboards/ then bump GRAFANA_VALIDATED_VERSION in this script."
    echo ""
fi

GRAFANA_HOME="$(brew --prefix grafana)/share/grafana"
GRAFANA_PLUGIN_DIR="$GRAFANA_HOME/data/plugins"

# Infinity plugin preflight — phase 3a dashboards bind dynamic-series
# colors via a "Config from query results" transform sourced from
# yesoreyeram-infinity-datasource. Install if missing; idempotent.
if [ ! -d "$GRAFANA_PLUGIN_DIR/yesoreyeram-infinity-datasource" ]; then
    echo "Installing yesoreyeram-infinity-datasource into $GRAFANA_PLUGIN_DIR ..."
    mkdir -p "$GRAFANA_PLUGIN_DIR"
    grafana cli \
        --homepath "$GRAFANA_HOME" \
        --pluginsDir "$GRAFANA_PLUGIN_DIR" \
        plugins install yesoreyeram-infinity-datasource
fi

cleanup() {
    echo ""
    echo "Shutting down..."
    kill "$PROM_PID" "$GRAF_PID" "$LOOKUPS_PID" 2>/dev/null
    wait "$PROM_PID" "$GRAF_PID" "$LOOKUPS_PID" 2>/dev/null
    echo "Done."
}
trap cleanup EXIT INT TERM

echo "Starting lookups HTTP server on :9091 (CSVs for Infinity)..."
python3 -m http.server 9091 --directory "$SCRIPT_DIR/lookups" \
    2>&1 | sed 's/^/[lookups]    /' &
LOOKUPS_PID=$!

echo "Starting Prometheus on :9090 (OTLP receiver enabled)..."
prometheus \
    --config.file="$SCRIPT_DIR/prometheus.yml" \
    --storage.tsdb.path="$SCRIPT_DIR/data/prometheus" \
    --web.listen-address=:9090 \
    --web.enable-otlp-receiver \
    2>&1 | sed 's/^/[prometheus] /' &
PROM_PID=$!

echo "Starting Grafana on :3000 (admin/coolant)..."
grafana server \
    --config="$SCRIPT_DIR/grafana.ini" \
    --homepath="$GRAFANA_HOME" \
    2>&1 | sed 's/^/[grafana]    /' &
GRAF_PID=$!

echo ""
echo "═══════════════════════════════════════════════════"
echo "  Prometheus:  http://localhost:9090"
echo "  Grafana:     http://localhost:3000  (admin/coolant)"
echo "  Dashboard:   http://localhost:3000/d/coolant-spend"
echo ""
echo "  Source env:   source dev/otel/env.sh"
echo "  Then launch Claude Code — metrics flow in ~10s"
echo "═══════════════════════════════════════════════════"
echo ""

wait
