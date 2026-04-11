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

echo "✓ Claude Code OTEL → localhost:9090 (Prometheus OTLP receiver)"
echo "  Metrics push every 10s, cumulative temporality"
echo "  Grafana: http://localhost:3000 (admin/coolant)"
