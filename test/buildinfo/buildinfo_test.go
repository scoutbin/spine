package buildinfo_test

import (
	"encoding/json"
	"testing"

	"github.com/scoutbin/spine/internal/buildinfo"
	"github.com/scoutbin/spine/test/testutil"
)

func TestInfoString(t *testing.T) {
	testcases := []struct {
		name string
		info buildinfo.Info
		want string
	}{
		{"release build", buildinfo.Info{Version: "v0.2.0", Commit: "4f2a9c1", Date: "2026-11-10T09:00:00Z", OS: "linux", Arch: "arm64"}, "demo v0.2.0 (commit 4f2a9c1, built 2026-11-10T09:00:00Z, linux/arm64)"},
		{"default build", buildinfo.Info{Version: "dev", Commit: "none", Date: "unknown", OS: "darwin", Arch: "amd64"}, "demo dev (commit none, built unknown, darwin/amd64)"},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.info.String(testutil.Tool().Name); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestInfoJSONKeys(t *testing.T) {
	data, err := json.Marshal(buildinfo.Info{Version: "v1", Commit: "c", Date: "d", OS: "o", Arch: "a"})
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"version":"v1","commit":"c","build_time":"d","os":"o","arch":"a"}`
	if string(data) != want {
		t.Errorf("JSON = %s, want %s", data, want)
	}
}
