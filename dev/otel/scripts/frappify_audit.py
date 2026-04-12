#!/usr/bin/env python3
"""Audit dashboard panels for Frappe theming coverage.

Emits a markdown table per dashboard: panel id, title, color mode,
remaining non-Frappe color refs, and a classification so we can decide
how to handle the leftovers (palette-classic, bare defaults, overrides).
"""
from __future__ import annotations

import json
import re
import sys
from pathlib import Path

FRAPPE_HEX = {
    "#f2d5cf", "#eebebe", "#f4b8e4", "#ca9ee6", "#e78284", "#ea999c",
    "#ef9f76", "#e5c890", "#a6d189", "#81c8be", "#99d1db", "#85c1dc",
    "#8caaee", "#babbf1", "#c6d0f5", "#b5bfe2", "#a5adce", "#949cbb",
    "#838ba7", "#737994", "#626880", "#51576d", "#414559", "#303446",
    "#292c3c", "#232634",
}

NAMED_GRAFANA = re.compile(
    r'"(red|green|blue|yellow|orange|purple|'
    r'(?:semi-dark|super-light|light|dark)-(?:red|green|blue|yellow|orange|purple))"'
)
HEX_RE = re.compile(r'"(#[0-9a-fA-F]{6})"')
CLASSIC_MODES = {"palette-classic", "palette-classic-by-name"}


def classify(panel: dict) -> tuple[str, str, list[str]]:
    fc = panel.get("fieldConfig", {}).get("defaults", {})
    color = fc.get("color", {})
    mode = color.get("mode", "(none)")
    overrides = panel.get("fieldConfig", {}).get("overrides", []) or []

    # Collect all color-valued strings in this panel subtree.
    blob = json.dumps(panel)
    non_frappe_hex = sorted(
        {h for h in HEX_RE.findall(blob) if h.lower() not in FRAPPE_HEX}
    )
    named_leftovers = sorted(set(NAMED_GRAFANA.findall(blob)))

    override_modes = []
    frappe_override_hits = 0
    for ov in overrides:
        matcher = ov.get("matcher", {}) or {}
        for prop in ov.get("properties", []) or []:
            if prop.get("id") == "color":
                v = prop.get("value", {}) or {}
                m = v.get("mode")
                fc = (v.get("fixedColor") or "").lower()
                if m and m not in ("fixed",):
                    override_modes.append(m)
                if fc in FRAPPE_HEX:
                    frappe_override_hits += 1
    # If classic-palette default is backstopped by Frappe byRegexp/byName
    # overrides covering common families, treat as themed.
    classic_with_frappe_fallback = (
        mode in CLASSIC_MODES and frappe_override_hits >= 2
    )

    # configFromData transform binds per-series colors from a lookup query.
    # When present, default color.mode is irrelevant — theming is driven
    # entirely by the joined color column.
    transforms = panel.get("transformations", []) or []
    has_config_from_data = any(
        (t or {}).get("id") == "configFromData" for t in transforms
    )

    issues = []
    if mode in CLASSIC_MODES and not classic_with_frappe_fallback and not has_config_from_data:
        issues.append(f"mode={mode}")
    if mode == "(none)":
        issues.append("no color block")
    if named_leftovers:
        issues.append("named=" + ",".join(named_leftovers))
    if non_frappe_hex:
        issues.append("hex=" + ",".join(non_frappe_hex))
    for om in override_modes:
        if om in CLASSIC_MODES:
            issues.append(f"override={om}")

    if not issues:
        status = "✅ themed"
    elif classic_with_frappe_fallback or has_config_from_data:
        status = "✅ themed"
    elif mode in CLASSIC_MODES and not named_leftovers and not non_frappe_hex:
        status = "🌈 classic-palette"
    elif mode == "(none)":
        status = "⚪ default"
    else:
        status = "⚠️ mixed"

    return status, mode, issues


def audit(path: Path) -> str:
    data = json.loads(path.read_text())
    panels = data.get("panels", []) or []
    lines = [f"## {path.name}", ""]
    lines.append("| id | title | status | mode | notes |")
    lines.append("|---|---|---|---|---|")

    def walk_panel(p, prefix=""):
        status, mode, issues = classify(p)
        title = (prefix + (p.get("title") or "")).replace("|", "\\|")
        pid = p.get("id", "?")
        notes = "; ".join(issues) or "—"
        lines.append(f"| {pid} | {title} | {status} | `{mode}` | {notes} |")
        for sub in p.get("panels", []) or []:
            walk_panel(sub, prefix=f"{title} › ")

    for p in panels:
        walk_panel(p)
    return "\n".join(lines) + "\n"


def main():
    root = Path(__file__).resolve().parent.parent / "dashboards"
    out = ["# Frappe theming audit", ""]
    totals = {"✅ themed": 0, "🌈 classic-palette": 0, "⚪ default": 0, "⚠️ mixed": 0}
    for path in sorted(root.glob("*.json")):
        section = audit(path)
        out.append(section)
        for status in totals:
            totals[status] += section.count(f"| {status} |")
    out.insert(2, "## Totals\n")
    for k, v in totals.items():
        out.insert(3, f"- {k}: {v}")
        out.insert(4, "")
    out.insert(len(totals) + 3, "")
    report = "\n".join(out)
    report_path = Path(__file__).resolve().parent.parent / "frappe-audit.md"
    report_path.write_text(report)
    print(report)
    print(f"\n(written to {report_path})", file=sys.stderr)


if __name__ == "__main__":
    main()
