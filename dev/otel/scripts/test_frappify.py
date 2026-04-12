#!/usr/bin/env python3
"""Sanity tests for frappify.py mutators.

Run: python3 dev/otel/scripts/test_frappify.py
"""
from __future__ import annotations

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import frappify  # noqa: E402

BARE_PANEL = """{
  "panels": [
    {
      "id": 99,
      "type": "timeseries",
      "gridPos": { "h": 1, "w": 1, "x": 0, "y": 0 },
      "fieldConfig": {
        "defaults": {
          "unit": "short"
        },
        "overrides": []
      }
    }
  ]
}"""

WITH_MODE_PANEL = """{
  "panels": [
    {
      "id": 99,
      "type": "timeseries",
      "gridPos": { "h": 1, "w": 1, "x": 0, "y": 0 },
      "fieldConfig": {
        "defaults": {
          "unit": "short",
          "color": { "mode": "palette-classic" }
        },
        "overrides": []
      }
    }
  ]
}"""


def assert_in(needle: str, haystack: str, msg: str) -> None:
    if needle not in haystack:
        raise AssertionError(f"{msg}: {needle!r} not found")


def assert_not_in(needle: str, haystack: str, msg: str) -> None:
    if needle in haystack:
        raise AssertionError(f"{msg}: {needle!r} unexpectedly present")


def test_inject_when_bare():
    out, changed = frappify.set_default_color_mode(BARE_PANEL, 99, "palette-classic-by-name")
    assert changed, "expected injection when color block absent"
    assert_in('"color": { "mode": "palette-classic-by-name" }', out, "injected color block")


def test_idempotent_when_already_set():
    once, _ = frappify.set_default_color_mode(BARE_PANEL, 99, "palette-classic-by-name")
    twice, changed = frappify.set_default_color_mode(once, 99, "palette-classic-by-name")
    assert not changed, "second pass should be a no-op"
    assert once == twice, "second pass must not mutate text"


def test_replace_existing_mode():
    out, changed = frappify.set_default_color_mode(WITH_MODE_PANEL, 99, "palette-classic-by-name")
    assert changed, "expected replacement when mode differs"
    assert_in('"mode": "palette-classic-by-name"', out, "new mode present")
    assert_not_in('"mode": "palette-classic"', out, "old mode removed")
    # Must not end up with duplicate color keys.
    assert out.count('"color"') == 1, "single color key after replacement"


def test_replace_skips_when_matching():
    out, changed = frappify.set_default_color_mode(WITH_MODE_PANEL, 99, "palette-classic")
    assert not changed, "same mode must short-circuit"
    assert out == WITH_MODE_PANEL, "no mutation when mode matches"


import frappify_audit  # noqa: E402


def test_audit_configfromdata_treated_as_themed():
    panel = {
        "id": 401,
        "title": "Cost by Organization",
        "type": "piechart",
        "fieldConfig": {
            "defaults": {"color": {"mode": "palette-classic-by-name"}},
            "overrides": [],
        },
        "transformations": [
            {"id": "configFromData", "options": {"configRefId": "B"}}
        ],
    }
    status, _, _ = frappify_audit.classify(panel)
    assert status == "✅ themed", f"expected themed, got {status}"


def test_audit_without_configfromdata_stays_classic():
    panel = {
        "id": 401,
        "title": "x",
        "type": "piechart",
        "fieldConfig": {
            "defaults": {"color": {"mode": "palette-classic-by-name"}},
            "overrides": [],
        },
    }
    status, _, _ = frappify_audit.classify(panel)
    assert status == "🌈 classic-palette", f"expected classic, got {status}"


def main():
    for name, fn in list(globals().items()):
        if name.startswith("test_") and callable(fn):
            fn()
            print(f"ok  {name}")


if __name__ == "__main__":
    main()
