package selfcmd_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/scoutbin/spine/internal/buildinfo"
	"github.com/scoutbin/spine/internal/release"
	"github.com/scoutbin/spine/internal/selfcmd"
	"github.com/scoutbin/spine/term"
	"github.com/scoutbin/spine/test/testutil"
)

func TestCheck(t *testing.T) {
	const latestBody = `{"tag_name":"v0.2.0"}`
	ahead := withVersion(testInfo, "v0.3.0")
	prerelease := withVersion(testInfo, "v0.2.0-rc.1")
	const releasesBody = `[{"tag_name":"v0.2.0"},{"tag_name":"v0.2.0-rc.1"}]`

	testcases := []struct {
		name       string
		build      buildinfo.Info
		args       []string
		env        map[string]string
		status     int
		body       string
		progress   bool
		wantCode   int
		wantStdout string
		wantAuth   string
		stderrHas  string
	}{
		{name: "update available text", build: testInfo, args: []string{"check"}, status: 200, body: latestBody,
			wantCode: 1, wantStdout: "Update available: v0.1.3 → v0.2.0\n"},
		{name: "update available json", build: testInfo, args: []string{"check", "--json"}, status: 200, body: latestBody,
			wantCode: 1, wantStdout: `{"current":"v0.1.3","latest":"v0.2.0","update_available":true,"track":"stable"}` + "\n"},
		{name: "up to date text", build: testInfo, args: []string{"check"}, status: 200, body: `{"tag_name":"v0.1.3"}`,
			wantStdout: "Up to date (v0.1.3)\n"},
		{name: "up to date json", build: testInfo, args: []string{"--json", "check"}, status: 200, body: `{"tag_name":"v0.1.3"}`,
			wantStdout: `{"current":"v0.1.3","latest":"v0.1.3","update_available":false,"track":"stable"}` + "\n"},
		{name: "ahead of latest is up to date", build: ahead, args: []string{"check"}, status: 200, body: latestBody,
			wantStdout: "Up to date (v0.3.0)\n"},
		{name: "prerelease is older than its release", build: prerelease, args: []string{"check"}, status: 200, body: releasesBody,
			wantCode: 1, wantStdout: "Update available: v0.2.0-rc.1 → v0.2.0 (track: rc)\n"},
		{name: "dev build reports update", build: devInfo, args: []string{"check"}, status: 200, body: latestBody,
			wantCode: 1, wantStdout: "Update available: dev → v0.2.0\n"},
		{name: "dev build json", build: devInfo, args: []string{"check", "--json"}, status: 200, body: latestBody,
			wantCode: 1, wantStdout: `{"current":"dev","latest":"v0.2.0","update_available":true,"track":"stable"}` + "\n"},
		{name: "token sent", build: testInfo, args: []string{"check"}, env: map[string]string{"GITHUB_TOKEN": "s3cret"}, status: 200, body: latestBody,
			wantCode: 1, wantStdout: "Update available: v0.1.3 → v0.2.0\n", wantAuth: "Bearer s3cret"},
		{name: "empty token not sent", build: testInfo, args: []string{"check"}, env: map[string]string{"GITHUB_TOKEN": ""}, status: 200, body: latestBody,
			wantCode: 1, wantStdout: "Update available: v0.1.3 → v0.2.0\n"},
		{name: "404", build: testInfo, args: []string{"check"}, status: 404, wantCode: 2, stderrHas: "error: look up latest release: no release found"},
		{name: "403 rate limit", build: testInfo, args: []string{"check"}, status: 403, wantCode: 2, stderrHas: "rate limit"},
		{name: "500", build: testInfo, args: []string{"check"}, status: 500, wantCode: 2, stderrHas: "HTTP 500"},
		{name: "malformed JSON", build: testInfo, args: []string{"check", "--json"}, status: 200, body: `not json`, wantCode: 2, stderrHas: "decode GitHub response"},
		{name: "check rejects extra args", build: testInfo, args: []string{"check", "extra"}, status: 200, body: latestBody, wantCode: 2, stderrHas: `unexpected argument "extra"`},
		{name: "spinner on stderr when progress is on", build: testInfo, args: []string{"check"}, status: 200, body: latestBody, progress: true,
			wantCode: 1, wantStdout: "Update available: v0.1.3 → v0.2.0\n", stderrHas: "Latest release is v0.2.0"},
		{name: "spinner failure on stderr", build: testInfo, args: []string{"check"}, status: 500, progress: true,
			wantCode: 2, stderrHas: "Could not check for updates"},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			var gotAuth, gotUA string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotAuth, gotUA = r.Header.Get("Authorization"), r.Header.Get("User-Agent")
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			var stdout, stderr bytes.Buffer
			a := &selfcmd.App{
				Tool: testutil.Tool(), Stdout: &stdout, Stderr: &stderr, Build: tc.build,
				LookupEnv: testutil.EnvOf(tc.env),
				Detect:    func(bool, bool) term.Mode { return term.Mode{Progress: tc.progress} },
				GitHub:    release.Client{BaseURL: srv.URL, HTTP: srv.Client()},
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
			if tc.wantCode <= 1 && strings.Contains(stderr.String(), "error:") {
				t.Errorf("stderr = %q, want no error line for exit code %d", stderr.String(), tc.wantCode)
			}
			if tc.wantCode <= 1 && !tc.progress && stderr.Len() != 0 {
				t.Errorf("stderr = %q, want empty when progress is off", stderr.String())
			}
			if tc.wantCode <= 1 && gotAuth != tc.wantAuth {
				t.Errorf("Authorization = %q, want %q", gotAuth, tc.wantAuth)
			}
			if gotUA != "" && gotUA != "demo" {
				t.Errorf("User-Agent = %q, want the tool name", gotUA)
			}
		})
	}
}

func TestCheckTracks(t *testing.T) {
	const (
		latestPath   = "/repos/scoutbin/demo/releases/latest"
		releasesPath = "/repos/scoutbin/demo/releases"
	)
	list := func(tags ...string) string {
		items := make([]string, len(tags))
		for i, tag := range tags {
			items[i] = `{"tag_name":"` + tag + `"}`
		}
		return "[" + strings.Join(items, ",") + "]"
	}

	testcases := []struct {
		name       string
		version    string
		body       string
		wantPath   string
		wantCode   int
		wantText   string
		wantLatest string
		wantTrack  string
	}{
		{name: "stable uses releases/latest", version: "v0.1.3", body: `{"tag_name":"v0.2.0"}`, wantPath: latestPath,
			wantCode: 1, wantText: "Update available: v0.1.3 → v0.2.0\n", wantLatest: "v0.2.0", wantTrack: "stable"},
		{name: "unknown prerelease is stable", version: "v0.0.0-ci", body: `{"tag_name":"v0.2.0"}`, wantPath: latestPath,
			wantCode: 1, wantText: "Update available: v0.0.0-ci → v0.2.0\n", wantLatest: "v0.2.0", wantTrack: "stable"},
		{name: "nightly build is stable", version: "v0.1.4-nightly.20261020", body: `{"tag_name":"v0.2.0"}`, wantPath: latestPath,
			wantCode: 1, wantText: "Update available: v0.1.4-nightly.20261020 → v0.2.0\n", wantLatest: "v0.2.0", wantTrack: "stable"},
		{name: "beta moves to rc, never to alpha", version: "v0.2.0-beta.1", wantPath: releasesPath,
			body:     list("v0.2.0-alpha.3", "v0.2.0-beta.1", "v0.2.0-rc.1", "v0.1.3", "v0.2.0-nightly.20261025"),
			wantCode: 1, wantText: "Update available: v0.2.0-beta.1 → v0.2.0-rc.1 (track: beta)\n", wantLatest: "v0.2.0-rc.1", wantTrack: "beta"},
		{name: "beta never moves to alpha", version: "v0.2.0-beta.1", wantPath: releasesPath,
			body:     list("v0.2.0-alpha.3", "v0.2.0-beta.1", "v0.1.3"),
			wantText: "Up to date (v0.2.0-beta.1, track: beta)\n", wantLatest: "v0.2.0-beta.1", wantTrack: "beta"},
		{name: "beta newer than every release in its path", version: "v0.2.0-beta.2", wantPath: releasesPath,
			body:     list("v0.2.0-beta.1", "v0.2.0-alpha.3", "v0.1.3"),
			wantText: "Up to date (v0.2.0-beta.2, track: beta)\n", wantLatest: "v0.2.0-beta.2", wantTrack: "beta"},
		{name: "alpha moves up to beta", version: "v0.2.0-alpha.1", wantPath: releasesPath,
			body:     list("v0.2.0-alpha.1", "v0.2.0-beta.2", "v0.1.3"),
			wantCode: 1, wantText: "Update available: v0.2.0-alpha.1 → v0.2.0-beta.2 (track: alpha)\n", wantLatest: "v0.2.0-beta.2", wantTrack: "alpha"},
		{name: "rc moves to stable", version: "v0.2.0-rc.1", wantPath: releasesPath,
			body:     list("v0.2.0-rc.1", "v0.2.0", "v0.3.0-beta.1"),
			wantCode: 1, wantText: "Update available: v0.2.0-rc.1 → v0.2.0 (track: rc)\n", wantLatest: "v0.2.0", wantTrack: "rc"},
		{name: "rc with no release in its path is up to date", version: "v0.2.0-rc.1", wantPath: releasesPath, body: list(),
			wantText: "Up to date (v0.2.0-rc.1, track: rc)\n", wantLatest: "v0.2.0-rc.1", wantTrack: "rc"},
	}

	for _, tc := range testcases {
		for _, asJSON := range []bool{false, true} {
			name := tc.name + " text"
			args := []string{"check"}
			if asJSON {
				name, args = tc.name+" json", []string{"check", "--json"}
			}
			t.Run(name, func(t *testing.T) {
				var paths []string
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					paths = append(paths, r.URL.Path)
					w.Write([]byte(tc.body))
				}))
				defer srv.Close()

				var stdout, stderr bytes.Buffer
				a := &selfcmd.App{
					Tool: testutil.Tool(), Stdout: &stdout, Stderr: &stderr, Build: withVersion(testInfo, tc.version),
					LookupEnv: testutil.EnvOf(nil),
					Detect:    func(bool, bool) term.Mode { return term.Mode{} },
					GitHub:    release.Client{BaseURL: srv.URL, HTTP: srv.Client()},
				}

				code := a.Run(args)

				want := tc.wantText
				if asJSON {
					want = fmt.Sprintf(`{"current":%q,"latest":%q,"update_available":%t,"track":%q}`+"\n", tc.version, tc.wantLatest, tc.wantCode == 1, tc.wantTrack)
				}
				if code != tc.wantCode {
					t.Errorf("exit code = %d, want %d (stderr: %q)", code, tc.wantCode, stderr.String())
				}
				if stdout.String() != want {
					t.Errorf("stdout = %q, want %q", stdout.String(), want)
				}
				if !slices.Equal(paths, []string{tc.wantPath}) {
					t.Errorf("requests = %q, want exactly one to %q", paths, tc.wantPath)
				}
			})
		}
	}
}
