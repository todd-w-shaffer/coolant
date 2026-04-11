# Thermal Enterprise OTEL Export: Security & Compliance Review

**Reviewer:** Security & Compliance Engineering
**Date:** 2026-04-11
**Status:** PRE-IMPLEMENTATION REVIEW — blocking on resolution of findings before development begins

---

## 1. Data Classification

Every metric and attribute Thermal would emit via OTLP must be classified. The collector currently gathers the following, mapped to their risk tier:

| Data Element | Source | Classification | Rationale |
|---|---|---|---|
| CPU %, MEM %, SWAP %, GPU % | sysctl, vm_stat, mach, ioreg | **Public** | Aggregate utilization percentages with no identifying content. |
| Memory bytes (used/total), swap bytes | sysctl, vm_stat | **Internal** | Machine sizing reveals hardware provisioning — low risk but not public. |
| Decompressions/tick | vm_stat delta | **Internal** | Operational metric, no content leakage. |
| Process count by category (build, shell, node, go, etc.) | ps -Ao | **Internal** | Aggregate counts are benign. Categories are fixed strings. |
| Threat level (COOL/WARM/HOT/MELTDOWN) | Derived | **Internal** | Computed from the above. No raw data. |
| Agent count, spawn/death rates | JSONL event bus | **Internal** | Operational cardinality. |
| Process names (`Comm` field: "node", "vitest", "cargo") | ps -Ao comm= | **Confidential** | Short names from `commToType` are safe (generic tooling names), but `ps` returns the *full comm field* which can include path prefixes like `/Users/jane/repos/secret-project/node_modules/.bin/vitest`. The `basename()` call strips paths for dashboard display, but the raw `ProcessInfo.Comm` retains whatever `ps` reported after field splitting. Any OTEL attribute that serializes `Comm` without explicit basename stripping leaks directory structure. |
| Process PIDs, PPIDs | ps -Ao pid=,ppid= | **Confidential** | PIDs are ephemeral but can be correlated across metrics windows to reconstruct process trees and infer workflow patterns. |
| Session IDs, Agent IDs | JSONL events | **Confidential** | Correlation identifiers. Could link activity across time windows. If agent IDs are deterministic or sequential, they reveal workload volume. |
| Command strings from JSONL events (`Command`, `Original`, `Rewritten` fields) | Bash hooks via `_nested_command` | **Restricted** | These capture the *literal command arguments* passed to tools. Commands routinely contain file paths, environment variable references, API keys passed as arguments, database connection strings, and repository names. This is the single highest-risk data element. |
| Hostname, username | Implicit in `$TMPDIR` path, `$USER` | **Confidential** | GDPR-relevant PII in EU jurisdictions. Hostname may encode org structure (e.g., `jane.doe-mbp-eng-london`). |
| `CollectErrs` strings | Collector error messages | **Confidential** | Error messages can embed file paths, hostnames, and system details. |

**Finding:** The `GateEvent.Command`, `GateEvent.Original`, and `GateEvent.Rewritten` fields are **Restricted** and must never be emitted via OTEL under any configuration. Process `Comm` values must pass through `basename()` before export. Hostnames and usernames must be opt-in, not opt-out.

## 2. Exfiltration Surface

Thermal currently has **zero outbound data flow**. The `CheckOnline` function opens a TCP socket to `api.anthropic.com:443` and immediately closes it — no application data is sent. OTEL export fundamentally changes this posture: Thermal becomes an active data exporter.

**Attack vectors:**

- **Config file manipulation:** A TOML config file at `~/.config/coolant/config.toml` controls behavior. If an attacker (or malicious insider) modifies `otel.endpoint` to point to `https://attacker.example.com:4317`, all metrics silently redirect. File permissions are the only barrier. No integrity verification exists.
- **SSRF via endpoint URL:** If the endpoint URL accepts arbitrary schemes or hostnames, Thermal becomes an SSRF proxy. An attacker who can write the config can probe internal networks: `http://169.254.169.254/latest/meta-data/` (cloud metadata), `http://internal-service:8080/admin`, etc. The OTLP client will happily connect.
- **DNS rebinding:** Even with TLS validation, a DNS rebinding attack could redirect the OTLP connection mid-session to an attacker-controlled IP that presents a valid certificate for a different domain.
- **Ambient credential capture:** If OTEL headers contain bearer tokens and the config file is world-readable, any local process can harvest the token.

**Finding:** The transition from zero-egress to active-export is a **material change to the security posture** that requires explicit customer acknowledgment during deployment. Enterprise customers must be able to audit exactly what leaves the machine.

## 3. Transport Security

**Mandatory defaults:**

- **TLS required, plaintext refused.** OTLP plaintext (port 4317 without TLS) must not be supported. The `otel.endpoint` field must require `https://` or `grpcs://` scheme. If a plaintext URL is provided, Thermal must refuse to start and log the reason.
- **Certificate validation enabled by default.** No `insecure_skip_verify`. If customers need custom CAs (common in enterprise with TLS-intercepting proxies), provide `otel.ca_cert` pointing to a PEM bundle.
- **mTLS support.** Enterprises running zero-trust networks require mutual TLS. Configuration: `otel.client_cert` and `otel.client_key` fields. These files must be validated at startup (exist, parseable, not expired).
- **Certificate file permissions.** Private keys (`otel.client_key`) must be `0600` or stricter. Thermal should refuse to start if the key file is group- or world-readable, logging the exact permission bits found. CA certs and client certs can be `0644`.
- **Certificate storage.** Keys should live in `~/.config/coolant/certs/` with the directory itself at `0700`. Consider supporting macOS Keychain references (`security find-identity`) as an alternative to on-disk keys for managed fleets.

## 4. Label/Attribute Sanitization

Enterprise customers will attach custom resource attributes — `team`, `cost_center`, `environment`, `department`. These flow through OTLP as string key-value pairs with no schema enforcement.

**Risks:**

- **PII in label values:** Nothing prevents `team=jane.doe@company.com` or `cost_center=Project Narwhal (Confidential)`. Thermal cannot validate semantic content, but it can enforce length limits (256 chars), character class restrictions (alphanumeric + hyphen + underscore + dot), and emit warnings for values that match email/path patterns.
- **Auto-discovered attributes:** The OTEL SDK will, by default, attach `host.name`, `os.type`, `process.pid`, `process.executable.name`, and `service.instance.id`. Several of these are **Confidential** per Section 1. Thermal must explicitly construct its OTEL resource with a curated allowlist rather than relying on SDK auto-detection.
- **Attribute cardinality explosion:** Unbounded label values (timestamps, UUIDs, PIDs as labels) can overwhelm OTEL backends and inflate storage costs. Enforce a maximum of 20 custom labels with a combined value size cap of 4KB.

**Finding:** Thermal must use a **closed attribute set** — only explicitly listed attributes are emitted. The OTEL SDK's automatic resource detection must be disabled. The default attribute set should be: `service.name=thermal`, `service.version`, `os.type`, `host.arch`. Everything else (hostname, username, team labels) requires explicit opt-in in the config file.

## 5. Compliance Frameworks

### SOC 2 Type II
OTEL export creates a new "data flow to third-party systems" that must appear in the system description. The CC6.1 (logical access) and CC6.7 (data transmission) controls require documentation of: what data leaves, where it goes, how it's encrypted in transit, and who authorized the destination. Thermal must ship a **data flow diagram** showing: collector sources -> OTEL serialization -> TLS -> customer OTEL collector. The diagram must distinguish what is and is not emitted.

### ISO 27001
Annex A.13 (communications security) requires documented network controls. The OTEL endpoint constitutes a new external interface. The risk assessment must cover endpoint validation, transport encryption, and credential management. Thermal should provide a pre-filled **Statement of Applicability (SoA) fragment** covering relevant controls.

### GDPR
If a European developer's machine hostname is `firstname.lastname-mbp` or their OS username is their real name, these are personal data under GDPR Art. 4(1). Emitting them to an OTEL collector (even within the same company) constitutes processing. **Thermal must not emit hostname or username by default.** If opted in, the customer's DPO must be informed. Thermal should ship a **Privacy Impact Assessment (PIA) template** that enterprise security teams can complete, covering: data subjects (developers), data elements, legal basis (legitimate interest or consent), retention, and processor relationships.

### Required Documentation for Enterprise Approval
1. Data flow diagram (source -> serialization -> transport -> destination)
2. Complete attribute inventory with classifications (this document's Section 1, formalized)
3. PIA template pre-filled with Thermal-specific data elements
4. Security questionnaire answers (SIG Lite / CAIQ format) covering the OTEL feature
5. Incident response runbook for credential compromise (leaked OTEL auth token)

## 6. Configuration Security

The TOML config at `~/.config/coolant/config.toml` will contain:

```toml
[otel]
endpoint = "https://otel-collector.corp.example.com:4317"
# auth_header = "Bearer eyJ..."   # DO NOT PUT TOKENS HERE
```

**Requirements:**

- **No secrets in config files.** Auth tokens must be sourced from environment variables (`COOLANT_OTEL_AUTH_HEADER`) or, preferably, a secret manager integration. The config file should support `auth_header_env = "MY_OTEL_TOKEN"` syntax that reads from the named env var at runtime. If a literal `auth_header` value is detected in the config file, Thermal should log a warning at startup.
- **File permissions.** `config.toml` must be `0600`. Thermal should refuse to load OTEL config from a file with group/world read permissions and log the violation.
- **Config file integrity.** For managed fleet deployments, support an optional `otel.config_hash` field validated against a hash provided via environment variable, so MDM-pushed configs can't be silently modified by local users. This is defense-in-depth against the exfiltration vector in Section 2.
- **Endpoint allowlisting.** Support `otel.allowed_endpoints` (set via MDM/env, not the config file itself) so fleet administrators can restrict which OTEL collector URLs are acceptable. Thermal refuses to export to any endpoint not on the list.

## 7. Threat Model

**Actors:** (1) Malicious insider with local access, (2) Compromised developer machine (malware), (3) Network attacker (MitM), (4) Supply chain compromise of OTEL dependencies.

### Top 3 Risks

**Risk 1: Command string exfiltration via OTEL attributes (Severity: Critical)**
The JSONL event bus carries literal shell commands that routinely contain secrets. If any code path serializes `GateEvent.Command` into an OTEL span attribute or metric label, secrets leave the machine.
*Mitigation:* Command strings, `Original`, and `Rewritten` fields must be architecturally excluded — never referenced in OTEL serialization code. Enforce via code review policy and a unit test that asserts the OTEL exporter's attribute set contains zero Restricted-classified fields.

**Risk 2: Config file redirection to attacker endpoint (Severity: High)**
A compromised machine or malicious insider modifies `config.toml` to redirect metrics. All subsequent data (process trees, agent activity patterns, operational tempo) flows to the attacker.
*Mitigation:* Endpoint allowlisting via env/MDM (not the config file), file permission enforcement at startup, optional config hash validation. Log the active OTEL endpoint at startup so fleet monitoring can detect unexpected destinations.

**Risk 3: OTEL dependency supply chain attack (Severity: High)**
The `go.opentelemetry.io/otel` module tree is large (~30 transitive dependencies). A compromised dependency could exfiltrate data, modify metric values, or open additional network connections.
*Mitigation:* Pin exact dependency versions in `go.sum`. Enable Go module checksum verification (`GONOSUMCHECK` must not be set). Audit new OTEL dependency updates before adoption. Consider vendoring. Run `govulncheck` in CI.

## 8. Recommendations — Gate Criteria for Enterprise Approval

The following must be true before an enterprise infosec team should approve Thermal Enterprise for deployment:

1. **Closed attribute set with classification enforcement.** Every OTEL attribute is explicitly listed in code with its classification tier. A unit test asserts no Restricted or unclassified attributes are present in the export path. No SDK auto-discovery.

2. **Zero plaintext export.** TLS is mandatory. No `--insecure` flag, no `http://` endpoint support. mTLS supported for zero-trust environments.

3. **No secrets in config files.** Auth credentials sourced exclusively from environment variables or secret manager references. Config file permission checks enforced at startup.

4. **Endpoint allowlisting.** Fleet administrators can restrict permitted OTEL destinations via MDM-managed environment variables, independent of the local config file.

5. **Command strings never exported.** The `GateEvent.Command`, `Original`, and `Rewritten` fields are architecturally excluded from OTEL serialization. Enforced by test.

6. **Hostname and username opt-in, not opt-out.** Default resource attributes contain no PII. Explicit config required to add machine-identifying attributes.

7. **Documentation package ships with the binary.** Data flow diagram, attribute inventory, PIA template, and SIG Lite answers are available to enterprise security teams before deployment — not after they ask.

8. **Dependency hygiene.** OTEL dependencies pinned, vendored or checksum-verified, and scanned with `govulncheck` in CI. Dependency update PRs require security review.

9. **Audit logging.** Thermal logs (locally) the active OTEL endpoint, TLS configuration, and attribute set at startup. Any config validation failure (permissions, endpoint allowlist, cert expiry) produces a structured log entry that fleet monitoring can alert on.

10. **Kill switch.** A single environment variable (`COOLANT_OTEL_DISABLE=1`) immediately and completely disables all OTEL export, regardless of config file contents. This allows incident response to halt data flow fleet-wide via MDM push without modifying config files.
