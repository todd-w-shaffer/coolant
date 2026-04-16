package updater

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultURL = "https://raw.githubusercontent.com/todd-w-shaffer/coolant/main/VERSION"

func Check(currentVersion, cachePath string, ttlSeconds int) (string, bool) {
	return CheckWithURL(currentVersion, cachePath, ttlSeconds, defaultURL)
}

func CheckWithURL(currentVersion, cachePath string, ttlSeconds int, url string) (string, bool) {
	if currentVersion == "dev" {
		return "", false
	}

	latest, ok := readFreshCache(cachePath, ttlSeconds)
	if !ok {
		fetched, err := fetch(url)
		if err != nil {
			return "", false
		}
		latest = fetched
		os.WriteFile(cachePath, []byte(latest), 0644)
	}

	if compareSemver(latest, currentVersion) > 0 {
		return latest, true
	}
	return latest, false
}

func readFreshCache(path string, ttlSeconds int) (string, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return "", false
	}
	age := time.Since(info.ModTime())
	if age > time.Duration(ttlSeconds)*time.Second {
		return "", false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	v := strings.TrimSpace(string(data))
	if v == "" {
		return "", false
	}
	return v, true
}

func fetch(url string) (string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(body)), nil
}

func compareSemver(a, b string) int {
	ap := parseSemver(a)
	bp := parseSemver(b)
	if ap == nil || bp == nil {
		return 0
	}
	for i := 0; i < 3; i++ {
		if ap[i] > bp[i] {
			return 1
		}
		if ap[i] < bp[i] {
			return -1
		}
	}
	return 0
}

func parseSemver(s string) []int {
	s = strings.TrimPrefix(s, "v")
	parts := strings.SplitN(s, ".", 3)
	if len(parts) != 3 {
		return nil
	}
	nums := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		nums[i] = n
	}
	return nums
}
