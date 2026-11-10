package release_test

import (
	"testing"

	"github.com/scoutbin/spine/internal/release"
)

func TestCompare(t *testing.T) {
	testcases := []struct {
		name string
		a, b string
		want int
	}{
		{"equal", "v1.2.3", "v1.2.3", 0},
		{"missing v on one side", "1.2.3", "v1.2.3", 0},
		{"missing v on both sides", "1.2.4", "1.2.3", 1},
		{"major", "v2.0.0", "v1.9.9", 1},
		{"minor", "v1.2.0", "v1.10.0", -1},
		{"patch", "v1.2.4", "v1.2.3", 1},
		{"numeric not lexical", "v1.10.0", "v1.9.0", 1},
		{"prerelease below release", "v1.0.0-rc.1", "v1.0.0", -1},
		{"release above prerelease", "v1.0.0", "v1.0.0-rc.1", 1},
		{"prerelease of newer core above older release", "v1.1.0-alpha", "v1.0.0", 1},
		{"numeric identifiers compare numerically", "v1.0.0-rc.2", "v1.0.0-rc.10", -1},
		{"numeric below alphanumeric", "v1.0.0-1", "v1.0.0-alpha", -1},
		{"alphanumeric compares lexically", "v1.0.0-alpha", "v1.0.0-beta", -1},
		{"fewer identifiers is lower", "v1.0.0-alpha", "v1.0.0-alpha.1", -1},
		{"semver spec chain 1", "v1.0.0-alpha.1", "v1.0.0-alpha.beta", -1},
		{"semver spec chain 2", "v1.0.0-beta.2", "v1.0.0-beta.11", -1},
		{"semver spec chain 3", "v1.0.0-beta.11", "v1.0.0-rc.1", -1},
		{"build metadata ignored", "v1.0.0+build.5", "v1.0.0+build.9", 0},
		{"build metadata ignored on prerelease", "v1.0.0-rc.1+a", "v1.0.0-rc.1", 0},
		{"dev older than release", "dev", "v0.0.1", -1},
		{"release newer than dev", "v0.0.1", "dev", 1},
		{"commit hash older than release", "4f2a9c1", "v1.0.0", -1},
		{"empty older than release", "", "v1.0.0", -1},
		{"dev older than prerelease", "dev", "v1.0.0-rc.1", -1},
		{"two unparseable equal", "dev", "4f2a9c1", 0},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			if got := release.Compare(tc.a, tc.b); got != tc.want {
				t.Errorf("Compare(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
			}
			if got := release.Compare(tc.b, tc.a); got != -tc.want {
				t.Errorf("Compare(%q, %q) = %d, want %d", tc.b, tc.a, got, -tc.want)
			}
		})
	}
}

func TestParseVersion(t *testing.T) {
	testcases := []struct {
		input string
		want  release.Version
		ok    bool
	}{
		{"v0.1.3", release.Version{Major: 0, Minor: 1, Patch: 3}, true},
		{"0.1.3", release.Version{Major: 0, Minor: 1, Patch: 3}, true},
		{"v1.0.0-rc.1", release.Version{Major: 1, Pre: []string{"rc", "1"}}, true},
		{"v1.0.0-rc.1+build.5", release.Version{Major: 1, Pre: []string{"rc", "1"}}, true},
		{"v1.0.0+build.5", release.Version{Major: 1}, true},
		{"v1.0.0-x-y.z", release.Version{Major: 1, Pre: []string{"x-y", "z"}}, true},
		{"dev", release.Version{}, false},
		{"4f2a9c1", release.Version{}, false},
		{"", release.Version{}, false},
		{"v", release.Version{}, false},
		{"v1.2", release.Version{}, false},
		{"v1.2.3.4", release.Version{}, false},
		{"vv1.2.3", release.Version{}, false},
		{"V1.2.3", release.Version{}, false},
		{"v1.2.x", release.Version{}, false},
		{"v01.2.3", release.Version{}, false},
		{"v1.2.3-", release.Version{}, false},
		{"v1.2.3-rc..1", release.Version{}, false},
		{"v1.2.3-01", release.Version{}, false},
		{"v1.2.3+", release.Version{}, false},
		{"v1.2.3 ", release.Version{}, false},
		{"v99999999999999999999.0.0", release.Version{}, false},
	}

	for _, tc := range testcases {
		t.Run(tc.input, func(t *testing.T) {
			got, ok := release.ParseVersion(tc.input)
			if ok != tc.ok {
				t.Fatalf("ParseVersion(%q) ok = %v, want %v", tc.input, ok, tc.ok)
			}
			if got.Compare(tc.want) != 0 || got.Major != tc.want.Major || got.Minor != tc.want.Minor || got.Patch != tc.want.Patch || len(got.Pre) != len(tc.want.Pre) {
				t.Errorf("ParseVersion(%q) = %+v, want %+v", tc.input, got, tc.want)
			}
		})
	}
}
