package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// --- image saving ---

func saveIdeaImages(entries []IdeaEntry) ([]string, error) {
	var saved []string
	if err := os.MkdirAll(shared.OutputDir, 0755); err != nil {
		return saved, fmt.Errorf("cannot create output directory: %w", err)
	}
	for _, e := range entries {
		for _, imgURL := range e.ImageURLs {
			if imgURL == "" {
				continue
			}
			name := filepath.Base(imgURL)
			path := filepath.Join(shared.OutputDir, name)
			// Skip if already exists
			if _, err := os.Stat(path); err == nil {
				saved = append(saved, path)
				continue
			}
			data, err := downloadImage(imgURL)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to download %s: %v\n", imgURL, err)
				continue
			}
			if err := os.WriteFile(path, data, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save %s: %v\n", name, err)
				continue
			}
			saved = append(saved, path)
		}
	}
	return saved, nil
}

func localImagePath(remoteURL string) string {
	if remoteURL == "" {
		return ""
	}
	return filepath.Join(shared.OutputDir, filepath.Base(remoteURL))
}

// downloadImage downloads a URL to a byte slice with browser-like headers
// and retry on transient errors (EOF, connection reset).
// Inherits proxy settings from http.DefaultClient (configured by ConfigureDefaultClient).
func downloadImage(url string) ([]byte, error) {
	// Use DefaultClient's transport to inherit proxy configuration;
	// fall back to http.DefaultTransport if DefaultClient was not customized.
	transport := http.DefaultClient.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}

	var lastErr error
	for attempt := range 3 {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}

		// Browser-like User-Agent to avoid CDN blocking
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")
		// Set Referer for Twitter CDN images
		if strings.Contains(url, "twimg.com") || strings.Contains(url, "x.com") {
			req.Header.Set("Referer", "https://x.com/")
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			// Retry on EOF or connection reset -- transient CDN issues
			if strings.Contains(err.Error(), "EOF") || strings.Contains(err.Error(), "connection reset") {
				continue
			}
			return nil, err
		}

		data, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
			continue
		}
		return data, nil
	}
	return nil, fmt.Errorf("failed after 3 attempts: %w", lastErr)
}
