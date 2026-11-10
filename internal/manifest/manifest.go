package manifest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// ChecksumsFile is the name of the checksums file published with each release.
const ChecksumsFile = "checksums.txt"

const maxNpmPackageLength = 214

var (
	namePattern       = regexp.MustCompile(`^[a-z][a-z0-9]*$`)
	repoPattern       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9._-]+$`)
	tapPattern        = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9-]*)/homebrew-([A-Za-z0-9_-]+)$`)
	npmPattern        = regexp.MustCompile(`^(@[a-z0-9][a-z0-9._~-]*/)?[a-z0-9][a-z0-9._~-]*$`)
	chocolateyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*$`)
)

// Tool is the parsed spine.json manifest of a tool.
type Tool struct {
	Name     string   `json:"name"`
	Repo     string   `json:"repo"`
	Summary  string   `json:"summary"`
	License  string   `json:"license"`
	Channels Channels `json:"channels"`
}

// Channels lists the distribution channels of a tool. A channel left at its zero value is disabled.
type Channels struct {
	Homebrew   Homebrew   `json:"homebrew"`
	Npm        Npm        `json:"npm"`
	Chocolatey Chocolatey `json:"chocolatey"`
	Action     bool       `json:"action"`
}

// Homebrew configures the Homebrew channel; Tap is "owner/homebrew-name".
type Homebrew struct {
	Tap string `json:"tap"`
}

// Npm configures the npm channel; Package is an optionally scoped package name.
type Npm struct {
	Package string `json:"package"`
}

// Chocolatey configures the Chocolatey channel. It is accepted in the schema but not used yet.
type Chocolatey struct {
	ID string `json:"id"`
}

// Target is an operating system and architecture pair that releases are built for.
type Target struct {
	GOOS   string
	GOARCH string
}

// Load parses and validates a spine.json manifest. Unknown fields and trailing data are errors.
func Load(data []byte) (Tool, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var t Tool
	if err := dec.Decode(&t); err != nil {
		return Tool{}, fmt.Errorf("spine.json: %w", err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Tool{}, errors.New("spine.json: trailing data after the manifest object")
	}
	if err := t.validate(); err != nil {
		return Tool{}, fmt.Errorf("spine.json: %w", err)
	}
	return t, nil
}

// MustLoad is like Load but panics on error.
func MustLoad(data []byte) Tool {
	t, err := Load(data)
	if err != nil {
		panic(err)
	}
	return t
}

func (t Tool) validate() error {
	switch {
	case !namePattern.MatchString(t.Name):
		return fmt.Errorf("name %q must match %s", t.Name, namePattern)
	case !repoPattern.MatchString(t.Repo):
		return fmt.Errorf("repo %q must be owner/name", t.Repo)
	case strings.TrimSpace(t.Summary) == "":
		return errors.New("summary is required")
	case strings.TrimSpace(t.License) == "":
		return errors.New("license is required")
	case t.Channels.Homebrew.Tap != "" && !tapPattern.MatchString(t.Channels.Homebrew.Tap):
		return fmt.Errorf("channels.homebrew.tap %q must be owner/homebrew-name", t.Channels.Homebrew.Tap)
	case t.Channels.Npm.Package != "" && (len(t.Channels.Npm.Package) > maxNpmPackageLength || !npmPattern.MatchString(t.Channels.Npm.Package)):
		return fmt.Errorf("channels.npm.package %q must be a lower-case npm package name of at most %d characters", t.Channels.Npm.Package, maxNpmPackageLength)
	case t.Channels.Chocolatey.ID != "" && !chocolateyPattern.MatchString(t.Channels.Chocolatey.ID):
		return fmt.Errorf("channels.chocolatey.id %q must be a lower-case Chocolatey id", t.Channels.Chocolatey.ID)
	}
	return nil
}

// BinaryName returns the executable file name for goos.
func (t Tool) BinaryName(goos string) string {
	if goos == "windows" {
		return t.Name + ".exe"
	}
	return t.Name
}

// ArchiveName returns the release archive file name; a leading "v" on version is dropped.
func (t Tool) ArchiveName(version, goos, goarch string) string {
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return t.Name + "_" + strings.TrimPrefix(version, "v") + "_" + goos + "_" + goarch + ext
}

// EnvVar returns the environment variable name for suffix, prefixed with the upper-cased tool name.
func (t Tool) EnvVar(suffix string) string {
	return strings.ToUpper(t.Name) + "_" + suffix
}

// InstallDir returns a display string for the default install directory on goos.
// It is not an expanded or creatable path.
func (t Tool) InstallDir(goos string) string {
	if goos == "windows" {
		return `%LOCALAPPDATA%\` + t.Name + `\bin`
	}
	return "~/.local/bin"
}

// FormulaClass returns the Homebrew formula class name. It assumes a validated Tool, i.e. one obtained via Load.
func (t Tool) FormulaClass() string {
	return strings.ToUpper(t.Name[:1]) + t.Name[1:]
}

// HomebrewFormula returns the lower-case "owner/tap/name" formula reference, or "" when no tap is set.
// It assumes a validated Tool, i.e. one obtained via Load.
func (t Tool) HomebrewFormula() string {
	m := tapPattern.FindStringSubmatch(t.Channels.Homebrew.Tap)
	if m == nil {
		return ""
	}
	return strings.ToLower(m[1] + "/" + m[2] + "/" + t.Name)
}

// PackageOwner returns the owner part of Repo. It assumes a validated Tool, i.e. one obtained via Load.
func (t Tool) PackageOwner() string {
	owner, _, _ := strings.Cut(t.Repo, "/")
	return owner
}

// Targets returns a new slice of the platforms releases are built for.
func (t Tool) Targets() []Target {
	return []Target{
		{"darwin", "amd64"}, {"darwin", "arm64"},
		{"linux", "amd64"}, {"linux", "arm64"},
		{"windows", "amd64"}, {"windows", "arm64"},
	}
}
