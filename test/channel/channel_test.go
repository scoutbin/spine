package channel_test

import (
	"testing"

	"github.com/scoutbin/spine"
	"github.com/scoutbin/spine/internal/channel"
	"github.com/scoutbin/spine/test/testutil"
)

func TestDetect(t *testing.T) {

	testcases := []struct {
		name      string
		path      string
		goos      string
		isRelease bool
		want      channel.Channel
	}{
		{name: "homebrew macOS arm64", path: "/opt/homebrew/Cellar/demo/0.1.3/bin/demo", goos: "darwin", isRelease: true, want: channel.Homebrew},
		{name: "homebrew macOS intel", path: "/usr/local/Cellar/demo/0.1.3/bin/demo", goos: "darwin", isRelease: true, want: channel.Homebrew},
		{name: "linuxbrew", path: "/home/linuxbrew/.linuxbrew/Cellar/demo/0.1.3/bin/demo", goos: "linux", isRelease: true, want: channel.Homebrew},
		{name: "homebrew caskroom", path: "/opt/homebrew/Caskroom/demo/0.1.3/demo", goos: "darwin", isRelease: true, want: channel.Homebrew},
		{name: "npm global unix", path: "/usr/local/lib/node_modules/@scoutbin/demo/bin/demo", goos: "linux", isRelease: true, want: channel.NPM},
		{name: "npm global windows", path: `C:\Users\me\AppData\Roaming\npm\node_modules\@scoutbin\demo\bin\demo.exe`, goos: "windows", isRelease: true, want: channel.NPM},
		{name: "chocolatey path is not a known channel", path: `C:\ProgramData\chocolatey\lib\scoutbin-demo\tools\demo.exe`, goos: "windows", isRelease: true, want: channel.Unknown},
		{name: "install.sh /usr/local/bin", path: "/usr/local/bin/demo", goos: "darwin", isRelease: true, want: channel.InstallSh},
		{name: "install.sh ~/.local/bin", path: "/home/me/.local/bin/demo", goos: "linux", isRelease: true, want: channel.InstallSh},
		{name: "dev build outside managed paths", path: "/usr/local/bin/demo", goos: "linux", isRelease: false, want: channel.Unknown},
		{name: "dev build still detects homebrew", path: "/opt/homebrew/Cellar/demo/dev/bin/demo", goos: "darwin", isRelease: false, want: channel.Homebrew},
		{name: "windows non-managed path", path: `C:\Users\me\bin\demo.exe`, goos: "windows", isRelease: true, want: channel.Unknown},
		{name: "directory merely named like a marker", path: "/home/me/cellarage/demo", goos: "linux", isRelease: true, want: channel.InstallSh},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			if got := channel.Detect(tc.path, tc.goos, tc.isRelease); got != tc.want {
				t.Errorf("Detect(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestUpgradeCommand(t *testing.T) {
	bare := spine.MustLoad([]byte(`{"name":"bare","repo":"scoutbin/bare","summary":"s","license":"l"}`))

	testcases := []struct {
		name    string
		tool    spine.Tool
		channel channel.Channel
		distTag string
		want    string
		wantOK  bool
	}{
		{name: "npm latest", tool: testutil.Tool(), channel: channel.NPM, distTag: "latest", want: "npm install -g @scoutbin/demo@latest", wantOK: true},
		{name: "npm dist-tag", tool: testutil.Tool(), channel: channel.NPM, distTag: "rc", want: "npm install -g @scoutbin/demo@rc", wantOK: true},
		{name: "homebrew ignores dist-tag", tool: testutil.Tool(), channel: channel.Homebrew, distTag: "rc", want: "brew upgrade scoutbin/tap/demo", wantOK: true},
		{name: "install.sh is not package managed", tool: testutil.Tool(), channel: channel.InstallSh, distTag: "latest"},
		{name: "unknown is not package managed", tool: testutil.Tool(), channel: channel.Unknown, distTag: "latest"},
		{name: "npm not enabled for the tool", tool: bare, channel: channel.NPM, distTag: "latest"},
		{name: "homebrew not enabled for the tool", tool: bare, channel: channel.Homebrew, distTag: "latest"},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := channel.UpgradeCommand(tc.tool, tc.channel, tc.distTag)
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("UpgradeCommand(%q, %q) = (%q, %v), want (%q, %v)", tc.channel, tc.distTag, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}
