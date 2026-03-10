package version

import (
	"strings"
	"testing"
)

func TestImageRef(t *testing.T) {
	const registry = "ghcr.io/frostyard/ubuntu-intune"

	tests := []struct {
		version string
		want    string
	}{
		{"dev", registry + ":latest"},
		{"0.4.0", registry + ":v0.4.0"},
		{"v0.4.0", registry + ":v0.4.0"},
		{"1.0.0", registry + ":v1.0.0"},
		{"v1.0.0", registry + ":v1.0.0"},
		{"v0.4.0-2-g98e23e6", registry + ":latest"},
		{"v0.4.0-dirty", registry + ":latest"},
		{"none", registry + ":latest"},
		{"", registry + ":latest"},
		{"v0.4.0-rc1", registry + ":latest"},
		{"0.4.0-beta.1", registry + ":latest"},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			Version = tt.version
			got := ImageRef()
			if got != tt.want {
				t.Errorf("ImageRef() = %q, want %q", got, tt.want)
			}
		})
	}
}

func FuzzImageRef(f *testing.F) {
	f.Add("dev")
	f.Add("v0.4.0")
	f.Add("0.4.0")
	f.Add("v0.4.0-2-g98e23e6")
	f.Add("v0.4.0-dirty")
	f.Add("")
	f.Add("v0.4.0-rc1")
	f.Add("999.999.999")

	f.Fuzz(func(t *testing.T, version string) {
		Version = version
		ref := ImageRef()
		// Must always return a valid image reference starting with the base.
		if !strings.HasPrefix(ref, "ghcr.io/frostyard/ubuntu-intune:") {
			t.Errorf("ImageRef() = %q, missing expected prefix", ref)
		}
	})
}
