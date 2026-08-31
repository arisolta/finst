package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type Target struct {
	GOOS   string
	GOARCH string
	IsZip  bool
}

func main() {
	targets := []Target{
		{GOOS: "darwin", GOARCH: "arm64", IsZip: false},
		{GOOS: "darwin", GOARCH: "amd64", IsZip: false},
		{GOOS: "linux", GOARCH: "amd64", IsZip: false},
		{GOOS: "linux", GOARCH: "arm64", IsZip: false},
		{GOOS: "windows", GOARCH: "amd64", IsZip: true},
	}

	distDir := "dist"
	_ = os.RemoveAll(distDir)
	if err := os.MkdirAll(distDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating dist: %v\n", err)
		os.Exit(1)
	}

	var checksums []string

	for _, t := range targets {
		binName := "finst"
		if t.GOOS == "windows" {
			binName = "finst.exe"
		}
		binPath := filepath.Join(distDir, binName)

		fmt.Printf("==> Compiling for %s/%s...\n", t.GOOS, t.GOARCH)
		cmd := exec.Command("go", "build", "-ldflags=-s -w", "-o", binPath, "./cmd/finst")
		cmd.Env = append(os.Environ(),
			"CGO_ENABLED=0",
			"GOOS="+t.GOOS,
			"GOARCH="+t.GOARCH,
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Build failed for %s/%s: %v\n", t.GOOS, t.GOARCH, err)
			os.Exit(1)
		}

		binData, err := os.ReadFile(binPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read binary: %v\n", err)
			os.Exit(1)
		}

		archiveName := fmt.Sprintf("finst_%s_%s", t.GOOS, t.GOARCH)
		var archiveFile string

		if t.IsZip {
			archiveFile = filepath.Join(distDir, archiveName+".zip")
			f, err := os.Create(archiveFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to create zip: %v\n", err)
				os.Exit(1)
			}
			zw := zip.NewWriter(f)
			w, err := zw.Create(binName)
			if err != nil {
				f.Close()
				os.Exit(1)
			}
			_, _ = w.Write(binData)
			_ = zw.Close()
			_ = f.Close()
		} else {
			archiveFile = filepath.Join(distDir, archiveName+".tar.gz")
			f, err := os.Create(archiveFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to create tar.gz: %v\n", err)
				os.Exit(1)
			}
			gw := gzip.NewWriter(f)
			tw := tar.NewWriter(gw)
			header := &tar.Header{
				Name: binName,
				Mode: 0755,
				Size: int64(len(binData)),
			}
			_ = tw.WriteHeader(header)
			_, _ = tw.Write(binData)
			_ = tw.Close()
			_ = gw.Close()
			_ = f.Close()
		}

		_ = os.Remove(binPath)

		// Calculate SHA256
		arcData, _ := os.ReadFile(archiveFile)
		sum := sha256.Sum256(arcData)
		sumHex := hex.EncodeToString(sum[:])
		checksums = append(checksums, fmt.Sprintf("%s  %s", sumHex, filepath.Base(archiveFile)))
		fmt.Printf("✓ Created %s (SHA256: %s)\n", filepath.Base(archiveFile), sumHex[:12])
	}

	checksumFile := filepath.Join(distDir, "checksums.txt")
	checksumContent := ""
	for _, c := range checksums {
		checksumContent += c + "\n"
	}
	_ = os.WriteFile(checksumFile, []byte(checksumContent), 0644)
	fmt.Println("✓ Generated checksums.txt")
}
