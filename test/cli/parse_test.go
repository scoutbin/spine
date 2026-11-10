package cli_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/scoutbin/spine/cli"
)

func testRoot() *cli.Root {
	return &cli.Root{
		Command: cli.Command{
			Name:  "app",
			Flags: []cli.Flag{{Name: "version", Kind: cli.KindBool}},
		},
		Globals: []cli.Flag{
			{Name: "json", Kind: cli.KindBool},
			{Name: "no-color", Kind: cli.KindBool, Env: "NO_COLOR"},
		},
		Commands: []*cli.Command{
			{Name: "version"},
			{Name: "upgrade", Flags: []cli.Flag{
				{Name: "version", Short: "v", Kind: cli.KindString, Env: "APP_VERSION"},
				{Name: "channel", Kind: cli.KindString, Default: "stable"},
			}},
			{Name: "greet", Args: []cli.Arg{{Name: "name", Required: true}, {Name: "title"}}},
		},
	}
}

func TestParse(t *testing.T) {
	testcases := []struct {
		name        string
		args        []string
		env         map[string]string
		wantCmd     string
		wantBools   map[string]bool
		wantStrings map[string]string
		wantArgs    []string
		wantErr     cli.ErrorKind
	}{
		{name: "no args selects root", wantCmd: "app"},
		{name: "root bool --version", args: []string{"--version"}, wantCmd: "app", wantBools: map[string]bool{"version": true}},
		{name: "global before command", args: []string{"--json", "version"}, wantCmd: "version", wantBools: map[string]bool{"json": true}},
		{name: "global after command", args: []string{"version", "--json"}, wantCmd: "version", wantBools: map[string]bool{"json": true}},
		{name: "global on root", args: []string{"--json"}, wantCmd: "app", wantBools: map[string]bool{"json": true}},
		{name: "-h on root", args: []string{"-h"}, wantCmd: "app", wantBools: map[string]bool{"help": true}},
		{name: "--help after command", args: []string{"upgrade", "--help"}, wantCmd: "upgrade", wantBools: map[string]bool{"help": true}},
		{name: "-h skips arg validation", args: []string{"greet", "-h"}, wantCmd: "greet", wantBools: map[string]bool{"help": true}},
		{name: "subcommand string --version value", args: []string{"upgrade", "--version", "v0.2.0"}, wantCmd: "upgrade", wantStrings: map[string]string{"version": "v0.2.0"}},
		{name: "--flag=value", args: []string{"upgrade", "--version=v0.2.0"}, wantCmd: "upgrade", wantStrings: map[string]string{"version": "v0.2.0"}},
		{name: "short flag value", args: []string{"upgrade", "-v", "v0.2.0"}, wantCmd: "upgrade", wantStrings: map[string]string{"version": "v0.2.0"}},
		{name: "short flag =value", args: []string{"upgrade", "-v=v0.2.0"}, wantCmd: "upgrade", wantStrings: map[string]string{"version": "v0.2.0"}},
		{name: "string default", args: []string{"upgrade"}, wantCmd: "upgrade", wantStrings: map[string]string{"channel": "stable", "version": ""}},
		{name: "global between command flags", args: []string{"upgrade", "--json", "--channel", "beta"}, wantCmd: "upgrade", wantBools: map[string]bool{"json": true}, wantStrings: map[string]string{"channel": "beta"}},
		{name: "bool explicit false", args: []string{"--json=false"}, wantCmd: "app", wantBools: map[string]bool{"json": false}},
		{name: "missing string value at end", args: []string{"upgrade", "--version"}, wantErr: cli.MissingValue},
		{name: "missing string value before flag", args: []string{"upgrade", "--version", "--json"}, wantErr: cli.MissingValue},
		{name: "missing string value before --", args: []string{"upgrade", "--version", "--"}, wantErr: cli.MissingValue},
		{name: "invalid bool value", args: []string{"--json=maybe"}, wantErr: cli.InvalidValue},
		{name: "unknown long flag", args: []string{"--bogus"}, wantErr: cli.UnknownFlag},
		{name: "unknown short flag", args: []string{"version", "-x"}, wantErr: cli.UnknownFlag},
		{name: "empty short name", args: []string{"-=true", "version"}, wantErr: cli.UnknownFlag},
		{name: "empty long name", args: []string{"--=x", "version"}, wantErr: cli.UnknownFlag},
		{name: "other command's local flag", args: []string{"version", "--version"}, wantErr: cli.UnknownFlag},
		{name: "root local flag before command", args: []string{"--version", "version"}, wantErr: cli.UnknownFlag},
		{name: "subcommand local flag before command", args: []string{"--channel", "beta", "upgrade"}, wantErr: cli.UnknownFlag},
		{name: "unknown command", args: []string{"nope"}, wantErr: cli.UnknownCommand},
		{name: "too many positionals", args: []string{"version", "extra"}, wantErr: cli.TooManyArgs},
		{name: "too many positionals with spec", args: []string{"greet", "a", "b", "c"}, wantErr: cli.TooManyArgs},
		{name: "too few positionals", args: []string{"greet"}, wantErr: cli.MissingArgs},
		{name: "required only", args: []string{"greet", "bob"}, wantCmd: "greet", wantArgs: []string{"bob"}},
		{name: "required and optional", args: []string{"greet", "bob", "dr"}, wantCmd: "greet", wantArgs: []string{"bob", "dr"}},
		{name: "flag between positionals", args: []string{"greet", "bob", "--json", "dr"}, wantCmd: "greet", wantBools: map[string]bool{"json": true}, wantArgs: []string{"bob", "dr"}},
		{name: "tokens after -- are positionals", args: []string{"greet", "--", "--json", "-x"}, wantCmd: "greet", wantBools: map[string]bool{"json": false}, wantArgs: []string{"--json", "-x"}},
		{name: "-- after positional", args: []string{"greet", "bob", "--", "-v"}, wantCmd: "greet", wantArgs: []string{"bob", "-v"}},
		{name: "-- before command selects command", args: []string{"--", "version"}, wantCmd: "version"},
		{name: "lone dash is positional", args: []string{"greet", "-"}, wantCmd: "greet", wantArgs: []string{"-"}},
		{name: "bool env non-empty sets", args: []string{"version"}, env: map[string]string{"NO_COLOR": "1"}, wantCmd: "version", wantBools: map[string]bool{"no-color": true}},
		{name: "bool env any non-empty value sets", args: []string{"version"}, env: map[string]string{"NO_COLOR": "0"}, wantCmd: "version", wantBools: map[string]bool{"no-color": true}},
		{name: "bool env empty ignored", args: []string{"version"}, env: map[string]string{"NO_COLOR": ""}, wantCmd: "version", wantBools: map[string]bool{"no-color": false}},
		{name: "bool env overridden by flag", args: []string{"--no-color=false"}, env: map[string]string{"NO_COLOR": "1"}, wantCmd: "app", wantBools: map[string]bool{"no-color": false}},
		{name: "string env used when flag absent", args: []string{"upgrade"}, env: map[string]string{"APP_VERSION": "v1.0.0"}, wantCmd: "upgrade", wantStrings: map[string]string{"version": "v1.0.0"}},
		{name: "string env overridden by flag", args: []string{"upgrade", "--version", "v2.0.0"}, env: map[string]string{"APP_VERSION": "v1.0.0"}, wantCmd: "upgrade", wantStrings: map[string]string{"version": "v2.0.0"}},
		{name: "string env empty falls to default", args: []string{"upgrade"}, env: map[string]string{"APP_VERSION": ""}, wantCmd: "upgrade", wantStrings: map[string]string{"version": ""}},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			lookupEnv := func(k string) (string, bool) { v, ok := tc.env[k]; return v, ok }
			ctx, err := testRoot().Parse(tc.args, lookupEnv)

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
			if ctx.Command.Name != tc.wantCmd {
				t.Errorf("command = %q, want %q", ctx.Command.Name, tc.wantCmd)
			}
			for name, want := range tc.wantBools {
				if got := ctx.Bool(name); got != want {
					t.Errorf("Bool(%q) = %v, want %v", name, got, want)
				}
			}
			for name, want := range tc.wantStrings {
				if got := ctx.String(name); got != want {
					t.Errorf("String(%q) = %q, want %q", name, got, want)
				}
			}
			if len(tc.wantArgs) > 0 && !reflect.DeepEqual(ctx.Args(), tc.wantArgs) {
				t.Errorf("Args() = %v, want %v", ctx.Args(), tc.wantArgs)
			}
		})
	}
}

func TestParseKeepsRootAndSubcommandVersionApart(t *testing.T) {
	testcases := []struct {
		name string
		args []string
		want func(*cli.Context) bool
	}{
		{"root --version is bool", []string{"--version"}, func(c *cli.Context) bool { return c.Bool("version") }},
		{"upgrade --version is string", []string{"upgrade", "--version", "v0.2.0"}, func(c *cli.Context) bool { return c.String("version") == "v0.2.0" }},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, err := testRoot().Parse(tc.args, nil)
			if err != nil || !tc.want(ctx) {
				t.Fatalf("ctx = %+v, err = %v", ctx, err)
			}
		})
	}
}

func TestArgAndKindAccessors(t *testing.T) {
	ctx, err := testRoot().Parse([]string{"greet", "bob"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	testcases := []struct {
		name      string
		call      func()
		wantPanic bool
	}{
		{"present arg", func() { _ = ctx.Arg("name") }, false},
		{"absent optional arg", func() { _ = ctx.Arg("title") }, false},
		{"unknown arg", func() { _ = ctx.Arg("nope") }, true},
		{"unknown flag", func() { _ = ctx.Bool("nope") }, true},
		{"wrong kind", func() { _ = ctx.String("json") }, true},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if (recover() != nil) != tc.wantPanic {
					t.Errorf("panic = %v, want %v", !tc.wantPanic, tc.wantPanic)
				}
			}()
			tc.call()
		})
	}
	if got := ctx.Arg("name"); got != "bob" {
		t.Errorf("Arg(name) = %q, want bob", got)
	}
	if got := ctx.Arg("title"); got != "" {
		t.Errorf("Arg(title) = %q, want empty", got)
	}
}

func TestParseRejectsInvalidDefinitions(t *testing.T) {
	testcases := []struct {
		name   string
		mutate func(*cli.Root)
	}{
		{"duplicate global and local flag", func(r *cli.Root) { r.Commands[0].Flags = []cli.Flag{{Name: "json", Kind: cli.KindBool}} }},
		{"shadowing built-in help", func(r *cli.Root) { r.Globals = append(r.Globals, cli.Flag{Name: "help", Kind: cli.KindBool}) }},
		{"duplicate short", func(r *cli.Root) { r.Globals = append(r.Globals, cli.Flag{Name: "x", Short: "h", Kind: cli.KindBool}) }},
		{"multi-letter short", func(r *cli.Root) { r.Globals = append(r.Globals, cli.Flag{Name: "x", Short: "xy", Kind: cli.KindBool}) }},
		{"bool with default", func(r *cli.Root) {
			r.Globals = append(r.Globals, cli.Flag{Name: "x", Kind: cli.KindBool, Default: "true"})
		}},
		{"required after optional", func(r *cli.Root) { r.Commands[2].Args = []cli.Arg{{Name: "a"}, {Name: "b", Required: true}} }},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			root := testRoot()
			tc.mutate(root)
			_, err := root.Parse(nil, nil)
			var usage *cli.UsageError
			if err == nil || errors.As(err, &usage) {
				t.Fatalf("err = %v, want non-usage definition error", err)
			}
		})
	}
}
