package upgrader

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestURLForPlatform(t *testing.T) {
	const base = "https://github.com/todd-w-shaffer/coolant/releases/latest/download"
	cases := []struct {
		goos, goarch, want string
	}{
		{"darwin", "arm64", base + "/thermo-darwin-arm64"},
		{"darwin", "amd64", base + "/thermo-darwin-amd64"},
		{"linux", "amd64", ""},
		{"darwin", "386", ""},
	}
	for _, c := range cases {
		if got := URLForPlatform(c.goos, c.goarch); got != c.want {
			t.Errorf("URLForPlatform(%s,%s) = %q, want %q", c.goos, c.goarch, got, c.want)
		}
	}
}

func TestRunReplacesTargetWithDownloadedBytes(t *testing.T) {
	payload := []byte("#!/bin/sh\necho hello from new binary\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(payload)
	}))
	defer srv.Close()

	dir := t.TempDir()
	target := filepath.Join(dir, "thermo")
	if err := os.WriteFile(target, []byte("old binary"), 0755); err != nil {
		t.Fatal(err)
	}

	err := Run(Config{
		URL:        srv.URL,
		TargetPath: target,
		Stdout:     io.Discard,
		Stderr:     io.Discard,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Errorf("target contents = %q, want %q", got, payload)
	}
}

func TestRunPreservesExecutableBit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("new"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	target := filepath.Join(dir, "thermo")
	if err := os.WriteFile(target, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}

	if err := Run(Config{URL: srv.URL, TargetPath: target, Stdout: io.Discard, Stderr: io.Discard}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0100 == 0 {
		t.Errorf("target is not executable after upgrade: mode = %v", info.Mode())
	}
}

func TestRunHTTPErrorLeavesTargetUntouched(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dir := t.TempDir()
	target := filepath.Join(dir, "thermo")
	original := []byte("original binary")
	if err := os.WriteFile(target, original, 0755); err != nil {
		t.Fatal(err)
	}

	err := Run(Config{URL: srv.URL, TargetPath: target, Stdout: io.Discard, Stderr: io.Discard})
	if err == nil {
		t.Fatal("expected error from HTTP 404, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error should mention HTTP status, got %q", err.Error())
	}

	got, _ := os.ReadFile(target)
	if string(got) != string(original) {
		t.Errorf("target was clobbered on HTTP failure: got %q, want %q", got, original)
	}
}

func TestRunNoTempFilesLeftBehind(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	dir := t.TempDir()
	target := filepath.Join(dir, "thermo")
	os.WriteFile(target, []byte("old"), 0755)

	_ = Run(Config{URL: srv.URL, TargetPath: target, Stdout: io.Discard, Stderr: io.Discard})

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), "thermo-upgrade-") {
			t.Errorf("temp file leaked: %s", e.Name())
		}
	}
}

func TestRunInvalidatesVersionCache(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("new"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	target := filepath.Join(dir, "thermo")
	cache := filepath.Join(dir, "latest-version")
	os.WriteFile(target, []byte("old"), 0755)
	os.WriteFile(cache, []byte("0.5.0"), 0644)

	if err := Run(Config{URL: srv.URL, TargetPath: target, CachePath: cache, Stdout: io.Discard, Stderr: io.Discard}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if _, err := os.Stat(cache); !os.IsNotExist(err) {
		t.Errorf("version cache should be removed after upgrade; stat err = %v", err)
	}
}

func TestRunMissingCacheIsNotAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("new"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	target := filepath.Join(dir, "thermo")
	cache := filepath.Join(dir, "absent-cache")
	os.WriteFile(target, []byte("old"), 0755)

	if err := Run(Config{URL: srv.URL, TargetPath: target, CachePath: cache, Stdout: io.Discard, Stderr: io.Discard}); err != nil {
		t.Errorf("absent cache should not fail upgrade: %v", err)
	}
}

func TestRunFailsOnEmptyDownload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 200 OK with zero bytes — guards against a CDN edge case
		// silently shipping an empty binary that bricks thermo.
	}))
	defer srv.Close()

	dir := t.TempDir()
	target := filepath.Join(dir, "thermo")
	original := []byte("original")
	os.WriteFile(target, original, 0755)

	err := Run(Config{URL: srv.URL, TargetPath: target, Stdout: io.Discard, Stderr: io.Discard})
	if err == nil {
		t.Fatal("expected error on empty download, got nil")
	}

	got, _ := os.ReadFile(target)
	if string(got) != string(original) {
		t.Errorf("target was clobbered by empty download: %q", got)
	}
}
