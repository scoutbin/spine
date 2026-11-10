package channel

import (
	"strings"

	"github.com/scoutbin/spine/internal/manifest"
)

// Channel is how the running binary was installed. The string values are a
// stable contract (the `channel` key of `<tool> info --json`).
type Channel string

const (
	InstallSh Channel = "install.sh"
	NPM       Channel = "npm"
	Homebrew  Channel = "homebrew"
	Unknown   Channel = "unknown"
)

// UpgradeCommand returns the command a package manager runs to upgrade t, for a
// build on the npm dist-tag distTag. It returns false for channels that are not
// package-manager managed, and for a channel t does not publish to. Homebrew
// only carries stable releases, so distTag does not change its command.
func UpgradeCommand(t manifest.Tool, c Channel, distTag string) (string, bool) {
	switch {
	case c == NPM && t.Channels.Npm.Package != "":
		return "npm install -g " + t.Channels.Npm.Package + "@" + distTag, true
	case c == Homebrew && t.HomebrewFormula() != "":
		return "brew upgrade " + t.HomebrewFormula(), true
	}
	return "", false
}

// Detect classifies an install from the symlink-resolved executable path. It
// accepts both path separators and ignores case, so Windows paths classify
// the same on any host. isRelease reports whether the build has a release
// version; only release builds on non-Windows hosts are install.sh.
func Detect(path, goos string, isRelease bool) Channel {
	p := normalize(path)
	segments := strings.Split(p, "/")

	switch {
	case hasSegment(segments, "cellar") || hasSegment(segments, "caskroom"):
		return Homebrew
	case hasSegment(segments, "node_modules"):
		return NPM
	case isRelease && goos != "windows":
		return InstallSh
	}
	return Unknown
}

func normalize(path string) string {
	return strings.ToLower(strings.ReplaceAll(path, `\`, "/"))
}

func hasSegment(segments []string, name string) bool {
	for _, s := range segments {
		if s == name {
			return true
		}
	}
	return false
}
