package testutil

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scoutbin/spine"
)

const Manifest = `{
  "name": "demo",
  "repo": "scoutbin/demo",
  "summary": "Demonstrate the spine",
  "license": "Apache-2.0",
  "channels": {
    "homebrew": {"tap": "scoutbin/homebrew-tap"},
    "npm": {"package": "@scoutbin/demo"},
    "chocolatey": {"id": "scoutbin-demo"},
    "action": true
  }
}`

const MinimalManifest = `{
  "name": "demo",
  "repo": "scoutbin/demo",
  "summary": "Demonstrate the spine",
  "license": "Apache-2.0"
}`

// WriteManifest writes content as spine.json in a new temp dir and returns the dir.
func WriteManifest(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "spine.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func Tool() spine.Tool {
	return spine.MustLoad([]byte(Manifest))
}

// Checksums returns the contents of a checksums.txt that lists the archives of
// tag for every target of the fixture Tool, each with its own digest.
func Checksums(tag string) string {
	tool := Tool()
	var b strings.Builder
	for i, target := range tool.Targets() {
		fmt.Fprintf(&b, "%064x  %s\n", i+1, tool.ArchiveName(tag, target.GOOS, target.GOARCH))
	}
	return b.String()
}

// ChecksumsWithout returns checksums without the lines that mention archive.
func ChecksumsWithout(checksums, archive string) string {
	var kept []string
	for _, line := range strings.SplitAfter(checksums, "\n") {
		if !strings.Contains(line, archive) {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "")
}

type TarEntry struct {
	Name     string
	Body     string
	Typeflag byte
	Linkname string
}

func TarGz(t *testing.T, entries ...TarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		typeflag := e.Typeflag
		if typeflag == 0 {
			typeflag = tar.TypeReg
		}
		hdr := &tar.Header{Name: e.Name, Typeflag: typeflag, Mode: 0o755, Linkname: e.Linkname}
		if typeflag == tar.TypeReg {
			hdr.Size = int64(len(e.Body))
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(e.Body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func EnvOf(vars map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := vars[k]; return v, ok }
}
