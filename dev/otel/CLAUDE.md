# dev/otel/ — Local OTEL dogfood stack

Local Prometheus + Grafana prototype for validating the Thermal Cloud data model and dashboard queries before touching hosted infra. Claude Code pushes OTLP metrics directly to Prometheus (no collector needed — Prometheus 3.x native OTLP receiver). Grafana auto-provisions the datasource and five dashboards via file-based config.

- `start.sh` — launches both processes with prefixed logs, Ctrl-C kills both.
- `env.sh` — source before launching Claude Code; `cclaude` alias in `~/.zshrc` does this automatically.
- `dashboards/` — claude-spend, claude-insights, claude-models, claude-cfo, claude-vpeng.
- `data/` — gitignored Prometheus TSDB and Grafana state.

## Verified Claude Code metric names (authoritative, 8 metrics)

Sourced from `dev/otel/cc-otel-reference.md` §3 (captured 2026-04-26 from code.claude.com docs). Re-verify against the source URLs if more than 2 weeks have passed; an earlier 6-metric capture missed PR/commit emissions because the test session didn't trigger them.

- `claude_code_session_count_total`
- `claude_code_lines_of_code_count_total`
- `claude_code_pull_request_count_total`
- `claude_code_commit_count_total`
- `claude_code_cost_usage_USD_total`
- `claude_code_token_usage_tokens_total`
- `claude_code_code_edit_tool_decision_total`
- `claude_code_active_time_seconds_total`
