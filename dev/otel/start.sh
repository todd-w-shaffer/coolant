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

cleanup() {
    echo ""
    echo "Shutting down..."
    kill "$PROM_PID" "$GRAF_PID" 2>/dev/null
    wait "$PROM_PID" "$GRAF_PID" 2>/dev/null
    echo "Done."
}
trap cleanup EXIT INT TERM

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
