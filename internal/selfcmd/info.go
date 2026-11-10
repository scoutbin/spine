package selfcmd

import (
	"encoding/json"
	"fmt"

	"github.com/scoutbin/spine/cli"
	"github.com/scoutbin/spine/internal/channel"
	"github.com/scoutbin/spine/internal/release"
)

type installInfo struct {
	Path    string          `json:"path"`
	Channel channel.Channel `json:"channel"`
	Version string          `json:"version"`
}

// installChannel resolves the running executable through symlinks and
// classifies how it was installed.
func (a *App) installChannel() (installInfo, error) {
	path, err := a.Executable()
	if err != nil {
		return installInfo{}, fmt.Errorf("locate executable: %w", err)
	}
	path, err = a.EvalSymlinks(path)
	if err != nil {
		return installInfo{}, fmt.Errorf("resolve executable path: %w", err)
	}

	_, isRelease := release.ParseVersion(a.Build.Version)
	return installInfo{
		Path:    path,
		Channel: channel.Detect(path, a.Build.OS, isRelease),
		Version: a.Build.Version,
	}, nil
}

func (a *App) info(ctx *cli.Context) error {
	got, err := a.installChannel()
	if err != nil {
		return err
	}
	if ctx.Bool("json") {
		return json.NewEncoder(a.Stdout).Encode(got)
	}
	_, err = fmt.Fprintf(a.Stdout, "path:    %s\nchannel: %s\nversion: %s\n", got.Path, got.Channel, got.Version)
	return err
}
