# Phase 1b — Telemetry: coolant-emit, Hooks, Commit Trailer

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **This plan is parallel-native.** Task B0 is serial (bootstrap). Tasks B1–B8 can run concurrently via subagents. Task B9 is serial (merge into `hooks/hooks.json`).

**Goal:** Ship the OSS telemetry runtime: a stateless `coolant-emit` CLI, six bash hooks wired into Claude Code's hook points, and a `Coolant-Session-V1:` git trailer emitted by the `/commit` skill.

**Architecture:** Hooks invoke `coolant-emit` which writes to the shared JSONL event log (always) and pushes OTLP/HTTP JSON to the configured Prometheus endpoint (best-effort, 500ms timeout). The `/commit` skill queries Prometheus at commit time to aggregate session cost and token totals into the commit trailer.

**Tech Stack:** Go 1.25 (no external OTel SDK — manual OTLP/HTTP JSON for cold-start speed), bash 3.2, bats-core, Prometheus 3.x OTLP endpoint.

---

## Parallel execution guide for subagents

When dispatching tasks B1–B8 via `superpowers:subagent-driven-development`, each subagent's isolation boundary is:

| Task | Touches (writes) | Reads (no writes) |
|---|---|---|
| B1 | `scripts/prompt-submit.sh`, `tests/prompt-submit.bats` | `scripts/common.sh`, `bin/coolant-emit` |
| B2 | `scripts/preflight.sh` (extend), `tests/preflight.bats` (extend) | `scripts/common.sh`, `bin/coolant-emit` |
| B3 | `scripts/gate.sh` (extend), `tests/gate.bats` (extend) | `scripts/common.sh`, `bin/coolant-emit` |
| B4 | `scripts/tool-post.sh`, `tests/tool-post.bats` | `scripts/common.sh`, `bin/coolant-emit` |
| B5 | `scripts/compact.sh`, `tests/compact.bats` | `scripts/common.sh`, `bin/coolant-emit` |
| B6 | `scripts/session-end.sh`, `tests/session-end.bats` | `scripts/common.sh`, `bin/coolant-emit` |
| B7 | `scripts/agent-start.sh`, `scripts/agent-stop.sh` (extend both), `tests/agent-start.bats`, `tests/agent-stop.bats` | `scripts/common.sh`, `bin/coolant-emit` |
| B8 | **External to this repo** — `~/.claude/plugins/local/personal-plugins/plugins/commit-skill/skills/commit/SKILL.md` and a new `bin/coolant-trailer.sh` in that plugin | Prometheus OTLP endpoint at query time, JSONL event log for session_id resolution |

**No two tasks write the same file.** B9 is the only task that edits `hooks/hooks.json`.

---

## Task B0 — `coolant-emit` CLI and shared test-helper prep (SERIAL bootstrap)

**Files:**
- Create: `cmd/coolant-emit/main.go`
- Create: `cmd/coolant-emit/main_test.go`
- Modify: `tests/test_helper.bash` (extract reusable `_common_setup` and `_common_teardown`)
- Modify: `install.sh` (add build step for `coolant-emit`)
- Modify: `CLAUDE.md` (document the binary in the Quick reference section)

### Step 0: Extract reusable setup/teardown helpers from `tests/test_helper.bash`

The current `tests/test_helper.bash` defines `setup()` and `teardown()` at the top level. Bats calls those automatically for every test that `load 'test_helper'`. But tests in B1–B8 need to *add* behavior to setup (installing a `coolant-emit` stub) without losing the existing env-var exports. Extract the body into reusable helpers.

Open `tests/test_helper.bash`. Current content:
```bash
#!/usr/bin/env bash
# Shared test setup — isolates all coolant state to a temp directory
# so tests never touch the real /tmp/coolant-* files.

TESTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$TESTS_DIR/.." && pwd)"

setup() {
  TEST_TMPDIR="$(mktemp -d)"
  export COOLANT_LOCKFILE="${TEST_TMPDIR}/coolant.lock"
  export COOLANT_COUNTER="${TEST_TMPDIR}/coolant.count"
  export COOLANT_LOG="${TEST_TMPDIR}/coolant.log"
  export COOLANT_EVENTS="${TEST_TMPDIR}/coolant.events.jsonl"
  export COOLANT_THRESHOLD=3
  export _COOLANT_NCPU=10
}

make_pre_tool_use() { ... }  # unchanged

teardown() {
  rm -rf "$TEST_TMPDIR"
}
```

Replace with:
```bash
#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Shared test setup — isolates all coolant state to a temp directory
# so tests never touch the real /tmp/coolant-* files.

TESTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$TESTS_DIR/.." && pwd)"

# Reusable setup body — tests that need extra setup should define:
#   setup() { _common_setup; ...extra... }
_common_setup() {
  TEST_TMPDIR="$(mktemp -d)"
  export COOLANT_LOCKFILE="${TEST_TMPDIR}/coolant.lock"
  export COOLANT_COUNTER="${TEST_TMPDIR}/coolant.count"
  export COOLANT_LOG="${TEST_TMPDIR}/coolant.log"
  export COOLANT_EVENTS="${TEST_TMPDIR}/coolant.events.jsonl"
  export COOLANT_THRESHOLD=3
  export _COOLANT_NCPU=10
}

_common_teardown() {
  rm -rf "$TEST_TMPDIR"
}

# Default setup/teardown — existing tests that don't override inherit these.
setup() { _common_setup; }
teardown() { _common_teardown; }

# Build a PreToolUse stdin JSON payload for testing gate.sh.
make_pre_tool_use() {
  printf '{"session_id":"test-s","tool_name":"%s","tool_input":{"command":"%s","description":"test"},"hook_event_name":"PreToolUse"}' "$1" "$2"
}
```

Verify existing tests still pass:
```bash
bats tests/
```
Expected: same pass count as before. No regressions.

### Step 1: Write the failing test

Create `cmd/coolant-emit/main_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildBinary compiles coolant-emit into a temp path and returns the path.
func buildBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "coolant-emit")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to build coolant-emit: %v", err)
	}
	return bin
}

// captureReq starts a mock OTLP endpoint and returns the first request body received.
func captureReq(t *testing.T) (*httptest.Server, <-chan []byte) {
	t.Helper()
	ch := make(chan []byte, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		select {
		case ch <- body:
		default:
		}
		w.WriteHeader(200)
	}))
	return srv, ch
}

func TestCounterEmission(t *testing.T) {
	bin := buildBinary(t)
	srv, ch := captureReq(t)
	defer srv.Close()

	eventsFile := filepath.Join(t.TempDir(), "events.jsonl")
	cmd := exec.Command(bin, "counter", "test_metric_total", "session_id=abc123", "tool_name=Bash")
	cmd.Env = append(os.Environ(),
		"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT="+srv.URL,
		"COOLANT_EVENTS="+eventsFile,
		"OTEL_RESOURCE_ATTRIBUTES=repo=testrepo",
	)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("coolant-emit failed: %v", err)
	}

	// Check JSONL was written
	jsonlBytes, err := os.ReadFile(eventsFile)
	if err != nil {
		t.Fatalf("reading events file: %v", err)
	}
	if !strings.Contains(string(jsonlBytes), `"metric":"test_metric_total"`) {
		t.Errorf("JSONL missing metric name; got: %s", jsonlBytes)
	}
	if !strings.Contains(string(jsonlBytes), `"session_id":"abc123"`) {
		t.Errorf("JSONL missing session_id label; got: %s", jsonlBytes)
	}

	// Check OTLP payload
	var body []byte
	select {
	case body = <-ch:
	default:
		t.Fatal("OTLP endpoint received no request")
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("OTLP body not valid JSON: %v; body=%s", err, body)
	}
	rm, ok := payload["resourceMetrics"].([]any)
	if !ok || len(rm) == 0 {
		t.Fatalf("OTLP payload missing resourceMetrics: %v", payload)
	}
	// Resource labels should include repo=testrepo
	resource := rm[0].(map[string]any)["resource"].(map[string]any)
	attrs := resource["attributes"].([]any)
	foundRepo := false
	for _, a := range attrs {
		kv := a.(map[string]any)
		if kv["key"] == "repo" {
			v := kv["value"].(map[string]any)
			if v["stringValue"] == "testrepo" {
				foundRepo = true
			}
		}
	}
	if !foundRepo {
		t.Errorf("resource attributes missing repo=testrepo: %v", attrs)
	}
}

func TestHistogramEmission(t *testing.T) {
	bin := buildBinary(t)
	srv, ch := captureReq(t)
	defer srv.Close()

	eventsFile := filepath.Join(t.TempDir(), "events.jsonl")
	cmd := exec.Command(bin, "histogram", "test_latency_ms", "42.5", "repo=testrepo")
	cmd.Env = append(os.Environ(),
		"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT="+srv.URL,
		"COOLANT_EVENTS="+eventsFile,
	)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("coolant-emit failed: %v", err)
	}
	var body []byte
	select {
	case body = <-ch:
	default:
		t.Fatal("no OTLP request received")
	}
	if !strings.Contains(string(body), `"histogram"`) {
		t.Errorf("expected histogram in payload; got: %s", body)
	}
}

func TestJSONLAlwaysWrittenEvenIfOTLPDown(t *testing.T) {
	bin := buildBinary(t)
	eventsFile := filepath.Join(t.TempDir(), "events.jsonl")

	// Point at a closed server — OTLP will fail.
	cmd := exec.Command(bin, "counter", "broken_test_total")
	cmd.Env = append(os.Environ(),
		"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT=http://127.0.0.1:1/unreachable",
		"COOLANT_EVENTS="+eventsFile,
	)
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		t.Fatalf("coolant-emit exited non-zero despite OTLP failure: %v", err)
	}
	if _, err := os.Stat(eventsFile); err != nil {
		t.Errorf("JSONL was not written when OTLP failed: %v", err)
	}
}

func TestBadArgsExitNonZero(t *testing.T) {
	bin := buildBinary(t)
	cmd := exec.Command(bin)
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit with no args")
	}
}
```

### Step 2: Run test to verify it fails

Run:
```bash
go test ./cmd/coolant-emit/... -v
```
Expected: test fails because `main.go` does not exist yet (`build failed`).

### Step 3: Write minimal implementation

Create `cmd/coolant-emit/main.go`:

```go
// SPDX-License-Identifier: Apache-2.0

// Command coolant-emit emits a single OTLP metric and appends one
// event to the shared JSONL event log. Stateless, fire-and-forget.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type kv struct {
	Key   string    `json:"key"`
	Value attrValue `json:"value"`
}

type attrValue struct {
	StringValue string `json:"stringValue"`
}

type dataPoint struct {
	Attributes     []kv      `json:"attributes"`
	TimeUnixNano   string    `json:"timeUnixNano"`
	AsInt          *string   `json:"asInt,omitempty"`
	AsDouble       *float64  `json:"asDouble,omitempty"`
	Count          *string   `json:"count,omitempty"`
	Sum            *float64  `json:"sum,omitempty"`
	BucketCounts   []string  `json:"bucketCounts,omitempty"`
	ExplicitBounds []float64 `json:"explicitBounds,omitempty"`
}

type metric struct {
	Name      string     `json:"name"`
	Sum       *sumData   `json:"sum,omitempty"`
	Histogram *histData  `json:"histogram,omitempty"`
	Gauge     *gaugeData `json:"gauge,omitempty"`
}

type sumData struct {
	DataPoints             []dataPoint `json:"dataPoints"`
	AggregationTemporality int         `json:"aggregationTemporality"`
	IsMonotonic            bool        `json:"isMonotonic"`
}

type histData struct {
	DataPoints             []dataPoint `json:"dataPoints"`
	AggregationTemporality int         `json:"aggregationTemporality"`
}

type gaugeData struct {
	DataPoints []dataPoint `json:"dataPoints"`
}

type scopeMetric struct {
	Scope   scope    `json:"scope"`
	Metrics []metric `json:"metrics"`
}

type scope struct {
	Name string `json:"name"`
}

type resourceBlock struct {
	Attributes []kv `json:"attributes"`
}

type resourceMetric struct {
	Resource     resourceBlock `json:"resource"`
	ScopeMetrics []scopeMetric `json:"scopeMetrics"`
}

type otlpPayload struct {
	ResourceMetrics []resourceMetric `json:"resourceMetrics"`
}

var verbose = flag.Bool("verbose", false, "log OTLP errors to stderr")

func main() {
	flag.Parse()
	args := flag.Args()
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: coolant-emit <counter|histogram|gauge> <metric_name> [value] [key=value ...]")
		os.Exit(1)
	}
	kind, name := args[0], args[1]
	rest := args[2:]

	var value float64
	var labels []kv
	switch kind {
	case "counter":
		value = 1
		labels = parseLabels(rest)
	case "histogram", "gauge":
		if len(rest) == 0 {
			fmt.Fprintf(os.Stderr, "coolant-emit %s requires a value\n", kind)
			os.Exit(1)
		}
		v, err := strconv.ParseFloat(rest[0], 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "coolant-emit: invalid value: %s\n", rest[0])
			os.Exit(1)
		}
		value = v
		labels = parseLabels(rest[1:])
	default:
		fmt.Fprintf(os.Stderr, "coolant-emit: unknown metric type: %s\n", kind)
		os.Exit(1)
	}

	labels = injectDefaults(labels)

	if err := appendJSONL(kind, name, value, labels); err != nil {
		fmt.Fprintf(os.Stderr, "coolant-emit: JSONL write failed: %v\n", err)
		os.Exit(2)
	}
	if err := pushOTLP(kind, name, value, labels); err != nil && *verbose {
		fmt.Fprintf(os.Stderr, "coolant-emit: OTLP push failed: %v\n", err)
	}
}

func parseLabels(args []string) []kv {
	out := make([]kv, 0, len(args))
	for _, a := range args {
		k, v, ok := strings.Cut(a, "=")
		if !ok {
			continue
		}
		out = append(out, kv{Key: k, Value: attrValue{StringValue: v}})
	}
	return out
}

func injectDefaults(existing []kv) []kv {
	has := make(map[string]bool, len(existing))
	for _, l := range existing {
		has[l.Key] = true
	}
	if !has["repo"] {
		if r := detectRepo(); r != "" {
			existing = append(existing, kv{Key: "repo", Value: attrValue{StringValue: r}})
		}
	}
	if !has["user_email"] {
		if u := gitUserEmail(); u != "" {
			existing = append(existing, kv{Key: "user_email", Value: attrValue{StringValue: u}})
		}
	}
	return existing
}

func detectRepo() string {
	if attr := os.Getenv("OTEL_RESOURCE_ATTRIBUTES"); attr != "" {
		for _, part := range strings.Split(attr, ",") {
			if strings.HasPrefix(part, "repo=") {
				return strings.TrimPrefix(part, "repo=")
			}
		}
	}
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return ""
	}
	return filepath.Base(strings.TrimSpace(string(out)))
}

func gitUserEmail() string {
	out, err := exec.Command("git", "config", "user.email").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func eventsPath() string {
	if p := os.Getenv("COOLANT_EVENTS"); p != "" {
		return p
	}
	tmp := os.Getenv("TMPDIR")
	if tmp == "" {
		tmp = "/tmp/"
	}
	user := os.Getenv("USER")
	if user == "" {
		user = "unknown"
	}
	return filepath.Join(tmp, "coolant-"+user+".events.jsonl")
}

func appendJSONL(kind, name string, value float64, labels []kv) error {
	path := eventsPath()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	labelMap := make(map[string]string, len(labels))
	for _, l := range labels {
		labelMap[l.Key] = l.Value.StringValue
	}
	record := map[string]any{
		"ts":     time.Now().UTC().Format(time.RFC3339Nano),
		"source": "coolant-emit",
		"kind":   kind,
		"metric": name,
		"value":  value,
		"labels": labelMap,
	}
	b, err := json.Marshal(record)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = f.Write(b)
	return err
}

func pushOTLP(kind, name string, value float64, labels []kv) error {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT")
	if endpoint == "" {
		return fmt.Errorf("no OTLP endpoint configured")
	}
	payload := buildPayload(kind, name, value, labels)
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 500 * time.Millisecond}
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("OTLP returned %d", resp.StatusCode)
	}
	return nil
}

func buildPayload(kind, name string, value float64, labels []kv) otlpPayload {
	resourceKeys := map[string]bool{"repo": true, "user_email": true}
	var resLabels, dpLabels []kv
	for _, l := range labels {
		if resourceKeys[l.Key] {
			resLabels = append(resLabels, l)
		} else {
			dpLabels = append(dpLabels, l)
		}
	}
	now := strconv.FormatInt(time.Now().UnixNano(), 10)
	dp := dataPoint{Attributes: dpLabels, TimeUnixNano: now}

	m := metric{Name: name}
	switch kind {
	case "counter":
		asInt := strconv.FormatInt(int64(value), 10)
		dp.AsInt = &asInt
		m.Sum = &sumData{DataPoints: []dataPoint{dp}, AggregationTemporality: 2, IsMonotonic: true}
	case "gauge":
		v := value
		dp.AsDouble = &v
		m.Gauge = &gaugeData{DataPoints: []dataPoint{dp}}
	case "histogram":
		count := "1"
		sum := value
		bounds := []float64{1, 10, 100, 1000, 10000, 100000, 1000000}
		buckets := make([]string, len(bounds)+1)
		placed := false
		for i, b := range bounds {
			if !placed && value <= b {
				buckets[i] = "1"
				placed = true
			} else {
				buckets[i] = "0"
			}
		}
		if !placed {
			buckets[len(bounds)] = "1"
		} else {
			buckets[len(bounds)] = "0"
		}
		dp.Count = &count
		dp.Sum = &sum
		dp.BucketCounts = buckets
		dp.ExplicitBounds = bounds
		m.Histogram = &histData{DataPoints: []dataPoint{dp}, AggregationTemporality: 2}
	}

	return otlpPayload{
		ResourceMetrics: []resourceMetric{{
			Resource:     resourceBlock{Attributes: resLabels},
			ScopeMetrics: []scopeMetric{{Scope: scope{Name: "coolant-emit"}, Metrics: []metric{m}}},
		}},
	}
}
```

### Step 4: Run test to verify it passes

Run:
```bash
go test ./cmd/coolant-emit/... -v
```
Expected: all four test functions PASS.

### Step 5: Integration smoke test against a live (or stubbed) Prometheus

Build and run against the local dev stack:
```bash
go build -o bin/coolant-emit ./cmd/coolant-emit/
source dev/otel/env.sh
bin/coolant-emit counter coolant_smoketest_total marker=smoketest
# Verify the metric is visible
curl -s 'http://localhost:9090/api/v1/query?query=coolant_smoketest_total' | python3 -m json.tool | head -20
```
Expected: query returns a vector containing `coolant_smoketest_total` with `marker=smoketest`. If the local stack isn't running, start it first: `cd dev/otel && ./start.sh`.

### Step 6: Update `install.sh` to build `coolant-emit`

Open `install.sh`. Find the existing `thermo` build line (after Plan 1a's rewrites it should read `go build -o bin/thermo ./cmd/thermo/`). Immediately after it, add:

```bash
go build -o bin/coolant-emit ./cmd/coolant-emit/
```

If there are multi-arch blocks (`GOARCH=arm64 …` and `GOARCH=amd64 …`), mirror the same pattern for `coolant-emit`. Each architecture gets both binaries.

### Step 7: Document the binary in `CLAUDE.md`

Open `CLAUDE.md`. In the Quick reference section, add after the existing build command:
```bash
go build -o bin/coolant-emit ./cmd/coolant-emit/       # build coolant-emit (OTLP/JSONL CLI for hooks)
```

Also add a short note in the project-structure tree under `cmd/coolant-emit/`: `stateless OTLP emitter invoked by bash hooks`.

### Step 8: Commit via `/commit` skill

Invoke `/commit`.

---

## Parallel fan-out block: Tasks B1–B8

Before starting any parallel task, confirm:
- [ ] Task B0 is committed and green
- [ ] `bin/coolant-emit` exists and is on `$PATH` (or invoked by full path in hooks — the plan uses `${CLAUDE_PLUGIN_ROOT}/bin/coolant-emit`)

Each parallel task follows a consistent shape: write bats test → run red → write hook → run green → commit. **Do not edit `hooks/hooks.json` inside these tasks.** That file is edited only in B9.

---

## Task B1 — `prompt-submit.sh` hook (UserPromptSubmit)

**Files:**
- Create: `scripts/prompt-submit.sh`
- Create: `tests/prompt-submit.bats`

Emits `coolant_prompt_total` counter and `coolant_prompt_length_chars` histogram on every user prompt.

### Step 1: Write the failing bats test

Create `tests/prompt-submit.bats`:

```bash
#!/usr/bin/env bats

load 'test_helper'

setup() {
  _common_setup
  # Stub coolant-emit by intercepting its invocation — write to a log.
  export PATH="$BATS_TEST_TMPDIR/stub:$PATH"
  mkdir -p "$BATS_TEST_TMPDIR/stub"
  cat > "$BATS_TEST_TMPDIR/stub/coolant-emit" <<'EOF'
#!/usr/bin/env bash
echo "$@" >> "$COOLANT_EMIT_LOG"
EOF
  chmod +x "$BATS_TEST_TMPDIR/stub/coolant-emit"
  export COOLANT_EMIT_LOG="$BATS_TEST_TMPDIR/emit.log"
}

teardown() { _common_teardown; }

@test "prompt-submit emits counter with session_id from stdin" {
  echo '{"session_id":"test-sid-1","prompt":"hello world"}' | \
    "$BATS_TEST_DIRNAME/../scripts/prompt-submit.sh"
  run cat "$COOLANT_EMIT_LOG"
  [ "$status" -eq 0 ]
  [[ "$output" == *"counter coolant_prompt_total"* ]]
  [[ "$output" == *"session_id=test-sid-1"* ]]
}

@test "prompt-submit emits histogram of prompt length" {
  echo '{"session_id":"s1","prompt":"twelve chars"}' | \
    "$BATS_TEST_DIRNAME/../scripts/prompt-submit.sh"
  run cat "$COOLANT_EMIT_LOG"
  [[ "$output" == *"histogram coolant_prompt_length_chars 12"* ]]
}

@test "prompt-submit does not fail when stdin is empty" {
  run bash -c "echo '' | $BATS_TEST_DIRNAME/../scripts/prompt-submit.sh"
  [ "$status" -eq 0 ]
}
```

Note: if `test_helper.bash` doesn't already expose `_common_setup`/`_common_teardown`, use whatever naming convention the existing test files use. Check `tests/test_helper.bash` first and follow its pattern.

### Step 2: Run test to verify it fails

Run:
```bash
bats tests/prompt-submit.bats
```
Expected: FAIL — `scripts/prompt-submit.sh: No such file or directory`.

### Step 3: Write the hook

Create `scripts/prompt-submit.sh`:

```bash
#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# UserPromptSubmit hook: emit prompt counter and length histogram.

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./common.sh
source "$SCRIPT_DIR/common.sh"

EMIT="${COOLANT_EMIT:-${CLAUDE_PLUGIN_ROOT:-$SCRIPT_DIR/..}/bin/coolant-emit}"

# Read the hook payload from stdin (JSON, one shot).
payload=$(cat)

session_id=$(printf '%s' "$payload" | _json_field session_id)
prompt=$(printf '%s' "$payload" | _json_field prompt)

# Length in characters. Fall back to 0 if prompt is empty/unreadable.
length=${#prompt}

labels=""
if [ -n "$session_id" ]; then
  labels="session_id=$session_id"
fi

"$EMIT" counter coolant_prompt_total $labels || true
"$EMIT" histogram coolant_prompt_length_chars "$length" $labels || true

exit 0
```

### Step 4: Run test to verify it passes

Run:
```bash
chmod +x scripts/prompt-submit.sh
bats tests/prompt-submit.bats
```
Expected: all three tests PASS.

### Step 5: Commit via `/commit` skill

Invoke `/commit`.

---

## Task B2 — `preflight.sh` SessionStart extension

**Files:**
- Modify: `scripts/preflight.sh`
- Modify/Create: `tests/preflight.bats`

Extend existing preflight hook to: (a) emit `coolant_session_start_total{branch_state=clean|dirty}` counter, and (b) cache the current HEAD SHA to a session-scoped file so `session-end.sh` can diff against it.

### Step 1: Write new test cases

Open `tests/preflight.bats` (create if missing). Add at the end (preserve existing tests):

```bash
@test "preflight caches HEAD SHA to session-scoped file" {
  cd "$(mktemp -d)"
  git init -q && git commit -q --allow-empty -m "initial"
  sha=$(git rev-parse HEAD)

  echo "{\"session_id\":\"sess-42\"}" | \
    bash "$BATS_TEST_DIRNAME/../scripts/preflight.sh"

  cache="${TMPDIR:-/tmp/}coolant-${USER}.session-sess-42.head"
  [ -f "$cache" ]
  [ "$(cat "$cache")" = "$sha" ]
}

@test "preflight emits session_start_total with branch_state=clean" {
  cd "$(mktemp -d)"
  git init -q && git commit -q --allow-empty -m "initial"

  export PATH="$BATS_TEST_TMPDIR/stub:$PATH"
  mkdir -p "$BATS_TEST_TMPDIR/stub"
  cat > "$BATS_TEST_TMPDIR/stub/coolant-emit" <<'EOF'
#!/usr/bin/env bash
echo "$@" >> "$COOLANT_EMIT_LOG"
EOF
  chmod +x "$BATS_TEST_TMPDIR/stub/coolant-emit"
  export COOLANT_EMIT_LOG="$BATS_TEST_TMPDIR/emit.log"

  echo '{"session_id":"s1"}' | \
    bash "$BATS_TEST_DIRNAME/../scripts/preflight.sh"

  run cat "$COOLANT_EMIT_LOG"
  [[ "$output" == *"counter coolant_session_start_total"* ]]
  [[ "$output" == *"branch_state=clean"* ]]
}

@test "preflight emits branch_state=dirty when working tree has changes" {
  cd "$(mktemp -d)"
  git init -q && git commit -q --allow-empty -m "initial"
  echo "dirty" > file
  git add file

  export PATH="$BATS_TEST_TMPDIR/stub:$PATH"
  mkdir -p "$BATS_TEST_TMPDIR/stub"
  cat > "$BATS_TEST_TMPDIR/stub/coolant-emit" <<'EOF'
#!/usr/bin/env bash
echo "$@" >> "$COOLANT_EMIT_LOG"
EOF
  chmod +x "$BATS_TEST_TMPDIR/stub/coolant-emit"
  export COOLANT_EMIT_LOG="$BATS_TEST_TMPDIR/emit.log"

  echo '{"session_id":"s2"}' | \
    bash "$BATS_TEST_DIRNAME/../scripts/preflight.sh"

  run cat "$COOLANT_EMIT_LOG"
  [[ "$output" == *"branch_state=dirty"* ]]
}
```

### Step 2: Run to verify new tests fail

Run:
```bash
bats tests/preflight.bats
```
Expected: the three new tests FAIL; existing tests continue to PASS.

### Step 3: Extend `preflight.sh`

The existing `preflight.sh` reads stdin into a variable named `input` at line 10. It has no final `exit 0` — the script ends naturally. The existing script's structure (for reference):

```
line 10:  input=$(cat)
line 13:  coolant_event '"event":"counter.reset"'
line 15:  cwd=$(echo "$input" | _json_field cwd)
lines 20-41:  worktree exclusion warnings, emits hookSpecificOutput JSON
```

Insert the new telemetry block **between line 13 and line 15** (after `counter.reset` event, before the `cwd` extraction). This keeps the session-start counter-reset ordering correct and preserves all existing behavior. The block reuses `$input` — do NOT re-read stdin.

The block to insert:

```bash
# ---- Phase 1 telemetry: session start metrics + HEAD cache ----

EMIT="${COOLANT_EMIT:-${CLAUDE_PLUGIN_ROOT:-$SCRIPT_DIR/..}/bin/coolant-emit}"

_session_id=$(echo "$input" | _json_field session_id)

# Cache HEAD SHA for session-end diff.
if [ -n "$_session_id" ]; then
  _cache="${TMPDIR:-/tmp/}coolant-${USER}.session-${_session_id}.head"
  if _head=$(git rev-parse HEAD 2>/dev/null); then
    printf '%s' "$_head" > "$_cache"
  fi
fi

# Detect branch state.
_branch_state="clean"
if ! git diff-index --quiet HEAD -- 2>/dev/null; then
  _branch_state="dirty"
fi

_labels=""
if [ -n "$_session_id" ]; then
  _labels="session_id=$_session_id"
fi
"$EMIT" counter coolant_session_start_total branch_state="$_branch_state" $_labels || true

unset _session_id _cache _head _branch_state _labels
```

Do not modify any other part of `preflight.sh`. The worktree exclusion warnings and `hookSpecificOutput` emission below the insertion point must remain intact.

### Step 4: Run all preflight tests to verify green

Run:
```bash
bats tests/preflight.bats
```
Expected: all tests (existing + three new) PASS.

### Step 5: Commit via `/commit` skill

Invoke `/commit`.

---

## Task B3 — `gate.sh` tool-invocation counter extension

**Files:**
- Modify: `scripts/gate.sh`
- Modify: `tests/gate.bats`

Extend existing `gate.sh` to emit `coolant_tool_invocation_total{tool_name}` on every PreToolUse invocation, before the gate logic runs.

### Step 1: Add failing test

Open `tests/gate.bats`. Add at the end:

```bash
@test "gate emits tool_invocation_total counter with tool_name label" {
  export PATH="$BATS_TEST_TMPDIR/stub:$PATH"
  mkdir -p "$BATS_TEST_TMPDIR/stub"
  cat > "$BATS_TEST_TMPDIR/stub/coolant-emit" <<'EOF'
#!/usr/bin/env bash
echo "$@" >> "$COOLANT_EMIT_LOG"
EOF
  chmod +x "$BATS_TEST_TMPDIR/stub/coolant-emit"
  export COOLANT_EMIT_LOG="$BATS_TEST_TMPDIR/emit.log"

  echo '{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"echo hi"}}' | \
    bash "$BATS_TEST_DIRNAME/../scripts/gate.sh" >/dev/null 2>&1

  run cat "$COOLANT_EMIT_LOG"
  [[ "$output" == *"counter coolant_tool_invocation_total"* ]]
  [[ "$output" == *"tool_name=Bash"* ]]
}
```

### Step 2: Run to verify fail

Run:
```bash
bats tests/gate.bats
```
Expected: the new test fails; existing gate tests still pass.

### Step 3: Extend `gate.sh`

The existing `gate.sh` reads stdin into `input` at line 14, then at line 17 does an early exit for non-Bash tool calls:
```
line 14:  input=$(cat)
line 17:  if [[ "$input" != *'"tool_name"'*'"Bash"'* ]]; then exit 0; fi
```

Insert the telemetry counter **between line 14 and line 17** so it emits for ALL tool invocations, not just Bash. This captures tool-level observability without changing gate's existing Bash-only gating behavior.

The block to insert between the stdin read and the non-Bash early exit:

```bash
# ---- Phase 1 telemetry: tool invocation counter (all tools) ----
_tool_name=$(echo "$input" | _json_field tool_name)
_session_id=$(echo "$input" | _json_field session_id)
if [ -n "$_tool_name" ]; then
  _emit_labels="tool_name=$_tool_name"
  [ -n "$_session_id" ] && _emit_labels="$_emit_labels session_id=$_session_id"
  EMIT="${COOLANT_EMIT:-${CLAUDE_PLUGIN_ROOT:-$SCRIPT_DIR/..}/bin/coolant-emit}"
  "$EMIT" counter coolant_tool_invocation_total $_emit_labels || true
  unset _emit_labels EMIT
fi
unset _tool_name _session_id
```

Do not modify any other part of `gate.sh`. The capping and suppression logic below the insertion point must remain untouched — verify by running the existing `bats tests/gate.bats` after the edit and confirming the pre-existing tests still pass.

### Step 4: Run all gate tests

Run:
```bash
bats tests/gate.bats
```
Expected: all PASS. Critically, the existing gate behavior (test capping, build suppression) must still work — verify by examining a subset of existing tests by name.

### Step 5: Commit via `/commit` skill

Invoke `/commit`.

---

## Task B4 — `tool-post.sh` PostToolUse hook

**Files:**
- Create: `scripts/tool-post.sh`
- Create: `tests/tool-post.bats`

Emits `coolant_tool_error_total{tool_name,exit_code}` when a tool finishes with non-zero exit.

### Step 1: Write failing bats test

Create `tests/tool-post.bats`:

```bash
#!/usr/bin/env bats

load 'test_helper'

setup() {
  _common_setup
  export PATH="$BATS_TEST_TMPDIR/stub:$PATH"
  mkdir -p "$BATS_TEST_TMPDIR/stub"
  cat > "$BATS_TEST_TMPDIR/stub/coolant-emit" <<'EOF'
#!/usr/bin/env bash
echo "$@" >> "$COOLANT_EMIT_LOG"
EOF
  chmod +x "$BATS_TEST_TMPDIR/stub/coolant-emit"
  export COOLANT_EMIT_LOG="$BATS_TEST_TMPDIR/emit.log"
}

teardown() { _common_teardown; }

@test "tool-post emits error counter when exit_code is non-zero" {
  echo '{"session_id":"s1","tool_name":"Bash","tool_response":{"exit_code":1}}' | \
    "$BATS_TEST_DIRNAME/../scripts/tool-post.sh"
  run cat "$COOLANT_EMIT_LOG"
  [[ "$output" == *"counter coolant_tool_error_total"* ]]
  [[ "$output" == *"tool_name=Bash"* ]]
  [[ "$output" == *"exit_code=1"* ]]
}

@test "tool-post emits nothing when exit_code is zero" {
  echo '{"session_id":"s1","tool_name":"Bash","tool_response":{"exit_code":0}}' | \
    "$BATS_TEST_DIRNAME/../scripts/tool-post.sh"
  [ ! -s "$COOLANT_EMIT_LOG" ]
}

@test "tool-post is silent when exit_code field is missing" {
  echo '{"session_id":"s1","tool_name":"Read"}' | \
    "$BATS_TEST_DIRNAME/../scripts/tool-post.sh"
  [ ! -s "$COOLANT_EMIT_LOG" ]
}
```

### Step 2: Run to verify fail

Run: `bats tests/tool-post.bats` — FAIL (script missing).

### Step 3: Write the hook

Create `scripts/tool-post.sh`:

```bash
#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# PostToolUse hook: emit tool error counter when exit_code != 0.

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./common.sh
source "$SCRIPT_DIR/common.sh"

EMIT="${COOLANT_EMIT:-${CLAUDE_PLUGIN_ROOT:-$SCRIPT_DIR/..}/bin/coolant-emit}"

payload=$(cat)
tool_name=$(printf '%s' "$payload" | _json_field tool_name)
session_id=$(printf '%s' "$payload" | _json_field session_id)

# Extract exit_code from tool_response.exit_code via bash regex.
exit_code=""
if [[ "$payload" =~ \"tool_response\"[[:space:]]*:[[:space:]]*\{[^\}]*\"exit_code\"[[:space:]]*:[[:space:]]*([0-9]+) ]]; then
  exit_code="${BASH_REMATCH[1]}"
fi

if [ -n "$exit_code" ] && [ "$exit_code" != "0" ]; then
  labels="tool_name=$tool_name exit_code=$exit_code"
  [ -n "$session_id" ] && labels="$labels session_id=$session_id"
  "$EMIT" counter coolant_tool_error_total $labels || true
fi

exit 0
```

### Step 4: Run tests to verify green

Run:
```bash
chmod +x scripts/tool-post.sh
bats tests/tool-post.bats
```
Expected: all three tests PASS.

### Step 5: Commit via `/commit` skill

Invoke `/commit`.

---

## Task B5 — `compact.sh` PreCompact hook

**Files:**
- Create: `scripts/compact.sh`
- Create: `tests/compact.bats`

Emits `coolant_context_compaction_total` each time Claude Code compacts the context window.

### Step 1: Write failing bats test

Create `tests/compact.bats`:

```bash
#!/usr/bin/env bats

load 'test_helper'

setup() {
  _common_setup
  export PATH="$BATS_TEST_TMPDIR/stub:$PATH"
  mkdir -p "$BATS_TEST_TMPDIR/stub"
  cat > "$BATS_TEST_TMPDIR/stub/coolant-emit" <<'EOF'
#!/usr/bin/env bash
echo "$@" >> "$COOLANT_EMIT_LOG"
EOF
  chmod +x "$BATS_TEST_TMPDIR/stub/coolant-emit"
  export COOLANT_EMIT_LOG="$BATS_TEST_TMPDIR/emit.log"
}

teardown() { _common_teardown; }

@test "compact emits context_compaction_total counter" {
  echo '{"session_id":"sess-x"}' | \
    "$BATS_TEST_DIRNAME/../scripts/compact.sh"
  run cat "$COOLANT_EMIT_LOG"
  [[ "$output" == *"counter coolant_context_compaction_total"* ]]
  [[ "$output" == *"session_id=sess-x"* ]]
}
```

### Step 2: Run, fail

Run: `bats tests/compact.bats` — FAIL.

### Step 3: Write the hook

Create `scripts/compact.sh`:

```bash
#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# PreCompact hook: emit context compaction counter.

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./common.sh
source "$SCRIPT_DIR/common.sh"

EMIT="${COOLANT_EMIT:-${CLAUDE_PLUGIN_ROOT:-$SCRIPT_DIR/..}/bin/coolant-emit}"

payload=$(cat)
session_id=$(printf '%s' "$payload" | _json_field session_id)

labels=""
[ -n "$session_id" ] && labels="session_id=$session_id"

"$EMIT" counter coolant_context_compaction_total $labels || true

exit 0
```

### Step 4: Run, pass

```bash
chmod +x scripts/compact.sh
bats tests/compact.bats
```
Expected: PASS.

### Step 5: Commit via `/commit` skill

---

## Task B6 — `session-end.sh` Stop hook

**Files:**
- Create: `scripts/session-end.sh`
- Create: `tests/session-end.bats`

Diffs current HEAD against the cached session-start HEAD (populated by B2). Emits `coolant_session_outcome_total{outcome}` plus per-commit `coolant_session_commits_total` and `coolant_session_lines_in_commits` histogram. Also writes the `{session_id, [shas]}` mapping to the JSONL event log. Cleans up the cache file.

### Step 1: Write failing bats test

Create `tests/session-end.bats`:

```bash
#!/usr/bin/env bats

load 'test_helper'

setup() {
  _common_setup
  export PATH="$BATS_TEST_TMPDIR/stub:$PATH"
  mkdir -p "$BATS_TEST_TMPDIR/stub"
  cat > "$BATS_TEST_TMPDIR/stub/coolant-emit" <<'EOF'
#!/usr/bin/env bash
echo "$@" >> "$COOLANT_EMIT_LOG"
EOF
  chmod +x "$BATS_TEST_TMPDIR/stub/coolant-emit"
  export COOLANT_EMIT_LOG="$BATS_TEST_TMPDIR/emit.log"
  export COOLANT_EVENTS="$BATS_TEST_TMPDIR/events.jsonl"
}

teardown() { _common_teardown; }

@test "session-end reports outcome=committed when new commits exist" {
  cd "$(mktemp -d)"
  git init -q
  git commit -q --allow-empty -m "initial"
  start_sha=$(git rev-parse HEAD)
  cache="${TMPDIR:-/tmp/}coolant-${USER}.session-sess-A.head"
  printf '%s' "$start_sha" > "$cache"
  # Make a commit.
  git commit -q --allow-empty -m "work"

  echo '{"session_id":"sess-A"}' | \
    "$BATS_TEST_DIRNAME/../scripts/session-end.sh"

  run cat "$COOLANT_EMIT_LOG"
  [[ "$output" == *"counter coolant_session_outcome_total outcome=committed"* ]]
  [[ "$output" == *"counter coolant_session_commits_total"* ]]
  [ ! -f "$cache" ]  # cache cleaned up
}

@test "session-end reports outcome=no_commit when HEAD unchanged" {
  cd "$(mktemp -d)"
  git init -q && git commit -q --allow-empty -m "initial"
  start_sha=$(git rev-parse HEAD)
  cache="${TMPDIR:-/tmp/}coolant-${USER}.session-sess-B.head"
  printf '%s' "$start_sha" > "$cache"

  echo '{"session_id":"sess-B"}' | \
    "$BATS_TEST_DIRNAME/../scripts/session-end.sh"

  run cat "$COOLANT_EMIT_LOG"
  [[ "$output" == *"outcome=no_commit"* ]]
}

@test "session-end writes session->shas mapping to JSONL" {
  cd "$(mktemp -d)"
  git init -q && git commit -q --allow-empty -m "initial"
  start_sha=$(git rev-parse HEAD)
  cache="${TMPDIR:-/tmp/}coolant-${USER}.session-sess-C.head"
  printf '%s' "$start_sha" > "$cache"
  git commit -q --allow-empty -m "work"
  new_sha=$(git rev-parse HEAD)

  echo '{"session_id":"sess-C"}' | \
    "$BATS_TEST_DIRNAME/../scripts/session-end.sh"

  run cat "$COOLANT_EVENTS"
  [[ "$output" == *"session-shas"* ]]
  [[ "$output" == *"sess-C"* ]]
  [[ "$output" == *"$new_sha"* ]]
}
```

### Step 2: Run, fail

Run: `bats tests/session-end.bats` — FAIL.

### Step 3: Write the hook

Create `scripts/session-end.sh`:

```bash
#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Stop hook: emit session outcome based on HEAD diff vs cached start SHA.

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./common.sh
source "$SCRIPT_DIR/common.sh"

EMIT="${COOLANT_EMIT:-${CLAUDE_PLUGIN_ROOT:-$SCRIPT_DIR/..}/bin/coolant-emit}"

payload=$(cat)
session_id=$(printf '%s' "$payload" | _json_field session_id)

if [ -z "$session_id" ]; then
  exit 0
fi

cache="${TMPDIR:-/tmp/}coolant-${USER}.session-${session_id}.head"
if [ ! -f "$cache" ]; then
  # No cached start — nothing to diff against.
  "$EMIT" counter coolant_session_outcome_total outcome=no_start session_id="$session_id" || true
  exit 0
fi

start_sha=$(cat "$cache")
rm -f "$cache"

current_sha=$(git rev-parse HEAD 2>/dev/null || echo "")
if [ -z "$current_sha" ] || [ "$current_sha" = "$start_sha" ]; then
  "$EMIT" counter coolant_session_outcome_total outcome=no_commit session_id="$session_id" || true
  exit 0
fi

# Commits landed. Collect SHAs.
shas=$(git rev-list "$start_sha..$current_sha" 2>/dev/null)
if [ -z "$shas" ]; then
  "$EMIT" counter coolant_session_outcome_total outcome=no_commit session_id="$session_id" || true
  exit 0
fi

"$EMIT" counter coolant_session_outcome_total outcome=committed session_id="$session_id" || true

# Count commits and total lines changed.
commit_count=0
total_lines=0
sha_json_list=""
while IFS= read -r sha; do
  [ -z "$sha" ] && continue
  commit_count=$((commit_count + 1))
  lines=$(git show --numstat --format='' "$sha" 2>/dev/null | awk '{ if ($1 != "-") sum += $1 + $2 } END { print sum+0 }')
  total_lines=$((total_lines + lines))
  if [ -n "$sha_json_list" ]; then sha_json_list="$sha_json_list,"; fi
  sha_json_list="$sha_json_list\"$sha\""
  "$EMIT" counter coolant_session_commits_total session_id="$session_id" || true
done <<< "$shas"

"$EMIT" histogram coolant_session_lines_in_commits "$total_lines" session_id="$session_id" || true

# Write session->shas mapping into JSONL for Phase 3 correlator to consume.
coolant_event "\"event\":\"session-shas\",\"session_id\":\"$(_json_escape "$session_id")\",\"shas\":[$sha_json_list],\"commit_count\":$commit_count,\"total_lines\":$total_lines"

exit 0
```

### Step 4: Run, pass

```bash
chmod +x scripts/session-end.sh
bats tests/session-end.bats
```
Expected: all three tests PASS.

### Step 5: Commit via `/commit` skill

---

## Task B7 — `agent-start.sh` / `agent-stop.sh` active gauge extension

**Files:**
- Modify: `scripts/agent-start.sh`
- Modify: `scripts/agent-stop.sh`
- Modify: `tests/agent-start.bats`
- Modify: `tests/agent-stop.bats`

Extend existing counter-reconcile behavior to also emit `coolant_subagent_active_gauge` — a gauge showing the current number of active subagents.

### Step 1: Add failing test to `tests/agent-start.bats`

Append:
```bash
@test "agent-start emits subagent_active_gauge" {
  export PATH="$BATS_TEST_TMPDIR/stub:$PATH"
  mkdir -p "$BATS_TEST_TMPDIR/stub"
  cat > "$BATS_TEST_TMPDIR/stub/coolant-emit" <<'EOF'
#!/usr/bin/env bash
echo "$@" >> "$COOLANT_EMIT_LOG"
EOF
  chmod +x "$BATS_TEST_TMPDIR/stub/coolant-emit"
  export COOLANT_EMIT_LOG="$BATS_TEST_TMPDIR/emit.log"
  echo '{"session_id":"s1"}' | \
    bash "$BATS_TEST_DIRNAME/../scripts/agent-start.sh" >/dev/null 2>&1 || true
  run cat "$COOLANT_EMIT_LOG"
  [[ "$output" == *"gauge coolant_subagent_active_gauge"* ]]
}
```

Similar test appended to `tests/agent-stop.bats`.

### Step 2: Run, fail

Run: `bats tests/agent-start.bats tests/agent-stop.bats` — FAIL on the new tests.

### Step 3: Extend both scripts

The existing `agent-start.sh` and `agent-stop.sh` both compute a `next` variable holding the new agent count, then write it to `$COOLANT_COUNTER`, then emit a `coolant_event`, then conditionally print a system message. Neither script has a final `exit 0` — they end naturally.

**In `scripts/agent-start.sh`:** add this block at the **end of the file** (after the final `fi` that closes the threshold-warning block). `$next` is still in scope.

```bash
# ---- Phase 1 telemetry: subagent active gauge ----
"${COOLANT_EMIT:-${CLAUDE_PLUGIN_ROOT:-$SCRIPT_DIR/..}/bin/coolant-emit}" gauge coolant_subagent_active_gauge "$next" session_id="$_agent_session_id" || true
```

**In `scripts/agent-stop.sh`:** add the same block at the **end of the file** (after the final `fi` that closes the auto-disengage block). Same `$next` variable is in scope.

```bash
# ---- Phase 1 telemetry: subagent active gauge ----
"${COOLANT_EMIT:-${CLAUDE_PLUGIN_ROOT:-$SCRIPT_DIR/..}/bin/coolant-emit}" gauge coolant_subagent_active_gauge "$next" session_id="$_agent_session_id" || true
```

Note: `$_agent_session_id` is populated by the existing `_extract_agent_fields "$input"` call near the top of each script, so no additional parsing is needed. Do not modify any other part of either script.

### Step 4: Run, pass

```bash
bats tests/agent-start.bats tests/agent-stop.bats
```
Expected: all PASS.

### Step 5: Commit via `/commit` skill

---

## Task B8 — `/commit` skill session trailer (CROSS-REPO)

**⚠️ This task modifies a file outside this repo.** The user's `/commit` skill lives at:
```
~/.claude/plugins/local/personal-plugins/plugins/commit-skill/skills/commit/SKILL.md
```

That's a separate personal-plugins plugin with its own git repo (or loose filesystem — the user may not have it under version control yet). All edits go to that file and any supporting scripts alongside it, NOT to this repo. This task's "commit via `/commit`" step therefore produces a commit in the *commit-skill* plugin repo, not in coolant.

**Files:**
- Modify: `~/.claude/plugins/local/personal-plugins/plugins/commit-skill/skills/commit/SKILL.md`
- Possibly create: `~/.claude/plugins/local/personal-plugins/plugins/commit-skill/bin/coolant-trailer.sh` (a helper script the skill calls to generate the trailer)
- Possibly create: a bats test alongside the plugin repo if it has a test tree; otherwise test manually

**This task is parallel-safe with B1–B7 because it touches a completely different filesystem tree.** No coordination needed with the parallel hook tasks.

### Step 1: Inspect the existing `/commit` skill and confirm git status of its containing repo

```bash
SKILL="${HOME}/.claude/plugins/local/personal-plugins/plugins/commit-skill/skills/commit/SKILL.md"
PLUGIN_ROOT="${HOME}/.claude/plugins/local/personal-plugins/plugins/commit-skill"

# Verify the file exists
[ -f "$SKILL" ] && echo "skill present" || echo "MISSING — abort task"

# Is the plugin directory a git repo?
git -C "$PLUGIN_ROOT" rev-parse --show-toplevel 2>/dev/null && echo "tracked" || echo "NOT a git repo"

# Read the skill
cat "$SKILL"
```

Note the skill's workflow structure. Identify the step where the commit message is built and the step where `git commit` is invoked. The trailer block needs to be appended to the message body *before* `git commit` runs.

If the plugin directory is NOT a git repo, surface this to the user immediately. Modifying ad-hoc plugin files without version control is a reversibility risk — offer to `git init` the plugin dir first or back up the SKILL.md before edits.

### Step 2: Write the trailer-generation helper

Create `~/.claude/plugins/local/personal-plugins/plugins/commit-skill/bin/coolant-trailer.sh`:

```bash
#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Emits a Coolant-Session-V1 trailer block on stdout for use by /commit.
# Queries Prometheus at commit time for cost and token totals associated
# with the current session. Silently omits unresolvable fields.

set -u

# Resolve session_id from the most recent JSONL event (source of truth
# for the active Claude Code session's ID).
_events="${COOLANT_EVENTS:-${TMPDIR:-/tmp/}coolant-${USER}.events.jsonl}"
_sid=""
if [ -f "$_events" ]; then
  # Find the most recent event line that carries a session_id.
  _sid=$(tac "$_events" 2>/dev/null | grep -m1 -oE '"session_id"[[:space:]]*:[[:space:]]*"[^"]+"' | head -1 | sed -E 's/.*"([^"]+)"$/\1/')
  # macOS lacks `tac`; fallback to awk-reverse if empty.
  if [ -z "$_sid" ]; then
    _sid=$(awk '{lines[NR]=$0} END{for(i=NR;i>0;i--) print lines[i]}' "$_events" 2>/dev/null | grep -m1 -oE '"session_id"[[:space:]]*:[[:space:]]*"[^"]+"' | head -1 | sed -E 's/.*"([^"]+)"$/\1/')
  fi
fi

if [ -z "$_sid" ]; then
  # No session to attribute — emit nothing.
  exit 0
fi

# Derive Prometheus base URL from the OTLP endpoint.
_otlp="${OTEL_EXPORTER_OTLP_METRICS_ENDPOINT:-}"
_prom=""
if [ -n "$_otlp" ]; then
  # Strip the OTLP path (/api/v1/otlp/...) to get the base.
  _prom="${_otlp%/api/v1/otlp*}"
fi

if [ -z "$_prom" ]; then
  # No Prometheus to query — emit just the session ID.
  printf 'Coolant-Session-V1: %s\n' "$_sid"
  exit 0
fi

_query() {
  local q="$1"
  curl -sf --max-time 2 --data-urlencode "query=$q" "$_prom/api/v1/query" 2>/dev/null \
    | python3 -c 'import json,sys; r=json.load(sys.stdin)["data"]["result"]; print(r[0]["value"][1] if r else "")' 2>/dev/null
}

_cost=$(_query "sum(claude_code_cost_usage_USD_total{session_id=\"$_sid\"})")
_in=$(_query   "sum(claude_code_token_usage_tokens_total{session_id=\"$_sid\",type=\"input\"})")
_out=$(_query  "sum(claude_code_token_usage_tokens_total{session_id=\"$_sid\",type=\"output\"})")
_cr=$(_query   "sum(claude_code_token_usage_tokens_total{session_id=\"$_sid\",type=\"cacheRead\"})")
_cc=$(_query   "sum(claude_code_token_usage_tokens_total{session_id=\"$_sid\",type=\"cacheCreation\"})")

printf 'Coolant-Session-V1: %s\n' "$_sid"
[ -n "$_cost" ] && [ "$_cost" != "0" ]  && printf 'Coolant-Cost-USD: %s\n' "$_cost"
[ -n "$_in" ]   && [ "$_in" != "0" ]    && printf 'Coolant-Tokens-Input: %d\n' "${_in%.*}"
[ -n "$_out" ]  && [ "$_out" != "0" ]   && printf 'Coolant-Tokens-Output: %d\n' "${_out%.*}"
[ -n "$_cr" ]   && [ "$_cr" != "0" ]    && printf 'Coolant-Tokens-CacheRead: %d\n' "${_cr%.*}"
[ -n "$_cc" ]   && [ "$_cc" != "0" ]    && printf 'Coolant-Tokens-CacheCreation: %d\n' "${_cc%.*}"
```

Make it executable: `chmod +x "${PLUGIN_ROOT}/bin/coolant-trailer.sh"`.

### Step 3: Modify `SKILL.md` to invoke the helper

Open the `SKILL.md` at `$SKILL`. Locate the section that builds the commit message body (the skill's workflow step that assembles `Recipe:` and `Changes:` blocks).

Add, immediately before the step that invokes `git commit`, a new sub-step:

```markdown
### Append Coolant session trailer

Before running `git commit`, invoke the trailer helper and append its output
to the commit message body, separated by a blank line:

```bash
trailer="$(${HOME}/.claude/plugins/local/personal-plugins/plugins/commit-skill/bin/coolant-trailer.sh)"
if [ -n "$trailer" ]; then
  commit_msg="${commit_msg}

${trailer}"
fi
```

The helper silently emits nothing if it cannot resolve a session ID, so
this is safe to invoke unconditionally.
```

Adjust variable names (`commit_msg`) to match whatever the skill already uses for its assembled message. Preserve all existing skill behavior.

### Step 4: Smoke test against the live stack

With `dev/otel/start.sh` running and a recent Claude Code session having emitted metrics:

```bash
# Ensure env is sourced
source /Users/toddwshaffer/Desktop/apps/coolant/dev/otel/env.sh
# Invoke the helper directly
"${HOME}/.claude/plugins/local/personal-plugins/plugins/commit-skill/bin/coolant-trailer.sh"
```
Expected: output includes `Coolant-Session-V1: <uuid>` and, if Prometheus has data for that session, the cost/token lines.

Make a trivial change to this repo, invoke `/commit`, and inspect the resulting commit:
```bash
cd /Users/toddwshaffer/Desktop/apps/coolant
echo "" >> CLAUDE.md
# Invoke /commit via Claude Code (this is user-driven, not scripted)
# After commit succeeds:
git log -1 --format=%B
```
Expected: commit message body ends with a blank line followed by the `Coolant-Session-V1:` block.

### Step 5: Commit the skill changes

This commit lands in the commit-skill plugin repo, not coolant. Use `/commit` (the now-modified skill) to commit inside the plugin directory:

```bash
cd "${HOME}/.claude/plugins/local/personal-plugins/plugins/commit-skill"
# Then invoke /commit via Claude Code
```

If the plugin directory is not a git repo, surface this to the user and stop — they need to decide whether to `git init` the plugin directory or track changes some other way. Do NOT delete or rewrite SKILL.md without version control safety.

---

## Task B9 — Wire all hooks into `hooks/hooks.json` (SERIAL merge)

**Files:**
- Modify: `hooks/hooks.json`

**This task runs AFTER all parallel tasks (B1–B8) have committed.** It is the only task that writes to `hooks/hooks.json`.

### Step 1: Read current `hooks/hooks.json`

Run:
```bash
cat hooks/hooks.json
```
Note the existing entries: `SessionStart`, `PreToolUse`, `SubagentStart`, `SubagentStop`.

### Step 2: Replace `hooks/hooks.json` with the complete manifest

Overwrite the file with:

```json
{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup",
        "hooks": [
          {
            "type": "command",
            "command": "${CLAUDE_PLUGIN_ROOT}/scripts/preflight.sh",
            "timeout": 10,
            "statusMessage": "Coolant: preflight checks..."
          }
        ]
      }
    ],
    "UserPromptSubmit": [
      {
        "matcher": ".*",
        "hooks": [
          {
            "type": "command",
            "command": "${CLAUDE_PLUGIN_ROOT}/scripts/prompt-submit.sh",
            "timeout": 5,
            "statusMessage": "Coolant: recording prompt..."
          }
        ]
      }
    ],
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "${CLAUDE_PLUGIN_ROOT}/scripts/gate.sh",
            "timeout": 5,
            "statusMessage": "Coolant: gating..."
          }
        ]
      }
    ],
    "PostToolUse": [
      {
        "matcher": ".*",
        "hooks": [
          {
            "type": "command",
            "command": "${CLAUDE_PLUGIN_ROOT}/scripts/tool-post.sh",
            "timeout": 5,
            "statusMessage": "Coolant: post-tool..."
          }
        ]
      }
    ],
    "PreCompact": [
      {
        "matcher": ".*",
        "hooks": [
          {
            "type": "command",
            "command": "${CLAUDE_PLUGIN_ROOT}/scripts/compact.sh",
            "timeout": 5,
            "statusMessage": "Coolant: compacting..."
          }
        ]
      }
    ],
    "Stop": [
      {
        "matcher": ".*",
        "hooks": [
          {
            "type": "command",
            "command": "${CLAUDE_PLUGIN_ROOT}/scripts/session-end.sh",
            "timeout": 10,
            "statusMessage": "Coolant: session end..."
          }
        ]
      }
    ],
    "SubagentStart": [
      {
        "matcher": ".*",
        "hooks": [
          {
            "type": "command",
            "command": "${CLAUDE_PLUGIN_ROOT}/scripts/agent-start.sh",
            "timeout": 5,
            "statusMessage": "Coolant: agent starting..."
          }
        ]
      }
    ],
    "SubagentStop": [
      {
        "matcher": ".*",
        "hooks": [
          {
            "type": "command",
            "command": "${CLAUDE_PLUGIN_ROOT}/scripts/agent-stop.sh",
            "timeout": 5,
            "statusMessage": "Coolant: agent stopping..."
          }
        ]
      }
    ]
  }
}
```

### Step 3: Validate the JSON

Run:
```bash
python3 -m json.tool hooks/hooks.json > /dev/null && echo "JSON valid"
```
Expected: `JSON valid`.

### Step 4: Integration smoke test

Restart Claude Code (or reload plugins) so the new hooks register. Then:

1. Launch a session in this repo via `cclaude`.
2. Type a prompt: `echo test`.
3. Exit.
4. Query Prometheus for the new metrics:
```bash
for m in coolant_prompt_total coolant_tool_invocation_total coolant_session_start_total coolant_session_outcome_total; do
  echo "=== $m ==="
  curl -s "http://localhost:9090/api/v1/query?query=$m" | python3 -m json.tool | head -15
done
```
Expected: each metric returns at least one sample with `repo=coolant` in its labels.

### Step 5: Commit via `/commit` skill

---

## Exit criteria for Plan 1b

Before starting Plan 1c, verify:

- [ ] `bin/coolant-emit` builds and its tests pass
- [ ] All six new/extended hook scripts exist in `scripts/` and pass their bats tests
- [ ] `hooks/hooks.json` contains entries for all eight hook points listed in Task B9
- [ ] A live smoke test against the local Prometheus shows the new metrics populating
- [ ] `/commit` skill emits a `Coolant-Session-V1:` trailer in the most recent commit
- [ ] All existing tests still pass: `go test ./... && bats tests/`

If any of the above fails, do not proceed to Plan 1c.
