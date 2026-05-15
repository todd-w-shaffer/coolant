package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/toddwshaffer/coolant/thermal/internal/updater"
	"github.com/toddwshaffer/coolant/thermal/internal/upgrader"
	"github.com/toddwshaffer/coolant/thermal/internal/version"
)

const (
	upgradeExitOK    = 0
	upgradeExitError = 1

	// Both spellings are user-facing aliases; the dispatch switch in
	// main.go references these constants directly.
	upgradeVerb = "upgrade"
	upgradeFlag = "--upgrade"
)

// runUpgrade pulls the latest thermo binary from GitHub Releases and
// atomically replaces the running executable. Statusline ships
// separately as a bash artifact; install.sh --upgrade refreshes both.
func runUpgrade(stdout, stderr io.Writer) int {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(stderr, "upgrade: locate running binary: %v\n", err)
		return upgradeExitError
	}

	fmt.Fprintf(stdout, "thermo: upgrading %s (%s)...\n", exe, version.Version)

	cachePath := coolantTmpPath(updater.CacheFilename)

	if err := upgrader.Run(upgrader.Config{
		TargetPath: exe,
		CachePath:  cachePath,
		Stdout:     stdout,
		Stderr:     stderr,
	}); err != nil {
		fmt.Fprintf(stderr, "upgrade: %v\n", err)
		return upgradeExitError
	}

	printUpgradeResult(stdout, version.Version, readNewVersion(exe))
	return upgradeExitOK
}

// printUpgradeResult renders the post-upgrade message — either a
// version transition or a same-version "already current" note, with
// the trailing reminder that the statusline ships separately.
func printUpgradeResult(stdout io.Writer, oldVersion, newVersion string) {
	if newVersion == oldVersion {
		fmt.Fprintf(stdout, "thermo: %s (already current)\n", newVersion)
	} else {
		fmt.Fprintf(stdout, "thermo: %s → %s\n", oldVersion, newVersion)
	}
	fmt.Fprintln(stdout, "(statusline ships separately; run install.sh --upgrade to refresh both.)")
}

// readNewVersion shells out to the just-replaced binary so we surface
// the actual new version string, not the one our running process was
// linked with. Bounded by a short timeout so an unsigned-binary
// Gatekeeper prompt or first-launch loader stall can't hang upgrade.
func readNewVersion(exe string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, exe, "--version").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}
