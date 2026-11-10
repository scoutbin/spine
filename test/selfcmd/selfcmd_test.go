package selfcmd_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/scoutbin/spine/cli"
	"github.com/scoutbin/spine/internal/buildinfo"
	"github.com/scoutbin/spine/internal/selfcmd"
	"github.com/scoutbin/spine/test/testutil"
)

var testInfo = buildinfo.Info{
	Version: "v0.1.3",
	Commit:  "4f2a9c1",
	Date:    "2026-10-25T14:02:11Z",
	OS:      "darwin",
	Arch:    "arm64",
}

var devInfo = buildinfo.Info{Version: "dev", Commit: "none", Date: "unknown", OS: "linux", Arch: "amd64"}

func withVersion(b buildinfo.Info, version string) buildinfo.Info {
	b.Version = version
	return b
}

func withOS(b buildinfo.Info, goos string) buildinfo.Info {
	b.OS = goos
	return b
}

func TestRoot(t *testing.T) {
	const (
		versionLine = "demo v0.1.3 (commit 4f2a9c1, built 2026-10-25T14:02:11Z, darwin/arm64)\n"
		versionJSON = `{"version":"v0.1.3","commit":"4f2a9c1","build_time":"2026-10-25T14:02:11Z","os":"darwin","arch":"arm64"}` + "\n"
	)
	hello := &cli.Command{Name: "hello", Summary: "Say hello", Run: func(*cli.Context) error { return nil }}
	clash := func(name string) *cli.Command { return &cli.Command{Name: name, Summary: "clash"} }

	testcases := []struct {
		name       string
		args       []string
		cmds       []*cli.Command
		wantCode   int
		wantStdout string   // exact match when non-empty
		stdoutHas  []string // substrings
		stderrHas  string
	}{
		{name: "version text", args: []string{"version"}, wantStdout: versionLine},
		{name: "root --version", args: []string{"--version"}, wantStdout: versionLine},
		{name: "version --json", args: []string{"version", "--json"}, wantStdout: versionJSON},
		{name: "--json version", args: []string{"--json", "version"}, wantStdout: versionJSON},
		{name: "--version --json", args: []string{"--version", "--json"}, wantStdout: versionJSON},
		{name: "--no-color after command", args: []string{"version", "--no-color"}, wantStdout: versionLine},
		{name: "bare root shows help", stdoutHas: []string{"Demonstrate the spine", "demo [command] [flags]", "version", "info", "check", "upgrade", "--json", "env NO_COLOR"}},
		{name: "tool commands are listed", cmds: []*cli.Command{hello}, args: []string{"help"}, stdoutHas: []string{"hello", "Say hello"}},
		{name: "version --help", args: []string{"version", "--help"}, stdoutHas: []string{"demo version [flags]", "--json"}},
		{name: "unknown flag", args: []string{"--bogus"}, wantCode: 2, stderrHas: "unknown flag --bogus"},
		{name: "unknown command", args: []string{"bogus"}, wantCode: 2, stderrHas: `unknown command "bogus"`},
		{name: "version rejects root flag", args: []string{"version", "--version"}, wantCode: 2, stderrHas: "unknown flag --version"},
		{name: "tool command reuses version", cmds: []*cli.Command{clash("version")}, wantCode: 2, stderrHas: `command "version" is built in`},
		{name: "tool command reuses upgrade", cmds: []*cli.Command{hello, clash("upgrade")}, args: []string{"hello"}, wantCode: 2, stderrHas: `command "upgrade" is built in`},
		{name: "tool command reuses help", cmds: []*cli.Command{clash("help")}, wantCode: 2, stderrHas: `"help" is reserved`},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			a := &selfcmd.App{Tool: testutil.Tool(), Stdout: &stdout, Stderr: &stderr, Build: testInfo, LookupEnv: testutil.EnvOf(nil)}

			code := a.Run(tc.args, tc.cmds...)

			if code != tc.wantCode {
				t.Errorf("exit code = %d, want %d (stderr: %q)", code, tc.wantCode, stderr.String())
			}
			if tc.wantStdout != "" && stdout.String() != tc.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout.String(), tc.wantStdout)
			}
			for _, s := range tc.stdoutHas {
				if !strings.Contains(stdout.String(), s) {
					t.Errorf("stdout missing %q:\n%s", s, stdout.String())
				}
			}
			if !strings.Contains(stderr.String(), tc.stderrHas) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tc.stderrHas)
			}
			if tc.wantCode != 0 && stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty on error", stdout.String())
			}
		})
	}
}
