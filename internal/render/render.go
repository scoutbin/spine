// Package render generates the files a tool commits to its repository from its
// manifest.
package render

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"
	"text/template"
	"unicode"

	"github.com/scoutbin/spine/internal/manifest"
	"github.com/scoutbin/spine/internal/release"
)

//go:embed templates/*.tmpl
var templates embed.FS

// File is a generated file, with a path relative to the tool repository root.
type File struct {
	Path    string
	Mode    fs.FileMode
	Content []byte
}

type trackPath struct {
	Name string
	Path []string
}

type data struct {
	manifest.Tool
	Generator string
	Tracks    []trackPath
}

var funcs = template.FuncMap{
	"comment":    comment,
	"yaml":       yaml,
	"join":       strings.Join,
	"trimPrefix": strings.TrimPrefix,
}

type spec struct {
	path    string
	mode    fs.FileMode
	enabled bool
}

func specs(t manifest.Tool) []spec {
	return []spec{
		{"install.sh", 0o755, true},
		{"install.ps1", 0o644, true},
		{"action.yml", 0o644, t.Channels.Action},
	}
}

// Files renders the files of t. spineVersion names the spine release in the
// header of each file. action.yml is rendered only when the action channel is
// enabled.
func Files(t manifest.Tool, spineVersion string) ([]File, error) {
	d := data{Tool: t, Generator: spineVersion, Tracks: trackPaths()}
	var files []File
	for _, s := range specs(t) {
		if !s.enabled {
			continue
		}
		content, err := execute(s.path+".tmpl", d)
		if err != nil {
			return nil, fmt.Errorf("render %s: %w", s.path, err)
		}
		files = append(files, File{Path: s.path, Mode: s.mode, Content: content})
	}
	return files, nil
}

// Disabled returns the paths of the generated files that t does not enable.
func Disabled(t manifest.Tool) []string {
	var paths []string
	for _, s := range specs(t) {
		if !s.enabled {
			paths = append(paths, s.path)
		}
	}
	return paths
}

// execute uses {% %} delimiters because the shell sources contain ${ directly
// next to the places a value is inserted.
func execute(name string, d data) ([]byte, error) {
	tmpl, err := template.New(name).Delims("{%", "%}").Funcs(funcs).ParseFS(templates, "templates/"+name)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, d); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func trackPaths() []trackPath {
	var out []trackPath
	for _, t := range []release.Track{release.Stable, release.RC, release.Beta, release.Alpha} {
		var path []string
		for _, p := range t.Path() {
			path = append(path, string(p))
		}
		out = append(out, trackPath{Name: string(t), Path: path})
	}
	return out
}

// comment makes s safe for a line comment in sh and PowerShell, both of which
// end at a line break. Every run of line breaks and other control characters,
// including the Unicode line and paragraph separators, becomes one space.
func comment(s string) string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return unicode.IsControl(r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r)
	})
	return strings.Join(fields, " ")
}

// yaml renders s as a double-quoted YAML scalar. JSON string syntax is valid
// YAML and escapes quotes, backslashes, line breaks and ASCII control
// characters; the remaining unprintable characters, such as the Unicode line
// breaks, are escaped here. A "${{" is split, so that s can never open a GitHub
// Actions expression.
func yaml(s string) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(strings.ReplaceAll(s, "${{", "$ {{")); err != nil {
		return "", err
	}
	var out strings.Builder
	for _, r := range strings.TrimSuffix(buf.String(), "\n") {
		switch {
		case unicode.IsPrint(r):
			out.WriteRune(r)
		case r > 0xFFFF:
			fmt.Fprintf(&out, `\U%08x`, r)
		default:
			fmt.Fprintf(&out, `\u%04x`, r)
		}
	}
	return out.String(), nil
}
