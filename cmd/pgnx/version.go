package main

import (
	"fmt"
	"runtime/debug"
	"strings"
)

// devVersion is the version reported when nothing stamped one at build
// time: neither -ldflags -X nor module build info.
const devVersion = "dev"

// Build-time metadata. GoReleaser (via .goreleaser.yaml) stamps these with
// -ldflags "-X main.version=... -X main.commit=... -X main.date=...". A
// plain "go build" or "go install" leaves them at their zero values, and
// versionString falls back to the module build info Go records itself.
var (
	version = devVersion
	commit  = ""
	date    = ""
)

// versionString resolves the version reported by "pgnx --version" from the
// ldflags-stamped variables, falling back to runtime/debug build info.
func versionString() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		bi = nil
	}
	return formatVersion(version, commit, date, bi)
}

// formatVersion renders "VERSION (commit SHORT[-dirty], built DATE)",
// omitting the parenthetical parts that are unknown. When version is
// devVersion (not stamped), the module version and VCS settings from bi
// fill in whatever they can: "go install ...@v0.2.0" yields the module
// version, and a build from a git checkout yields the commit.
func formatVersion(version, commit, date string, bi *debug.BuildInfo) string {
	dirty := false
	if version == devVersion && bi != nil {
		// Go 1.24+ records a pseudo-version (v0.0.0-TIME-COMMIT[+dirty])
		// for a build from a checkout; that carries no release
		// information beyond what the vcs.* settings already give.
		if v := bi.Main.Version; v != "" && v != "(devel)" && !strings.HasPrefix(v, "v0.0.0-") {
			version = strings.TrimPrefix(v, "v")
		}
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				if commit == "" {
					commit = s.Value
				}
			case "vcs.time":
				if date == "" {
					date = s.Value
				}
			case "vcs.modified":
				dirty = s.Value == "true"
			}
		}
	}

	var details []string
	if commit != "" {
		if len(commit) > 7 {
			commit = commit[:7]
		}
		if dirty {
			commit += "-dirty"
		}
		details = append(details, "commit "+commit)
	}
	if date != "" {
		details = append(details, "built "+date)
	}
	if len(details) == 0 {
		return version
	}
	return fmt.Sprintf("%s (%s)", version, strings.Join(details, ", "))
}
