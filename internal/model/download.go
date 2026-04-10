package model

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"syscall"
	"time"

	"github.com/schollz/progressbar/v3"
)

// Download fetches a file from url to destPath with HTTP Range resume support.
// If a .part file exists, the download resumes from where it left off.
// The w parameter receives progress output (use os.Stderr in production, io.Discard in tests).
// If expectedHash is empty, SHA256 verification is skipped.
func Download(url, destPath string, expectedSize int64, expectedHash string, w io.Writer) error {
	partPath := destPath + ".part"

	// Check for existing partial download
	var offset int64
	if info, err := os.Stat(partPath); err == nil {
		offset = info.Size()
	}

	// Check disk space (warn only, do not fail)
	checkDiskSpace(destPath, expectedSize, w)

	// Create HTTP request with Range header for resume
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}

	client := &http.Client{
		Timeout: 0, // No timeout for large downloads
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	// Handle response status
	switch resp.StatusCode {
	case http.StatusOK:
		// Server sent full file -- truncate .part and start fresh
		offset = 0
	case http.StatusPartialContent:
		// Resume worked, append from offset
	default:
		return fmt.Errorf("unexpected HTTP status: %d", resp.StatusCode)
	}

	// Open .part file for writing
	flags := os.O_CREATE | os.O_WRONLY
	if offset > 0 {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(partPath, flags, 0644)
	if err != nil {
		return fmt.Errorf("opening part file: %w", err)
	}

	// Set up progress bar
	bar := progressbar.NewOptions64(
		expectedSize,
		progressbar.OptionSetDescription("Downloading"),
		progressbar.OptionSetWriter(w),
		progressbar.OptionShowBytes(true),
		progressbar.OptionShowCount(),
		progressbar.OptionSetWidth(40),
		progressbar.OptionThrottle(100*time.Millisecond),
		progressbar.OptionOnCompletion(func() { fmt.Fprintln(w) }),
	)
	_ = bar.Set64(offset) // Start from resume point

	// Stream download to file and progress bar
	_, err = io.Copy(io.MultiWriter(f, bar), resp.Body)
	closeErr := f.Close()
	if err != nil {
		// .part file preserved for future resume
		return fmt.Errorf("writing data: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("closing part file: %w", closeErr)
	}

	// Rename .part to final destination
	if err := os.Rename(partPath, destPath); err != nil {
		return fmt.Errorf("renaming part file: %w", err)
	}

	// Verify SHA256 after download (skip if hash not configured)
	if expectedHash != "" {
		match, err := VerifySHA256(destPath, expectedHash)
		if err != nil {
			return fmt.Errorf("verification failed: %w", err)
		}
		if !match {
			_ = os.Remove(destPath)
			return fmt.Errorf("SHA256 mismatch: file may be corrupted or model was updated upstream")
		}
	}

	return nil
}

// DownloadModel downloads the model specified in cfg to the cache location.
func DownloadModel(cfg *ModelConfig, w io.Writer) error {
	if cfg.EffectiveModelURL() == "" {
		return fmt.Errorf("model_url is required in config.json for download")
	}
	_, err := EnsureCacheDir()
	if err != nil {
		return err
	}
	return Download(cfg.EffectiveModelURL(), cfg.EffectiveModelPath(), cfg.EffectiveModelSize(), cfg.EffectiveModelHash(), w)
}

// checkDiskSpace warns (but does not fail) if free disk space is insufficient.
func checkDiskSpace(destPath string, requiredBytes int64, w io.Writer) {
	// Use parent directory for statfs (dest may not exist yet)
	dir := destPath
	for {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			break
		}
		parent := dir[:max(0, len(dir)-1)]
		if parent == dir {
			return // cannot determine
		}
		dir = parent
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs(dir, &stat); err != nil {
		return // cannot determine, skip warning
	}

	freeBytes := int64(stat.Bavail) * int64(stat.Bsize)
	buffer := int64(1_000_000_000) // 1GB buffer
	if freeBytes < requiredBytes+buffer {
		fmt.Fprintf(w, "Warning: low disk space. Need ~%dGB, have ~%dGB free.\n",
			requiredBytes/(1024*1024*1024), freeBytes/(1024*1024*1024))
	}
}
