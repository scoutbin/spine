package render

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/scoutbin/spine/internal/manifest"
	"github.com/scoutbin/spine/internal/release"
)

// Release is the input of the package renderers: a pushed tag and the contents
// of the checksums.txt that dist wrote for it.
type Release struct {
	Tag       string
	Checksums []byte
	// BaseURL replaces the GitHub download URL of the release assets. It is for
	// tests that serve the archives locally.
	BaseURL   string
	Generator string // spine version named in the header of each file
}

type asset struct {
	URL    string
	SHA256 string
}

type homebrewData struct {
	manifest.Tool
	Generator                                        string
	Version                                          string
	DarwinArm64, DarwinAmd64, LinuxArm64, LinuxAmd64 asset
}

// Homebrew renders the formula of a stable release, which is the only kind of
// release Homebrew carries. Each platform entry points at the release archive
// and carries its hash from r.Checksums.
func Homebrew(t manifest.Tool, r Release) ([]File, error) {
	if t.Channels.Homebrew.Tap == "" {
		return nil, errors.New("the homebrew channel is not enabled in spine.json")
	}
	if strings.ContainsFunc(t.License, func(r rune) bool { return unicode.IsSpace(r) || r == '(' || r == ')' }) {
		return nil, fmt.Errorf("license %q is an expression: Homebrew needs a single SPDX identifier", t.License)
	}
	track, err := release.ValidatePushedTag(r.Tag)
	if err != nil {
		return nil, err
	}
	if track != release.Stable {
		return nil, fmt.Errorf("tag %q is a %s release: Homebrew carries stable releases only", r.Tag, track)
	}

	d := homebrewData{Tool: t, Generator: r.Generator, Version: strings.TrimPrefix(r.Tag, "v")}
	platforms := []struct {
		goos, goarch string
		entry        *asset
	}{
		{"darwin", "arm64", &d.DarwinArm64},
		{"darwin", "amd64", &d.DarwinAmd64},
		{"linux", "arm64", &d.LinuxArm64},
		{"linux", "amd64", &d.LinuxAmd64},
	}
	for _, p := range platforms {
		name := t.ArchiveName(r.Tag, p.goos, p.goarch)
		sum, err := release.LookupChecksum(r.Checksums, name)
		if err != nil {
			return nil, err
		}
		*p.entry = asset{URL: r.assetURL(t, name), SHA256: sum}
	}

	content, err := execute("homebrew.rb.tmpl", d)
	if err != nil {
		return nil, fmt.Errorf("render formula: %w", err)
	}
	return []File{{Path: t.Name + ".rb", Mode: 0o644, Content: content}}, nil
}

func (r Release) assetURL(t manifest.Tool, name string) string {
	if r.BaseURL == "" {
		return release.DownloadURL(t, r.Tag, name)
	}
	return strings.TrimRight(r.BaseURL, "/") + "/" + name
}
