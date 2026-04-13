#!/usr/bin/env python3
"""Rewrite Grafana dashboard colors to the Catppuccin Frappe palette.

Mirrors thermal/internal/theme/frappe.go so terminal + browser feel like
one theme. Three passes, all text-based to preserve the hand-tuned
compact JSON formatting:

  1. Name/hex substitution (named colors → Frappe hex, literal hex
     remaps, continuous-* → thresholds).
  2. Model-keyed overrides — injects byRegexp overrides mapping
     opus/sonnet/haiku/claude-3 families to Frappe accents, for panels
     whose series are keyed by the Prometheus `model` label.
  3. Bare-default fix — sets a sensible color.mode on panels whose
     fieldConfig.defaults has no color block.

Re-runnable: idempotent on already-Frappe input.
"""
from __future__ import annotations

import csv
import json
import re
import sys
from pathlib import Path

NAMED = {
    "red":      "#e78284",
    "green":    "#a6d189",
    "blue":     "#8caaee",
    "yellow":   "#e5c890",
    "orange":   "#ef9f76",
    "purple":   "#ca9ee6",
    "semi-dark-green":   "#81c8be",
    "semi-dark-red":     "#ea999c",
    "semi-dark-blue":    "#85c1dc",
    "semi-dark-yellow":  "#e5c890",
    "semi-dark-orange":  "#ef9f76",
    "semi-dark-purple":  "#ca9ee6",
    "super-light-green":  "#a6d189",
    "super-light-blue":   "#99d1db",
    "super-light-purple": "#babbf1",
    "super-light-red":    "#eebebe",
    "super-light-yellow": "#e5c890",
    "super-light-orange": "#f2d5cf",
    "light-green":  "#a6d189",
    "light-blue":   "#99d1db",
    "dark-green":   "#81c8be",
    "dark-blue":    "#85c1dc",
    "dark-red":     "#e78284",
}

HEX_REMAP = {
    "#FFB86C": "#ef9f76",
    "#EAB839": "#e5c890",
}

CONTINUOUS = {
    "continuous-GrYlRd", "continuous-RdYlGr", "continuous-BlYlRd",
    "continuous-BlPu", "continuous-blues", "continuous-greens",
    "continuous-reds", "continuous-purples", "continuous-YlBl",
    "continuous-YlRd",
}

# Model family → Frappe accent. Regex matched against full series name.
MODEL_OVERRIDES = [
    (".*opus.*",     "#ca9ee6"),  # mauve — premium/heavy
    (".*sonnet.*",   "#babbf1"),  # lavender — balanced
    (".*haiku.*",    "#81c8be"),  # teal — fast/light
    (".*claude-3.*", "#f4b8e4"),  # pink — legacy 3.x fallback
]

# Panels keyed by Prometheus `model` label. (filename, panel id).
MODEL_KEYED_PANELS = {
    ("claude-models.json", 500),
    ("claude-models.json", 501),
    ("claude-models.json", 502),
    ("claude-models.json", 530),
    ("claude-models.json", 531),
    ("claude-spend.json",  20),
    ("claude-spend.json",  30),
}

# Panels whose series are dynamic data labels (repo, organization_id)
# themed via a static name→color CSV under dev/otel/lookups/. One byName
# override per CSV row, skipping `__slot*__` / `__default__` placeholders.
# (filename, panel_id) → csv basename.
CSV_KEYED_PANELS = {
    ("claude-techdebt.json", 301): "repo_colors.csv",
    ("claude-cfo.json",      401): "org_colors.csv",
}


# Panels whose default color mode needs to be set or corrected. Injects
# a color block when defaults has none; replaces the existing mode when
# one is already present with a different value.
BARE_COLOR_PANELS = {
    ("claude-cfo.json",      301): "palette-classic-by-name",
    ("claude-insights.json", 531): "palette-classic-by-name",
    ("claude-spend.json",    41):  "palette-classic-by-name",
    # Model Mix table — every data column has per-column threshold colors
    # via overrides. Default mode is cosmetic here.
    ("claude-models.json",   540): "thresholds",
}


def pass1_text(text: str) -> str:
    for name in sorted(NAMED, key=len, reverse=True):
        text = re.sub(rf'"{re.escape(name)}"', f'"{NAMED[name]}"', text)
    for old, new in HEX_REMAP.items():
        text = text.replace(f'"{old}"', f'"{new}"')
    for mode in CONTINUOUS:
        text = text.replace(f'"mode": "{mode}"', '"mode": "thresholds"')
    return text


def find_panel_span(text: str, panel_id: int) -> tuple[int, int] | None:
    """Locate the outer `{...}` span of the panel object with the given id."""
    for m in re.finditer(rf'"id":\s*{panel_id}\b', text):
        # Walk back to the enclosing `{`
        i = m.start()
        depth = 0
        while i >= 0:
            c = text[i]
            if c == '}':
                depth += 1
            elif c == '{':
                if depth == 0:
                    break
                depth -= 1
            i -= 1
        if i < 0:
            continue
        # Balance forward to find matching `}`
        depth = 0
        in_str = False
        esc = False
        for j in range(i, len(text)):
            c = text[j]
            if esc:
                esc = False
                continue
            if c == '\\':
                esc = True
                continue
            if c == '"':
                in_str = not in_str
                continue
            if in_str:
                continue
            if c == '{':
                depth += 1
            elif c == '}':
                depth -= 1
                if depth == 0:
                    # Sanity check: this block must contain a top-level
                    # `"type": "..."` (panels always do); skip nested matches.
                    block = text[i:j + 1]
                    if '"type":' in block and '"gridPos"' in block:
                        return (i, j + 1)
                    break
    return None


def render_model_overrides(indent: str) -> str:
    lines = []
    for pattern, color in MODEL_OVERRIDES:
        lines.append(
            f'{indent}{{ "matcher": {{ "id": "byRegexp", "options": "{pattern}" }}, '
            f'"properties": [{{ "id": "color", "value": {{ "fixedColor": "{color}", "mode": "fixed" }} }}] }}'
        )
    return ",\n".join(lines)


def _splice_overrides(text: str, start: int, end: int, body: str) -> tuple[str, bool]:
    """Replace the panel's `"overrides": [...]` array body with `body`
    (whether empty or non-empty), or insert a new overrides key after
    the `"defaults": {...}` block when no overrides key exists."""
    block = text[start:end]

    arr_m = re.search(r'"overrides":\s*\[', block)
    if arr_m:
        i = arr_m.end() - 1  # position of the opening `[`
        depth = 0
        in_str = False
        esc = False
        close_idx = None
        for j in range(i, len(block)):
            c = block[j]
            if esc:
                esc = False
                continue
            if c == '\\':
                esc = True
                continue
            if c == '"':
                in_str = not in_str
                continue
            if in_str:
                continue
            if c == '[':
                depth += 1
            elif c == ']':
                depth -= 1
                if depth == 0:
                    close_idx = j
                    break
        if close_idx is None:
            return text, False
        replacement = f'"overrides": [\n{body}\n        ]'
        new_block = block[:arr_m.start()] + replacement + block[close_idx + 1:]
        return text[:start] + new_block + text[end:], True

    defaults_m = re.search(r'"defaults":\s*\{', block)
    if not defaults_m:
        return text, False
    i = defaults_m.end() - 1
    depth = 0
    in_str = False
    esc = False
    close_idx = None
    for j in range(i, len(block)):
        c = block[j]
        if esc:
            esc = False
            continue
        if c == '\\':
            esc = True
            continue
        if c == '"':
            in_str = not in_str
            continue
        if in_str:
            continue
        if c == '{':
            depth += 1
        elif c == '}':
            depth -= 1
            if depth == 0:
                close_idx = j
                break
    if close_idx is None:
        return text, False

    insertion = f',\n        "overrides": [\n{body}\n        ]'
    new_block = block[:close_idx + 1] + insertion + block[close_idx + 1:]
    return text[:start] + new_block + text[end:], True


def inject_model_overrides(text: str, panel_id: int) -> tuple[str, bool]:
    span = find_panel_span(text, panel_id)
    if not span:
        return text, False
    start, end = span
    block = text[start:end]

    # Idempotent: opus override already present.
    if '"byRegexp"' in block and '".*opus.*"' in block:
        return text, False

    return _splice_overrides(text, start, end, render_model_overrides("          "))


def load_csv_colors(csv_path: Path) -> list[tuple[str, str]]:
    rows: list[tuple[str, str]] = []
    with csv_path.open() as f:
        for row in csv.DictReader(f):
            name = (row.get("name") or "").strip()
            color = (row.get("color") or "").strip()
            if not name or not color:
                continue
            if name.startswith("__"):
                continue
            rows.append((name, color))
    return rows


def render_csv_overrides(rows: list[tuple[str, str]], indent: str) -> str:
    lines = []
    for name, color in rows:
        lines.append(
            f'{indent}{{ "matcher": {{ "id": "byName", "options": "{name}" }}, '
            f'"properties": [{{ "id": "color", "value": {{ "fixedColor": "{color}", "mode": "fixed" }} }}] }}'
        )
    return ",\n".join(lines)


def inject_csv_overrides(text: str, panel_id: int, csv_path: Path) -> tuple[str, bool]:
    rows = load_csv_colors(csv_path)
    if not rows:
        return text, False
    span = find_panel_span(text, panel_id)
    if not span:
        return text, False
    start, end = span
    block = text[start:end]

    # Idempotent: every CSV row already present as a byName override.
    if all(f'"id": "byName", "options": "{n}"' in block for n, _ in rows):
        return text, False

    return _splice_overrides(text, start, end, render_csv_overrides(rows, "          "))


def set_default_color_mode(text: str, panel_id: int, mode: str) -> tuple[str, bool]:
    span = find_panel_span(text, panel_id)
    if not span:
        return text, False
    start, end = span
    block = text[start:end]

    defaults_m = re.search(r'"defaults":\s*\{', block)
    if not defaults_m:
        return text, False

    # If defaults already has a color block with "mode", either skip (match)
    # or surgically replace just the mode value (mismatch).
    color_m = re.search(r'"color"\s*:\s*\{\s*"mode"\s*:\s*"([^"]+)"', block)
    if color_m:
        if color_m.group(1) == mode:
            return text, False
        mstart, mend = color_m.span(1)
        new_block = block[:mstart] + mode + block[mend:]
        return text[:start] + new_block + text[end:], True

    # No color block — inject one after the opening `{` of defaults.
    insert_at = defaults_m.end()
    tail = block[insert_at:]
    indent_m = re.match(r'(\s*)', tail)
    indent = indent_m.group(1) if indent_m else "\n          "
    insertion = f'{indent}"color": {{ "mode": "{mode}" }},'
    new_block = block[:insert_at] + insertion + block[insert_at:]
    return text[:start] + new_block + text[end:], True


def process(path: Path) -> list[str]:
    changes = []
    text = path.read_text()

    new_text = pass1_text(text)
    if new_text != text:
        changes.append("text")
    text = new_text

    for fname, pid in MODEL_KEYED_PANELS:
        if fname != path.name:
            continue
        text, ch = inject_model_overrides(text, pid)
        if ch:
            changes.append(f"model#{pid}")

    lookups_root = Path(__file__).resolve().parent.parent / "lookups"
    for (fname, pid), csv_name in CSV_KEYED_PANELS.items():
        if fname != path.name:
            continue
        text, ch = inject_csv_overrides(text, pid, lookups_root / csv_name)
        if ch:
            changes.append(f"csv#{pid}")

    for (fname, pid), mode in BARE_COLOR_PANELS.items():
        if fname != path.name:
            continue
        text, ch = set_default_color_mode(text, pid, mode)
        if ch:
            changes.append(f"bare#{pid}")

    json.loads(text)  # validate
    if changes:
        path.write_text(text)
    return changes


def main():
    root = Path(__file__).resolve().parent.parent / "dashboards"
    files = sorted(root.glob("*.json"))
    if not files:
        sys.exit(f"no dashboards at {root}")
    for path in files:
        changes = process(path)
        tag = ",".join(changes) if changes else "unchanged"
        print(f"{path.name:<28} {tag}")


if __name__ == "__main__":
    main()
