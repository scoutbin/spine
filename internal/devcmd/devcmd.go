// Package devcmd implements the handlers of the spine dev CLI, which tools run
// as "go tool spine". Results go to stdout as key=value lines, errors go to
// stderr with exit code 2.
package devcmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/scoutbin/spine/cli"
	"github.com/scoutbin/spine/internal/manifest"
	"github.com/scoutbin/spine/internal/release"
)

// Contract is the version of the dev CLI outputs and the spine.json schema that
// the reusable workflows depend on. Raise it on any incompatible change to
// either; each workflow hardcodes the value it expects.
const Contract = 1

// Run executes args (without the program name) and returns the exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	root := &cli.Root{
		Command:  cli.Command{Name: "spine", Summary: "Development tool for projects built on spine"},
		Commands: []*cli.Command{manifestCommand(stdout), classifyCommand(stdout)},
	}
	return root.Execute(args, nil, stdout, stderr)
}

func manifestCommand(stdout io.Writer) *cli.Command {
	return &cli.Command{
		Name:    "manifest",
		Summary: "Print the contract number and the manifest values as key=value lines",
		Flags:   []cli.Flag{{Name: "manifest", Kind: cli.KindString, Default: "spine.json", Help: "Path to the manifest"}},
		Run: func(ctx *cli.Context) error {
			data, err := os.ReadFile(ctx.String("manifest"))
			if err != nil {
				return err
			}
			tool, err := manifest.Load(data)
			if err != nil {
				return err
			}
			return printPairs(stdout,
				"contract", fmt.Sprint(Contract),
				"name", tool.Name,
				"homebrew", enabled(tool.Channels.Homebrew.Tap),
				"homebrew_tap", tool.Channels.Homebrew.Tap,
				"homebrew_formula", tool.HomebrewFormula(),
				"npm", enabled(tool.Channels.Npm.Package),
				"npm_package", tool.Channels.Npm.Package,
				"action", fmt.Sprint(tool.Channels.Action),
			)
		},
	}
}

func classifyCommand(stdout io.Writer) *cli.Command {
	return &cli.Command{
		Name:    "classify",
		Summary: "Validate a pushed release tag and print its track as key=value lines",
		Args:    []cli.Arg{{Name: "tag", Required: true, Help: "Release tag such as v1.2.3 or v1.2.3-rc.1"}},
		Run: func(ctx *cli.Context) error {
			track, err := release.ValidatePushedTag(ctx.Arg("tag"))
			if err != nil {
				return err
			}
			return printPairs(stdout,
				"track", string(track),
				"prerelease", fmt.Sprint(track.Prerelease()),
				"dist_tag", track.DistTag(),
			)
		},
	}
}

func enabled(value string) string { return fmt.Sprint(value != "") }

// printPairs writes alternating keys and values as one write, so a failure
// never leaves partial output.
func printPairs(w io.Writer, kv ...string) error {
	var b strings.Builder
	for i := 0; i < len(kv); i += 2 {
		b.WriteString(kv[i] + "=" + kv[i+1] + "\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}
