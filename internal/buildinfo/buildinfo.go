package buildinfo

import (
	"fmt"
	"runtime"
)

// Set at build time via -ldflags "-X ...".
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// Info is the build identity of the binary. Its JSON keys are a stable contract.
type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"build_time"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
}

// Current returns the Info of the running binary.
func Current() Info {
	return Info{Version: Version, Commit: Commit, Date: Date, OS: runtime.GOOS, Arch: runtime.GOARCH}
}

// String returns the version line of the tool called name. Its format is a stable contract.
func (i Info) String(name string) string {
	return fmt.Sprintf("%s %s (commit %s, built %s, %s/%s)", name, i.Version, i.Commit, i.Date, i.OS, i.Arch)
}
