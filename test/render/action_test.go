package render_test

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/scoutbin/spine/internal/render"
	"github.com/scoutbin/spine/test/testutil"
)

func renderAction(t *testing.T, summary string) []string {
	t.Helper()
	tool := testutil.Tool()
	tool.Summary = summary
	files, err := render.Files(tool, "v0.0.0-test")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f.Path == "action.yml" {
			return strings.Split(string(f.Content), "\n")
		}
	}
	t.Fatal("action.yml not rendered")
	return nil
}

func TestFilesByChannel(t *testing.T) {
	testcases := []struct {
		name   string
		action bool
		want   []string
	}{
		{name: "action enabled", action: true, want: []string{"install.sh", "install.ps1", "action.yml"}},
		{name: "action disabled", action: false, want: []string{"install.sh", "install.ps1"}},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			tool := testutil.Tool()
			tool.Channels.Action = tc.action
			files, err := render.Files(tool, "v0.0.0-test")
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, f := range files {
				got = append(got, f.Path)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("files = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestActionInputsOnlyThroughEnv(t *testing.T) {
	envEntry := regexp.MustCompile(`^\s+[A-Z_]+: \$\{\{ inputs\.[a-z-]+ \}\}$`)
	runBlock := regexp.MustCompile(`^(\s*)(?:- )?run: \|\s*$`)
	indent := func(l string) int { return len(l) - len(strings.TrimLeft(l, " ")) }
	runIndent := -1
	for _, line := range renderAction(t, "Demonstrate the spine") {
		if runIndent >= 0 && (strings.TrimSpace(line) == "" || indent(line) > runIndent) {
			if strings.Contains(line, "${{") {
				t.Errorf("expression inside a run block: %q", line)
			}
			continue
		}
		runIndent = -1
		if m := runBlock.FindStringSubmatch(line); m != nil {
			runIndent = len(m[1])
		}
		if strings.Contains(line, "inputs.") && !envEntry.MatchString(line) {
			t.Errorf("input used outside an env entry: %q", line)
		}
	}
}
