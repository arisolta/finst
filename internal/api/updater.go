package api

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// CheckAndSelfUpdate checks GitHub releases for the latest version and updates the binary in-place.
func CheckAndSelfUpdate(ctx context.Context, currentVersion string) error {
	repo := "arisolta/finst"
	latestTag, err := fetchLatestReleaseTag(ctx, repo)
	if err != nil {
		return fmt.Errorf("failed to check for updates: %w", err)
	}

	cmp := compareSemver(latestTag, currentVersion)
	if cmp <= 0 {
		if cmp == 0 {
			fmt.Printf("✓ finst is already up to date (%s).\n", currentVersion)
		} else {
			fmt.Printf("✓ finst (%s) is newer than the latest published release (%s).\n", currentVersion, latestTag)
		}
		return nil
	}

	fmt.Printf("==> Updating finst from %s to %s...\n", currentVersion, latestTag)

	osName := runtime.GOOS
	archName := runtime.GOARCH

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to find current executable path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("failed to resolve symlink: %w", err)
	}

	var downloadURL string
	var isZip bool

	if osName == "windows" {
		isZip = true
		downloadURL = fmt.Sprintf("https://github.com/%s/releases/download/%s/finst_windows_%s.zip", repo, latestTag, archName)
	} else {
		downloadURL = fmt.Sprintf("https://github.com/%s/releases/download/%s/finst_%s_%s.tar.gz", repo, latestTag, osName, archName)
	}

	dlReq, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}
	dlReq.Header.Set("User-Agent", "finst-updater")

	dlClient := &http.Client{Timeout: 30 * time.Second}
	dlResp, err := dlClient.Do(dlReq)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer dlResp.Body.Close()

	if dlResp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download release asset (%s): %s", downloadURL, dlResp.Status)
	}

	bodyBytes, err := io.ReadAll(dlResp.Body)
	if err != nil {
		return fmt.Errorf("failed to read downloaded data: %w", err)
	}

	var newBinary []byte
	if isZip {
		zipReader, err := zip.NewReader(bytes.NewReader(bodyBytes), int64(len(bodyBytes)))
		if err != nil {
			return fmt.Errorf("failed to read zip: %w", err)
		}
		for _, f := range zipReader.File {
			if strings.HasSuffix(f.Name, "finst.exe") || f.Name == "finst" {
				rc, err := f.Open()
				if err != nil {
					return err
				}
				newBinary, err = io.ReadAll(rc)
				rc.Close()
				break
			}
		}
	} else {
		gzReader, err := gzip.NewReader(bytes.NewReader(bodyBytes))
		if err != nil {
			return fmt.Errorf("failed to read gzip: %w", err)
		}
		defer gzReader.Close()
		tarReader := tar.NewReader(gzReader)
		for {
			header, err := tarReader.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			if header.Name == "finst" || strings.HasSuffix(header.Name, "/finst") {
				newBinary, err = io.ReadAll(tarReader)
				break
			}
		}
	}

	if len(newBinary) == 0 {
		return fmt.Errorf("could not extract finst binary from download archive")
	}

	// Write to temporary file in same directory then atomically rename
	tmpFile := execPath + ".new"
	if err := os.WriteFile(tmpFile, newBinary, 0755); err != nil {
		return fmt.Errorf("failed to write new binary to %s: %w", tmpFile, err)
	}

	if err := os.Rename(tmpFile, execPath); err != nil {
		oldFile := execPath + ".old"
		_ = os.Rename(execPath, oldFile)
		if err := os.Rename(tmpFile, execPath); err != nil {
			return fmt.Errorf("failed to replace executable %s: %w", execPath, err)
		}
		_ = os.Remove(oldFile)
	}

	fmt.Printf("✓ Successfully updated finst to %s!\n", latestTag)
	return nil
}

func fetchLatestReleaseTag(ctx context.Context, repo string) (string, error) {
	// 1. Primary: GitHub REST API (always returns the exact latest release without CDN caching delays)
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	apiReq, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err == nil {
		apiReq.Header.Set("User-Agent", "finst-updater")
		apiClient := &http.Client{Timeout: 10 * time.Second}
		apiResp, rErr := apiClient.Do(apiReq)
		if rErr == nil {
			defer apiResp.Body.Close()
			if apiResp.StatusCode == http.StatusOK {
				var rel struct {
					TagName string `json:"tag_name"`
				}
				if jsonErr := json.NewDecoder(apiResp.Body).Decode(&rel); jsonErr == nil {
					if tag := strings.TrimSpace(rel.TagName); tag != "" {
						return tag, nil
					}
				}
			}
		}
	}

	// 2. Fallback: GitHub web release redirect (in case API is unavailable or rate-limited)
	webURL := fmt.Sprintf("https://github.com/%s/releases/latest", repo)
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, webURL, nil)
	if err == nil {
		req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; finst-updater)")
		resp, rErr := client.Do(req)
		if rErr == nil {
			defer resp.Body.Close()
			loc := resp.Header.Get("Location")
			if loc != "" && strings.Contains(loc, "/tag/") {
				parts := strings.Split(loc, "/tag/")
				tag := strings.TrimSpace(parts[len(parts)-1])
				tag = strings.Trim(tag, "\r\n/ ")
				if tag != "" {
					return tag, nil
				}
			}
		}
	}

	return "", fmt.Errorf("could not determine latest release version")
}

func parseSemver(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	parts := strings.Split(v, ".")
	var nums []int
	for _, p := range parts {
		var n int
		fmt.Sscanf(p, "%d", &n)
		nums = append(nums, n)
	}
	for len(nums) < 3 {
		nums = append(nums, 0)
	}
	return nums
}

func compareSemver(v1, v2 string) int {
	s1 := parseSemver(v1)
	s2 := parseSemver(v2)
	for i := 0; i < len(s1) && i < len(s2); i++ {
		if s1[i] > s2[i] {
			return 1
		}
		if s1[i] < s2[i] {
			return -1
		}
	}
	return 0
}
