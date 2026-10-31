package main

import (
	"runtime/debug"
	"strings"
	"testing"
)

// TestVersionFlag verifies the spec-mandated "pgnx --version" prints the
// resolved version string and exits 0.
func TestVersionFlag(t *testing.T) {
	out, _, code := runCLI(t, "", "--version")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	want := versionString()
	if !strings.Contains(out, want) {
		t.Errorf("output should contain version %q, got %q", want, out)
	}
}

func buildInfo(version string, settings map[string]string) *debug.BuildInfo {
	bi := &debug.BuildInfo{}
	bi.Main.Version = version
	for k, v := range settings {
		bi.Settings = append(bi.Settings, debug.BuildSetting{Key: k, Value: v})
	}
	return bi
}

func TestFormatVersion(t *testing.T) {
	tests := []struct {
		name                  string
		version, commit, date string
		bi                    *debug.BuildInfo
		want                  string
	}{
		{
			name:    "stamped by ldflags (goreleaser)",
			version: "0.2.0", commit: "abc1234def", date: "2026-10-31T12:00:00Z",
			bi:   buildInfo("(devel)", map[string]string{"vcs.revision": "ignored"}),
			want: "0.2.0 (commit abc1234, built 2026-10-31T12:00:00Z)",
		},
		{
			name:    "go install @version fills version and vcs info",
			version: devVersion,
			bi: buildInfo("v0.2.0", map[string]string{
				"vcs.revision": "0123456789abcdef",
				"vcs.time":     "2026-10-30T08:00:00Z",
			}),
			want: "0.2.0 (commit 0123456, built 2026-10-30T08:00:00Z)",
		},
		{
			name:    "go build in a checkout: devel with vcs info",
			version: devVersion,
			bi: buildInfo("(devel)", map[string]string{
				"vcs.revision": "0123456789abcdef",
				"vcs.modified": "true",
			}),
			want: "dev (commit 0123456-dirty)",
		},
		{
			name:    "go build in a checkout: Go 1.24+ pseudo-version is not a release",
			version: devVersion,
			bi: buildInfo("v0.0.0-20261031150840-6a58b0553719+dirty", map[string]string{
				"vcs.revision": "6a58b0553719aaaa",
				"vcs.time":     "2026-10-31T15:08:40Z",
				"vcs.modified": "true",
			}),
			want: "dev (commit 6a58b05-dirty, built 2026-10-31T15:08:40Z)",
		},
		{
			name:    "no build info at all",
			version: devVersion,
			bi:      nil,
			want:    "dev",
		},
		{
			name:    "ldflags version without commit or date",
			version: "0.3.0",
			bi:      nil,
			want:    "0.3.0",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formatVersion(tc.version, tc.commit, tc.date, tc.bi)
			if got != tc.want {
				t.Errorf("formatVersion() = %q, want %q", got, tc.want)
			}
		})
	}
}
