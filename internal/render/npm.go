package render

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/scoutbin/spine/internal/manifest"
	"github.com/scoutbin/spine/internal/release"
)

const nodeEngine = ">=22"

type archive struct {
	Name   string
	SHA256 string
}

type npmData struct {
	manifest.Tool
	Generator  string
	Version    string
	ReleaseURL string
	Archives   []archive
}

// npmManifest is the published package.json. The field order is the output
// order, so the JSON is byte-stable. It has no dist-tag: the tag is only passed
// to `npm publish --tag`.
type npmManifest struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	License     string `json:"license"`
	Homepage    string `json:"homepage"`
	Repository  struct {
		Type string `json:"type"`
		URL  string `json:"url"`
	} `json:"repository"`
	Bin     map[string]string `json:"bin"`
	Scripts struct {
		Postinstall string `json:"postinstall"`
	} `json:"scripts"`
	Files   []string `json:"files"`
	OS      []string `json:"os"`
	CPU     []string `json:"cpu"`
	Engines struct {
		Node string `json:"node"`
	} `json:"engines"`
	PublishConfig struct {
		Access string `json:"access"`
	} `json:"publishConfig"`
}

// Npm renders the package of a release on any track. The package carries the
// sha256 of every release archive from r.Checksums, and its install script
// verifies the archive it downloads against them.
func Npm(t manifest.Tool, r Release) ([]File, error) {
	if t.Channels.Npm.Package == "" {
		return nil, errors.New("the npm channel is not enabled in spine.json")
	}
	if _, err := release.ValidatePushedTag(r.Tag); err != nil {
		return nil, err
	}

	d := npmData{Tool: t, Generator: r.Generator, Version: strings.TrimPrefix(r.Tag, "v"), ReleaseURL: strings.TrimRight(r.assetURL(t, ""), "/")}
	for _, target := range t.Targets() {
		name := t.ArchiveName(r.Tag, target.GOOS, target.GOARCH)
		sum, err := release.LookupChecksum(r.Checksums, name)
		if err != nil {
			return nil, err
		}
		d.Archives = append(d.Archives, archive{Name: name, SHA256: sum})
	}

	packageJSON, err := npmPackageJSON(t, d.Version)
	if err != nil {
		return nil, err
	}
	install, err := execute("npm-install.js.tmpl", d)
	if err != nil {
		return nil, fmt.Errorf("render install.js: %w", err)
	}
	launcher, err := execute("npm-launcher.js.tmpl", d)
	if err != nil {
		return nil, fmt.Errorf("render launcher.js: %w", err)
	}
	return []File{
		{Path: "package.json", Mode: 0o644, Content: packageJSON},
		{Path: "install.js", Mode: 0o644, Content: install},
		{Path: "launcher.js", Mode: 0o755, Content: launcher},
	}, nil
}

func npmPackageJSON(t manifest.Tool, version string) ([]byte, error) {
	bin := map[string]string{t.Name: "launcher.js"}
	if t.Alias != "" {
		bin[t.Alias] = "launcher.js"
	}
	m := npmManifest{
		Name:        t.Channels.Npm.Package,
		Version:     version,
		Description: t.Summary,
		License:     t.License,
		Homepage:    "https://github.com/" + t.Repo,
		Bin:         bin,
		Files:       []string{"install.js", "launcher.js"},
		OS:          []string{"darwin", "linux", "win32"},
		CPU:         []string{"x64", "arm64"},
	}
	m.Repository.Type = "git"
	m.Repository.URL = "git+https://github.com/" + t.Repo + ".git"
	m.Scripts.Postinstall = "node install.js"
	m.Engines.Node = nodeEngine
	m.PublishConfig.Access = "public"

	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(m); err != nil {
		return nil, fmt.Errorf("render package.json: %w", err)
	}
	return out.Bytes(), nil
}
