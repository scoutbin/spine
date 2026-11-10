// Package devcmd implements the handlers of the spine dev CLI, which tools run
// as "go tool spine". Results go to stdout as key=value lines, errors go to
// stderr with exit code 2.
package devcmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/scoutbin/spine/cli"
	"github.com/scoutbin/spine/internal/dist"
	"github.com/scoutbin/spine/internal/manifest"
	"github.com/scoutbin/spine/internal/release"
	"github.com/scoutbin/spine/internal/render"
)

// Contract is the version of the dev CLI outputs and the spine.json schema that
// the reusable workflows depend on. Raise it on any incompatible change to
// either; each workflow hardcodes the value it expects.
const Contract = 1

// Run executes args (without the program name) and returns the exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	root := &cli.Root{
		Command:  cli.Command{Name: "spine", Summary: "Development tool for projects built on spine"},
		Globals:  []cli.Flag{{Name: "manifest", Kind: cli.KindString, Default: "spine.json", Help: "Path to the manifest"}},
		Commands: []*cli.Command{manifestCommand(stdout), classifyCommand(stdout), distCommand(), genCommand()},
	}
	return root.Execute(args, nil, stdout, stderr)
}

func manifestCommand(stdout io.Writer) *cli.Command {
	return &cli.Command{
		Name:    "manifest",
		Summary: "Print the contract number and the manifest values as key=value lines",
		Run: func(ctx *cli.Context) error {
			tool, err := loadTool(ctx)
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

func distCommand() *cli.Command {
	return &cli.Command{
		Name:    "dist",
		Summary: "Build the release archives and checksums.txt for every target",
		Flags: []cli.Flag{
			{Name: "version", Kind: cli.KindString, Help: "Release version with a leading v, such as v1.2.3 or v0.0.0-ci"},
			{Name: "package", Kind: cli.KindString, Default: ".", Help: "Main package to build"},
			{Name: "out", Kind: cli.KindString, Default: "dist", Help: "Directory to write the artifacts to"},
		},
		Run: func(ctx *cli.Context) error {
			tool, err := loadTool(ctx)
			if err != nil {
				return err
			}
			return dist.Build(dist.Options{
				Tool:    tool,
				Version: ctx.String("version"),
				Package: ctx.String("package"),
				OutDir:  ctx.String("out"),
			})
		},
	}
}

func genCommand() *cli.Command {
	return &cli.Command{
		Name:    "gen",
		Summary: "Write the generated files of the tool: install.sh and install.ps1",
		Flags: []cli.Flag{
			{Name: "check", Kind: cli.KindBool, Help: "Fail if a file differs from what would be generated, and write nothing"},
			{Name: "out", Kind: cli.KindString, Default: ".", Help: "Repository root to write the files to"},
		},
		Run: func(ctx *cli.Context) error {
			tool, err := loadTool(ctx)
			if err != nil {
				return err
			}
			files, err := render.Files(tool, spineVersion())
			if err != nil {
				return err
			}
			if ctx.Bool("check") {
				return checkFiles(ctx.String("out"), files)
			}
			return writeFiles(ctx.String("out"), files)
		},
	}
}

func writeFiles(dir string, files []render.File) error {
	for _, f := range files {
		path := filepath.Join(dir, f.Path)
		if err := os.WriteFile(path, f.Content, f.Mode); err != nil {
			return err
		}
		if err := os.Chmod(path, f.Mode); err != nil {
			return err
		}
	}
	return nil
}

func checkFiles(dir string, files []render.File) error {
	var stale []string
	for _, f := range files {
		path := filepath.Join(dir, f.Path)
		current, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err != nil || !bytes.Equal(current, f.Content) {
			stale = append(stale, f.Path)
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if runtime.GOOS != "windows" && info.Mode()&0o111 != f.Mode&0o111 {
			stale = append(stale, f.Path)
		}
	}
	if len(stale) > 0 {
		return fmt.Errorf("out of date: %s; run go tool spine gen", strings.Join(stale, ", "))
	}
	return nil
}

// spineVersion is the version of the spine module this binary was built from,
// or "devel" for a build that is not from a released version.
func spineVersion() string {
	const module = "github.com/scoutbin/spine"
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "devel"
	}
	m := info.Main
	for _, dep := range info.Deps {
		if dep.Path == module {
			m = *dep
		}
	}
	if m.Path != module || m.Version == "" || m.Version == "(devel)" {
		return "devel"
	}
	return m.Version
}

func loadTool(ctx *cli.Context) (manifest.Tool, error) {
	data, err := os.ReadFile(ctx.String("manifest"))
	if err != nil {
		return manifest.Tool{}, err
	}
	return manifest.Load(data)
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
