// Package selfcmd implements the built-in commands every spine tool has:
// version, info, check and upgrade.
package selfcmd

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/scoutbin/spine/cli"
	"github.com/scoutbin/spine/internal/buildinfo"
	"github.com/scoutbin/spine/internal/manifest"
	"github.com/scoutbin/spine/internal/release"
	"github.com/scoutbin/spine/term"
)

// App holds what the built-in commands need, so they can be tested with
// injected writers and a fake terminal detector.
type App struct {
	Tool           manifest.Tool
	Stdout, Stderr io.Writer
	Build          buildinfo.Info
	LookupEnv      func(string) (string, bool)
	Detect         func(jsonOutput, noColor bool) term.Mode
	Executable     func() (string, error)
	EvalSymlinks   func(string) (string, error)
	GitHub         release.Client // its Tool is set from App.Tool
}

// Run executes args as the tool: the built-in commands plus cmds. It returns
// the exit code: 0 on success, the code of a command's *cli.ExitError, or 2 on
// any other error, including a command in cmds that reuses a built-in name.
func (a *App) Run(args []string, cmds ...*cli.Command) int {
	builtins := a.Commands()
	for _, c := range cmds {
		for _, b := range builtins {
			if c.Name == b.Name {
				fmt.Fprintf(a.Stderr, "error: command %q is built in and cannot be redefined\n", c.Name)
				return 2
			}
		}
	}

	root := &cli.Root{
		Command: cli.Command{
			Name:    a.Tool.Name,
			Summary: a.Tool.Summary,
			Flags:   []cli.Flag{{Name: "version", Kind: cli.KindBool, Help: "Print version information"}},
		},
		Globals: []cli.Flag{
			{Name: "json", Kind: cli.KindBool, Help: "Output machine-readable JSON"},
			{Name: "no-color", Kind: cli.KindBool, Env: "NO_COLOR", Help: "Disable colored output"},
		},
		Commands: append(builtins, cmds...),
	}
	root.Run = func(ctx *cli.Context) error {
		if ctx.Bool("version") {
			return a.version(ctx)
		}
		_, err := fmt.Fprint(a.Stdout, root.HelpText(&root.Command))
		return err
	}
	return root.Execute(args, a.LookupEnv, a.Stdout, a.Stderr)
}

// Commands returns the built-in commands.
func (a *App) Commands() []*cli.Command {
	return []*cli.Command{
		{Name: "version", Summary: "Print version, commit, build time and OS/arch", Run: a.version},
		{Name: "info", Summary: "Print executable path, install channel and version", Run: a.info},
		{Name: "check", Summary: "Check for a newer release (exit 1 if one is available)", Run: a.check},
		{
			Name:    "upgrade",
			Summary: "Upgrade to the latest release using the method matching how " + a.Tool.Name + " was installed",
			Flags:   []cli.Flag{{Name: "version", Kind: cli.KindString, Help: "Upgrade or downgrade to this release tag (install.sh installs only)"}},
			Run:     a.upgrade,
		},
	}
}

// version prints the version line, or the build info as JSON with --json.
func (a *App) version(ctx *cli.Context) error {
	if ctx.Bool("json") {
		return json.NewEncoder(a.Stdout).Encode(a.Build)
	}
	_, err := fmt.Fprintln(a.Stdout, a.Build.String(a.Tool.Name))
	return err
}

// client returns the GitHub client for Tool, authenticated when GITHUB_TOKEN is set.
func (a *App) client() release.Client {
	c := a.GitHub
	c.Tool = a.Tool
	if token, ok := a.LookupEnv("GITHUB_TOKEN"); ok {
		c.Token = token
	}
	return c
}

// buildTrack returns the track of the running build. A version that is not a
// pushed release tag (dev, a git describe string, v0.0.0-ci) counts as stable.
// That fallback is only safe because the release workflow gates pushed tags
// with release.ValidatePushedTag, so a shipped binary never takes it.
func (a *App) buildTrack() release.Track {
	if track, err := release.ValidatePushedTag(a.Build.Version); err == nil {
		return track
	}
	return release.Stable
}

// reporter returns the progress reporter for a command, honoring --json,
// --no-color and NO_COLOR. It writes to stderr.
func (a *App) reporter(ctx *cli.Context) term.Reporter {
	return term.NewReporter(a.Stderr, a.Detect(ctx.Bool("json"), ctx.Bool("no-color")))
}
