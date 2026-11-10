package selfcmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/scoutbin/spine/cli"
	"github.com/scoutbin/spine/internal/release"
)

type checkResult struct {
	Current         string `json:"current"`
	Latest          string `json:"latest"`
	UpdateAvailable bool   `json:"update_available"`
	Track           string `json:"track"`
}

// latestRelease returns the newest release a build of current should have:
// releases/latest for stable, otherwise the highest release in the upgrade path
// of its track. It returns current when the path holds no release.
func (a *App) latestRelease(ctx context.Context, client release.Client) (string, error) {
	track := a.buildTrack()
	if track == release.Stable {
		return client.LatestTag(ctx)
	}
	tags, err := client.ReleaseTags(ctx)
	if err != nil {
		return "", err
	}
	if tag, ok := release.HighestInPath(track, tags); ok && release.Compare(tag, a.Build.Version) >= 0 {
		return tag, nil
	}
	return a.Build.Version, nil
}

// check exits 0 when up to date and 1 when an update is available. A build
// that does not parse as a release (such as dev) is older than any release.
func (a *App) check(ctx *cli.Context) error {
	task := a.reporter(ctx).Spin("Checking for updates")
	latest, err := a.latestRelease(context.Background(), a.client())
	if err != nil {
		task.Fail("Could not check for updates")
		return fmt.Errorf("look up latest release: %w", err)
	}
	task.Done("Latest release is " + latest)

	got := checkResult{
		Current:         a.Build.Version,
		Latest:          latest,
		UpdateAvailable: release.Compare(a.Build.Version, latest) < 0,
		Track:           string(a.buildTrack()),
	}
	if err := a.printCheck(ctx, got); err != nil {
		return err
	}
	if got.UpdateAvailable {
		return &cli.ExitError{Code: 1}
	}
	return nil
}

func (a *App) printCheck(ctx *cli.Context, got checkResult) error {
	track := ""
	if got.Track != string(release.Stable) {
		track = "track: " + got.Track
	}
	var err error
	switch {
	case ctx.Bool("json"):
		err = json.NewEncoder(a.Stdout).Encode(got)
	case got.UpdateAvailable && track != "":
		_, err = fmt.Fprintf(a.Stdout, "Update available: %s → %s (%s)\n", got.Current, got.Latest, track)
	case got.UpdateAvailable:
		_, err = fmt.Fprintf(a.Stdout, "Update available: %s → %s\n", got.Current, got.Latest)
	case track != "":
		_, err = fmt.Fprintf(a.Stdout, "Up to date (%s, %s)\n", got.Current, track)
	default:
		_, err = fmt.Fprintf(a.Stdout, "Up to date (%s)\n", got.Current)
	}
	return err
}
