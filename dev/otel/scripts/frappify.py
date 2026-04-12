#!/usr/bin/env python3
"""Rewrite Grafana dashboard colors to the Catppuccin Frappe palette.

Mirrors thermal/internal/theme/frappe.go so terminal + browser feel like
one theme. Operates as text substitution to preserve the hand-tuned
compact JSON formatting. Re-runnable: already-Frappe hex passes through.
"""
from __future__ import annotations

import json
import re
import sys
from pathlib import Path

# Named Grafana colors → Frappe hex
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

# Literal hex in-the-wild → Frappe hex
HEX_REMAP = {
    "#FFB86C": "#ef9f76",
    "#EAB839": "#e5c890",
}

# Continuous palettes → thresholds (so Frappe step colors drive the gradient).
CONTINUOUS = {
    "continuous-GrYlRd",
    "continuous-RdYlGr",
    "continuous-BlYlRd",
    "continuous-BlPu",
    "continuous-blues",
    "continuous-greens",
    "continuous-reds",
    "continuous-purples",
    "continuous-YlBl",
    "continuous-YlRd",
}


def frappify(text: str) -> str:
    # fixedColor: "<name>" and color: "<name>" — match quoted names only.
    # Order: longest keys first to avoid e.g. "semi-dark-red" matching "red".
    for name in sorted(NAMED, key=len, reverse=True):
        text = re.sub(rf'"{re.escape(name)}"', f'"{NAMED[name]}"', text)
    for old, new in HEX_REMAP.items():
        text = text.replace(f'"{old}"', f'"{new}"')
    for mode in CONTINUOUS:
        text = text.replace(f'"mode": "{mode}"', '"mode": "thresholds"')
    return text


def main():
    root = Path(__file__).resolve().parent.parent / "dashboards"
    files = sorted(root.glob("*.json"))
    if not files:
        sys.exit(f"no dashboards at {root}")
    for path in files:
        original = path.read_text()
        updated = frappify(original)
        json.loads(updated)  # validate
        if updated != original:
            path.write_text(updated)
            print(f"frappified {path.name}")
        else:
            print(f"unchanged  {path.name}")


if __name__ == "__main__":
    main()
