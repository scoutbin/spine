// Package dist cross-builds a tool and writes its release archives and
// checksums file.
package dist

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/scoutbin/spine/internal/manifest"
	"github.com/scoutbin/spine/internal/release"
)

// buildinfoPath is the symbol path the linker stamps. It is the same for every
// tool because buildinfo lives in spine, not in the tool.
const buildinfoPath = "github.com/scoutbin/spine/internal/buildinfo"

const commitLength = 7

// Options configures Build.
type Options struct {
	Tool    manifest.Tool
	Version string // v-prefixed semantic version, e.g. v1.2.3 or v0.0.0-ci
	Package string // main package to build, as accepted by go build
	OutDir  string
}

// Build writes one archive per target and the checksums file into OutDir. The
// build date and archive mtimes are the HEAD commit time, so rebuilding the
// same commit does not change the archive metadata.
func Build(o Options) error {
	if _, ok := release.ParseVersion(o.Version); !ok || !strings.HasPrefix(o.Version, "v") {
		return fmt.Errorf("version %q must be a v-prefixed semantic version such as v1.2.3", o.Version)
	}
	if strings.Contains(o.Version, "+") {
		return fmt.Errorf("version %q must not contain build metadata", o.Version)
	}
	if err := requireEmpty(o.OutDir); err != nil {
		return err
	}
	commit, when, err := head()
	if err != nil {
		return err
	}
	stage, err := os.MkdirTemp("", o.Tool.Name+"-dist-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := os.MkdirAll(o.OutDir, 0o755); err != nil {
		return err
	}

	ldflags := fmt.Sprintf("-s -w -X %[1]s.Version=%[2]s -X %[1]s.Commit=%[3]s -X %[1]s.Date=%[4]s",
		buildinfoPath, o.Version, commit, when.Format(time.RFC3339))

	var checksums strings.Builder
	for _, t := range o.Tool.Targets() {
		binName := o.Tool.BinaryName(t.GOOS)
		binPath := filepath.Join(stage, t.GOOS+"_"+t.GOARCH, binName)
		if err := goBuild(t, ldflags, binPath, o.Package); err != nil {
			return err
		}
		binary, err := os.ReadFile(binPath)
		if err != nil {
			return err
		}
		archive, err := pack(t.GOOS == "windows", binName, binary, when)
		if err != nil {
			return err
		}
		name := o.Tool.ArchiveName(o.Version, t.GOOS, t.GOARCH)
		if err := os.WriteFile(filepath.Join(o.OutDir, name), archive, 0o644); err != nil {
			return err
		}
		sum := sha256.Sum256(archive)
		fmt.Fprintf(&checksums, "%s  %s\n", hex.EncodeToString(sum[:]), name)
	}
	return os.WriteFile(filepath.Join(o.OutDir, manifest.ChecksumsFile), []byte(checksums.String()), 0o644)
}

// requireEmpty accepts a missing or empty directory, so artifacts of another
// version are never mixed into one release.
func requireEmpty(dir string) error {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return fmt.Errorf("output directory %s is not empty; remove it first", dir)
	}
	return nil
}

func goBuild(t manifest.Target, ldflags, out, pkg string) error {
	cmd := exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-ldflags", ldflags, "-o", out, pkg)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+t.GOOS, "GOARCH="+t.GOARCH)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("build %s/%s: %w\n%s", t.GOOS, t.GOARCH, err, bytes.TrimSpace(output))
	}
	return nil
}

// head returns the abbreviated hash and the commit time of HEAD. The hash is
// cut to a fixed length because git's own abbreviation grows with the repository.
func head() (string, time.Time, error) {
	out, err := exec.Command("git", "log", "-1", "--format=%H %ct").Output()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("read HEAD commit: %w", err)
	}
	fields := strings.Fields(string(out))
	if len(fields) != 2 || len(fields[0]) < commitLength {
		return "", time.Time{}, fmt.Errorf("read HEAD commit: unexpected git output %q", out)
	}
	seconds, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("read HEAD commit: %w", err)
	}
	return fields[0][:commitLength], time.Unix(seconds, 0).UTC(), nil
}

func pack(zipFormat bool, name string, binary []byte, modTime time.Time) ([]byte, error) {
	if zipFormat {
		return packZip(name, binary, modTime)
	}
	return packTarGz(name, binary, modTime)
}

func packTarGz(name string, binary []byte, modTime time.Time) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{Typeflag: tar.TypeReg, Name: name, Mode: 0o755, Size: int64(len(binary)), ModTime: modTime}
	if err := tw.WriteHeader(hdr); err != nil {
		return nil, err
	}
	if _, err := tw.Write(binary); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func packZip(name string, binary []byte, modTime time.Time) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	hdr := &zip.FileHeader{Name: name, Method: zip.Deflate, Modified: modTime}
	hdr.SetMode(0o755)
	w, err := zw.CreateHeader(hdr)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(binary); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
