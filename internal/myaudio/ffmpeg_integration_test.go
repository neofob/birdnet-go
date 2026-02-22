package myaudio

import (
	"context"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tphakala/birdnet-go/internal/errors"
)

func TestGetAudioDurationIntegration(t *testing.T) {
	// Skip if sox is not available
	if !isSoxAvailable() {
		t.Skip("sox not available, skipping integration test")
	}

	// Look for a real audio file in clips directory
	clipsDir := filepath.Join("..", "..", "clips")
	var testFile string

	// Try to find any audio file in a format sox supports natively
	// Note: sox does not support m4a/aac - those require ffmpeg
	err := filepath.WalkDir(clipsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Return the error to stop walking, not nil
			return err
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".mp3" || ext == ".wav" || ext == ".flac" || ext == ".ogg" {
			testFile = path
			return fs.SkipAll // Stop walking once we find a file
		}
		return nil
	})

	// Check if walk failed (but ignore SkipAll which is expected)
	if err != nil && !errors.Is(err, fs.SkipAll) {
		t.Logf("Warning: Error walking clips directory: %v", err)
	}

	if testFile == "" {
		t.Skip("No audio files found in clips directory, skipping integration test")
	}

	t.Logf("Testing with file: %s", testFile)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)

	start := time.Now()
	duration, err := GetAudioDuration(ctx, testFile)
	elapsed := time.Since(start)

	require.NoError(t, err, "GetAudioDuration() failed")

	t.Logf("Duration: %.2f seconds", duration)
	t.Logf("Time taken: %v", elapsed)

	// Basic sanity checks
	assert.Positive(t, duration, "Duration should be positive")
	assert.LessOrEqual(t, duration, 3600.0,
		"Duration seems unreasonably long (more than 1 hour for test clips)")

	// Performance check - should be fast
	if elapsed > 100*time.Millisecond {
		t.Logf("Warning: GetAudioDuration took longer than expected: %v", elapsed)
	}
}
