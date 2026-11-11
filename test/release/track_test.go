package release_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/scoutbin/spine/internal/release"
)

func TestTrackOf(t *testing.T) {
	testcases := []struct {
		version string
		want    release.Track
	}{
		{"v1.2.3", release.Stable},
		{"1.2.3", release.Stable},
		{"v1.2.3-nightly.20261025", release.Stable},
		{"v1.2.3-alpha.1", release.Alpha},
		{"v1.2.3-beta.10", release.Beta},
		{"v1.2.3-rc.1", release.RC},
		{"v1.2.3-rc.1+build.5", release.RC},
		{"v0.0.0-ci", release.Stable},
		{"v0.1.0-3-g4f2a9c1", release.Stable},
		{"v0.1.0-3-g4f2a9c1-dirty", release.Stable},
		{"4f2a9c1", release.Stable},
		{"dev", release.Stable},
		{"", release.Stable},
		{"v1.2.3-RC.1", release.Stable},
		{"v1.2.3-rc-1", release.Stable},
	}

	for _, tc := range testcases {
		t.Run(tc.version, func(t *testing.T) {
			if got := release.TrackOf(tc.version); got != tc.want {
				t.Errorf("TrackOf(%q) = %q, want %q", tc.version, got, tc.want)
			}
		})
	}
}

func TestTrackPath(t *testing.T) {
	testcases := []struct {
		track      release.Track
		want       []release.Track
		distTag    string
		prerelease bool
	}{
		{release.Stable, []release.Track{release.Stable}, "latest", false},
		{release.RC, []release.Track{release.RC, release.Stable}, "rc", true},
		{release.Beta, []release.Track{release.Beta, release.RC, release.Stable}, "beta", true},
		{release.Alpha, []release.Track{release.Alpha, release.Beta, release.RC, release.Stable}, "alpha", true},
	}

	for _, tc := range testcases {
		t.Run(string(tc.track), func(t *testing.T) {
			if got := tc.track.Path(); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Path() = %v, want %v", got, tc.want)
			}
			if got := tc.track.DistTag(); got != tc.distTag {
				t.Errorf("DistTag() = %q, want %q", got, tc.distTag)
			}
			if got := tc.track.Prerelease(); got != tc.prerelease {
				t.Errorf("Prerelease() = %v, want %v", got, tc.prerelease)
			}
		})
	}
}

func TestSelectTarget(t *testing.T) {
	testcases := []struct {
		name     string
		current  string
		releases []string
		want     string
	}{
		{"beta to rc", "v1.3.0-beta.1", []string{"v1.3.0-rc.1", "v1.3.0-beta.1"}, "v1.3.0-rc.1"},
		{"beta to a newer beta", "v1.3.0-beta.1", []string{"v1.3.0-beta.2", "v1.3.0-beta.1"}, "v1.3.0-beta.2"},
		{"rc to stable", "v1.3.0-rc.1", []string{"v1.3.0", "v1.3.0-rc.2"}, "v1.3.0"},
		{"rc to a newer rc", "v1.3.0-rc.1", []string{"v1.3.0-rc.2", "v1.2.0"}, "v1.3.0-rc.2"},
		{"beta never to alpha", "v1.3.0-beta.1", []string{"v1.3.0-alpha.9", "v1.4.0-alpha.1"}, ""},
		{"alpha to beta", "v1.3.0-alpha.1", []string{"v1.3.0-beta.1"}, "v1.3.0-beta.1"},
		{"alpha to rc over beta", "v1.3.0-alpha.1", []string{"v1.3.0-beta.1", "v1.3.0-rc.1"}, "v1.3.0-rc.1"},
		{"alpha to stable over rc", "v1.3.0-alpha.1", []string{"v1.3.0-rc.1", "v1.3.0", "v1.3.0-nightly.20261025"}, "v1.3.0"},
		{"stable never to a prerelease", "v1.2.3", []string{"v1.3.0-rc.1", "v1.3.0-beta.1", "v1.3.0-alpha.1", "v1.3.0-nightly.20261025"}, ""},
		{"stable to a newer stable", "v1.2.3", []string{"v1.3.0-rc.1", "v1.2.4", "v1.2.2"}, "v1.2.4"},
		{"nightly tags are ignored", "v1.0.0-alpha.1", []string{"v9.0.0-nightly.20261025"}, ""},
		{"up to date on the newest stable", "v1.2.4", []string{"v1.2.4", "v1.2.3"}, ""},
		{"build metadata on current is ignored", "v1.2.4+abc", []string{"v1.2.4"}, ""},
		{"empty list", "v1.2.3", nil, ""},
		{"unparseable and unknown tags are ignored", "v1.2.3", []string{"latest", "vfoo", "v9.0.0-ci", "v9.0.0-foo.1", "v1.2.4"}, "v1.2.4"},
		{"dev build is offered the newest stable", "dev", []string{"v1.2.3", "v1.3.0-rc.1"}, "v1.2.3"},
		{"v0.0.0-ci is stable", "v0.0.0-ci", []string{"v0.1.0", "v0.1.1-rc.1"}, "v0.1.0"},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := release.SelectTarget(tc.current, tc.releases)
			if got != tc.want || ok != (tc.want != "") {
				t.Errorf("SelectTarget(%q, %v) = %q, %v, want %q", tc.current, tc.releases, got, ok, tc.want)
			}
		})
	}
}

func TestHighestInPath(t *testing.T) {
	testcases := []struct {
		name     string
		track    release.Track
		releases []string
		want     string
	}{
		{"beta path skips alpha and nightly", release.Beta, []string{"v1.3.0-alpha.1", "v1.3.0-nightly.20261025", "v1.3.0-beta.1"}, "v1.3.0-beta.1"},
		{"stable path", release.Stable, []string{"v1.3.0-rc.1", "v1.2.3"}, "v1.2.3"},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := release.HighestInPath(tc.track, tc.releases)
			if got != tc.want || ok != (tc.want != "") {
				t.Errorf("HighestInPath(%q, %v) = %q, %v, want %q", tc.track, tc.releases, got, ok, tc.want)
			}
		})
	}
}

func TestPreviousTag(t *testing.T) {
	testcases := []struct {
		name string
		tag  string
		tags []string
		want string
	}{
		{"no tags", "v1.0.0", nil, ""},
		{"stable after stable", "v1.2.0", []string{"v1.0.0", "v1.1.0", "v1.1.1"}, "v1.1.1"},
		{"stable skips prereleases", "v1.2.0", []string{"v1.1.0", "v1.2.0-alpha.1", "v1.2.0-beta.1", "v1.2.0-rc.2"}, "v1.1.0"},
		{"stable with only prereleases below", "v1.2.0", []string{"v1.2.0-rc.1"}, ""},
		{"rc after rc", "v1.3.0-rc.2", []string{"v1.2.0", "v1.3.0-alpha.1", "v1.3.0-beta.1", "v1.3.0-rc.1"}, "v1.3.0-rc.1"},
		{"first prerelease after a stable", "v1.3.0-alpha.1", []string{"v1.1.0", "v1.2.0"}, "v1.2.0"},
		{"invalid and nightly tags are ignored", "v1.3.0-rc.1", []string{"v1.2.0", "v1.3.0-nightly.1", "v1.3.0-rc.0", "v1.3.0-ci", "v1.2.5+build", "1.2.9", "latest"}, "v1.2.0"},
		{"higher tags are ignored", "v1.2.4", []string{"v1.2.3", "v1.3.0", "v1.3.0-rc.1", "v2.0.0"}, "v1.2.3"},
		{"higher prereleases are ignored for a prerelease", "v1.2.4-rc.1", []string{"v1.2.3", "v1.2.4-rc.2", "v1.3.0-alpha.1"}, "v1.2.3"},
		{"the tag itself is ignored", "v1.2.0", []string{"v1.2.0", "v1.1.0"}, "v1.1.0"},
		{"only the tag itself", "v1.2.0", []string{"v1.2.0"}, ""},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := release.PreviousTag(tc.tag, tc.tags)
			if got != tc.want || ok != (tc.want != "") {
				t.Errorf("PreviousTag(%q, %v) = %q, %v, want %q", tc.tag, tc.tags, got, ok, tc.want)
			}
		})
	}
}

func TestNextTag(t *testing.T) {
	testcases := []struct {
		name    string
		tags    []string
		bump    string
		track   release.Track
		want    string
		wantErr string
	}{
		{name: "first patch", bump: "patch", track: release.Stable, want: "v0.0.1"},
		{name: "first minor", bump: "minor", track: release.Stable, want: "v0.1.0"},
		{name: "first major", bump: "major", track: release.Stable, want: "v1.0.0"},
		{name: "first alpha", bump: "minor", track: release.Alpha, want: "v0.1.0-alpha.1"},
		{name: "patch after stable", tags: []string{"v1.2.3", "v1.2.0"}, bump: "patch", track: release.Stable, want: "v1.2.4"},
		{name: "minor after stable", tags: []string{"v1.2.3"}, bump: "minor", track: release.Stable, want: "v1.3.0"},
		{name: "major after stable", tags: []string{"v1.2.3"}, bump: "major", track: release.Stable, want: "v2.0.0"},
		{name: "second rc", tags: []string{"v1.2.0", "v1.3.0-rc.1"}, bump: "minor", track: release.RC, want: "v1.3.0-rc.2"},
		{name: "rc number compares numerically", tags: []string{"v1.2.0", "v1.3.0-rc.9", "v1.3.0-rc.2"}, bump: "minor", track: release.RC, want: "v1.3.0-rc.10"},
		{name: "rc promoted to stable", tags: []string{"v1.2.0", "v1.3.0-rc.2"}, bump: "minor", track: release.Stable, want: "v1.3.0"},
		{name: "alpha to beta", tags: []string{"v1.2.0", "v1.3.0-alpha.2"}, bump: "minor", track: release.Beta, want: "v1.3.0-beta.1"},
		{name: "only prereleases and no stable", tags: []string{"v0.1.0-alpha.1"}, bump: "minor", track: release.Alpha, want: "v0.1.0-alpha.2"},
		{name: "invalid and nightly tags are ignored", tags: []string{"v1.2.0", "v1.3.0-nightly.1", "v1.9.0-rc.0", "1.9.0", "v2.0.0+build", "latest"}, bump: "patch", track: release.Stable, want: "v1.2.1"},
		{name: "rc back to beta", tags: []string{"v1.2.0", "v1.3.0-rc.1"}, bump: "minor", track: release.Beta, wantErr: "not higher than v1.3.0-rc.1"},
		{name: "patch below a pending prerelease", tags: []string{"v1.2.0", "v1.3.0-rc.1"}, bump: "patch", track: release.Stable, wantErr: "not higher than v1.3.0-rc.1"},
		{name: "prerelease below a pending prerelease of a higher core", tags: []string{"v1.2.0", "v1.4.0-rc.1"}, bump: "minor", track: release.RC, wantErr: "not higher than v1.4.0-rc.1"},
		{name: "patch below the only prerelease", tags: []string{"v0.1.0-alpha.1"}, bump: "patch", track: release.Stable, wantErr: "not higher than v0.1.0-alpha.1"},
		{name: "unknown bump", tags: []string{"v1.2.0"}, bump: "rc", track: release.Stable, wantErr: `unknown bump "rc"`},
		{name: "empty bump", bump: "", track: release.Stable, wantErr: `unknown bump ""`},
		{name: "unknown track", bump: "patch", track: release.Track("nightly"), wantErr: `unknown track "nightly"`},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := release.NextTag(tc.tags, tc.bump, tc.track)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("NextTag(%v, %q, %q) = %q, %v, want error containing %q", tc.tags, tc.bump, tc.track, got, err, tc.wantErr)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Errorf("NextTag(%v, %q, %q) = %q, %v, want %q", tc.tags, tc.bump, tc.track, got, err, tc.want)
			}
		})
	}
}

func TestValidatePushedTag(t *testing.T) {
	testcases := []struct {
		tag     string
		want    release.Track
		wantErr string
	}{
		{tag: "v1.2.3", want: release.Stable},
		{tag: "v0.2.0", want: release.Stable},
		{tag: "v1.3.0-rc.1", want: release.RC},
		{tag: "v1.3.0-beta.10", want: release.Beta},
		{tag: "v1.3.0-alpha.2", want: release.Alpha},
		{tag: "v1.3.0-rc.0", wantErr: "N of 1 or more"},
		{tag: "v1.3.0-rc.01", wantErr: "not a valid semantic version"},
		{tag: "v1.3.0-rc", wantErr: "N of 1 or more"},
		{tag: "v1.3.0-rc.1.2", wantErr: "N of 1 or more"},
		{tag: "v1.3.0-rc.x", wantErr: "N of 1 or more"},
		{tag: "v1.3.0-nightly.20261025", wantErr: "unknown prerelease"},
		{tag: "v1.3.0-ci", wantErr: "unknown prerelease"},
		{tag: "v1.3.0+build", wantErr: "build metadata"},
		{tag: "v1.3.0-rc.1+build", wantErr: "build metadata"},
		{tag: "1.3.0", wantErr: "must start with v"},
		{tag: "vfoo", wantErr: "not a valid semantic version"},
		{tag: "v1.3", wantErr: "not a valid semantic version"},
		{tag: "", wantErr: "must start with v"},
	}

	for _, tc := range testcases {
		t.Run(tc.tag, func(t *testing.T) {
			got, err := release.ValidatePushedTag(tc.tag)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("ValidatePushedTag(%q) error = %v, want containing %q", tc.tag, err, tc.wantErr)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Errorf("ValidatePushedTag(%q) = %q, %v, want %q", tc.tag, got, err, tc.want)
			}
		})
	}
}

func TestDistTagChanges(t *testing.T) {
	allTags := func(v string) map[string]string {
		return map[string]string{"latest": "1.2.3", "alpha": v, "beta": v, "rc": v}
	}

	testcases := []struct {
		name      string
		published []string
		distTags  map[string]string
		want      []release.DistTagChange
	}{
		{
			name:      "stable above the newest rc moves alpha, beta and rc",
			published: []string{"1.2.3", "1.3.0-rc.1", "1.3.0"},
			distTags:  allTags("1.3.0-rc.1"),
			want: []release.DistTagChange{
				{Tag: "alpha", Version: "1.3.0"}, {Tag: "beta", Version: "1.3.0"}, {Tag: "rc", Version: "1.3.0"},
			},
		},
		{
			name:      "a newer rc takes rc back",
			published: []string{"1.2.4", "1.3.0-rc.1"},
			distTags:  map[string]string{"latest": "1.2.4", "rc": "1.2.4"},
			want:      []release.DistTagChange{{Tag: "rc", Version: "1.3.0-rc.1"}},
		},
		{
			name:      "a lower rc published late does not win",
			published: []string{"1.2.3", "1.3.0-rc.2", "1.3.0-rc.1"},
			distTags:  map[string]string{"latest": "1.2.3", "rc": "1.3.0-rc.2"},
			want:      nil,
		},
		{
			name:      "a late publish that pulled rc down is repaired",
			published: []string{"1.2.3", "1.3.0-rc.2", "1.3.0-rc.1"},
			distTags:  map[string]string{"latest": "1.2.3", "rc": "1.3.0-rc.1"},
			want:      []release.DistTagChange{{Tag: "rc", Version: "1.3.0-rc.2"}},
		},
		{
			name:      "an rc moves alpha and beta",
			published: []string{"1.2.3", "1.3.0-rc.1"},
			distTags: map[string]string{
				"latest": "1.2.3", "alpha": "1.3.0-alpha.1", "beta": "1.3.0-beta.1", "rc": "1.3.0-rc.1",
			},
			want: []release.DistTagChange{{Tag: "alpha", Version: "1.3.0-rc.1"}, {Tag: "beta", Version: "1.3.0-rc.1"}},
		},
		{
			name:      "a dist-tag that does not exist is not created",
			published: []string{"1.2.3", "1.3.0-beta.1"},
			distTags:  map[string]string{"latest": "1.2.3"},
			want:      nil,
		},
		{
			name:      "latest is never changed",
			published: []string{"1.2.3", "1.2.4", "1.3.0-rc.1"},
			distTags:  map[string]string{"latest": "1.2.3"},
			want:      nil,
		},
		{
			name:      "a dist-tag outside the prerelease tracks is never touched",
			published: []string{"1.2.3", "1.2.4"},
			distTags:  map[string]string{"latest": "1.2.3", "nightly": "1.2.3-nightly.20261024"},
			want:      nil,
		},
		{
			name:      "a re-run after the reconcile changes nothing",
			published: []string{"1.2.3", "1.3.0-rc.1", "1.3.0"},
			distTags:  allTags("1.3.0"),
			want:      nil,
		},
		{
			name:      "versions given with a v prefix compare equal",
			published: []string{"v1.2.4"},
			distTags:  map[string]string{"rc": "1.2.4"},
			want:      nil,
		},
		{
			name:      "emitted versions never have a v prefix",
			published: []string{"v1.2.4"},
			distTags:  map[string]string{"rc": "1.2.3-rc.1"},
			want:      []release.DistTagChange{{Tag: "rc", Version: "1.2.4"}},
		},
		{
			name:      "nothing published in the path leaves the dist-tag alone",
			published: []string{"1.3.0-beta.1"},
			distTags:  map[string]string{"rc": "1.2.4-rc.1"},
			want:      nil,
		},
		{
			name:      "nightly versions are ignored",
			published: []string{"1.2.3", "1.3.0-nightly.1"},
			distTags:  map[string]string{"alpha": "1.2.3"},
			want:      nil,
		},
		{
			name:      "no published versions",
			published: nil,
			distTags:  allTags("1.3.0"),
			want:      nil,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			if got := release.DistTagChanges(tc.published, tc.distTags); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("DistTagChanges = %v, want %v", got, tc.want)
			}
		})
	}
}
