# Claude Code OTEL → local Prometheus
# Source this in your shell before launching Claude Code:
#   source dev/otel/env.sh

export CLAUDE_CODE_ENABLE_TELEMETRY=1
export OTEL_METRICS_EXPORTER=otlp
export OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
export OTEL_EXPORTER_OTLP_METRICS_ENDPOINT=http://localhost:9090/api/v1/otlp/v1/metrics
export OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE=cumulative
export OTEL_METRIC_EXPORT_INTERVAL=10000
export OTEL_METRICS_INCLUDE_SESSION_ID=true

# Auto-detect repo name from current git worktree so metrics can be sliced
# by repo downstream. Falls back to the basename of the cwd for non-git dirs.
_coolant_repo_root=$(git rev-parse --show-toplevel 2>/dev/null)
if [ -n "$_coolant_repo_root" ]; then
  _coolant_repo_name=$(basename "$_coolant_repo_root")
else
  _coolant_repo_name=$(basename "$PWD")
fi
export OTEL_RESOURCE_ATTRIBUTES="repo=${_coolant_repo_name}"
unset _coolant_repo_root _coolant_repo_name

echo "✓ Claude Code OTEL → localhost:9090 (Prometheus OTLP receiver)"
echo "  Metrics push every 10s, cumulative temporality"
echo "  Repo tag: ${OTEL_RESOURCE_ATTRIBUTES#repo=}"
echo "  Grafana: http://localhost:3000 (admin/coolant)"
