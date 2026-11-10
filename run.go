package spine

import (
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/scoutbin/spine/cli"
	"github.com/scoutbin/spine/internal/buildinfo"
	"github.com/scoutbin/spine/internal/release"
	"github.com/scoutbin/spine/internal/selfcmd"
	"github.com/scoutbin/spine/term"
)

// Env holds the process dependencies of Run, injected so tools can test their
// commands through the real root. Run fills every nil field of the first group
// from OSEnv, so a zero Env runs against the real process.
type Env struct {
	Stdout, Stderr io.Writer
	LookupEnv      func(string) (string, bool)
	Executable     func() (string, error)
	EvalSymlinks   func(string) (string, error)
	Detect         func(jsonOutput, noColor bool) term.Mode

	// Test overrides for the release lookup and download. The zero values
	// target GitHub with http.DefaultClient.
	HTTP         *http.Client
	GitHubAPI    string // API base URL
	DownloadBase string // release download base URL
}

// OSEnv returns the Env of the real process.
func OSEnv() Env {
	return Env{
		Stdout:       os.Stdout,
		Stderr:       os.Stderr,
		LookupEnv:    os.LookupEnv,
		Executable:   os.Executable,
		EvalSymlinks: filepath.EvalSymlinks,
		Detect: func(jsonOutput, noColor bool) term.Mode {
			return term.Detect(os.Stdout, os.Stderr, jsonOutput, noColor)
		},
	}
}

// Run executes args (without the program name) as the tool t, with the built-in
// commands version, info, check and upgrade next to cmds. It returns the exit
// code: 0 on success, 1 when check finds an update or a command returns a
// *cli.ExitError with that code, and 2 on any other error, including a tool
// command that reuses a built-in name.
func Run(t Tool, env Env, args []string, cmds ...*cli.Command) int {
	env = withOSDefaults(env)
	app := &selfcmd.App{
		Tool:         t,
		Stdout:       env.Stdout,
		Stderr:       env.Stderr,
		Build:        buildinfo.Current(),
		LookupEnv:    env.LookupEnv,
		Detect:       env.Detect,
		Executable:   env.Executable,
		EvalSymlinks: env.EvalSymlinks,
		GitHub:       release.Client{BaseURL: env.GitHubAPI, HTTP: env.HTTP, DownloadBase: env.DownloadBase},
	}
	return app.Run(args, cmds...)
}

func withOSDefaults(env Env) Env {
	real := OSEnv()
	if env.Stdout == nil {
		env.Stdout = real.Stdout
	}
	if env.Stderr == nil {
		env.Stderr = real.Stderr
	}
	if env.LookupEnv == nil {
		env.LookupEnv = real.LookupEnv
	}
	if env.Executable == nil {
		env.Executable = real.Executable
	}
	if env.EvalSymlinks == nil {
		env.EvalSymlinks = real.EvalSymlinks
	}
	if env.Detect == nil {
		env.Detect = real.Detect
	}
	return env
}
