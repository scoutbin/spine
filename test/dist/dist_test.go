package dist_test

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/scoutbin/spine/internal/devcmd"
	"github.com/scoutbin/spine/internal/manifest"
	"github.com/scoutbin/spine/internal/release"
)

const version = "v0.0.0-ci"

var tool = func() manifest.Tool {
	data, err := os.ReadFile("../../internal/example/spine.json")
	if err != nil {
		panic(err)
	}
	return manifest.MustLoad(data)
}()

var (
	sharedOnce sync.Once
	sharedOut  string
	sharedErr  error
)

func TestMain(m *testing.M) {
	code := m.Run()
	if sharedOut != "" {
		os.RemoveAll(sharedOut)
	}
	os.Exit(code)
}

func runDist(out string) error {
	args := []string{"dist", "--version", version, "--manifest", "../../internal/example/spine.json",
		"--package", "github.com/scoutbin/spine/internal/example", "--out", out}
	var stdout, stderr bytes.Buffer
	if code := devcmd.Run(args, &stdout, &stderr); code != 0 {
		return fmt.Errorf("dist exit code = %d, stderr %q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		return fmt.Errorf("stdout = %q, want empty", stdout.String())
	}
	return nil
}

// sharedDist builds once per test binary run; callers must not modify the result.
func sharedDist(t *testing.T) string {
	t.Helper()
	sharedOnce.Do(func() {
		sharedOut, sharedErr = os.MkdirTemp("", "dist-test-")
		if sharedErr == nil {
			sharedErr = runDist(sharedOut)
		}
	})
	if sharedErr != nil {
		t.Fatal(sharedErr)
	}
	return sharedOut
}

func headCommit(t *testing.T) (short string, when time.Time) {
	t.Helper()
	out, err := exec.Command("git", "log", "-1", "--format=%H %ct").Output()
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(string(out))
	seconds, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	return fields[0][:7], time.Unix(seconds, 0).UTC()
}

func readBinary(t *testing.T, path string, windows bool, wantTime time.Time) (name string, mode os.FileMode, content []byte) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if windows {
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatal(err)
		}
		if len(zr.File) != 1 {
			t.Fatalf("zip has %d entries, want 1", len(zr.File))
		}
		f := zr.File[0]
		if !f.Modified.Equal(wantTime) {
			t.Errorf("zip mtime = %v, want %v", f.Modified, wantTime)
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		defer rc.Close()
		content, err = io.ReadAll(rc)
		if err != nil {
			t.Fatal(err)
		}
		return f.Name, f.Mode().Perm(), content
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	hdr, err := tr.Next()
	if err != nil {
		t.Fatal(err)
	}
	if !hdr.ModTime.Equal(wantTime) {
		t.Errorf("tar mtime = %v, want %v", hdr.ModTime, wantTime)
	}
	content, err = io.ReadAll(tr)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Next(); err != io.EOF {
		t.Errorf("tar has more than one entry (next error %v)", err)
	}
	return hdr.Name, os.FileMode(hdr.Mode).Perm(), content
}

func TestDist(t *testing.T) {
	out := sharedDist(t)
	commit, when := headCommit(t)
	checksums, err := os.ReadFile(filepath.Join(out, manifest.ChecksumsFile))
	if err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(out)
	if err != nil {
		t.Fatal(err)
	}
	if want := len(tool.Targets()) + 1; len(entries) != want {
		t.Errorf("dist has %d files, want %d", len(entries), want)
	}

	for _, target := range tool.Targets() {
		archive := tool.ArchiveName(version, target.GOOS, target.GOARCH)
		t.Run(archive, func(t *testing.T) {
			path := filepath.Join(out, archive)
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			if err := release.VerifyChecksum(checksums, archive, f); err != nil {
				t.Error(err)
			}

			name, mode, binary := readBinary(t, path, target.GOOS == "windows", when)
			if want := tool.BinaryName(target.GOOS); name != want {
				t.Errorf("entry name = %q, want %q", name, want)
			}
			if mode != 0o755 {
				t.Errorf("entry mode = %o, want 755", mode)
			}
			if len(binary) == 0 {
				t.Fatal("entry is empty")
			}

			if target.GOOS != runtime.GOOS || target.GOARCH != runtime.GOARCH {
				return
			}
			exe := filepath.Join(t.TempDir(), name)
			if err := os.WriteFile(exe, binary, 0o755); err != nil {
				t.Fatal(err)
			}
			got, err := exec.Command(exe, "--version").Output()
			if err != nil {
				t.Fatal(err)
			}
			want := fmt.Sprintf("%s %s (commit %s, built %s, %s/%s)\n", tool.Name, version, commit, when.Format(time.RFC3339), target.GOOS, target.GOARCH)
			if string(got) != want {
				t.Errorf("--version = %q, want %q", got, want)
			}
		})
	}
}

func TestDistIsReproducible(t *testing.T) {
	if testing.Short() {
		t.Skip("rebuilds every target")
	}
	second := t.TempDir()
	if err := runDist(second); err != nil {
		t.Fatal(err)
	}
	var sums [2][]byte
	for i, dir := range []string{sharedDist(t), second} {
		data, err := os.ReadFile(filepath.Join(dir, manifest.ChecksumsFile))
		if err != nil {
			t.Fatal(err)
		}
		sums[i] = data
	}
	if !bytes.Equal(sums[0], sums[1]) {
		t.Errorf("checksums differ between two builds of the same commit:\n%s\n%s", sums[0], sums[1])
	}
}
