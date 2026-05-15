package updater

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCacheFilenameLocksBashContract(t *testing.T) {
	// claude-statusline/statusline.sh and scripts/upgrade.sh hard-code
	// "latest-version" — renaming the Go constant compiles fine but
	// silently desynchronizes both bash readers. Lock the value.
	if CacheFilename != "latest-version" {
		t.Errorf("CacheFilename = %q, want %q (bash readers will break on rename)", CacheFilename, "latest-version")
	}
}

func TestDevVersionShortCircuits(t *testing.T) {
	_, avail := Check("dev", "/nonexistent", 86400)
	if avail {
		t.Error("dev builds should never report updates available")
	}
}

func TestCacheHitReturnsWithoutNetwork(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, "version-cache")
	if err := os.WriteFile(cache, []byte("0.5.0\n"), 0644); err != nil {
		t.Fatal(err)
	}

	latest, avail := Check("0.4.0", cache, 86400)
	if !avail {
		t.Error("expected update available")
	}
	if latest != "0.5.0" {
		t.Errorf("latest = %q, want %q", latest, "0.5.0")
	}
}

func TestCacheHitSameVersionNotAvailable(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, "version-cache")
	if err := os.WriteFile(cache, []byte("0.4.0\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, avail := Check("0.4.0", cache, 86400)
	if avail {
		t.Error("same version should not report update available")
	}
}

func TestCacheMissFetchesFromServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("0.6.0\n"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	cache := filepath.Join(dir, "version-cache")

	latest, avail := CheckWithURL("0.4.0", cache, 0, srv.URL)
	if !avail {
		t.Error("expected update available")
	}
	if latest != "0.6.0" {
		t.Errorf("latest = %q, want %q", latest, "0.6.0")
	}

	data, err := os.ReadFile(cache)
	if err != nil {
		t.Fatalf("cache file not written: %v", err)
	}
	if string(data) != "0.6.0" {
		t.Errorf("cache contents = %q, want %q", string(data), "0.6.0")
	}
}

func TestStaleCacheRefetches(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("0.7.0\n"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	cache := filepath.Join(dir, "version-cache")
	if err := os.WriteFile(cache, []byte("0.5.0"), 0644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	os.Chtimes(cache, old, old)

	latest, avail := CheckWithURL("0.4.0", cache, 86400, srv.URL)
	if !avail {
		t.Error("expected update available after stale cache refresh")
	}
	if latest != "0.7.0" {
		t.Errorf("latest = %q, want %q", latest, "0.7.0")
	}
}

func TestCacheOlderThanCurrentNotAvailable(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, "version-cache")
	if err := os.WriteFile(cache, []byte("0.3.0\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, avail := Check("0.4.0", cache, 86400)
	if avail {
		t.Error("cache older than current should not report update available")
	}
}

func TestSemverCompare(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"0.4.0", "0.5.0", -1},
		{"0.5.0", "0.5.0", 0},
		{"1.0.0", "0.9.0", 1},
		{"0.10.0", "0.9.0", 1},
		{"1.0.0", "0.99.99", 1},
	}
	for _, tt := range tests {
		got := compareSemver(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("compareSemver(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestMalformedVersionHandledGracefully(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, "version-cache")
	if err := os.WriteFile(cache, []byte("not-a-version\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, avail := Check("0.4.0", cache, 86400)
	if avail {
		t.Error("malformed version should not report update available")
	}
}

func TestNon200ResponseHandledGracefully(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("404: Not Found"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	cache := filepath.Join(dir, "version-cache")

	_, avail := CheckWithURL("0.4.0", cache, 0, srv.URL)
	if avail {
		t.Error("404 response should not report update available")
	}
	if _, err := os.Stat(cache); err == nil {
		t.Error("cache file should not be written on non-200 response")
	}
}

func TestNetworkErrorReturnsGracefully(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, "version-cache")

	_, avail := CheckWithURL("0.4.0", cache, 0, "http://127.0.0.1:1")
	if avail {
		t.Error("network error should not report update available")
	}
}
