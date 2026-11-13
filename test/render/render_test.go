package render_test

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/scoutbin/spine/internal/render"
	"github.com/scoutbin/spine/test/testutil"
)

var update = flag.Bool("update", false, "rewrite the golden files")

const summaryPrefix = "# demo - "

func TestFilesGolden(t *testing.T) {
	testcases := []struct {
		path   string
		mode   os.FileMode
		golden string
	}{
		{path: "install.sh", mode: 0o755, golden: "testdata/install.sh.golden"},
		{path: "install.ps1", mode: 0o644, golden: "testdata/install.ps1.golden"},
		{path: "action.yml", mode: 0o644, golden: "testdata/action.yml.golden"},
	}
	files, err := render.Files(testutil.Tool(), "v0.0.0-test")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != len(testcases) {
		t.Fatalf("rendered %d files, want %d", len(files), len(testcases))
	}
	for i, tc := range testcases {
		t.Run(tc.path, func(t *testing.T) {
			got := files[i]
			if got.Path != tc.path || got.Mode != tc.mode {
				t.Errorf("file = %s (%v), want %s (%v)", got.Path, got.Mode, tc.path, tc.mode)
			}
			if *update {
				if err := os.WriteFile(tc.golden, got.Content, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(tc.golden)
			if err != nil {
				t.Fatal(err)
			}
			if string(got.Content) != string(want) {
				t.Errorf("%s differs from %s; run go test ./test/render -update and review the diff", tc.path, tc.golden)
			}
			for _, word := range []string{"canary", "m10rten"} {
				if strings.Contains(strings.ToLower(string(got.Content)), word) {
					t.Errorf("%s contains %q", tc.path, word)
				}
			}
		})
	}
}

func TestSummaryEscaping(t *testing.T) {
	const actionSuffix = `) from a GitHub release`
	prefixes := map[string]string{
		"install.sh":  summaryPrefix,
		"install.ps1": summaryPrefix,
		"action.yml":  `description: "Installs demo (`,
	}
	testcases := []struct {
		name    string
		summary string
		comment string // the summary line of install.sh and install.ps1, after the prefix
		yaml    string // the scalar inside the description line of action.yml
	}{
		{name: "plain", summary: "Demonstrate the spine", comment: "Demonstrate the spine", yaml: "Demonstrate the spine"},
		{name: "yaml syntax stays inside the quotes", summary: "a: b # c - {d: e} &f !g & <h>", comment: "a: b # c - {d: e} &f !g & <h>", yaml: "a: b # c - {d: e} &f !g & <h>"},
		{name: "shell metacharacters, quotes and backslash", summary: "`id` $(id) \"x\" 'y' \\", comment: "`id` $(id) \"x\" 'y' \\", yaml: "`id` $(id) \\\"x\\\" 'y' \\\\"},
		{name: "newline followed by a command", summary: "x\ntouch \"$HOME/pwned\"\n", comment: "x touch \"$HOME/pwned\"", yaml: `x\ntouch \"$HOME/pwned\"\n`},
		{name: "newline followed by a step", summary: "x\n    run: id\n", comment: "x     run: id", yaml: `x\n    run: id\n`},
		{name: "carriage return", summary: "x\r#>\rEvil", comment: "x #> Evil", yaml: `x\r#>\rEvil`},
		{name: "unicode line and paragraph separators", summary: "x\u2028Evil\u2029Evil", comment: "x Evil Evil", yaml: `x\u2028Evil\u2029Evil`},
		{name: "next line control", summary: "x\u0085Evil", comment: "x Evil", yaml: `x\u0085Evil`},
		{name: "null and tab", summary: "x\x00\tEvil", comment: "x Evil", yaml: `x\u0000\tEvil`},
		{name: "unprintable character outside the BMP", summary: "x\U000E0001Evil", comment: "x\U000E0001Evil", yaml: `x\U000e0001Evil`},
		{name: "expression", summary: "${{ secrets.TOKEN }}", comment: "${{ secrets.TOKEN }}", yaml: "$ {{ secrets.TOKEN }}"},
		{name: "expression assembled from repeated characters", summary: "$${{{ secrets.TOKEN }}}", comment: "$${{{ secrets.TOKEN }}}", yaml: "$$ {{{ secrets.TOKEN }}}"},
	}
	renderWith := func(summary string) []render.File {
		tool := testutil.Tool()
		tool.Summary = summary
		files, err := render.Files(tool, "v0.0.0-test")
		if err != nil {
			t.Fatal(err)
		}
		return files
	}
	lineIndex := func(lines []string, prefix string) int {
		return slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(l, prefix) })
	}
	withoutLine := func(lines []string, prefix string) []string {
		i := lineIndex(lines, prefix)
		return slices.Concat(lines[:i], lines[i+1:])
	}
	isSeparator := func(r rune) bool { return unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) }
	baseline := renderWith("baseline")
	script := filepath.Join(t.TempDir(), "install.sh")

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			for i, f := range renderWith(tc.summary) {
				prefix := prefixes[f.Path]
				got := strings.Split(string(f.Content), "\n")
				base := strings.Split(string(baseline[i].Content), "\n")
				if len(got) != len(base) {
					t.Fatalf("%s has %d lines, want %d", f.Path, len(got), len(base))
				}
				line := got[lineIndex(got, prefix)]
				if !slices.Equal(withoutLine(got, prefix), withoutLine(base, prefix)) {
					t.Errorf("%s differs from the baseline outside the summary line", f.Path)
				}
				if strings.IndexFunc(line, isSeparator) >= 0 {
					t.Errorf("%s summary line holds a line separator", f.Path)
				}
				switch f.Path {
				case "action.yml":
					if !strings.HasPrefix(line, prefix+tc.yaml+actionSuffix) || !strings.HasSuffix(line, `"`) {
						t.Errorf("%s description line = %q, want a quoted scalar holding %q", f.Path, line, tc.yaml)
					}
					if strings.Contains(line, "${{") {
						t.Errorf("%s description line = %q, holds an expression", f.Path, line)
					}
				default:
					if want := prefix + tc.comment; line != want {
						t.Errorf("%s summary line = %q, want %q", f.Path, line, want)
					}
				}
				if f.Path != "install.sh" {
					continue
				}
				if err := os.WriteFile(script, f.Content, 0o644); err != nil {
					t.Fatal(err)
				}
				if out, err := exec.Command("sh", "-n", script).CombinedOutput(); err != nil {
					t.Errorf("sh -n: %v\n%s", err, out)
				}
			}
		})
	}
}

func TestAliasRendering(t *testing.T) {
	const alias = "dmx"
	needles := map[string]string{
		"install.sh":  alias,
		"install.ps1": alias + ".exe",
	}
	testcases := []struct {
		name  string
		alias string
	}{
		{name: "alias set", alias: alias},
		{name: "no alias"},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			tool := testutil.Tool()
			tool.Alias = tc.alias
			files, err := render.Files(tool, "v0.0.0-test")
			if err != nil {
				t.Fatal(err)
			}
			for _, f := range files {
				needle, ok := needles[f.Path]
				if !ok {
					continue
				}
				if got := strings.Contains(string(f.Content), needle); got != (tc.alias != "") {
					t.Errorf("%s contains %q = %v, want %v", f.Path, needle, got, tc.alias != "")
				}
			}
		})
	}
}
