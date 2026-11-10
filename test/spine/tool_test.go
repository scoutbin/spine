package spine_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/scoutbin/spine"
	"github.com/scoutbin/spine/test/testutil"
)

func pin(mutate func(*spine.Tool)) spine.Tool {
	tool := spine.Tool{Name: "pin", Repo: "scoutbin/pin", Summary: "Audit", License: "Apache-2.0"}
	if mutate != nil {
		mutate(&tool)
	}
	return tool
}

func TestLoad(t *testing.T) {
	const base = `"name": "pin", "repo": "scoutbin/pin", "summary": "Audit", "license": "Apache-2.0"`
	const rest = `"repo":"scoutbin/pin","summary":"s","license":"l"`
	npmName := func(n int) string { return strings.Repeat("a", n) }
	testcases := []struct {
		name    string
		data    string
		want    spine.Tool
		wantErr string
	}{
		{name: "valid minimal", data: `{` + base + `}`, want: pin(nil)},
		{
			name: "valid all channels",
			data: testutil.Manifest,
			want: func() spine.Tool {
				tool := spine.Tool{Name: "demo", Repo: "scoutbin/demo", Summary: "Demonstrate the spine", License: "Apache-2.0"}
				tool.Channels.Homebrew.Tap = "scoutbin/homebrew-tap"
				tool.Channels.Npm.Package = "@scoutbin/demo"
				tool.Channels.Chocolatey.ID = "scoutbin-demo"
				tool.Channels.Action = true
				return tool
			}(),
		},
		{name: "unscoped npm", data: `{` + base + `,"channels":{"npm":{"package":"pin-cli"}}}`,
			want: pin(func(p *spine.Tool) { p.Channels.Npm.Package = "pin-cli" })},
		{name: "npm with dot and underscore", data: `{` + base + `,"channels":{"npm":{"package":"@scoutbin/pin_cli.js"}}}`,
			want: pin(func(p *spine.Tool) { p.Channels.Npm.Package = "@scoutbin/pin_cli.js" })},
		{name: "npm at max length", data: `{` + base + `,"channels":{"npm":{"package":"` + npmName(214) + `"}}}`,
			want: pin(func(p *spine.Tool) { p.Channels.Npm.Package = npmName(214) })},
		{name: "tap with non-tap suffix", data: `{` + base + `,"channels":{"homebrew":{"tap":"scoutbin/homebrew-Tools_x"}}}`,
			want: pin(func(p *spine.Tool) { p.Channels.Homebrew.Tap = "scoutbin/homebrew-Tools_x" })},
		{name: "empty npm object is disabled", data: `{` + base + `,"channels":{"npm":{}}}`, want: pin(nil)},
		{name: "empty tap is disabled", data: `{` + base + `,"channels":{"homebrew":{"tap":""}}}`, want: pin(nil)},
		{name: "action false is disabled", data: `{` + base + `,"channels":{"action":false}}`, want: pin(nil)},

		{name: "empty input", data: ``, wantErr: "spine.json: EOF"},
		{name: "not json", data: `name: pin`, wantErr: "spine.json: invalid character"},
		{name: "trailing data", data: `{` + base + `} {}`, wantErr: "spine.json: trailing data after the manifest object"},
		{name: "unknown field", data: `{` + base + `,"nmae":"x"}`, wantErr: `unknown field "nmae"`},
		{name: "unknown channel field", data: `{` + base + `,"channels":{"homebrew":{"tap":"a/homebrew-b","x":1}}}`, wantErr: `unknown field "x"`},
		{name: "unknown channel", data: `{` + base + `,"channels":{"scoop":{}}}`, wantErr: `unknown field "scoop"`},
		{name: "action not a bool", data: `{` + base + `,"channels":{"action":"yes"}}`, wantErr: "cannot unmarshal string into Go struct field"},
		{name: "missing name", data: `{` + rest + `}`, wantErr: `name ""`},
		{name: "missing repo", data: `{"name":"pin","summary":"s","license":"l"}`, wantErr: `repo ""`},
		{name: "missing summary", data: `{"name":"pin","repo":"scoutbin/pin","license":"l"}`, wantErr: "summary is required"},
		{name: "blank summary", data: `{"name":"pin","repo":"scoutbin/pin","summary":"  ","license":"l"}`, wantErr: "summary is required"},
		{name: "missing license", data: `{"name":"pin","repo":"scoutbin/pin","summary":"s"}`, wantErr: "license is required"},
		{name: "upper case name", data: `{"name":"Pin",` + rest + `}`, wantErr: `name "Pin"`},
		{name: "hyphen in name", data: `{"name":"my-pin",` + rest + `}`, wantErr: `name "my-pin"`},
		{name: "leading digit name", data: `{"name":"1pin",` + rest + `}`, wantErr: `name "1pin"`},
		{name: "trailing newline name", data: `{"name":"pin\n",` + rest + `}`, wantErr: `name "pin\n"`},
		{name: "repo without owner", data: `{"name":"pin","repo":"pin","summary":"s","license":"l"}`, wantErr: `repo "pin"`},
		{name: "repo with url", data: `{"name":"pin","repo":"github.com/scoutbin/pin","summary":"s","license":"l"}`, wantErr: `repo "github.com/scoutbin/pin"`},
		{name: "repo with space", data: `{"name":"pin","repo":"scoutbin/p in","summary":"s","license":"l"}`, wantErr: `repo "scoutbin/p in"`},
		{name: "tap not a homebrew repo", data: `{` + base + `,"channels":{"homebrew":{"tap":"scoutbin/tap"}}}`, wantErr: `channels.homebrew.tap "scoutbin/tap"`},
		{name: "tap without owner", data: `{` + base + `,"channels":{"homebrew":{"tap":"homebrew-tap"}}}`, wantErr: `channels.homebrew.tap "homebrew-tap"`},
		{name: "npm upper case", data: `{` + base + `,"channels":{"npm":{"package":"@scoutbin/Pin"}}}`, wantErr: `channels.npm.package "@scoutbin/Pin"`},
		{name: "npm empty scope", data: `{` + base + `,"channels":{"npm":{"package":"@/pin"}}}`, wantErr: `channels.npm.package "@/pin"`},
		{name: "npm too many segments", data: `{` + base + `,"channels":{"npm":{"package":"@a/b/c"}}}`, wantErr: `channels.npm.package "@a/b/c"`},
		{name: "npm leading dot", data: `{` + base + `,"channels":{"npm":{"package":".pin"}}}`, wantErr: `channels.npm.package ".pin"`},
		{name: "npm over max length", data: `{` + base + `,"channels":{"npm":{"package":"` + npmName(215) + `"}}}`, wantErr: "channels.npm.package"},
		{name: "chocolatey upper case", data: `{` + base + `,"channels":{"chocolatey":{"id":"Scoutbin-Pin"}}}`, wantErr: `channels.chocolatey.id "Scoutbin-Pin"`},
		{name: "chocolatey space", data: `{` + base + `,"channels":{"chocolatey":{"id":"scoutbin pin"}}}`, wantErr: `channels.chocolatey.id "scoutbin pin"`},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := spine.Load([]byte(tc.data))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestMustLoad(t *testing.T) {
	testcases := []struct {
		name      string
		data      string
		wantPanic bool
	}{
		{"valid", testutil.Manifest, false},
		{"invalid", `{}`, true},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if got := recover() != nil; got != tc.wantPanic {
					t.Errorf("panicked = %v, want %v", got, tc.wantPanic)
				}
			}()
			spine.MustLoad([]byte(tc.data))
		})
	}
}

func TestDerived(t *testing.T) {
	demo := testutil.Tool()
	scout := spine.MustLoad([]byte(`{"name":"scout","repo":"acme/scout","summary":"s","license":"MIT","channels":{"homebrew":{"tap":"Acme/homebrew-Tools_x"}}}`))
	noTap := spine.MustLoad([]byte(`{"name":"pin","repo":"scoutbin/pin","summary":"s","license":"l"}`))
	testcases := []struct {
		name string
		tool spine.Tool
		got  func(spine.Tool) string
		want string
	}{
		{"binary unix", demo, func(t spine.Tool) string { return t.BinaryName("linux") }, "demo"},
		{"binary windows", demo, func(t spine.Tool) string { return t.BinaryName("windows") }, "demo.exe"},
		{"binary follows name", scout, func(t spine.Tool) string { return t.BinaryName("windows") }, "scout.exe"},
		{"archive strips v", demo, func(t spine.Tool) string { return t.ArchiveName("v1.2.3", "linux", "amd64") }, "demo_1.2.3_linux_amd64.tar.gz"},
		{"archive without v", demo, func(t spine.Tool) string { return t.ArchiveName("1.2.3", "linux", "arm64") }, "demo_1.2.3_linux_arm64.tar.gz"},
		{"archive windows zip", demo, func(t spine.Tool) string { return t.ArchiveName("v1.2.3-rc.1", "windows", "arm64") }, "demo_1.2.3-rc.1_windows_arm64.zip"},
		{"env version", demo, func(t spine.Tool) string { return t.EnvVar("VERSION") }, "DEMO_VERSION"},
		{"env follows name", scout, func(t spine.Tool) string { return t.EnvVar("TRACK") }, "SCOUT_TRACK"},
		{"install dir unix", demo, func(t spine.Tool) string { return t.InstallDir("linux") }, "~/.local/bin"},
		{"install dir windows", demo, func(t spine.Tool) string { return t.InstallDir("windows") }, `%LOCALAPPDATA%\demo\bin`},
		{"formula class", demo, func(t spine.Tool) string { return t.FormulaClass() }, "Demo"},
		{"formula class follows name", scout, func(t spine.Tool) string { return t.FormulaClass() }, "Scout"},
		{"homebrew formula", demo, func(t spine.Tool) string { return t.HomebrewFormula() }, "scoutbin/tap/demo"},
		{"homebrew formula is lower-cased", scout, func(t spine.Tool) string { return t.HomebrewFormula() }, "acme/tools_x/scout"},
		{"homebrew formula without tap", noTap, func(t spine.Tool) string { return t.HomebrewFormula() }, ""},
		{"package owner", demo, func(t spine.Tool) string { return t.PackageOwner() }, "scoutbin"},
		{"package owner follows repo", scout, func(t spine.Tool) string { return t.PackageOwner() }, "acme"},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.got(tc.tool); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
