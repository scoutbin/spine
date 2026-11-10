package release_test

import (
	"testing"

	"github.com/scoutbin/spine/internal/manifest"
	"github.com/scoutbin/spine/internal/release"
	"github.com/scoutbin/spine/test/testutil"
)

var tool = testutil.Tool()

func TestDownloadURL(t *testing.T) {
	testcases := []struct {
		tag, asset, want string
	}{
		{"v0.1.3", tool.ArchiveName("v0.1.3", "linux", "amd64"), "https://github.com/scoutbin/demo/releases/download/v0.1.3/demo_0.1.3_linux_amd64.tar.gz"},
		{"v0.1.3", tool.ArchiveName("v0.1.3", "windows", "arm64"), "https://github.com/scoutbin/demo/releases/download/v0.1.3/demo_0.1.3_windows_arm64.zip"},
		{"v0.1.3", manifest.ChecksumsFile, "https://github.com/scoutbin/demo/releases/download/v0.1.3/checksums.txt"},
	}

	for _, tc := range testcases {
		t.Run(tc.asset, func(t *testing.T) {
			if got := release.DownloadURL(tool, tc.tag, tc.asset); got != tc.want {
				t.Errorf("DownloadURL(%q, %q) = %q, want %q", tc.tag, tc.asset, got, tc.want)
			}
		})
	}
}
