package render_test

import (
	"archive/zip"
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/scoutbin/spine/internal/manifest"
	"github.com/scoutbin/spine/internal/render"
	"github.com/scoutbin/spine/test/testutil"
)

const npmGoldenDir = "testdata/npm"

var npmFiles = []struct {
	path string
	mode os.FileMode
}{
	{"package.json", 0o644},
	{"install.js", 0o644},
	{"launcher.js", 0o755},
}

func npmOf(t *testing.T, tool manifest.Tool, r render.Release) map[string]string {
	t.Helper()
	files, err := render.Npm(tool, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != len(npmFiles) {
		t.Fatalf("rendered %d files, want %d", len(files), len(npmFiles))
	}
	out := make(map[string]string)
	for i, want := range npmFiles {
		if files[i].Path != want.path || files[i].Mode != want.mode {
			t.Errorf("file %d = %s (%v), want %s (%v)", i, files[i].Path, files[i].Mode, want.path, want.mode)
		}
		out[files[i].Path] = string(files[i].Content)
	}
	return out
}

func requireNode(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	return path
}

func nodeSyntaxCheck(t *testing.T, name, source string) {
	t.Helper()
	file := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(file, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(requireNode(t), "--check", file).CombinedOutput(); err != nil {
		t.Errorf("node --check %s: %v\n%s", name, err, out)
	}
}

func evalJSLiteral(t *testing.T, literal string) string {
	t.Helper()
	cmd := exec.Command(requireNode(t), "-e", "process.stdout.write(eval(process.argv[1]))", literal)
	cmd.Dir = t.TempDir()
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("node cannot evaluate %s: %v", literal, err)
	}
	return string(out)
}

func differingLines(t *testing.T, got, baseline string) []int {
	t.Helper()
	g, b := strings.Split(got, "\n"), strings.Split(baseline, "\n")
	if len(g) != len(b) {
		t.Fatalf("file has %d lines, want %d", len(g), len(b))
	}
	var differing []int
	for i := range g {
		if g[i] != b[i] {
			differing = append(differing, i)
		}
	}
	return differing
}

func TestNpmGolden(t *testing.T) {
	got := npmOf(t, testutil.Tool(), stableRelease())
	for _, f := range npmFiles {
		t.Run(f.path, func(t *testing.T) {
			golden := filepath.Join(npmGoldenDir, f.path+".golden")
			if *update {
				if err := os.WriteFile(golden, []byte(got[f.path]), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatal(err)
			}
			if got[f.path] != string(want) {
				t.Errorf("%s differs from %s; run go test ./test/render -update and review the diff", f.path, golden)
			}
			for _, word := range []string{"canary", "m10rten"} {
				if strings.Contains(strings.ToLower(got[f.path]), word) {
					t.Errorf("%s contains %q", f.path, word)
				}
			}
			if strings.HasSuffix(f.path, ".js") {
				nodeSyntaxCheck(t, f.path, got[f.path])
			}
		})
	}
}

func TestNpmTracks(t *testing.T) {
	testcases := []struct {
		tag     string
		version string
	}{
		{tag: "v1.2.3", version: "1.2.3"},
		{tag: "v1.2.3-rc.1", version: "1.2.3-rc.1"},
		{tag: "v1.2.3-beta.2", version: "1.2.3-beta.2"},
		{tag: "v1.2.3-alpha.10", version: "1.2.3-alpha.10"},
	}
	for _, tc := range testcases {
		t.Run(tc.tag, func(t *testing.T) {
			files := npmOf(t, testutil.Tool(), render.Release{Tag: tc.tag, Checksums: []byte(testutil.Checksums(tc.tag)), Generator: "v0.0.0-test"})

			var pkg map[string]any
			if err := json.Unmarshal([]byte(files["package.json"]), &pkg); err != nil {
				t.Fatal(err)
			}
			if pkg["version"] != tc.version {
				t.Errorf("package.json version = %v, want %s", pkg["version"], tc.version)
			}
			for _, want := range []string{
				`const VERSION = "` + tc.version + `";`,
				`const RELEASE_URL = "https://github.com/scoutbin/demo/releases/download/` + tc.tag + `";`,
				fmt.Sprintf(`["demo_%s_linux_amd64.tar.gz", "%064x"],`, tc.version, 3),
				fmt.Sprintf(`["demo_%s_windows_arm64.zip", "%064x"],`, tc.version, 6),
			} {
				if !strings.Contains(files["install.js"], want) {
					t.Errorf("install.js does not contain %q", want)
				}
			}
		})
	}
}

func TestNpmBin(t *testing.T) {
	testcases := []struct {
		name  string
		alias string
		want  map[string]string
	}{
		{name: "without alias", want: map[string]string{"demo": "launcher.js"}},
		{name: "with alias", alias: "dm", want: map[string]string{"demo": "launcher.js", "dm": "launcher.js"}},
	}
	baseline := npmOf(t, testutil.Tool(), stableRelease())
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			tool := testutil.Tool()
			tool.Alias = tc.alias
			files := npmOf(t, tool, stableRelease())

			var pkg struct {
				Bin map[string]string `json:"bin"`
			}
			if err := json.Unmarshal([]byte(files["package.json"]), &pkg); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(pkg.Bin, tc.want) {
				t.Errorf("bin = %v, want %v", pkg.Bin, tc.want)
			}
			if files["launcher.js"] != baseline["launcher.js"] {
				t.Error("launcher.js depends on the alias")
			}
		})
	}
}

// hostileValues are strings that would break out of a JavaScript string literal
// or a line comment if they were embedded as they are.
var hostileValues = []struct {
	name  string
	value string
	want  string // the JavaScript string literal of value
}{
	{name: "plain", value: "https://mirror.example/dl", want: `"https://mirror.example/dl"`},
	{name: "quote breakout", value: `x"; process.exit(1); "`, want: `"x\"; process.exit(1); \""`},
	{name: "template literal syntax", value: "`${process.exit(1)}`", want: "\"`${process.exit(1)}`\""},
	{name: "trailing backslash", value: `x\`, want: `"x\\"`},
	{name: "newline followed by code", value: "x\nprocess.exit(1)\ny", want: `"x\nprocess.exit(1)\ny"`},
	{name: "line and paragraph separators", value: "x\u2028y\u2029z", want: `"x\u2028y\u2029z"`},
	{name: "script element end", value: "</script>", want: `"\u003c/script\u003e"`},
	{name: "null and tab", value: "x\x00\ty", want: `"x\u0000\ty"`},
	{name: "invalid UTF-8 becomes U+FFFD", value: "x\xffy", want: `"x\ufffdy"`},
}

func TestNpmSummaryEscaping(t *testing.T) {
	baseline := npmOf(t, testutil.Tool(), stableRelease())
	var baselinePkg map[string]any
	if err := json.Unmarshal([]byte(baseline["package.json"]), &baselinePkg); err != nil {
		t.Fatal(err)
	}
	delete(baselinePkg, "description")

	for _, tc := range hostileValues {
		t.Run(tc.name, func(t *testing.T) {
			tool := testutil.Tool()
			tool.Summary = tc.value
			files := npmOf(t, tool, stableRelease())

			var pkg map[string]any
			if err := json.Unmarshal([]byte(files["package.json"]), &pkg); err != nil {
				t.Fatalf("package.json is not valid JSON: %v", err)
			}
			if want := strings.ToValidUTF8(tc.value, "�"); pkg["description"] != want {
				t.Errorf("description = %q, want %q", pkg["description"], want)
			}
			delete(pkg, "description")
			if !reflect.DeepEqual(pkg, baselinePkg) {
				t.Error("package.json differs from the baseline outside the description")
			}
			for _, name := range []string{"install.js", "launcher.js"} {
				if files[name] != baseline[name] {
					t.Errorf("%s depends on the summary", name)
				}
			}
		})
	}
}

func TestNpmBaseURLEscaping(t *testing.T) {
	const prefix = "const RELEASE_URL = "
	baseline := npmOf(t, testutil.Tool(), stableRelease())
	for _, tc := range hostileValues {
		t.Run(tc.name, func(t *testing.T) {
			r := stableRelease()
			r.BaseURL = tc.value
			files := npmOf(t, testutil.Tool(), r)

			diff := differingLines(t, files["install.js"], baseline["install.js"])
			lines := strings.Split(files["install.js"], "\n")
			if len(diff) != 1 || !strings.HasPrefix(lines[diff[0]], prefix) {
				t.Fatalf("install.js differs from the baseline on lines %v, want only the RELEASE_URL line", diff)
			}
			if want := prefix + tc.want + ";"; lines[diff[0]] != want {
				t.Errorf("line = %s, want %s", lines[diff[0]], want)
			}
			if files["launcher.js"] != baseline["launcher.js"] || files["package.json"] != baseline["package.json"] {
				t.Error("launcher.js or package.json depends on the base URL")
			}
			nodeSyntaxCheck(t, "install.js", files["install.js"])
			if got, want := evalJSLiteral(t, tc.want), strings.ToValidUTF8(tc.value, "�"); got != want {
				t.Errorf("node evaluates the literal to %q, want %q", got, want)
			}
		})
	}
}

func TestNpmGeneratorHeader(t *testing.T) {
	testcases := []struct {
		name      string
		generator string
		want      string
	}{
		{name: "version", generator: "v1.2.3", want: "v1.2.3"},
		{name: "newline followed by code", generator: "v1\nprocess.exit(1)", want: "v1 process.exit(1)"},
		{name: "carriage return", generator: "v1\rprocess.exit(1)", want: "v1 process.exit(1)"},
		{name: "line and paragraph separators", generator: "v1\u2028process.exit(1)\u2029x", want: "v1 process.exit(1) x"},
	}
	baseline := npmOf(t, testutil.Tool(), stableRelease())
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			r := stableRelease()
			r.Generator = tc.generator
			files := npmOf(t, testutil.Tool(), r)

			for _, name := range []string{"install.js", "launcher.js"} {
				diff := differingLines(t, files[name], baseline[name])
				lines := strings.Split(files[name], "\n")
				if len(diff) != 1 || lines[diff[0]] != "// Generated by spine "+tc.want+". Do not edit." {
					t.Errorf("%s differs from the baseline on lines %v, want only the header line with %q", name, diff, tc.want)
				}
				nodeSyntaxCheck(t, name, files[name])
			}
		})
	}
}

func TestNpmRejections(t *testing.T) {
	testcases := []struct {
		name      string
		tag       string
		checksums string // the checksums of v1.2.3 when empty
		disabled  bool
		wantErr   string
	}{
		{name: "unknown prerelease", tag: "v1.2.3-nightly.1", wantErr: "unknown prerelease"},
		{name: "prerelease without a number", tag: "v1.2.3-rc", wantErr: "must look like v1.2.3-rc.N"},
		{name: "tag without v", tag: "1.2.3", wantErr: "must start with v"},
		{name: "empty tag", tag: "", wantErr: "must start with v"},
		{name: "build metadata", tag: "v1.2.3+abc", wantErr: "build metadata"},
		{name: "windows archive missing from the checksums", tag: "v1.2.3", checksums: testutil.ChecksumsWithout(testutil.Checksums("v1.2.3"), "demo_1.2.3_windows_arm64.zip"), wantErr: "no checksum listed for demo_1.2.3_windows_arm64.zip"},
		{name: "checksums of another version", tag: "v1.2.4", wantErr: "no checksum listed for demo_1.2.4_"},
		{name: "malformed checksums", tag: "v1.2.3", checksums: "not a checksum line\n", wantErr: "checksums line 1: malformed"},
		{name: "npm channel disabled", tag: "v1.2.3", disabled: true, wantErr: "npm channel is not enabled"},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			tool := testutil.Tool()
			if tc.disabled {
				tool.Channels.Npm.Package = ""
			}
			files, err := render.Npm(tool, render.Release{Tag: tc.tag, Checksums: []byte(cmp.Or(tc.checksums, testutil.Checksums("v1.2.3")))})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %v, want it to contain %q", err, tc.wantErr)
			}
			if files != nil {
				t.Errorf("files = %v, want none", files)
			}
		})
	}
}

// TestNpmInstallScript runs the node:test suite against a package rendered with
// the checksums of a mirror of release archives.
func TestNpmInstallScript(t *testing.T) {
	node := requireNode(t)
	tool := testutil.Tool()
	root := t.TempDir()
	mirror := filepath.Join(root, "mirror")
	if err := os.MkdirAll(mirror, 0o755); err != nil {
		t.Fatal(err)
	}
	var checksums strings.Builder
	for _, target := range tool.Targets() {
		name := tool.ArchiveName("v1.2.3", target.GOOS, target.GOARCH)
		archive := releaseArchive(t, tool.BinaryName(target.GOOS), fmt.Sprintf("%s %s binary", target.GOOS, target.GOARCH))
		if err := os.WriteFile(filepath.Join(mirror, name), archive, 0o644); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&checksums, "%x  %s\n", sha256.Sum256(archive), name)
	}

	pkg := filepath.Join(root, "pkg")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	for path, content := range npmOf(t, tool, render.Release{Tag: "v1.2.3", Checksums: []byte(checksums.String()), Generator: "v0.0.0-test"}) {
		if err := os.WriteFile(filepath.Join(pkg, path), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	suite, err := filepath.Abs(filepath.Join(npmGoldenDir, "install.test.js"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(node, "--test", suite)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "SPINE_NPM_PACKAGE="+pkg, "SPINE_NPM_MIRROR="+mirror)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("node --test: %v\n%s", err, out)
	}
}

func releaseArchive(t *testing.T, binary, content string) []byte {
	t.Helper()
	if !strings.HasSuffix(binary, ".exe") {
		return testutil.TarGz(t, testutil.TarEntry{Name: binary, Body: content})
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(binary)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
