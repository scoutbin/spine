package render_test

import (
	"cmp"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/scoutbin/spine/internal/manifest"
	"github.com/scoutbin/spine/internal/render"
	"github.com/scoutbin/spine/test/testutil"
)

func formulaOf(t *testing.T, tool manifest.Tool, r render.Release) string {
	t.Helper()
	files, err := render.Homebrew(tool, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Path != "demo.rb" || files[0].Mode != 0o644 {
		t.Fatalf("files = %+v, want one demo.rb with mode 0644", files)
	}
	return string(files[0].Content)
}

func stableRelease() render.Release {
	return render.Release{Tag: "v1.2.3", Checksums: []byte(testutil.Checksums("v1.2.3")), Generator: "v0.0.0-test"}
}

func requireRuby(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("ruby")
	if err != nil {
		t.Skip("ruby is not installed")
	}
	return path
}

func rubySyntaxCheck(t *testing.T, formula string) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "demo.rb")
	if err := os.WriteFile(file, []byte(formula), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(requireRuby(t), "-c", file).CombinedOutput(); err != nil {
		t.Errorf("ruby -c: %v\n%s", err, out)
	}
}

func TestHomebrewGolden(t *testing.T) {
	got := formulaOf(t, testutil.Tool(), stableRelease())
	if *update {
		if err := os.WriteFile(homebrewGolden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(homebrewGolden)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("formula differs from %s; run go test ./test/render -update and review the diff", homebrewGolden)
	}
	for _, word := range []string{"canary", "m10rten"} {
		if strings.Contains(strings.ToLower(got), word) {
			t.Errorf("formula contains %q", word)
		}
	}
	rubySyntaxCheck(t, got)
}

const homebrewGolden = "testdata/homebrew.rb.golden"

func TestHomebrewFreeTextEscaping(t *testing.T) {
	testcases := []struct {
		name  string
		value string
		want  string // the Ruby string literal
	}{
		{name: "plain", value: "Demonstrate the spine", want: `"Demonstrate the spine"`},
		{name: "printable non-ASCII", value: "café ☕", want: `"café ☕"`},
		{name: "quote breakout", value: `x"; system("exit 1"); "`, want: `"x\"; system(\"exit 1\"); \""`},
		{name: "interpolation openers", value: `#{system("exit 1")} #$0 #@x`, want: `"\#{system(\"exit 1\")} \#$0 \#@x"`},
		{name: "hash that opens nothing", value: "C# tool #", want: `"C# tool #"`},
		{name: "trailing backslash", value: `x\`, want: `"x\\"`},
		{name: "backslash before an interpolation opener", value: `\#{x}`, want: `"\\\#{x}"`},
		{name: "newline followed by code", value: "x\nend\nsystem(\"exit 1\")\n", want: `"x\u{a}end\u{a}system(\"exit 1\")\u{a}"`},
		{name: "heredoc opener", value: "<<~EOS\nx\nEOS", want: `"<<~EOS\u{a}x\u{a}EOS"`},
		{name: "carriage return", value: "x\ry", want: `"x\u{d}y"`},
		{name: "unicode line and paragraph separators", value: "x y z", want: `"x\u{2028}y\u{2029}z"`},
		{name: "next line control", value: "x\u0085y", want: `"x\u{85}y"`},
		{name: "null and tab", value: "x\x00\ty", want: `"x\u{0}\u{9}y"`},
		{name: "unprintable character outside the BMP", value: "x\U000E0001y", want: `"x\u{e0001}y"`},
		{name: "invalid UTF-8 becomes U+FFFD", value: "x\xffy", want: "\"x\uFFFDy\""},
	}
	linesOf := func(formula string) []string { return strings.Split(formula, "\n") }
	lineWith := func(lines []string, prefix string) int {
		return slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(l, prefix) })
	}
	withoutDesc := func(lines []string) []string {
		return slices.DeleteFunc(slices.Clone(lines), func(l string) bool { return strings.HasPrefix(l, "  desc ") })
	}
	baseline := linesOf(formulaOf(t, testutil.Tool(), stableRelease()))

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			tool := testutil.Tool()
			tool.Summary = tc.value
			formula := formulaOf(t, tool, stableRelease())
			lines := linesOf(formula)

			if len(lines) != len(baseline) {
				t.Fatalf("formula has %d lines, want %d", len(lines), len(baseline))
			}
			if !slices.Equal(withoutDesc(lines), withoutDesc(baseline)) {
				t.Error("formula differs from the baseline outside the desc line")
			}
			line := lines[lineWith(lines, "  desc ")]
			if line != "  desc "+tc.want {
				t.Errorf("desc line = %q, want %q", line, "  desc "+tc.want)
			}
			rubySyntaxCheck(t, formula)

			cmd := exec.Command(requireRuby(t), "-e", "$stdout.binmode; def desc(s); print s; end; eval(ARGV[0])", strings.TrimSpace(line))
			cmd.Dir = t.TempDir()
			out, err := cmd.Output()
			if want := strings.ToValidUTF8(tc.value, "\uFFFD"); err != nil || string(out) != want {
				t.Errorf("ruby evaluates %s to %q (err %v), want %q", line, out, err, want)
			}
		})
	}
}

func TestHomebrewURLs(t *testing.T) {
	testcases := []struct {
		name    string
		baseURL string
		want    string // the Ruby string literal content before "/<archive>"
	}{
		{name: "github release", want: `https://github.com/scoutbin/demo/releases/download/v1.2.3`},
		{name: "base url replaces the release URL", baseURL: "http://127.0.0.1:8080/v1.2.3", want: `http://127.0.0.1:8080/v1.2.3`},
		{name: "trailing slashes are dropped", baseURL: "http://127.0.0.1:8080//", want: `http://127.0.0.1:8080`},
		{name: "hostile base url", baseURL: `http://x/"#{system("id")}\`, want: `http://x/\"\#{system(\"id\")}\\`},
	}
	archives := []string{"darwin_arm64.tar.gz", "darwin_amd64.tar.gz", "linux_arm64.tar.gz", "linux_amd64.tar.gz"}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			r := stableRelease()
			r.BaseURL = tc.baseURL
			formula := formulaOf(t, testutil.Tool(), r)

			var got []string
			for _, line := range strings.Split(formula, "\n") {
				if rest, ok := strings.CutPrefix(line, "      url "); ok {
					got = append(got, rest)
				}
			}
			var want []string
			for _, a := range archives {
				want = append(want, `"`+tc.want+`/demo_1.2.3_`+a+`"`)
			}
			if !slices.Equal(got, want) {
				t.Errorf("url lines = %q, want %q", got, want)
			}
			rubySyntaxCheck(t, formula)
		})
	}
}

func TestHomebrewRejections(t *testing.T) {
	testcases := []struct {
		name      string
		tag       string
		checksums string // the checksums of v1.2.3 when empty
		license   string // the fixture license when empty
		disabled  bool
		wantErr   string
	}{
		{name: "rc", tag: "v1.2.3-rc.1", wantErr: "is a rc release: Homebrew carries stable releases only"},
		{name: "alpha", tag: "v1.2.3-alpha.1", wantErr: "is a alpha release"},
		{name: "unknown prerelease", tag: "v1.2.3-nightly.1", wantErr: "unknown prerelease"},
		{name: "tag without v", tag: "1.2.3", wantErr: "must start with v"},
		{name: "empty tag", tag: "", wantErr: "must start with v"},
		{name: "build metadata", tag: "v1.2.3+abc", wantErr: "build metadata"},
		{name: "archive missing from the checksums", tag: "v1.2.3", checksums: testutil.ChecksumsWithout(testutil.Checksums("v1.2.3"), "demo_1.2.3_linux_arm64.tar.gz"), wantErr: "no checksum listed for demo_1.2.3_linux_arm64.tar.gz"},
		{name: "checksums of another version", tag: "v1.2.4", wantErr: "no checksum listed for demo_1.2.4_"},
		{name: "empty checksums", tag: "v1.2.3", checksums: "\n", wantErr: "no checksum listed"},
		{name: "malformed checksums", tag: "v1.2.3", checksums: "not a checksum line\n", wantErr: "checksums line 1: malformed"},
		{name: "license with OR", tag: "v1.2.3", license: "MIT OR Apache-2.0", wantErr: `license "MIT OR Apache-2.0" is an expression: Homebrew needs a single SPDX identifier`},
		{name: "license with WITH", tag: "v1.2.3", license: "GPL-2.0-only WITH Classpath-exception-2.0", wantErr: "Homebrew needs a single SPDX identifier"},
		{name: "parenthesised license", tag: "v1.2.3", license: "(MIT)", wantErr: "Homebrew needs a single SPDX identifier"},
		{name: "homebrew channel disabled", tag: "v1.2.3", disabled: true, wantErr: "homebrew channel is not enabled"},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			tool := testutil.Tool()
			tool.License = cmp.Or(tc.license, tool.License)
			if tc.disabled {
				tool.Channels.Homebrew.Tap = ""
			}
			files, err := render.Homebrew(tool, render.Release{Tag: tc.tag, Checksums: []byte(cmp.Or(tc.checksums, testutil.Checksums("v1.2.3")))})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %v, want it to contain %q", err, tc.wantErr)
			}
			if files != nil {
				t.Errorf("files = %v, want none", files)
			}
		})
	}
}
