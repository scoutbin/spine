package testutil

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
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

func Tool() spine.Tool {
	return spine.MustLoad([]byte(Manifest))
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
