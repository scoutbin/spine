package selfcmd_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/scoutbin/spine/internal/buildinfo"
	"github.com/scoutbin/spine/internal/selfcmd"
	"github.com/scoutbin/spine/test/testutil"
)

func TestInfo(t *testing.T) {
	windows := withOS(testInfo, "windows")
	links := map[string]string{
		"/usr/local/bin/demo":    "/opt/homebrew/Cellar/demo/0.1.3/bin/demo",
		"/usr/local/bin/demo-js": "/usr/local/lib/node_modules/@scoutbin/demo/bin/demo",
	}

	testcases := []struct {
		name       string
		build      buildinfo.Info
		exe        string
		exeErr     error
		linkErr    error
		env        map[string]string
		args       []string
		wantCode   int
		wantStdout string
		stderrHas  string
	}{
		{name: "install.sh text", build: testInfo, exe: "/usr/local/bin/demo-plain", args: []string{"info"},
			wantStdout: "path:    /usr/local/bin/demo-plain\nchannel: install.sh\nversion: v0.1.3\n"},
		{name: "install.sh json", build: testInfo, exe: "/home/me/.local/bin/demo", args: []string{"info", "--json"},
			wantStdout: `{"path":"/home/me/.local/bin/demo","channel":"install.sh","version":"v0.1.3"}` + "\n"},
		{name: "--json before command", build: testInfo, exe: "/home/me/.local/bin/demo", args: []string{"--json", "info"},
			wantStdout: `{"path":"/home/me/.local/bin/demo","channel":"install.sh","version":"v0.1.3"}` + "\n"},
		{name: "symlink resolves to homebrew", build: testInfo, exe: "/usr/local/bin/demo", args: []string{"info", "--json"},
			wantStdout: `{"path":"/opt/homebrew/Cellar/demo/0.1.3/bin/demo","channel":"homebrew","version":"v0.1.3"}` + "\n"},
		{name: "symlink resolves to npm", build: testInfo, exe: "/usr/local/bin/demo-js", args: []string{"info", "--json"},
			wantStdout: `{"path":"/usr/local/lib/node_modules/@scoutbin/demo/bin/demo","channel":"npm","version":"v0.1.3"}` + "\n"},
		{name: "windows unmanaged is unknown", build: windows, exe: `C:\Users\me\bin\demo.exe`, args: []string{"info"},
			wantStdout: "path:    C:\\Users\\me\\bin\\demo.exe\nchannel: unknown\nversion: v0.1.3\n"},
		{name: "dev build is unknown", build: devInfo, exe: "/tmp/demo", args: []string{"info", "--json"},
			wantStdout: `{"path":"/tmp/demo","channel":"unknown","version":"dev"}` + "\n"},
		{name: "executable error", build: testInfo, exeErr: errors.New("boom"), args: []string{"info"}, wantCode: 2, stderrHas: "locate executable: boom"},
		{name: "symlink error", build: testInfo, exe: "/usr/local/bin/demo-plain", linkErr: errors.New("loop"), args: []string{"info"}, wantCode: 2, stderrHas: "resolve executable path: loop"},
		{name: "info rejects extra args", build: testInfo, exe: "/tmp/demo", args: []string{"info", "extra"}, wantCode: 2, stderrHas: `unexpected argument "extra"`},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			a := &selfcmd.App{
				Tool: testutil.Tool(), Stdout: &stdout, Stderr: &stderr, Build: tc.build,
				LookupEnv:  testutil.EnvOf(tc.env),
				Executable: func() (string, error) { return tc.exe, tc.exeErr },
				EvalSymlinks: func(p string) (string, error) {
					if tc.linkErr != nil {
						return "", tc.linkErr
					}
					if target, ok := links[p]; ok {
						return target, nil
					}
					return p, nil
				},
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
