# dev/otel/ — Local OTEL dogfood stack

Local Prometheus + Grafana prototype for validating the Thermal Cloud data model and dashboard queries before touching hosted infra. Claude Code pushes OTLP metrics directly to Prometheus (no collector needed — Prometheus 3.x native OTLP receiver). Grafana auto-provisions the datasource and five dashboards via file-based config.

- `start.sh` — launches both processes with prefixed logs, Ctrl-C kills both.
- `env.sh` — source before launching Claude Code; `cclaude` alias in `~/.zshrc` does this automatically.
- `dashboards/` — claude-spend, claude-insights, claude-models, claude-cfo, claude-vpeng.
- `data/` — gitignored Prometheus TSDB and Grafana state.

## Verified Claude Code metric names (live-checked, not guessed)

- `claude_code_cost_usage_USD_total`
- `claude_code_token_usage_tokens_total`
- `claude_code_lines_of_code_count_total`
- `claude_code_active_time_seconds_total`
- `claude_code_session_count_total`
- `claude_code_code_edit_tool_decision_total`
