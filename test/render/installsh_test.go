package render_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/scoutbin/spine"
	"github.com/scoutbin/spine/internal/release"
	"github.com/scoutbin/spine/internal/render"
	"github.com/scoutbin/spine/test/testutil"
)

const releaseTag = "v1.2.3"

// installScript renders install.sh for tool into a temp dir and returns its path.
func installScript(t *testing.T, tool spine.Tool) string {
	t.Helper()
	files, err := render.Files(tool, "v0.0.0-test")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "install.sh")
	if err := os.WriteFile(path, files[0].Content, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// runInstall runs the script with a minimal environment and returns its exit
// code and stderr. extra entries are appended after the defaults.
func runInstall(t *testing.T, script, home, path string, extra ...string) (int, string) {
	t.Helper()
	cmd := exec.Command("sh", script)
	cmd.Dir = home
	cmd.Env = append([]string{"PATH=" + path, "HOME=" + home, "TMPDIR=" + home}, extra...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return 0, stderr.String()
	}
	exit, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatal(err)
	}
	return exit.ExitCode(), stderr.String()
}

func requireTools(t *testing.T, names ...string) {
	t.Helper()
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("install.sh supports darwin and linux only")
	}
	for _, name := range names {
		if _, err := exec.LookPath(name); err != nil {
			t.Skipf("%s is not installed", name)
		}
	}
}

func TestInstallShFromMirror(t *testing.T) {
	requireTools(t, "sh", "curl", "tar")
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip("no release target for " + runtime.GOARCH)
	}
	tool := testutil.Tool()
	aliased := tool
	aliased.Alias = "dm"
	archive := tool.ArchiveName(releaseTag, runtime.GOOS, runtime.GOARCH)
	binary := "#!/bin/sh\necho installed\n"
	tarball := testutil.TarGz(t, testutil.TarEntry{Name: tool.BinaryName(runtime.GOOS), Body: binary})
	sum := sha256.Sum256(tarball)
	goodSum := hex.EncodeToString(sum[:])

	testcases := []struct {
		name       string
		checksums  string // checksums.txt content; {sum} and {archive} are filled in
		env        []string
		wantCode   int
		stderrHas  string
		wantBinary string // path relative to the home dir; empty when nothing must be installed
		alias      bool   // render install.sh for a tool with the alias dm
		foreign    bool   // a file that is not a link to demo sits at the alias path before the install
		rerun      bool   // run the install twice
		wantLink   bool   // dm must be a relative symlink to demo
		wantFiles  []string
		stderrNot  string
	}{
		{
			name:       "installs into the default directory",
			checksums:  "{sum}  {archive}\n",
			env:        []string{"DEMO_NO_MODIFY_PATH=1"},
			stderrHas:  "Downloading demo " + releaseTag,
			wantBinary: strings.TrimPrefix(tool.InstallDir("linux"), "~/") + "/demo",
		},
		{
			name:       "installs into DEMO_INSTALL_DIR",
			checksums:  "{sum}  {archive}\n",
			env:        []string{"DEMO_NO_MODIFY_PATH=1", "DEMO_INSTALL_DIR=custom/bin"},
			wantBinary: "custom/bin/demo",
		},
		{
			name:       "alias is a relative symlink to the binary",
			checksums:  "{sum}  {archive}\n",
			env:        []string{"DEMO_NO_MODIFY_PATH=1", "DEMO_INSTALL_DIR=bin"},
			alias:      true,
			wantBinary: "bin/demo",
			wantLink:   true,
			wantFiles:  []string{"demo", "dm"},
			stderrNot:  "skipping",
		},
		{
			name:       "rerun keeps the alias without a warning",
			checksums:  "{sum}  {archive}\n",
			env:        []string{"DEMO_NO_MODIFY_PATH=1", "DEMO_INSTALL_DIR=bin"},
			alias:      true,
			rerun:      true,
			wantBinary: "bin/demo",
			wantLink:   true,
			wantFiles:  []string{"demo", "dm"},
			stderrNot:  "skipping",
		},
		{
			name:       "foreign file at the alias path is kept with a warning",
			checksums:  "{sum}  {archive}\n",
			env:        []string{"DEMO_NO_MODIFY_PATH=1", "DEMO_INSTALL_DIR=bin"},
			alias:      true,
			foreign:    true,
			stderrHas:  "skipping the dm alias",
			wantBinary: "bin/demo",
			wantFiles:  []string{"demo", "dm"},
		},
		{
			name:       "tool without an alias writes only the binary",
			checksums:  "{sum}  {archive}\n",
			env:        []string{"DEMO_NO_MODIFY_PATH=1", "DEMO_INSTALL_DIR=bin"},
			wantBinary: "bin/demo",
			wantFiles:  []string{"demo"},
			stderrNot:  "skipping",
		},
		{
			name:      "checksum mismatch installs nothing",
			checksums: strings.Repeat("0", 64) + "  {archive}\n",
			env:       []string{"DEMO_NO_MODIFY_PATH=1"},
			wantCode:  1,
			stderrHas: "checksum mismatch for {archive}",
		},
		{
			name:      "checksums without the archive installs nothing",
			checksums: "{sum}  other.tar.gz\n",
			env:       []string{"DEMO_NO_MODIFY_PATH=1"},
			wantCode:  1,
			stderrHas: "checksums.txt has no line for {archive}",
		},
		{
			name:      "mirror without a version",
			env:       []string{"DEMO_VERSION="},
			wantCode:  1,
			stderrHas: "DEMO_VERSION is required when DEMO_DOWNLOAD_BASE_URL is set",
		},
		{
			name:      "version that is not a tag",
			env:       []string{"DEMO_VERSION=latest"},
			wantCode:  1,
			stderrHas: "DEMO_VERSION must be a release tag like v1.2.3, got: latest",
		},
		{
			name:      "version with a newline",
			env:       []string{"DEMO_VERSION=v1.2.3\nv1.2.3"},
			wantCode:  1,
			stderrHas: "DEMO_VERSION must be a release tag like v1.2.3, got: v1.2.3\nv1.2.3",
		},
		{
			name:      "nightly track is not supported",
			env:       []string{"DEMO_TRACK=nightly"},
			wantCode:  1,
			stderrHas: "DEMO_TRACK must be one of stable, alpha, beta or rc, got: nightly",
		},
	}
	scripts := map[bool]string{false: installScript(t, tool), true: installScript(t, aliased)}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			mirror := filepath.Join(t.TempDir(), "mirror")
			if err := os.MkdirAll(filepath.Join(mirror, releaseTag), 0o755); err != nil {
				t.Fatal(err)
			}
			fill := strings.NewReplacer("{sum}", goodSum, "{archive}", archive)
			for name, content := range map[string][]byte{archive: tarball, "checksums.txt": []byte(fill.Replace(tc.checksums))} {
				if err := os.WriteFile(filepath.Join(mirror, releaseTag, name), content, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			env := append([]string{"DEMO_VERSION=" + releaseTag, "DEMO_DOWNLOAD_BASE_URL=file://" + mirror}, tc.env...)

			if tc.foreign {
				if err := os.MkdirAll(filepath.Join(home, "bin"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(home, "bin", "dm"), []byte("foreign"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			code, stderr := runInstall(t, scripts[tc.alias], home, os.Getenv("PATH"), env...)
			if tc.rerun {
				code, stderr = runInstall(t, scripts[tc.alias], home, os.Getenv("PATH"), env...)
			}

			if code != tc.wantCode {
				t.Errorf("exit code = %d, want %d (stderr %q)", code, tc.wantCode, stderr)
			}
			if want := fill.Replace(tc.stderrHas); !strings.Contains(stderr, want) {
				t.Errorf("stderr = %q, want it to contain %q", stderr, want)
			}
			if tc.stderrNot != "" && strings.Contains(stderr, tc.stderrNot) {
				t.Errorf("stderr = %q, want it not to contain %q", stderr, tc.stderrNot)
			}
			installed, err := os.ReadFile(filepath.Join(home, tc.wantBinary))
			if tc.wantBinary == "" {
				if entries, _ := os.ReadDir(home); len(entries) != 0 {
					t.Errorf("home dir is not empty after a failed install: %v", entries)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if string(installed) != binary {
				t.Errorf("installed binary = %q, want %q", installed, binary)
			}
			if info, _ := os.Stat(filepath.Join(home, tc.wantBinary)); info.Mode().Perm() != 0o755 {
				t.Errorf("installed mode = %v, want 0755", info.Mode().Perm())
			}
			if tc.wantFiles != nil {
				entries, err := os.ReadDir(filepath.Dir(filepath.Join(home, tc.wantBinary)))
				if err != nil {
					t.Fatal(err)
				}
				var names []string
				for _, e := range entries {
					names = append(names, e.Name())
				}
				if !slices.Equal(names, tc.wantFiles) {
					t.Errorf("install dir holds %v, want %v", names, tc.wantFiles)
				}
			}
			aliasPath := filepath.Join(home, "bin", "dm")
			if tc.wantLink {
				if target, err := os.Readlink(aliasPath); err != nil || target != "demo" {
					t.Errorf("readlink(dm) = %q, %v, want demo", target, err)
				}
			}
			if tc.foreign {
				if content, err := os.ReadFile(aliasPath); err != nil || string(content) != "foreign" {
					t.Errorf("dm = %q, %v, want the foreign file kept", content, err)
				}
			}
		})
	}
}

// TestInstallShTrackResolution runs the script against a stand-in curl that
// serves only the releases list, and checks that the tag it resolves and the
// asset URL it requests are the ones the Go release code and the manifest give.
func TestInstallShTrackResolution(t *testing.T) {
	requireTools(t, "sh", "tar", "awk")
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip("no release target for " + runtime.GOARCH)
	}
	type entry struct {
		TagName    string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
		Body       string `json:"body"`
	}
	releases := []entry{
		{TagName: "v0.9.0"},
		{TagName: "v1.1.0-rc.1", Prerelease: true},
		{TagName: "v1.2.0-beta.1", Prerelease: true},
		{TagName: "v1.2.0-alpha.2", Prerelease: true},
		{TagName: "v1.3.0-alpha.10", Prerelease: true},
		{TagName: "v1.3.0-alpha.9", Prerelease: true, Body: "\"tag_name\": \"v9.9.9\""},
		{TagName: "v2.0.0", Draft: true},
		{TagName: "v1.4.0-rc.0", Prerelease: true},
		{TagName: "v1.4.0-nightly.20261110", Prerelease: true},
	}
	// Only tags that can be pushed and are not drafts are releases; the Go side
	// would also accept, for example, v1.4.0-rc.0, which no release can have.
	var published []string
	for _, r := range releases {
		if _, err := release.ValidatePushedTag(r.TagName); err == nil && !r.Draft {
			published = append(published, r.TagName)
		}
	}
	compact, err := json.Marshal(releases)
	if err != nil {
		t.Fatal(err)
	}
	indented, err := json.MarshalIndent(releases, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	testcases := []struct {
		name  string
		track release.Track
		json  []byte
	}{
		{name: "rc", track: release.RC, json: compact},
		{name: "beta", track: release.Beta, json: compact},
		{name: "alpha", track: release.Alpha, json: compact},
		{name: "alpha from an indented list", track: release.Alpha, json: indented},
	}
	tool := testutil.Tool()
	script := installScript(t, tool)
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			wantTag, ok := release.HighestInPath(tc.track, published)
			if !ok {
				t.Fatal("no expected tag")
			}
			bin := t.TempDir()
			list := filepath.Join(bin, "releases.json")
			stub := fmt.Sprintf(`#!/bin/sh
while [ $# -gt 1 ]; do
  [ "$1" = -o ] && out=$2
  shift
done
[ "$1" = "https://api.github.com/repos/%s/releases?per_page=100" ] || { echo "unexpected request: $1" >&2; exit 22; }
cp %q "$out"
`, tool.Repo, list)
			if err := os.WriteFile(list, tc.json, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(bin, "curl"), []byte(stub), 0o755); err != nil {
				t.Fatal(err)
			}

			code, stderr := runInstall(t, script, t.TempDir(), bin+string(os.PathListSeparator)+os.Getenv("PATH"), "DEMO_TRACK="+string(tc.track))

			// The stand-in refuses the asset download, so the script stops there.
			if code != 1 {
				t.Errorf("exit code = %d, want 1 (stderr %q)", code, stderr)
			}
			wantURL := "https://github.com/" + tool.Repo + "/releases/download/" + wantTag + "/" + tool.ArchiveName(wantTag, runtime.GOOS, runtime.GOARCH)
			if want := "download failed: " + wantURL; !strings.Contains(stderr, want) {
				t.Errorf("stderr = %q, want it to contain %q", stderr, want)
			}
		})
	}
}
