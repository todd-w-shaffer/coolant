package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestUpgradeCommandSpellings(t *testing.T) {
	// Both spellings dispatch to runUpgrade; renaming either silently
	// breaks the user-facing CLI surface documented in CLAUDE.md.
	if upgradeVerb != "upgrade" {
		t.Errorf("upgradeVerb = %q, want %q", upgradeVerb, "upgrade")
	}
	if upgradeFlag != "--upgrade" {
		t.Errorf("upgradeFlag = %q, want %q", upgradeFlag, "--upgrade")
	}
}

func TestPrintUpgradeResultVersionTransition(t *testing.T) {
	var buf bytes.Buffer
	printUpgradeResult(&buf, "v0.5.0", "v0.6.0")
	out := buf.String()
	if !strings.Contains(out, "v0.5.0 → v0.6.0") {
		t.Errorf("missing transition arrow line; got:\n%s", out)
	}
	if strings.Contains(out, "already current") {
		t.Errorf("unexpected 'already current' when versions differ; got:\n%s", out)
	}
	if !strings.Contains(out, "install.sh --upgrade") {
		t.Errorf("missing statusline-ships-separately tip; got:\n%s", out)
	}
}

func TestPrintUpgradeResultSameVersion(t *testing.T) {
	var buf bytes.Buffer
	printUpgradeResult(&buf, "v0.6.0", "v0.6.0")
	out := buf.String()
	if !strings.Contains(out, "v0.6.0 (already current)") {
		t.Errorf("missing 'already current' line; got:\n%s", out)
	}
	if strings.Contains(out, "→") {
		t.Errorf("unexpected transition arrow when versions match; got:\n%s", out)
	}
	if !strings.Contains(out, "install.sh --upgrade") {
		t.Errorf("missing statusline-ships-separately tip; got:\n%s", out)
	}
}

func TestPrintUpgradeResultUnknownNewVersion(t *testing.T) {
	// readNewVersion returns "unknown" when the freshly-installed
	// binary's --version invocation fails or times out. We still
	// render the transition rather than swallow it as "already current".
	var buf bytes.Buffer
	printUpgradeResult(&buf, "v0.6.0", "unknown")
	out := buf.String()
	if !strings.Contains(out, "v0.6.0 → unknown") {
		t.Errorf("expected 'v0.6.0 → unknown'; got:\n%s", out)
	}
}
