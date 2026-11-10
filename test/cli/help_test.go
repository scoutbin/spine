package cli_test

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/scoutbin/spine/cli"
)

func helpRoot() *cli.Root {
	r := testRoot()
	r.Summary = "Test app"
	r.Flags[0].Help = "Print the version"
	r.Globals[0].Help = "Output JSON"
	r.Globals[1].Help = "Disable color"
	r.Commands[0].Summary = "Print version details"
	r.Commands[1].Summary = "Upgrade app"
	r.Commands[1].Flags[0].Help = "Target release"
	r.Commands[1].Flags[1].Help = "Release channel"
	r.Commands[2].Summary = "Greet someone"
	r.Commands[2].Args[0].Help = "Who to greet"
	return r
}

func TestHelpTarget(t *testing.T) {
	testcases := []struct {
		name       string
		args       []string
		wantTarget string // empty: no help requested
		wantErr    cli.ErrorKind
	}{
		{name: "help", args: []string{"help"}, wantTarget: "app"},
		{name: "--help", args: []string{"--help"}, wantTarget: "app"},
		{name: "-h", args: []string{"-h"}, wantTarget: "app"},
		{name: "help <command>", args: []string{"help", "upgrade"}, wantTarget: "upgrade"},
		{name: "help <command> with global flag", args: []string{"help", "version", "--json"}, wantTarget: "version"},
		{name: "<command> --help", args: []string{"upgrade", "--help"}, wantTarget: "upgrade"},
		{name: "<command> -h", args: []string{"version", "-h"}, wantTarget: "version"},
		{name: "-h skips positional validation", args: []string{"greet", "-h"}, wantTarget: "greet"},
		{name: "help help", args: []string{"help", "help"}, wantTarget: "help"},
		{name: "help --help", args: []string{"help", "--help"}, wantTarget: "help"},
		{name: "help <unknown>", args: []string{"help", "nope"}, wantErr: cli.UnknownCommand},
		{name: "help with too many args", args: []string{"help", "version", "upgrade"}, wantErr: cli.TooManyArgs},
		{name: "plain command is not help", args: []string{"version"}},
		{name: "no args is not help", args: nil},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			root := helpRoot()
			ctx, err := root.Parse(tc.args, nil)
			var target *cli.Command
			var ok bool
			if err == nil {
				target, ok, err = root.HelpTarget(ctx)
			}

			if tc.wantErr != 0 {
				var usage *cli.UsageError
				if !errors.As(err, &usage) || usage.Kind != tc.wantErr {
					t.Fatalf("err = %v, want UsageError kind %d", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ok != (tc.wantTarget != "") {
				t.Fatalf("help requested = %v, want %v", ok, tc.wantTarget != "")
			}
			if ok && target.Name != tc.wantTarget {
				t.Errorf("target = %q, want %q", target.Name, tc.wantTarget)
			}
		})
	}
}

func TestParseRejectsUserCommandNamedHelp(t *testing.T) {
	root := helpRoot()
	root.Commands = append(root.Commands, &cli.Command{Name: "help"})
	if _, err := root.Parse(nil, nil); err == nil {
		t.Fatal("want definition error for a command named help")
	}
}

func TestHelpText(t *testing.T) {
	testcases := []struct {
		name   string
		target func(*cli.Root) *cli.Command
		want   string
	}{
		{
			name:   "root",
			target: func(r *cli.Root) *cli.Command { return &r.Command },
			want: `Test app

Usage:
  app [command] [flags]

Commands:
  version  Print version details
  upgrade  Upgrade app
  greet    Greet someone
  help     Show help for the program or a command

Flags:
      --version  Print the version

Global flags:
  -h, --help      Show help
      --json      Output JSON
      --no-color  Disable color (env NO_COLOR)

Run 'app help <command>' for more information on a command.
`,
		},
		{
			name:   "command with flags and no args",
			target: func(r *cli.Root) *cli.Command { return r.Commands[1] },
			want: `Upgrade app

Usage:
  app upgrade [flags]

Flags:
  -v, --version <value>  Target release (env APP_VERSION)
      --channel <value>  Release channel (default stable)

Global flags:
  -h, --help      Show help
      --json      Output JSON
      --no-color  Disable color (env NO_COLOR)
`,
		},
		{
			name:   "command with required and optional args",
			target: func(r *cli.Root) *cli.Command { return r.Commands[2] },
			want: `Greet someone

Usage:
  app greet [flags] <name> [title]

Arguments:
  <name>   Who to greet
  [title]

Global flags:
  -h, --help      Show help
      --json      Output JSON
      --no-color  Disable color (env NO_COLOR)
`,
		},
		{
			name: "help command",
			target: func(r *cli.Root) *cli.Command {
				ctx, err := r.Parse([]string{"help"}, nil)
				if err != nil {
					t.Fatalf("Parse(help): %v", err)
				}
				return ctx.Command
			},
			want: `Show help for the program or a command

Usage:
  app help [flags] [command]

Arguments:
  [command]  Command to show help for

Global flags:
  -h, --help      Show help
      --json      Output JSON
      --no-color  Disable color (env NO_COLOR)
`,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			root := helpRoot()
			got := root.HelpText(tc.target(root))
			if got != tc.want {
				t.Errorf("help text mismatch\ngot:\n%s\nwant:\n%s", got, tc.want)
			}
		})
	}
}

func TestHelpTextIsDerivedFromDefinitions(t *testing.T) {
	testcases := []struct {
		name   string
		mutate func(*cli.Root)
		want   string
	}{
		{"new command", func(r *cli.Root) {
			r.Commands = append(r.Commands, &cli.Command{Name: "check", Summary: "Look for updates"})
		}, "check    Look for updates"},
		{"new global flag", func(r *cli.Root) {
			r.Globals = append(r.Globals, cli.Flag{Name: "quiet", Kind: cli.KindBool, Help: "Less output"})
		}, "--quiet     Less output"},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			root := helpRoot()
			tc.mutate(root)
			if got := root.HelpText(&root.Command); !strings.Contains(got, tc.want) {
				t.Errorf("help text missing %q:\n%s", tc.want, got)
			}
		})
	}
}

func TestExecute(t *testing.T) {
	testcases := []struct {
		name       string
		args       []string
		runErr     error
		wantCode   int
		wantStdout string // substring
		wantStderr string // substring
		wantRan    bool
	}{
		{name: "help", args: []string{"help"}, wantStdout: "Usage:\n  app [command] [flags]"},
		{name: "--help", args: []string{"--help"}, wantStdout: "Commands:"},
		{name: "command --help", args: []string{"upgrade", "--help"}, wantStdout: "app upgrade [flags]"},
		{name: "help <command>", args: []string{"help", "version"}, wantStdout: "app version [flags]"},
		{name: "help <unknown>", args: []string{"help", "nope"}, wantCode: 2, wantStderr: "error: unknown command \"nope\"\nRun 'app help' for usage.\n"},
		{name: "unknown command", args: []string{"nope"}, wantCode: 2, wantStderr: "Run 'app help' for usage."},
		{name: "unknown flag", args: []string{"--bogus"}, wantCode: 2, wantStderr: "error: unknown flag --bogus"},
		{name: "runs selected command", args: []string{"version"}, wantRan: true},
		{name: "run error exits 2 without hint", args: []string{"version"}, runErr: errors.New("boom"), wantCode: 2, wantStderr: "error: boom\n", wantRan: true},
		{name: "ExitError code 1 is silent", args: []string{"version"}, runErr: &cli.ExitError{Code: 1}, wantCode: 1, wantRan: true},
		{name: "ExitError code 0 is success", args: []string{"version"}, runErr: &cli.ExitError{Code: 0}, wantRan: true},
		{name: "wrapped ExitError is recognised", args: []string{"version"}, runErr: fmt.Errorf("ctx: %w", &cli.ExitError{Code: 3}), wantCode: 3, wantRan: true},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			root := helpRoot()
			ran := false
			root.Commands[0].Run = func(*cli.Context) error { ran = true; return tc.runErr }
			var stdout, stderr bytes.Buffer

			code := root.Execute(tc.args, nil, &stdout, &stderr)

			if code != tc.wantCode {
				t.Errorf("code = %d, want %d", code, tc.wantCode)
			}
			if ran != tc.wantRan {
				t.Errorf("ran = %v, want %v", ran, tc.wantRan)
			}
			if !strings.Contains(stdout.String(), tc.wantStdout) {
				t.Errorf("stdout = %q, want substring %q", stdout.String(), tc.wantStdout)
			}
			if !strings.Contains(stderr.String(), tc.wantStderr) {
				t.Errorf("stderr = %q, want substring %q", stderr.String(), tc.wantStderr)
			}
			if tc.wantCode == 2 && stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty on error", stdout.String())
			}
			if tc.wantCode == 0 && stderr.Len() != 0 {
				t.Errorf("stderr = %q, want empty on success", stderr.String())
			}
		})
	}
}
