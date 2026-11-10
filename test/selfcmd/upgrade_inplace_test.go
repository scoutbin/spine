package selfcmd_test

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/scoutbin/spine/internal/buildinfo"
	"github.com/scoutbin/spine/internal/release"
	"github.com/scoutbin/spine/internal/selfcmd"
	"github.com/scoutbin/spine/term"
	"github.com/scoutbin/spine/test/testutil"
)

func writeZeros(w http.ResponseWriter, n int) {
	chunk := make([]byte, 1<<20)
	for ; n > 0; n -= len(chunk) {
		w.Write(chunk[:min(n, len(chunk))])
	}
}

func TestUpgradeInPlace(t *testing.T) {
	const archiveName = "demo_%s_darwin_arm64.tar.gz"
	good := []testutil.TarEntry{{Name: "README.md", Body: "readme", Typeflag: tar.TypeReg}, {Name: "demo", Body: "new", Typeflag: tar.TypeReg}}
	ahead := withVersion(testInfo, "v0.3.0")
	nightly := withVersion(testInfo, "v0.1.4-nightly.20261020")
	beta := withVersion(testInfo, "v0.2.0-beta.1")

	testcases := []struct {
		name         string
		build        buildinfo.Info
		args         []string
		latest       string // tag served by releases/latest; empty makes it fail
		releases     string // JSON served by the releases list; empty makes it fail
		target       string // tag expected from a releases-list lookup
		archive      []testutil.TarEntry
		checksums    string // "", "mismatch", "missing" or "too-large"
		archive404   bool
		archiveHuge  bool // serve more than the archive cap
		readOnlyDir  bool
		progress     bool
		wantCode     int
		wantStdout   string
		stderrHas    string
		wantReplaced bool
	}{
		{name: "upgrade to latest", build: testInfo, args: []string{"upgrade"}, latest: "v0.2.0", archive: good,
			wantStdout: "Upgraded demo v0.1.3 → v0.2.0\n", wantReplaced: true},
		{name: "upgrade to explicit version skips the lookup", build: testInfo, args: []string{"upgrade", "--version", "0.2.0"}, archive: good,
			wantStdout: "Upgraded demo v0.1.3 → v0.2.0\n", wantReplaced: true},
		{name: "upgrade to prerelease version", build: testInfo, args: []string{"upgrade", "--version", "0.3.0-rc.1"}, archive: good,
			wantStdout: "Upgraded demo v0.1.3 → v0.3.0-rc.1\n", wantReplaced: true},
		{name: "downgrade with --version", build: testInfo, args: []string{"upgrade", "--version", "v0.1.0"}, archive: good,
			wantStdout: "Downgraded demo v0.1.3 → v0.1.0\n", wantReplaced: true},
		{name: "already up to date", build: testInfo, args: []string{"upgrade"}, latest: "v0.1.3", archive: good,
			wantStdout: "Already up to date (v0.1.3)\n"},
		{name: "explicit current version is up to date", build: testInfo, args: []string{"upgrade", "--version", "v0.1.3"}, archive: good,
			wantStdout: "Already up to date (v0.1.3)\n"},
		{name: "latest older than a local build is not a downgrade", build: ahead, args: []string{"upgrade"}, latest: "v0.2.0", archive: good,
			wantStdout: "Already up to date (v0.3.0)\n"},
		{name: "unknown prerelease build follows the stable track", build: nightly, args: []string{"upgrade"}, latest: "v0.2.0", archive: good,
			wantStdout: "Upgraded demo v0.1.4-nightly.20261020 → v0.2.0\n", wantReplaced: true},
		{name: "beta installs rc, never alpha", build: beta, args: []string{"upgrade"}, target: "v0.2.0-rc.1", archive: good,
			releases:   `[{"tag_name":"v0.2.0-alpha.4"},{"tag_name":"v0.2.0-rc.1"},{"tag_name":"v0.2.0-beta.1"}]`,
			wantStdout: "Upgraded demo v0.2.0-beta.1 → v0.2.0-rc.1\n", wantReplaced: true},
		{name: "beta with only a newer alpha is up to date", build: beta, args: []string{"upgrade"}, archive: good,
			releases:   `[{"tag_name":"v0.2.0-alpha.4"},{"tag_name":"v0.2.0-beta.1"}]`,
			wantStdout: "Already up to date (v0.2.0-beta.1)\n"},
		{name: "non-stable --version skips the lookup", build: beta, args: []string{"upgrade", "--version", "v0.1.3"}, archive: good,
			wantStdout: "Downgraded demo v0.2.0-beta.1 → v0.1.3\n", wantReplaced: true},
		{name: "releases lookup fails", build: beta, args: []string{"upgrade"}, archive: good,
			wantCode: 2, stderrHas: "look up latest release"},
		{name: "unsupported OS freebsd", build: withOS(testInfo, "freebsd"), args: []string{"upgrade"}, latest: "v0.2.0", archive: good,
			wantCode: 2, stderrHas: "self-upgrade is not supported on freebsd"},
		{name: "latest lookup fails", build: testInfo, args: []string{"upgrade"}, archive: good,
			wantCode: 2, stderrHas: "look up latest release"},
		{name: "checksum mismatch", build: testInfo, args: []string{"upgrade", "--version", "v0.2.0"}, archive: good, checksums: "mismatch",
			wantCode: 2, stderrHas: "checksum mismatch"},
		{name: "checksum entry missing", build: testInfo, args: []string{"upgrade", "--version", "v0.2.0"}, archive: good, checksums: "missing",
			wantCode: 2, stderrHas: "no checksum listed"},
		{name: "checksums file over 1 MB", build: testInfo, args: []string{"upgrade", "--version", "v0.2.0"}, archive: good, checksums: "too-large",
			wantCode: 2, stderrHas: "checksums.txt exceeds 1 MB"},
		{name: "archive over 200 MB", build: testInfo, args: []string{"upgrade", "--version", "v0.2.0"}, archive: good, archiveHuge: true,
			wantCode: 2, stderrHas: "exceeds 200 MB"},
		{name: "archive 404", build: testInfo, args: []string{"upgrade", "--version", "v0.2.0"}, archive: good, archive404: true,
			wantCode: 2, stderrHas: "not found in release v0.2.0"},
		{name: "archive with path traversal", build: testInfo, args: []string{"upgrade", "--version", "v0.2.0"},
			archive:  []testutil.TarEntry{{Name: "../demo", Body: "evil", Typeflag: tar.TypeReg}},
			wantCode: 2, stderrHas: "unsafe path"},
		{name: "archive without the binary", build: testInfo, args: []string{"upgrade", "--version", "v0.2.0"},
			archive:  []testutil.TarEntry{{Name: "README.md", Body: "readme", Typeflag: tar.TypeReg}},
			wantCode: 2, stderrHas: "no demo at its top level"},
		{name: "archive with a symlink as the binary", build: testInfo, args: []string{"upgrade", "--version", "v0.2.0"},
			archive:  []testutil.TarEntry{{Name: "demo", Typeflag: tar.TypeSymlink, Linkname: "/bin/sh"}},
			wantCode: 2, stderrHas: "not a regular file"},
		{name: "read-only directory", build: testInfo, args: []string{"upgrade", "--version", "v0.2.0"}, archive: good, readOnlyDir: true,
			wantCode: 2, stderrHas: "sudo demo upgrade"},
		{name: "result lines when progress is on", build: testInfo, args: []string{"upgrade"}, latest: "v0.2.0", archive: good, progress: true,
			wantStdout: "Upgraded demo v0.1.3 → v0.2.0\n", wantReplaced: true, stderrHas: "Checksum verified"},
		{name: "failure line when progress is on", build: testInfo, args: []string{"upgrade", "--version", "v0.2.0"}, archive: good, checksums: "mismatch", progress: true,
			wantCode: 2, stderrHas: "Checksum verification failed"},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.archiveHuge && testing.Short() {
				t.Skip("streams more than 200 MB")
			}
			if tc.readOnlyDir && runtime.GOOS == "windows" {
				t.Skip("read-only directories via chmod are not supported on Windows")
			}
			if tc.readOnlyDir && os.Geteuid() == 0 {
				t.Skip("root ignores directory permissions")
			}
			tag := tc.latest
			if tc.target != "" {
				tag = tc.target
			}
			if len(tc.args) > 2 {
				tag = "v" + strings.TrimPrefix(tc.args[2], "v")
			}
			archive := testutil.TarGz(t, tc.archive...)
			name := fmt.Sprintf(archiveName, strings.TrimPrefix(tag, "v"))
			sum := sha256.Sum256(archive)
			checksums := hex.EncodeToString(sum[:]) + "  " + name + "\n"
			switch tc.checksums {
			case "mismatch":
				checksums = strings.Repeat("0", 64) + "  " + name + "\n"
			case "missing":
				checksums = hex.EncodeToString(sum[:]) + "  demo_9.9.9_darwin_arm64.tar.gz\n"
			case "too-large":
				checksums = strings.Repeat("\n", 1<<20+1)
			}

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/repos/scoutbin/demo/releases/latest":
					if tc.latest == "" {
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					fmt.Fprintf(w, `{"tag_name":%q}`, tc.latest)
				case "/repos/scoutbin/demo/releases":
					if tc.releases == "" {
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					w.Write([]byte(tc.releases))
				case "/" + tag + "/checksums.txt":
					w.Write([]byte(checksums))
				case "/" + tag + "/" + name:
					switch {
					case tc.archive404:
						w.WriteHeader(http.StatusNotFound)
					case tc.archiveHuge:
						writeZeros(w, 200<<20+1)
					default:
						w.Write(archive)
					}
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer srv.Close()

			dir := t.TempDir()
			exe := filepath.Join(dir, "demo")
			if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
				t.Fatal(err)
			}
			if tc.readOnlyDir {
				if err := os.Chmod(dir, 0o555); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { os.Chmod(dir, 0o755) })
			}

			var stdout, stderr bytes.Buffer
			a := &selfcmd.App{
				Tool: testutil.Tool(), Stdout: &stdout, Stderr: &stderr, Build: tc.build,
				LookupEnv:    testutil.EnvOf(nil),
				Detect:       func(bool, bool) term.Mode { return term.Mode{Progress: tc.progress} },
				Executable:   func() (string, error) { return exe, nil },
				EvalSymlinks: func(p string) (string, error) { return p, nil },
				GitHub:       release.Client{BaseURL: srv.URL, DownloadBase: srv.URL, HTTP: srv.Client()},
			}

			code := a.Run(tc.args)

			if code != tc.wantCode {
				t.Errorf("exit code = %d, want %d (stderr: %q)", code, tc.wantCode, stderr.String())
			}
			if stdout.String() != tc.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout.String(), tc.wantStdout)
			}
			if !strings.Contains(stderr.String(), tc.stderrHas) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tc.stderrHas)
			}
			wantContent := "old"
			if tc.wantReplaced {
				wantContent = "new"
			}
			if got, err := os.ReadFile(exe); err != nil || string(got) != wantContent {
				t.Errorf("binary = %q (err %v), want %q", got, err, wantContent)
			}
			if entries, _ := os.ReadDir(dir); len(entries) != 1 {
				t.Errorf("directory has %d entries, want only the binary (no temp files)", len(entries))
			}
		})
	}
}
