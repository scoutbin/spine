package devcmd_test

import (
	"bytes"
	"cmp"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

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
		full = "contract=2\nname=demo\n" +
			"homebrew=true\nhomebrew_tap=scoutbin/homebrew-tap\nhomebrew_formula=scoutbin/tap/demo\n" +
			"npm=true\nnpm_package=@scoutbin/demo\n" +
			"action=true\n"
		homebrewOnly = "contract=2\nname=demo\n" +
			"homebrew=true\nhomebrew_tap=scoutbin/homebrew-tap\nhomebrew_formula=scoutbin/tap/demo\n" +
			"npm=false\nnpm_package=\n" +
			"action=false\n"
		npmOnly = "contract=2\nname=demo\n" +
			"homebrew=false\nhomebrew_tap=\nhomebrew_formula=\n" +
			"npm=true\nnpm_package=@scoutbin/demo\n" +
			"action=false\n"
		minimal = "contract=2\nname=demo\n" +
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
		if os.IsNotExist(err) {
			return nil
		}
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
		manifest  string            // spine.json content; testutil.Manifest when empty
		nested    string            // output path below the temp directory, which does not exist beforehand
		generated bool              // run gen before the case
		edits     map[string]string // files rewritten before the case
		chmod     map[string]os.FileMode
		args      []string
		wantCode  int
		stderrHas string
		wantFresh bool     // files match a fresh gen afterwards; otherwise the directory is untouched
		wantFiles []string // file names in the directory afterwards, when set
		wantNoDir bool     // the output directory still does not exist afterwards
	}{
		{name: "writes the files", args: []string{"gen"}, wantFresh: true, wantFiles: []string{"action.yml", "install.ps1", "install.sh"}},
		{name: "writes no action.yml when the action channel is disabled", manifest: testutil.MinimalManifest, args: []string{"gen"}, wantFresh: true, wantFiles: []string{"install.ps1", "install.sh"}},
		{name: "check passes without action.yml when the action channel is disabled", manifest: testutil.MinimalManifest, generated: true, args: []string{"gen", "--check"}, wantFresh: true, wantFiles: []string{"install.ps1", "install.sh"}},
		{name: "check fails on a leftover action.yml when the action channel is disabled", manifest: testutil.MinimalManifest, generated: true, edits: map[string]string{"action.yml": "leftover\n"}, args: []string{"gen", "--check"}, wantCode: 2, stderrHas: "unexpected: action.yml (action channel disabled)", wantFiles: []string{"action.yml", "install.ps1", "install.sh"}},
		{name: "gen keeps a leftover action.yml when the action channel is disabled", manifest: testutil.MinimalManifest, generated: true, edits: map[string]string{"action.yml": "leftover\n"}, args: []string{"gen"}, wantFiles: []string{"action.yml", "install.ps1", "install.sh"}},
		{name: "check fails on a drifted action.yml", generated: true, edits: map[string]string{"action.yml": "edited\n"}, args: []string{"gen", "--check"}, wantCode: 2, stderrHas: "out of date: action.yml;"},
		{name: "overwrites a drifted file", generated: true, edits: map[string]string{"install.sh": "edited\n"}, args: []string{"gen"}, wantFresh: true},
		{name: "check passes on fresh files", generated: true, args: []string{"gen", "--check"}, wantFresh: true},
		{name: "check fails on a drifted file", generated: true, edits: map[string]string{"install.sh": "edited\n"}, args: []string{"gen", "--check"}, wantCode: 2, stderrHas: "out of date: install.sh;"},
		{name: "check fails on a file that lost its executable bit", generated: true, chmod: map[string]os.FileMode{"install.sh": 0o644}, args: []string{"gen", "--check"}, wantCode: 2, stderrHas: "out of date: install.sh;"},
		{name: "overwrites the mode of an unchanged file", generated: true, chmod: map[string]os.FileMode{"install.sh": 0o644}, args: []string{"gen"}, wantFresh: true},
		{name: "creates a missing output directory", nested: "a/b", args: []string{"gen"}, wantFresh: true, wantFiles: []string{"action.yml", "install.ps1", "install.sh"}},
		{name: "check fails on a missing output directory and does not create it", nested: "a/b", args: []string{"gen", "--check"}, wantCode: 2, stderrHas: "out of date: install.sh, install.ps1, action.yml;", wantNoDir: true},
		{name: "check fails on missing files and writes nothing", args: []string{"gen", "--check"}, wantCode: 2, stderrHas: "out of date: install.sh, install.ps1, action.yml;"},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.chmod) > 0 && runtime.GOOS == "windows" {
				t.Skip("windows has no executable bit")
			}
			manifest := testutil.WriteManifest(t, cmp.Or(tc.manifest, testutil.Manifest))
			dir, fresh := t.TempDir(), t.TempDir()
			dir = filepath.Join(dir, tc.nested)
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
			if tc.wantFiles != nil {
				if got := keys(snapshot(t, dir)); !slices.Equal(got, tc.wantFiles) {
					t.Errorf("files = %v, want %v", got, tc.wantFiles)
				}
			}
			if _, err := os.Stat(dir); tc.wantNoDir && !os.IsNotExist(err) {
				t.Errorf("output directory %s exists (stat error %v), want it missing", dir, err)
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

func TestPackage(t *testing.T) {
	const noChecksums = "-"
	const release = "https://github.com/scoutbin/demo/releases/download/v1.2.3/"
	wantFormula := []string{
		`class Demo < Formula`,
		`version "1.2.3"`,
		`url "` + release + `demo_1.2.3_darwin_arm64.tar.gz"`,
		`sha256 "` + fmt.Sprintf("%064x", 2) + `"`,
		`url "` + release + `demo_1.2.3_linux_amd64.tar.gz"`,
		`sha256 "` + fmt.Sprintf("%064x", 3) + `"`,
	}
	testcases := []struct {
		name      string
		checksums string // the checksums of v1.2.3 when empty, no file when noChecksums
		distDir   string // directory below the working directory that holds the checksums; dist when empty
		args      []string
		wantCode  int
		stderrHas string
		wantPath  string   // file written, relative to the working directory; nothing is written when empty
		wantHas   []string // substrings of the file
	}{
		{name: "renders into dist/packages by default", args: []string{"package", "homebrew", "--tag", "v1.2.3"}, wantPath: "dist/packages/homebrew/demo.rb", wantHas: wantFormula},
		{name: "reads checksums from --dist and writes to --out", distDir: "alt", args: []string{"package", "homebrew", "--tag", "v1.2.3", "--dist", "{dist}", "--out", "{work}/custom"}, wantPath: "custom/homebrew/demo.rb", wantHas: wantFormula},
		{name: "base url replaces the release URL", args: []string{"package", "homebrew", "--tag", "v1.2.3", "--base-url", "http://127.0.0.1:8080/"}, wantPath: "dist/packages/homebrew/demo.rb", wantHas: []string{`url "http://127.0.0.1:8080/demo_1.2.3_darwin_arm64.tar.gz"`}},
		{name: "npm renders package.json", args: []string{"package", "npm", "--tag", "v1.2.3"}, wantPath: "dist/packages/npm/package.json", wantHas: []string{`"name": "@scoutbin/demo"`, `"version": "1.2.3"`}},
		{name: "npm accepts a prerelease tag", checksums: testutil.Checksums("v1.2.3-rc.1"), args: []string{"package", "npm", "--tag", "v1.2.3-rc.1"}, wantPath: "dist/packages/npm/install.js", wantHas: []string{`const VERSION = "1.2.3-rc.1";`, `"demo_1.2.3-rc.1_windows_arm64.zip", "` + fmt.Sprintf("%064x", 6) + `"`}},
		{name: "npm base url replaces the release URL", args: []string{"package", "npm", "--tag", "v1.2.3", "--base-url", "http://127.0.0.1:8080/"}, wantPath: "dist/packages/npm/install.js", wantHas: []string{`const RELEASE_URL = "http://127.0.0.1:8080";`}},
		{name: "npm unknown prerelease", args: []string{"package", "npm", "--tag", "v1.2.3-nightly.1"}, wantCode: 2, stderrHas: "unknown prerelease"},
		{name: "npm archive missing from the checksums", checksums: testutil.ChecksumsWithout(testutil.Checksums("v1.2.3"), "windows_arm64"), args: []string{"package", "npm", "--tag", "v1.2.3"}, wantCode: 2, stderrHas: "no checksum listed for demo_1.2.3_windows_arm64.zip"},
		{name: "missing checksums file", checksums: noChecksums, args: []string{"package", "homebrew", "--tag", "v1.2.3"}, wantCode: 2, stderrHas: "checksums.txt"},
		{name: "archive missing from the checksums", checksums: testutil.ChecksumsWithout(testutil.Checksums("v1.2.3"), "linux_arm64"), args: []string{"package", "homebrew", "--tag", "v1.2.3"}, wantCode: 2, stderrHas: "no checksum listed for demo_1.2.3_linux_arm64.tar.gz"},
		{name: "rc tag", args: []string{"package", "homebrew", "--tag", "v1.2.3-rc.1"}, wantCode: 2, stderrHas: "Homebrew carries stable releases only"},
		{name: "missing tag", args: []string{"package", "homebrew"}, wantCode: 2, stderrHas: "--tag is required"},
		{name: "unknown channel", args: []string{"package", "bogus", "--tag", "v1.2.3"}, wantCode: 2, stderrHas: `unknown channel "bogus"`},
		{name: "missing channel", args: []string{"package", "--tag", "v1.2.3"}, wantCode: 2, stderrHas: "missing required argument <channel>"},
		{name: "missing manifest", args: []string{"package", "homebrew", "--tag", "v1.2.3", "--manifest", "nonexistent.json"}, wantCode: 2, stderrHas: "nonexistent.json"},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			work := t.TempDir()
			t.Chdir(work)
			manifestDir := testutil.WriteManifest(t, testutil.Manifest)
			dist := filepath.Join(work, cmp.Or(tc.distDir, "dist"))
			if err := os.MkdirAll(dist, 0o755); err != nil {
				t.Fatal(err)
			}
			if tc.checksums != noChecksums {
				if err := os.WriteFile(filepath.Join(dist, "checksums.txt"), []byte(cmp.Or(tc.checksums, testutil.Checksums("v1.2.3"))), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			args := []string{"--manifest", filepath.Join(manifestDir, "spine.json")}
			for _, a := range tc.args {
				args = append(args, strings.NewReplacer("{dist}", dist, "{work}", work).Replace(a))
			}

			var stdout, stderr bytes.Buffer
			code := devcmd.Run(args, &stdout, &stderr)

			if code != tc.wantCode {
				t.Errorf("exit code = %d, want %d (stderr %q)", code, tc.wantCode, stderr.String())
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty", stdout.String())
			}
			if !strings.Contains(stderr.String(), tc.stderrHas) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tc.stderrHas)
			}
			if tc.wantPath == "" {
				for _, dir := range []string{"dist/packages", "custom"} {
					if _, err := os.Stat(filepath.Join(work, dir)); !os.IsNotExist(err) {
						t.Errorf("%s exists after a failed run (stat err %v)", dir, err)
					}
				}
				return
			}
			got, err := os.ReadFile(filepath.Join(work, tc.wantPath))
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range tc.wantHas {
				if !strings.Contains(string(got), want) {
					t.Errorf("%s does not contain %q", tc.wantPath, want)
				}
			}
			for _, word := range []string{"canary", "m10rten"} {
				if strings.Contains(strings.ToLower(string(got)), word) {
					t.Errorf("%s contains %q", tc.wantPath, word)
				}
			}
		})
	}
}

func TestDistTags(t *testing.T) {
	const (
		versions = `["1.2.2","1.2.3-alpha.1","1.2.3-beta.1","1.2.3-rc.1","1.2.3"]`
		withRC   = `["1.2.2","1.2.3-alpha.1","1.2.3-beta.1","1.2.3-rc.1","1.2.3","1.3.0-rc.1"]`
		tags     = `{"latest":"1.2.3","alpha":"1.2.3-alpha.1","beta":"1.2.3-beta.1","rc":"1.2.3-rc.1"}`
	)
	testcases := []testcase{
		{name: "stable release moves every prerelease dist-tag", args: []string{"dist-tags", "v1.2.3", "--versions", versions, "--dist-tags", tags}, wantStdout: "alpha=1.2.3\nbeta=1.2.3\nrc=1.2.3\n"},
		{name: "rc release moves the alpha and beta dist-tags only", args: []string{"dist-tags", "v1.3.0-rc.1", "--versions", withRC, "--dist-tags", `{"latest":"1.2.3","alpha":"1.2.3-alpha.1","beta":"1.2.3-beta.1","rc":"1.3.0-rc.1"}`}, wantStdout: "alpha=1.3.0-rc.1\nbeta=1.3.0-rc.1\n"},
		{name: "pushed tag counts before the registry lists it", args: []string{"dist-tags", "v1.2.4", "--versions", versions, "--dist-tags", `{"rc":"1.2.3-rc.1"}`}, wantStdout: "rc=1.2.4\n"},
		{name: "nothing to move prints nothing", args: []string{"dist-tags", "v1.3.0-rc.1", "--versions", withRC, "--dist-tags", `{"latest":"1.2.3","rc":"1.3.0-rc.1"}`}},
		{name: "missing dist-tag is not created", args: []string{"dist-tags", "v1.2.3", "--versions", versions, "--dist-tags", `{"latest":"1.2.3"}`}},
		{name: "unknown prerelease tag", args: []string{"dist-tags", "v1.2.3-nightly.1", "--versions", versions, "--dist-tags", tags}, wantCode: 2, stderrHas: "unknown prerelease"},
		{name: "missing tag", args: []string{"dist-tags", "--versions", versions, "--dist-tags", tags}, wantCode: 2, stderrHas: "missing required argument <tag>"},
		{name: "missing versions", args: []string{"dist-tags", "v1.2.3", "--dist-tags", tags}, wantCode: 2, stderrHas: "--versions must be a JSON array"},
		{name: "single version as a bare string", args: []string{"dist-tags", "v1.2.3", "--versions", `"1.2.3-rc.1"`, "--dist-tags", `{"rc":"1.2.3-rc.1"}`}, wantStdout: "rc=1.2.3\n"},
		{name: "versions is not an array or string", args: []string{"dist-tags", "v1.2.3", "--versions", `{"1.2.3":true}`, "--dist-tags", tags}, wantCode: 2, stderrHas: "--versions must be a JSON array"},
		{name: "missing dist-tags", args: []string{"dist-tags", "v1.2.3", "--versions", versions}, wantCode: 2, stderrHas: "--dist-tags must be a JSON object"},
		{name: "dist-tags is not an object", args: []string{"dist-tags", "v1.2.3", "--versions", versions, "--dist-tags", `["rc"]`}, wantCode: 2, stderrHas: "--dist-tags must be a JSON object"},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) { run(t, tc) })
	}
}

func TestPackageNpmLayout(t *testing.T) {
	work := t.TempDir()
	t.Chdir(work)
	manifestDir := testutil.WriteManifest(t, testutil.Manifest)
	if err := os.MkdirAll("dist", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("dist/checksums.txt", []byte(testutil.Checksums("v1.2.3")), 0o644); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	if code := devcmd.Run([]string{"--manifest", filepath.Join(manifestDir, "spine.json"), "package", "npm", "--tag", "v1.2.3"}, io.Discard, &stderr); code != 0 {
		t.Fatalf("exit code = %d (stderr %q)", code, stderr.String())
	}

	entries, err := os.ReadDir("dist/packages/npm")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	if want := []string{"install.js", "launcher.js", "package.json"}; !slices.Equal(got, want) {
		t.Errorf("package files = %v, want %v", got, want)
	}
	if info, err := os.Stat("dist/packages/npm/launcher.js"); err != nil {
		t.Fatal(err)
	} else if runtime.GOOS != "windows" && info.Mode().Perm() != 0o755 {
		t.Errorf("launcher.js mode = %v, want 0755", info.Mode().Perm())
	}
}

// repo is a git repository in a temp directory that is the working directory.
type repo struct {
	t     *testing.T
	clock time.Time
}

// newGitRepo creates a repository in a temp directory, makes it the working
// directory and leaves its history as, oldest first: initial, add parser
// (v1.0.0), add flags (v1.1.0-rc.1), feature work and main work on two branches
// joined by a merge commit, polish output (HEAD). Every commit adds its own
// file. The user's git config is ignored and the identity is fixed, so commits
// and tags work on a machine with no git setup.
func newGitRepo(t *testing.T) *repo {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	t.Chdir(t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, role := range []string{"AUTHOR", "COMMITTER"} {
		t.Setenv("GIT_"+role+"_NAME", "Test")
		t.Setenv("GIT_"+role+"_EMAIL", "test@example.com")
	}
	r := &repo{t: t, clock: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	r.git("-c", "init.defaultBranch=main", "init")
	r.commit("initial", nil)
	r.commit("add parser", nil)
	r.git("tag", "v1.0.0")
	r.commit("add flags", nil)
	r.git("tag", "v1.1.0-rc.1")
	r.git("checkout", "-b", "feature")
	r.commit("feature work", nil)
	r.git("checkout", "main")
	r.commit("main work", nil)
	r.tick()
	r.git("merge", "--no-ff", "-m", "Merge feature", "feature")
	r.commit("polish output", nil)
	return r
}

func (r *repo) tick() {
	r.clock = r.clock.Add(time.Hour)
	r.t.Setenv("GIT_AUTHOR_DATE", r.clock.Format(time.RFC3339))
	r.t.Setenv("GIT_COMMITTER_DATE", r.clock.Format(time.RFC3339))
}

func (r *repo) git(args ...string) string {
	r.t.Helper()
	out, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// commit commits files, or a file named after the subject when files is nil.
func (r *repo) commit(subject string, files map[string]string) {
	r.t.Helper()
	if files == nil {
		files = map[string]string{subject + ".txt": subject}
	}
	for name, content := range files {
		if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
			r.t.Fatal(err)
		}
		r.git("add", name)
	}
	r.tick()
	r.git("commit", "-m", subject)
}

func (r *repo) state() string {
	changelog, _ := os.ReadFile("CHANGELOG.md")
	return strings.Join([]string{
		r.git("rev-parse", "HEAD"),
		r.git("tag", "--list"),
		r.git("status", "--porcelain"),
		string(changelog),
	}, "\n--\n")
}

func TestChange(t *testing.T) {
	const rc1 = "# Changelog\n\n## v1.1.0-rc.1 - 2026-01-01\n\nFirst candidate.\n\n- add flags\n"
	const rc2 = "# Changelog\n\n## v1.1.0-rc.2 - 2026-01-02\n\nSecond candidate.\n\n- more work\n\n## v1.1.0-rc.1 - 2026-01-01\n\nFirst candidate.\n\n- add flags\n"
	changelogOnly := func(content string) map[string]string { return map[string]string{"CHANGELOG.md": content} }

	successes := []struct {
		name     string
		setup    func(r *repo)
		args     []string
		wantOut  string
		wantFile string // CHANGELOG.md afterwards, with {date} for today
	}{
		{
			name:     "first release has only the message",
			setup:    func(r *repo) { r.git("tag", "-d", "v1.0.0", "v1.1.0-rc.1") },
			args:     []string{"change", "patch", "Initial release."},
			wantOut:  "tag=v0.0.1\nbase=\ncommits=0\npush=git push --atomic origin main v0.0.1\n",
			wantFile: "# Changelog\n\n## v0.0.1 - {date}\n\nInitial release.\n",
		},
		{
			name: "second rc is based on the first and goes above it",
			setup: func(r *repo) {
				r.commit("release v1.1.0-rc.1", changelogOnly(rc1))
				r.git("tag", "-f", "v1.1.0-rc.1")
				r.commit("more work", nil)
			},
			args:     []string{"change", "minor", "Second candidate.", "--pre", "rc"},
			wantOut:  "tag=v1.1.0-rc.2\nbase=v1.1.0-rc.1\ncommits=1\npush=git push --atomic origin main v1.1.0-rc.2\n",
			wantFile: "# Changelog\n\n## v1.1.0-rc.2 - {date}\n\nSecond candidate.\n\n- more work\n\n## v1.1.0-rc.1 - 2026-01-01\n\nFirst candidate.\n\n- add flags\n",
		},
		{
			name: "stable is based on the previous stable and skips the release commits",
			setup: func(r *repo) {
				r.commit("release v1.1.0-rc.1", changelogOnly(rc1))
				r.git("tag", "-f", "v1.1.0-rc.1")
				r.commit("more work", nil)
				r.commit("release v1.1.0-rc.2", changelogOnly(rc2))
				r.git("tag", "v1.1.0-rc.2")
			},
			args:     []string{"change", "minor", "Stable."},
			wantOut:  "tag=v1.1.0\nbase=v1.0.0\ncommits=5\npush=git push --atomic origin main v1.1.0\n",
			wantFile: "# Changelog\n\n## v1.1.0 - {date}\n\nStable.\n\n- more work\n- polish output\n- main work\n- feature work\n- add flags\n\n" + strings.TrimPrefix(rc2, "# Changelog\n\n"),
		},
		{
			name: "untracked files are ignored and left alone",
			setup: func(r *repo) {
				if err := os.WriteFile("scratch.txt", []byte("scratch"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			args:     []string{"change", "minor", "Fix."},
			wantOut:  "tag=v1.1.0\nbase=v1.0.0\ncommits=4\npush=git push --atomic origin main v1.1.0\n",
			wantFile: "",
		},
	}
	for _, tc := range successes {
		t.Run(tc.name, func(t *testing.T) {
			r := newGitRepo(t)
			tc.setup(r)

			var stdout, stderr bytes.Buffer
			code := devcmd.Run(tc.args, &stdout, &stderr)

			if code != 0 {
				t.Fatalf("exit code = %d, want 0 (stderr %q)", code, stderr.String())
			}
			if stdout.String() != tc.wantOut {
				t.Errorf("stdout = %q, want %q", stdout.String(), tc.wantOut)
			}
			tag := strings.TrimPrefix(strings.SplitN(tc.wantOut, "\n", 2)[0], "tag=")
			if got := r.git("log", "-1", "--format=%s"); got != "release "+tag {
				t.Errorf("HEAD subject = %q, want %q", got, "release "+tag)
			}
			if tagged, head := r.git("rev-parse", tag+"^{commit}"), r.git("rev-parse", "HEAD"); tagged != head {
				t.Errorf("tag %s points at %s, want HEAD %s", tag, tagged, head)
			}
			if got := r.git("show", "--name-only", "--format=", "HEAD"); got != "CHANGELOG.md" {
				t.Errorf("release commit changes %q, want only CHANGELOG.md", got)
			}
			if got := r.git("status", "--porcelain", "--untracked-files=no"); got != "" {
				t.Errorf("tracked changes after the run: %q", got)
			}
			if tc.wantFile == "" {
				return
			}
			got, err := os.ReadFile("CHANGELOG.md")
			if err != nil {
				t.Fatal(err)
			}
			if want := strings.ReplaceAll(tc.wantFile, "{date}", time.Now().Format(time.DateOnly)); string(got) != want {
				t.Errorf("CHANGELOG.md = %q, want %q", got, want)
			}
		})
	}

	failures := []struct {
		name      string
		setup     func(r *repo)
		args      []string
		stderrHas string
	}{
		{name: "unknown bump", args: []string{"change", "rc", "Msg."}, stderrHas: `unknown bump "rc"`},
		{name: "unknown pre", args: []string{"change", "minor", "Msg.", "--pre", "nightly"}, stderrHas: `invalid --pre "nightly"`},
		{name: "empty message", args: []string{"change", "minor", "  "}, stderrHas: "the message is empty"},
		{name: "heading in the message", args: []string{"change", "minor", "Intro\n## Oops"}, stderrHas: "must not have a line starting with"},
		{name: "detached HEAD", setup: func(r *repo) { r.git("checkout", "--detach") }, args: []string{"change", "minor", "Msg."}, stderrHas: "HEAD is not on a branch"},
		{
			name: "staged change",
			setup: func(r *repo) {
				if err := os.WriteFile("initial.txt", []byte("edited"), 0o644); err != nil {
					t.Fatal(err)
				}
				r.git("add", "initial.txt")
			},
			args: []string{"change", "minor", "Msg."}, stderrHas: "uncommitted changes",
		},
		{
			name: "unstaged change",
			setup: func(r *repo) {
				if err := os.WriteFile("initial.txt", []byte("edited"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			args: []string{"change", "minor", "Msg."}, stderrHas: "uncommitted changes",
		},
		{name: "patch below a pending prerelease", args: []string{"change", "patch", "Msg."}, stderrHas: "not higher than v1.1.0-rc.1"},
		{name: "next tag below a reachable tag", setup: func(r *repo) { r.git("tag", "v1.2.0-rc.1") }, args: []string{"change", "minor", "Msg.", "--pre", "beta"}, stderrHas: "not higher than v1.2.0-rc.1"},
		{
			name: "next tag exists on another branch",
			setup: func(r *repo) {
				r.git("checkout", "-b", "side")
				r.commit("side work", nil)
				r.git("tag", "v1.1.0")
				r.git("checkout", "main")
			},
			args: []string{"change", "minor", "Msg."}, stderrHas: "tag v1.1.0 already exists",
		},
		{
			name: "section already in the file",
			setup: func(r *repo) {
				r.commit("add changelog", changelogOnly("# Changelog\n\n## v1.1.0 - 2026-01-01\n\nOld.\n"))
			},
			args: []string{"change", "minor", "Msg."}, stderrHas: "already has a section for v1.1.0",
		},
		{name: "no commits since the base", setup: func(r *repo) { r.git("tag", "v1.1.0") }, args: []string{"change", "patch", "Msg."}, stderrHas: "no commits to list for v1.1.1 since v1.1.0"},
		{
			name: "only the changelog changed since the base",
			setup: func(r *repo) {
				r.git("tag", "v1.1.0")
				r.commit("release v1.1.0", changelogOnly("# Changelog\n\n## v1.1.0 - 2026-01-01\n\nOld.\n"))
			},
			args: []string{"change", "patch", "Msg."}, stderrHas: "no commits to list for v1.1.1 since v1.1.0",
		},
	}
	for _, tc := range failures {
		t.Run(tc.name, func(t *testing.T) {
			r := newGitRepo(t)
			if tc.setup != nil {
				tc.setup(r)
			}
			before := r.state()

			var stdout, stderr bytes.Buffer
			code := devcmd.Run(tc.args, &stdout, &stderr)

			if code != 2 {
				t.Errorf("exit code = %d, want 2 (stderr %q)", code, stderr.String())
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty", stdout.String())
			}
			if !strings.Contains(stderr.String(), tc.stderrHas) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tc.stderrHas)
			}
			if after := r.state(); after != before {
				t.Errorf("repository changed:\nbefore %q\nafter  %q", before, after)
			}
		})
	}
}

func TestNotes(t *testing.T) {
	const changelog = "# Changelog\n\n## v1.2.0 - 2026-02-01\n\n- add flags\n- add parser\n\n## v1.2.0-rc.1 - 2026-01-15\n\n- add parser\n\n## v1.1.0 - 2026-01-01\n\n- initial\n"
	testcases := []struct {
		name      string
		tag       string
		file      string // CHANGELOG.md content; no file when empty
		wantCode  int
		wantOut   string
		stderrHas string
	}{
		{name: "section in the middle", tag: "v1.2.0-rc.1", file: changelog, wantOut: "- add parser\n"},
		{name: "stable section does not match the prerelease section below it", tag: "v1.2.0", file: changelog, wantOut: "- add flags\n- add parser\n"},
		{name: "last section", tag: "v1.1.0", file: changelog, wantOut: "- initial\n"},
		{name: "missing section", tag: "v1.3.0", file: changelog, wantCode: 2, stderrHas: "has no section for v1.3.0: run go tool spine change"},
		{name: "missing file", tag: "v1.2.0", wantCode: 2, stderrHas: "CHANGELOG.md not found: run go tool spine change"},
		{name: "invalid tag", tag: "v1.2.0-nightly.1", file: changelog, wantCode: 2, stderrHas: "unknown prerelease"},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			if tc.file != "" {
				if err := os.WriteFile("CHANGELOG.md", []byte(tc.file), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			run(t, testcase{args: []string{"notes", tc.tag}, wantCode: tc.wantCode, wantStdout: tc.wantOut, stderrHas: tc.stderrHas})
		})
	}
}
