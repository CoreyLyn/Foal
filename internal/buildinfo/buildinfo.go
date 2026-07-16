// Package buildinfo exposes release metadata shared by every Foal executable.
package buildinfo

import (
	"runtime"
	"strings"
)

// Version and Commit are overridden by release builds through linker flags.
// Development builds intentionally retain deterministic fallback values.
var (
	Version = "dev"
	Commit  = "unknown"
)

// Info is the stable read model returned by the CLI version surface.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

// Current returns sanitized linker metadata plus runtime build information.
func Current() Info {
	version := strings.TrimSpace(Version)
	if version == "" {
		version = "dev"
	}
	commit := strings.TrimSpace(Commit)
	if commit == "" {
		commit = "unknown"
	}

	return Info{
		Version:   version,
		Commit:    commit,
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
}
