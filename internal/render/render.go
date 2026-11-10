// Package render generates the files a tool commits to its repository from its
// manifest.
package render

import (
	"bytes"
	"embed"
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
	"join":       strings.Join,
	"trimPrefix": strings.TrimPrefix,
}

// Files renders the files of t. spineVersion names the spine release in the
// header of each file.
func Files(t manifest.Tool, spineVersion string) ([]File, error) {
	d := data{Tool: t, Generator: spineVersion, Tracks: trackPaths()}
	specs := []struct {
		path string
		mode fs.FileMode
	}{
		{"install.sh", 0o755},
		{"install.ps1", 0o644},
	}
	files := make([]File, 0, len(specs))
	for _, s := range specs {
		content, err := execute(s.path+".tmpl", d)
		if err != nil {
			return nil, fmt.Errorf("render %s: %w", s.path, err)
		}
		files = append(files, File{Path: s.path, Mode: s.mode, Content: content})
	}
	return files, nil
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
