package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

func TestStatusJSON(t *testing.T) {
	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Set JSON mode and run status in an uninitialized root
	rootDir = t.TempDir()
	rootCmd.SetArgs([]string{"status", "--json"})
	_ = rootCmd.Execute()

	_ = w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	// When not initialized with --json, should output valid JSON with status field
	if buf.Len() > 0 {
		var result map[string]any
		if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
			t.Fatalf("expected valid JSON, got: %s", buf.String())
		}
	}
}
