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
	testcases := []struct {
		name    string
		summary string
		want    string
	}{
		{name: "plain", summary: "Demonstrate the spine", want: "Demonstrate the spine"},
		{name: "shell metacharacters stay inert in a comment", summary: "`id` $(id) \"x\" 'y' \\", want: "`id` $(id) \"x\" 'y' \\"},
		{name: "newline followed by a command", summary: "x\ntouch \"$HOME/pwned\"\n", want: "x touch \"$HOME/pwned\""},
		{name: "carriage return", summary: "x\r#>\rEvil", want: "x #> Evil"},
		{name: "unicode line and paragraph separators", summary: "x\u2028Evil\u2029Evil", want: "x Evil Evil"},
		{name: "next line control", summary: "x\u0085Evil", want: "x Evil"},
		{name: "null and tab", summary: "x\x00\tEvil", want: "x Evil"},
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
	summaryIndex := func(lines []string) int {
		return slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(l, summaryPrefix) })
	}
	withoutSummary := func(lines []string) []string {
		i := summaryIndex(lines)
		return slices.Concat(lines[:i], lines[i+1:])
	}
	isSeparator := func(r rune) bool { return unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) }
	baseline := renderWith("baseline")
	script := filepath.Join(t.TempDir(), "install.sh")

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			for i, f := range renderWith(tc.summary) {
				got := strings.Split(string(f.Content), "\n")
				base := strings.Split(string(baseline[i].Content), "\n")
				if len(got) != len(base) {
					t.Fatalf("%s has %d lines, want %d", f.Path, len(got), len(base))
				}
				line := got[summaryIndex(got)]
				if want := summaryPrefix + tc.want; line != want {
					t.Errorf("%s summary line = %q, want %q", f.Path, line, want)
				}
				if !slices.Equal(withoutSummary(got), withoutSummary(base)) {
					t.Errorf("%s differs from the baseline outside the summary line", f.Path)
				}
				if strings.IndexFunc(line, isSeparator) >= 0 {
					t.Errorf("%s summary line holds a line separator", f.Path)
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
