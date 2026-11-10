package selfcmd_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/scoutbin/spine"
	"github.com/scoutbin/spine/internal/buildinfo"
	"github.com/scoutbin/spine/internal/channel"
	"github.com/scoutbin/spine/internal/release"
	"github.com/scoutbin/spine/internal/selfcmd"
	"github.com/scoutbin/spine/test/testutil"
)

func TestUpgrade(t *testing.T) {
	tool := testutil.Tool()
	brew, _ := channel.UpgradeCommand(tool, channel.Homebrew, "latest")
	npm, _ := channel.UpgradeCommand(tool, channel.NPM, "latest")
	bare := spine.MustLoad([]byte(`{"name":"demo","repo":"scoutbin/demo","summary":"s","license":"l"}`))

	const (
		brewExe = "/opt/homebrew/Cellar/demo/0.1.3/bin/demo"
		npmExe  = "/usr/local/lib/node_modules/@scoutbin/demo/bin/demo"
	)

	testcases := []struct {
		name       string
		tool       spine.Tool
		build      buildinfo.Info
		exe        string
		args       []string
		wantCode   int
		wantStdout string
		stderrHas  string
	}{
		{name: "homebrew prints command", tool: tool, build: testInfo, exe: brewExe, args: []string{"upgrade"},
			wantStdout: "demo is managed by homebrew. To upgrade, run:\n  " + brew + "\n"},
		{name: "npm prints command", tool: tool, build: testInfo, exe: npmExe, args: []string{"upgrade"},
			wantStdout: "demo is managed by npm. To upgrade, run:\n  " + npm + "\n"},
		{name: "--json does not change output", tool: tool, build: testInfo, exe: brewExe, args: []string{"upgrade", "--json"},
			wantStdout: "demo is managed by homebrew. To upgrade, run:\n  " + brew + "\n"},
		{name: "homebrew with --version", tool: tool, build: testInfo, exe: brewExe, args: []string{"upgrade", "--version", "v0.2.0"},
			wantCode: 2, stderrHas: "error: --version only works for install.sh installs, but this install is managed by homebrew (to upgrade, run: " + brew + ")"},
		{name: "npm with --version", tool: tool, build: testInfo, exe: npmExe, args: []string{"upgrade", "--version=0.2.0"},
			wantCode: 2, stderrHas: "error: --version only works for install.sh installs, but this install is managed by npm (to upgrade, run: " + npm + ")"},
		{name: "unknown dev build", tool: tool, build: devInfo, exe: "/tmp/demo", args: []string{"upgrade"},
			wantCode: 2, stderrHas: "error: cannot upgrade: install channel is unknown; reinstall demo with install.sh"},
		{name: "unknown with --version", tool: tool, build: devInfo, exe: "/tmp/demo", args: []string{"upgrade", "--version", "v0.2.0"},
			wantCode: 2, stderrHas: "error: --version only works for install.sh installs, but this install is of unknown origin; reinstall demo with install.sh"},
		{name: "homebrew path but channel not enabled for the tool", tool: bare, build: testInfo, exe: brewExe, args: []string{"upgrade"},
			wantCode: 2, stderrHas: "error: cannot upgrade: install channel is unknown; reinstall demo with install.sh"},
		{name: "install.sh invalid --version", tool: tool, build: testInfo, exe: "/usr/local/bin/demo", args: []string{"upgrade", "--version", "banana"},
			wantCode: 2, stderrHas: `invalid --version "banana"`},
		{name: "install.sh --version is not a pushed tag", tool: tool, build: testInfo, exe: "/usr/local/bin/demo", args: []string{"upgrade", "--version", "v0.2.0-nightly.1"},
			wantCode: 2, stderrHas: `invalid --version "v0.2.0-nightly.1"`},
		{name: "upgrade rejects extra args", tool: tool, build: testInfo, exe: "/usr/local/bin/demo", args: []string{"upgrade", "extra"},
			wantCode: 2, stderrHas: `unexpected argument "extra"`},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			a := &selfcmd.App{
				Tool: tc.tool, Stdout: &stdout, Stderr: &stderr, Build: tc.build,
				LookupEnv:    testutil.EnvOf(nil),
				Executable:   func() (string, error) { return tc.exe, nil },
				EvalSymlinks: func(p string) (string, error) { return p, nil },
			}

			code := a.Run(tc.args)

			if code != tc.wantCode {
				t.Errorf("exit code = %d, want %d (stderr: %q)", code, tc.wantCode, stderr.String())
			}
			if stdout.String() != tc.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout.String(), tc.wantStdout)
			}
			if !strings.Contains(stderr.String(), tc.stderrHas) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tc.stderrHas)
			}
		})
	}
}

func TestUpgradeNPMHintPerTrack(t *testing.T) {
	testcases := []struct {
		version string
		want    string
	}{
		{"v0.1.3", "@latest"},
		{"v0.0.0-ci", "@latest"},
		{"dev", "@latest"},
		{"v0.2.0-nightly.20261025", "@latest"},
		{"v0.2.0-alpha.1", "@alpha"},
		{"v0.2.0-beta.1", "@beta"},
		{"v0.2.0-rc.1", "@rc"},
	}

	for _, tc := range testcases {
		t.Run(tc.version, func(t *testing.T) {
			requests := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer srv.Close()

			var stdout, stderr bytes.Buffer
			a := &selfcmd.App{
				Tool: testutil.Tool(), Stdout: &stdout, Stderr: &stderr, Build: withVersion(testInfo, tc.version),
				LookupEnv:    testutil.EnvOf(nil),
				Executable:   func() (string, error) { return "/usr/local/lib/node_modules/@scoutbin/demo/bin/demo", nil },
				EvalSymlinks: func(p string) (string, error) { return p, nil },
				GitHub:       release.Client{BaseURL: srv.URL, HTTP: srv.Client()},
			}

			code := a.Run([]string{"upgrade"})

			want := "demo is managed by npm. To upgrade, run:\n  npm install -g @scoutbin/demo" + tc.want + "\n"
			if code != 0 || stdout.String() != want {
				t.Errorf("exit code = %d, stdout = %q, want 0 and %q (stderr: %q)", code, stdout.String(), want, stderr.String())
			}
			if requests != 0 {
				t.Errorf("made %d network requests, want none for a managed channel", requests)
			}
		})
	}
}
