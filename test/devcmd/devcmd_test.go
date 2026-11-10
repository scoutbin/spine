package devcmd_test

import (
	"bytes"
	"io"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/scoutbin/spine/internal/devcmd"
	"github.com/scoutbin/spine/test/testutil"
)

type testcase struct {
	name       string
	manifest   string // spine.json content; written to a temp dir when non-empty
	chdir      bool   // run from the manifest dir, so the default ./spine.json applies
	args       []string
	wantCode   int
	wantStdout string // exact match
	stderrHas  string
}

func run(t *testing.T, tc testcase) {
	t.Helper()
	args := append([]string{}, tc.args...)
	if tc.manifest != "" {
		dir := testutil.WriteManifest(t, tc.manifest)
		if tc.chdir {
			t.Chdir(dir)
		}
		for i, a := range args {
			args[i] = strings.ReplaceAll(strings.ReplaceAll(a, "{manifest}", filepath.Join(dir, "spine.json")), "{dir}", dir)
		}
	}

	var stdout, stderr bytes.Buffer
	code := devcmd.Run(args, &stdout, &stderr)

	if code != tc.wantCode {
		t.Errorf("exit code = %d, want %d (stderr %q)", code, tc.wantCode, stderr.String())
	}
	if stdout.String() != tc.wantStdout {
		t.Errorf("stdout = %q, want %q", stdout.String(), tc.wantStdout)
	}
	if !strings.Contains(stderr.String(), tc.stderrHas) {
		t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tc.stderrHas)
	}
}

func TestManifest(t *testing.T) {
	// Changing the emitted keys or values means bumping devcmd.Contract.
	const (
		full = "contract=1\nname=demo\n" +
			"homebrew=true\nhomebrew_tap=scoutbin/homebrew-tap\nhomebrew_formula=scoutbin/tap/demo\n" +
			"npm=true\nnpm_package=@scoutbin/demo\n" +
			"action=true\n"
		homebrewOnly = "contract=1\nname=demo\n" +
			"homebrew=true\nhomebrew_tap=scoutbin/homebrew-tap\nhomebrew_formula=scoutbin/tap/demo\n" +
			"npm=false\nnpm_package=\n" +
			"action=false\n"
		npmOnly = "contract=1\nname=demo\n" +
			"homebrew=false\nhomebrew_tap=\nhomebrew_formula=\n" +
			"npm=true\nnpm_package=@scoutbin/demo\n" +
			"action=false\n"
		minimal = "contract=1\nname=demo\n" +
			"homebrew=false\nhomebrew_tap=\nhomebrew_formula=\n" +
			"npm=false\nnpm_package=\n" +
			"action=false\n"
	)
	testcases := []testcase{
		{name: "all channels", manifest: testutil.Manifest, args: []string{"manifest", "--manifest", "{manifest}"}, wantStdout: full},
		{name: "no channels", manifest: testutil.MinimalManifest, args: []string{"manifest", "--manifest={manifest}"}, wantStdout: minimal},
		{name: "homebrew only", manifest: `{"name":"demo","repo":"scoutbin/demo","summary":"s","license":"MIT","channels":{"homebrew":{"tap":"scoutbin/homebrew-tap"}}}`,
			args: []string{"manifest", "--manifest", "{manifest}"}, wantStdout: homebrewOnly},
		{name: "npm only", manifest: `{"name":"demo","repo":"scoutbin/demo","summary":"s","license":"MIT","channels":{"npm":{"package":"@scoutbin/demo"}}}`,
			args: []string{"manifest", "--manifest", "{manifest}"}, wantStdout: npmOnly},
		{name: "manifest flag before the command", manifest: testutil.Manifest, args: []string{"--manifest", "{manifest}", "manifest"}, wantStdout: full},
		{name: "default is ./spine.json", manifest: testutil.Manifest, chdir: true, args: []string{"manifest"}, wantStdout: full},
		{name: "missing manifest", args: []string{"manifest", "--manifest", "nonexistent.json"}, wantCode: 2, stderrHas: "nonexistent.json"},
		{name: "invalid manifest", manifest: `{"name": "demo", "typo": true}`, args: []string{"manifest", "--manifest", "{manifest}"}, wantCode: 2, stderrHas: `unknown field "typo"`},
		{name: "unexpected argument", manifest: testutil.Manifest, args: []string{"manifest", "extra"}, wantCode: 2, stderrHas: `unexpected argument "extra"`},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) { run(t, tc) })
	}
}

func TestClassify(t *testing.T) {
	testcases := []testcase{
		{name: "stable", args: []string{"classify", "v1.2.3"}, wantStdout: "track=stable\nprerelease=false\ndist_tag=latest\n"},
		{name: "alpha", args: []string{"classify", "v1.2.3-alpha.2"}, wantStdout: "track=alpha\nprerelease=true\ndist_tag=alpha\n"},
		{name: "beta", args: []string{"classify", "v1.2.3-beta.1"}, wantStdout: "track=beta\nprerelease=true\ndist_tag=beta\n"},
		{name: "rc", args: []string{"classify", "v0.0.1-rc.10"}, wantStdout: "track=rc\nprerelease=true\ndist_tag=rc\n"},
		{name: "unknown prerelease is rejected, not stable", args: []string{"classify", "v1.2.3-nightly.20261110"}, wantCode: 2, stderrHas: "unknown prerelease"},
		{name: "ci version is rejected, not stable", args: []string{"classify", "v0.0.0-ci"}, wantCode: 2, stderrHas: "unknown prerelease"},
		{name: "prerelease number zero", args: []string{"classify", "v1.2.3-rc.0"}, wantCode: 2, stderrHas: "N of 1 or more"},
		{name: "missing v", args: []string{"classify", "1.2.3"}, wantCode: 2, stderrHas: "must start with v"},
		{name: "build metadata", args: []string{"classify", "v1.2.3+abc"}, wantCode: 2, stderrHas: "build metadata"},
		{name: "not a version", args: []string{"classify", "vlatest"}, wantCode: 2, stderrHas: "not a valid semantic version"},
		{name: "manifest flag is accepted and ignored", args: []string{"classify", "v1.2.3", "--manifest", "x"}, wantStdout: "track=stable\nprerelease=false\ndist_tag=latest\n"},
		{name: "missing tag", args: []string{"classify"}, wantCode: 2, stderrHas: "missing required argument <tag>"},
		{name: "extra argument", args: []string{"classify", "v1.2.3", "v1.2.4"}, wantCode: 2, stderrHas: `unexpected argument "v1.2.4"`},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) { run(t, tc) })
	}
}

func TestDistRejections(t *testing.T) {
	testcases := []testcase{
		{name: "missing version", manifest: testutil.Manifest, args: []string{"dist", "--manifest", "{manifest}"}, wantCode: 2, stderrHas: "must be a v-prefixed semantic version"},
		{name: "version without v", manifest: testutil.Manifest, args: []string{"dist", "--manifest", "{manifest}", "--version", "1.2.3"}, wantCode: 2, stderrHas: `version "1.2.3" must be`},
		{name: "version not semver", manifest: testutil.Manifest, args: []string{"dist", "--manifest", "{manifest}", "--version", "v1.2"}, wantCode: 2, stderrHas: `version "v1.2" must be`},
		{name: "version with build metadata", manifest: testutil.Manifest, args: []string{"dist", "--manifest", "{manifest}", "--version", "v1.2.3+abc"}, wantCode: 2, stderrHas: "build metadata"},
		{name: "non-empty output directory", manifest: testutil.Manifest, args: []string{"dist", "--manifest", "{manifest}", "--version", "v1.2.3", "--out", "{dir}"}, wantCode: 2, stderrHas: "is not empty"},
		{name: "version with ldflags injection", manifest: testutil.Manifest, args: []string{"dist", "--manifest", "{manifest}", "--version", "v1.0.0 -X x=y"}, wantCode: 2, stderrHas: "must be"},
		{name: "missing manifest", args: []string{"dist", "--version", "v1.2.3", "--manifest", "nonexistent.json"}, wantCode: 2, stderrHas: "nonexistent.json"},
		{name: "invalid manifest", manifest: `{"name": "demo", "typo": true}`, args: []string{"dist", "--version", "v1.2.3", "--manifest", "{manifest}"}, wantCode: 2, stderrHas: `unknown field "typo"`},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) { run(t, tc) })
	}
}

func TestGen(t *testing.T) {
	snapshot := func(t *testing.T, dir string) map[string]string {
		t.Helper()
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		files := map[string]string{}
		for _, e := range entries {
			content, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			info, err := e.Info()
			if err != nil {
				t.Fatal(err)
			}
			files[e.Name()] = info.Mode().Perm().String() + " " + string(content)
		}
		return files
	}
	testcases := []struct {
		name      string
		generated bool              // run gen before the case
		edits     map[string]string // files rewritten before the case
		chmod     map[string]os.FileMode
		args      []string
		wantCode  int
		stderrHas string
		wantFresh bool // files match a fresh gen afterwards; otherwise the directory is untouched
	}{
		{name: "writes the files", args: []string{"gen"}, wantFresh: true},
		{name: "overwrites a drifted file", generated: true, edits: map[string]string{"install.sh": "edited\n"}, args: []string{"gen"}, wantFresh: true},
		{name: "check passes on fresh files", generated: true, args: []string{"gen", "--check"}, wantFresh: true},
		{name: "check fails on a drifted file", generated: true, edits: map[string]string{"install.sh": "edited\n"}, args: []string{"gen", "--check"}, wantCode: 2, stderrHas: "out of date: install.sh;"},
		{name: "check fails on a file that lost its executable bit", generated: true, chmod: map[string]os.FileMode{"install.sh": 0o644}, args: []string{"gen", "--check"}, wantCode: 2, stderrHas: "out of date: install.sh;"},
		{name: "overwrites the mode of an unchanged file", generated: true, chmod: map[string]os.FileMode{"install.sh": 0o644}, args: []string{"gen"}, wantFresh: true},
		{name: "check fails on missing files and writes nothing", args: []string{"gen", "--check"}, wantCode: 2, stderrHas: "out of date: install.sh, install.ps1;"},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.chmod) > 0 && runtime.GOOS == "windows" {
				t.Skip("windows has no executable bit")
			}
			manifest := testutil.WriteManifest(t, testutil.Manifest)
			dir, fresh := t.TempDir(), t.TempDir()
			withTarget := func(args []string, out string) []string {
				return slices.Concat(args, []string{"--manifest", filepath.Join(manifest, "spine.json"), "--out", out})
			}
			if devcmd.Run(withTarget([]string{"gen"}, fresh), io.Discard, io.Discard) != 0 {
				t.Fatal("reference gen failed")
			}
			if tc.generated && devcmd.Run(withTarget([]string{"gen"}, dir), io.Discard, io.Discard) != 0 {
				t.Fatal("initial gen failed")
			}
			for name, content := range tc.edits {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			for name, mode := range tc.chmod {
				if err := os.Chmod(filepath.Join(dir, name), mode); err != nil {
					t.Fatal(err)
				}
			}
			before := snapshot(t, dir)

			var stderr bytes.Buffer
			code := devcmd.Run(withTarget(tc.args, dir), io.Discard, &stderr)

			if code != tc.wantCode {
				t.Errorf("exit code = %d, want %d (stderr %q)", code, tc.wantCode, stderr.String())
			}
			if !strings.Contains(stderr.String(), tc.stderrHas) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tc.stderrHas)
			}
			want := before
			if tc.wantFresh {
				want = snapshot(t, fresh)
			}
			if got := snapshot(t, dir); !maps.Equal(got, want) {
				t.Errorf("files in %s = %v, want %v", dir, keys(got), keys(want))
			}
		})
	}
}

func keys(m map[string]string) []string {
	return slices.Sorted(maps.Keys(m))
}
