package release

import (
	"fmt"
	"slices"
	"strings"
)

// Track is the release type of a version, inferred from the version string.
type Track string

const (
	Stable Track = "stable"
	Alpha  Track = "alpha"
	Beta   Track = "beta"
	RC     Track = "rc"
)

var upgradePaths = map[Track][]Track{
	Stable: {Stable},
	RC:     {RC, Stable},
	Beta:   {Beta, RC, Stable},
	Alpha:  {Alpha, Beta, RC, Stable},
}

// distTagTracks are the npm dist-tags the reconcile moves. "latest" is set by
// npm publish and never changed here.
var distTagTracks = []Track{Alpha, Beta, RC}

// DistTagChange sets the npm dist-tag Tag to Version.
type DistTagChange struct {
	Tag     string
	Version string
}

// TrackOf infers the track of a version. A version with no prerelease part is
// stable. Otherwise the first prerelease identifier names the track. Anything
// else (v0.0.0-ci, a git describe string, "dev") is stable. It is not for
// classifying release tags: release code must use ValidatePushedTag and fail on
// its error.
func TrackOf(version string) Track {
	v, ok := ParseVersion(version)
	if !ok {
		return Stable
	}
	if t, ok := classify(v); ok {
		return t
	}
	return Stable
}

// Path returns the tracks a user on t can upgrade to, including t itself.
func (t Track) Path() []Track {
	if path, ok := upgradePaths[t]; ok {
		return slices.Clone(path)
	}
	return []Track{Stable}
}

// DistTag returns the npm dist-tag that releases of t are published under.
func (t Track) DistTag() string {
	if t == Stable {
		return "latest"
	}
	return string(t)
}

// Prerelease reports whether releases of t are published as GitHub prereleases.
func (t Track) Prerelease() bool {
	return t != Stable
}

// HighestInPath returns the highest release in the upgrade path of track. It is
// what a fresh install on that track resolves to. Tags that do not parse or
// belong to no track are ignored.
func HighestInPath(track Track, releases []string) (string, bool) {
	return highestTag(releases, inTracks(track.Path()))
}

// SelectTarget returns the highest release in the upgrade path of current's
// track, if it is higher than current.
func SelectTarget(current string, releases []string) (string, bool) {
	target, ok := HighestInPath(TrackOf(current), releases)
	if !ok || Compare(target, current) <= 0 {
		return "", false
	}
	return target, true
}

// ValidatePushedTag accepts the tags a person pushes to release: vX.Y.Z and
// vX.Y.Z-(alpha|beta|rc).N with N >= 1.
func ValidatePushedTag(tag string) (Track, error) {
	if !strings.HasPrefix(tag, "v") {
		return "", fmt.Errorf("tag %q must start with v", tag)
	}
	v, ok := ParseVersion(tag)
	if !ok {
		return "", fmt.Errorf("tag %q is not a valid semantic version such as v1.2.3", tag)
	}
	if strings.Contains(tag, "+") {
		return "", fmt.Errorf("tag %q must not have build metadata", tag)
	}
	if len(v.Pre) == 0 {
		return Stable, nil
	}
	track, known := classify(v)
	switch {
	case !known:
		return "", fmt.Errorf("tag %q has an unknown prerelease: want -alpha.N, -beta.N or -rc.N", tag)
	case len(v.Pre) != 2 || !isNumeric(v.Pre[1]) || v.Pre[1] == "0":
		return "", fmt.Errorf("tag %q must look like v1.2.3-%s.N with N of 1 or more", tag, track)
	}
	return track, nil
}

// DistTagChanges returns the npm dist-tag moves that make each existing
// alpha, beta and rc dist-tag point at the highest published version
// in that track's upgrade path. A dist-tag that does not exist is not created,
// and "latest" is never touched. Versions may be given with or without "v"; the
// emitted versions never have one.
func DistTagChanges(published []string, distTags map[string]string) []DistTagChange {
	var changes []DistTagChange
	for _, track := range distTagTracks {
		current, exists := distTags[track.DistTag()]
		if !exists {
			continue
		}
		target, ok := HighestInPath(track, published)
		if !ok || Compare(current, target) == 0 {
			continue
		}
		changes = append(changes, DistTagChange{Tag: track.DistTag(), Version: strings.TrimPrefix(target, "v")})
	}
	return changes
}

func classify(v Version) (Track, bool) {
	if len(v.Pre) == 0 {
		return Stable, true
	}
	switch t := Track(v.Pre[0]); t {
	case Alpha, Beta, RC:
		return t, true
	}
	return "", false
}

func inTracks(tracks []Track) func(Version) bool {
	return func(v Version) bool {
		t, ok := classify(v)
		return ok && slices.Contains(tracks, t)
	}
}

func highestTag(tags []string, accept func(Version) bool) (string, bool) {
	var best string
	var bestVersion Version
	found := false
	for _, tag := range tags {
		v, ok := ParseVersion(tag)
		if !ok || !accept(v) {
			continue
		}
		if !found || v.Compare(bestVersion) > 0 {
			best, bestVersion, found = tag, v, true
		}
	}
	return best, found
}
