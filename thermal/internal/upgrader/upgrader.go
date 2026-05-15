// Package upgrader self-replaces the running thermo binary with the
// latest GitHub Release artifact. Mirrors scripts/upgrade.sh's binary
// path; statusline (a separate bash artifact) is left to install.sh.
//
// Atomicity: download → temp file in target's directory → chmod →
// os.Rename over target. Same-directory rename guarantees an atomic
// swap; cross-device renames would not. macOS allows renaming over a
// running executable — already-loaded text pages stay valid for the
// running process, while the next launch picks up the new binary.
package upgrader

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// releaseURLBase mirrors the RELEASE_URL_BASE that scripts/upgrade.sh
// builds against the same repo — keep in sync on any repo rename.
const releaseURLBase = "https://github.com/todd-w-shaffer/coolant/releases/latest/download"

// Config bounds Run's side effects so tests can mock the network and
// target paths. Production callers leave URL/TargetPath empty to pick
// up runtime defaults.
type Config struct {
	// URL is the binary download URL. Empty = derive from
	// runtime.GOOS / runtime.GOARCH via URLForPlatform.
	URL string

	// TargetPath is the executable to replace. Empty = os.Executable().
	TargetPath string

	// CachePath is the version-cache file to invalidate on success.
	// Empty = skip invalidation. Missing file is not an error.
	CachePath string

	// HTTPTimeout caps the download. Defaults to 60s.
	HTTPTimeout time.Duration

	Stdout, Stderr io.Writer
}

// URLForPlatform returns the GitHub release asset URL for the given
// runtime os/arch, or "" if no prebuilt binary ships for that combo.
// The release matrix is darwin/{arm64,amd64} per .github/workflows/
// auto-release.yml.
func URLForPlatform(goos, goarch string) string {
	if goos != "darwin" {
		return ""
	}
	switch goarch {
	case "arm64", "amd64":
		return fmt.Sprintf("%s/thermo-%s-%s", releaseURLBase, goos, goarch)
	}
	return ""
}

// Run downloads the latest thermo binary and atomically replaces the
// target executable. Returns any error encountered; on error the
// target is guaranteed to be untouched (download to a sibling temp
// file, swap on success only).
func Run(cfg Config) error {
	if cfg.URL == "" {
		cfg.URL = URLForPlatform(runtime.GOOS, runtime.GOARCH)
	}
	if cfg.URL == "" {
		return fmt.Errorf("upgrade not supported on %s/%s — only darwin/arm64 and darwin/amd64 ship prebuilt binaries", runtime.GOOS, runtime.GOARCH)
	}

	if cfg.TargetPath == "" {
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("locate running binary: %w", err)
		}
		cfg.TargetPath = exe
	}

	if cfg.HTTPTimeout == 0 {
		cfg.HTTPTimeout = 60 * time.Second
	}

	// Sibling temp file so os.Rename is a same-device atomic swap.
	dir := filepath.Dir(cfg.TargetPath)
	tmp, err := os.CreateTemp(dir, "thermo-upgrade-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	// On the success path, os.Rename moves tmpPath away — the deferred
	// Remove then no-ops on ENOENT, and the deferred Close is a safe
	// double-close. On every error path we just return; cleanup runs.
	defer os.Remove(tmpPath)
	defer tmp.Close()

	client := &http.Client{Timeout: cfg.HTTPTimeout}
	resp, err := client.Get(cfg.URL)
	if err != nil {
		return fmt.Errorf("download %s: %w", cfg.URL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", cfg.URL, resp.StatusCode)
	}

	n, err := io.Copy(tmp, resp.Body)
	if err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("download %s returned zero bytes", cfg.URL)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	// Preserve target's existing mode + ensure exec bit. Falls through
	// to 0755 if target doesn't exist yet (first install scenario).
	mode := os.FileMode(0755)
	if info, err := os.Stat(cfg.TargetPath); err == nil {
		mode = info.Mode().Perm() | 0100
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}

	if err := os.Rename(tmpPath, cfg.TargetPath); err != nil {
		return fmt.Errorf("replace %s: %w", cfg.TargetPath, err)
	}

	if cfg.CachePath != "" {
		if err := os.Remove(cfg.CachePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(cfg.Stderr, "warning: could not clear version cache %s: %v\n", cfg.CachePath, err)
		}
	}

	return nil
}
