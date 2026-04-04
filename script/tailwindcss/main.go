package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "tailwindcss: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	root, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("finding repo root: %w", err)
	}

	version, err := readToolVersion(filepath.Join(root, ".tool-versions"), "tailwindcss")
	if err != nil {
		return fmt.Errorf("reading tailwindcss version: %w", err)
	}

	binPath := filepath.Join(root, "bin", fmt.Sprintf("tailwindcss-v%s", version))

	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		if err := download(binPath, version); err != nil {
			return fmt.Errorf("downloading tailwindcss v%s: %w", version, err)
		}
	}

	args := append([]string{"tailwindcss"}, os.Args[1:]...)
	return syscall.Exec(binPath, args, os.Environ()) //nolint:gosec // binPath is constructed from repo root + .tool-versions, not user input
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find go.mod in any parent directory")
		}
		dir = parent
	}
}

func readToolVersion(path, tool string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 2 && parts[0] == tool {
			return parts[1], nil
		}
	}
	return "", fmt.Errorf("%q not found in %s", tool, path)
}

func download(binPath, version string) error {
	osName := runtime.GOOS
	arch := runtime.GOARCH

	switch osName {
	case "darwin":
		osName = "macos"
	case "linux":
		// no change
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}

	switch arch {
	case "amd64":
		arch = "x64"
	case "arm64":
		// no change
	default:
		return fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
	}

	url := fmt.Sprintf("https://github.com/tailwindlabs/tailwindcss/releases/download/v%s/tailwindcss-%s-%s", version, osName, arch)
	fmt.Fprintf(os.Stderr, "Downloading tailwindcss v%s from %s\n", version, url)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("creating HTTP request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(binPath), 0o755); err != nil {
		return fmt.Errorf("creating bin directory: %w", err)
	}

	f, err := os.OpenFile(binPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("creating binary file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		os.Remove(binPath)
		return fmt.Errorf("writing binary: %w", err)
	}

	return nil
}
