package spine_test

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/scoutbin/spine"
	"github.com/scoutbin/spine/cli"
	"github.com/scoutbin/spine/internal/buildinfo"
	"github.com/scoutbin/spine/term"
	"github.com/scoutbin/spine/test/testutil"
)

func TestRun(t *testing.T) {
	version, commit, date := buildinfo.Version, buildinfo.Commit, buildinfo.Date
	buildinfo.Version, buildinfo.Commit, buildinfo.Date = "v0.1.3", "4f2a9c1", "2026-10-25T14:02:11Z"
	t.Cleanup(func() { buildinfo.Version, buildinfo.Commit, buildinfo.Date = version, commit, date })
	versionLine := fmt.Sprintf("demo v0.1.3 (commit 4f2a9c1, built 2026-10-25T14:02:11Z, %s/%s)\n", runtime.GOOS, runtime.GOARCH)

	var ran []string
	hello := &cli.Command{Name: "hello", Summary: "Say hello", Args: []cli.Arg{{Name: "who"}}, Run: func(ctx *cli.Context) error {
		ran = append(ran, ctx.Arg("who"))
		return nil
	}}
	exit3 := &cli.Command{Name: "exit3", Summary: "Exit with 3", Run: func(*cli.Context) error { return &cli.ExitError{Code: 3} }}
	fail := &cli.Command{Name: "fail", Summary: "Fail", Run: func(*cli.Context) error { return errors.New("boom") }}
	clash := &cli.Command{Name: "check", Summary: "Shadow the built-in"}

	testcases := []struct {
		name       string
		args       []string
		cmds       []*cli.Command
		latest     string // tag served by releases/latest
		zeroEnv    bool   // only Stdout, Detect and LookupEnv set; Run defaults the rest
		wantCode   int
		wantStdout string   // exact match when non-empty
		stdoutHas  []string // substrings
		stderrHas  string
		wantRan    []string
	}{
		{name: "version", args: []string{"version"}, wantStdout: versionLine},
		{name: "nil env fields default to the process", args: []string{"version"}, zeroEnv: true, wantStdout: versionLine},
		{name: "root --version", args: []string{"--version"}, wantStdout: versionLine},
		{name: "bare root shows help with tool commands", cmds: []*cli.Command{hello},
			stdoutHas: []string{"Demonstrate the spine", "demo [command] [flags]", "hello", "Say hello", "upgrade"}},
		{name: "tool command is reachable", args: []string{"hello", "world"}, cmds: []*cli.Command{hello}, wantRan: []string{"world"}},
		{name: "tool command help", args: []string{"hello", "--help"}, cmds: []*cli.Command{hello}, stdoutHas: []string{"demo hello [flags] [who]", "--json"}},
		{name: "tool command exit code", args: []string{"exit3"}, cmds: []*cli.Command{exit3}, wantCode: 3},
		{name: "tool command error", args: []string{"fail"}, cmds: []*cli.Command{fail}, wantCode: 2, stderrHas: "error: boom"},
		{name: "built-in name clash", args: []string{"check"}, cmds: []*cli.Command{clash}, wantCode: 2, stderrHas: `command "check" is built in`},
		{name: "check finds an update", args: []string{"check"}, latest: "v0.2.0", wantCode: 1, wantStdout: "Update available: v0.1.3 → v0.2.0\n"},
		{name: "check up to date", args: []string{"check", "--json"}, latest: "v0.1.3",
			wantStdout: `{"current":"v0.1.3","latest":"v0.1.3","update_available":false,"track":"stable"}` + "\n"},
		{name: "unknown command", args: []string{"bogus"}, wantCode: 2, stderrHas: `unknown command "bogus"`},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			ran = nil
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/repos/scoutbin/demo/releases/latest" {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				fmt.Fprintf(w, `{"tag_name":%q}`, tc.latest)
			}))
			defer srv.Close()

			var stdout, stderr bytes.Buffer
			env := spine.Env{
				Stdout: &stdout, Stderr: &stderr,
				LookupEnv: testutil.EnvOf(nil),
				Detect:    func(bool, bool) term.Mode { return term.Mode{} },
				HTTP:      srv.Client(), GitHubAPI: srv.URL, DownloadBase: srv.URL,
			}

			if tc.zeroEnv {
				env = spine.Env{Stdout: &stdout, Detect: env.Detect, LookupEnv: env.LookupEnv}
			}

			code := spine.Run(testutil.Tool(), env, tc.args, tc.cmds...)

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
			if tc.stderrHas != "" && !strings.Contains(stderr.String(), tc.stderrHas) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tc.stderrHas)
			}
			if strings.Join(ran, ",") != strings.Join(tc.wantRan, ",") {
				t.Errorf("tool command ran with %q, want %q", ran, tc.wantRan)
			}
		})
	}
}

func TestRunWithDefault(t *testing.T) {
	var got []string
	def := &cli.Command{
		Summary: "Digest paths",
		Flags:   []cli.Flag{{Name: "fail-on", Kind: cli.KindString}},
		Args:    []cli.Arg{{Name: "path", Variadic: true}},
		Run: func(ctx *cli.Context) error {
			got = append(ctx.Args(), "fail-on="+ctx.String("fail-on"))
			return &cli.ExitError{Code: 1}
		},
	}

	testcases := []struct {
		name      string
		def       *cli.Command // nil uses the shared def
		args      []string
		wantCode  int
		wantRan   []string
		stdoutHas string
		stderrHas string
	}{
		{name: "bare invocation runs the default", wantCode: 1, wantRan: []string{"fail-on="}},
		{name: "paths and root flag", args: []string{"a.log", "--fail-on", "error", "b.log"}, wantCode: 1, wantRan: []string{"a.log", "b.log", "fail-on=error"}},
		{name: "unknown first word is a path", args: []string{"bogus"}, wantCode: 1, wantRan: []string{"bogus", "fail-on="}},
		{name: "built-in command wins", args: []string{"version"}, stdoutHas: "demo "},
		{name: "--version wins over paths", args: []string{"a.log", "--version"}, stdoutHas: "demo "},
		{name: "--help wins over paths", args: []string{"a.log", "--help"}, stdoutHas: "demo [flags] [path...]\n  demo <command> [flags]"},
		{name: "root flag with a command is rejected", args: []string{"--fail-on", "error", "version"}, wantCode: 2},
		{name: "default without Run is rejected", def: &cli.Command{Summary: "No action"}, wantCode: 2, stderrHas: "default command has no Run"},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got = nil
			var stdout, stderr bytes.Buffer
			env := spine.Env{
				Stdout: &stdout, Stderr: &stderr,
				LookupEnv: testutil.EnvOf(nil),
				Detect:    func(bool, bool) term.Mode { return term.Mode{} },
			}
			d := def
			if tc.def != nil {
				d = tc.def
			}
			code := spine.RunWithDefault(testutil.Tool(), env, tc.args, d)

			if code != tc.wantCode {
				t.Errorf("exit code = %d, want %d (stderr: %q)", code, tc.wantCode, stderr.String())
			}
			if tc.stdoutHas != "" && !strings.Contains(stdout.String(), tc.stdoutHas) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tc.stdoutHas)
			}
			if tc.stderrHas != "" && !strings.Contains(stderr.String(), tc.stderrHas) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tc.stderrHas)
			}
			if strings.Join(got, ",") != strings.Join(tc.wantRan, ",") {
				t.Errorf("default ran with %q, want %q", got, tc.wantRan)
			}
		})
	}
}
