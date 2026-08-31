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
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "finst-updater")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to GitHub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		fmt.Printf("✓ finst %s is on the latest build (no remote releases published yet).\n", currentVersion)
		return nil
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub API returned status %s", resp.Status)
	}

	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return fmt.Errorf("failed to parse release info: %w", err)
	}

	latestTag := strings.TrimSpace(rel.TagName)
	if latestTag == "" {
		return fmt.Errorf("no valid tag found in latest release")
	}

	if latestTag == currentVersion {
		fmt.Printf("✓ finst is already up to date (%s).\n", currentVersion)
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

	dlResp, err := client.Do(dlReq)
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
