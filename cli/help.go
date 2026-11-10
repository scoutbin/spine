package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"
)

// helpCommand is the built-in "help [command]" command. It is selectable like
// any other command but is not listed in Root.Commands.
var helpCommand = &Command{
	Name:    "help",
	Summary: "Show help for the program or a command",
	Args:    []Arg{{Name: "command", Help: "Command to show help for"}},
}

// HelpTarget reports which command's help ctx asks for: --help/-h on any
// command, or "help [command]". ok is false when no help was requested.
// "help <unknown>" is a UsageError.
func (r *Root) HelpTarget(ctx *Context) (target *Command, ok bool, err error) {
	if ctx.Bool(helpFlag.Name) {
		return ctx.Command, true, nil
	}
	if ctx.Command != helpCommand {
		return nil, false, nil
	}
	name := ctx.Arg("command")
	if name == "" {
		return &r.Command, true, nil
	}
	target = r.find(name)
	if target == nil {
		return nil, false, usageErr(UnknownCommand, "unknown command %q", name)
	}
	return target, true, nil
}

// HelpText renders help for cmd (the root or one of its commands) from the
// definitions alone.
func (r *Root) HelpText(cmd *Command) string {
	var b strings.Builder
	if cmd.Summary != "" {
		fmt.Fprintf(&b, "%s\n\n", cmd.Summary)
	}
	fmt.Fprintf(&b, "Usage:\n  %s\n", r.usage(cmd))

	isRoot := cmd == &r.Command
	local := cmd.Flags
	var commands [][2]string
	if isRoot {
		local = r.Flags
		for _, c := range append(append([]*Command{}, r.Commands...), helpCommand) {
			commands = append(commands, [2]string{c.Name, c.Summary})
		}
	}
	var args [][2]string
	for _, a := range cmd.Args {
		args = append(args, [2]string{argLabel(a), a.Help})
	}

	writeSection(&b, "Commands", commands)
	writeSection(&b, "Arguments", args)
	writeSection(&b, "Flags", flagRows(local))
	writeSection(&b, "Global flags", flagRows(r.GlobalFlags()))
	if isRoot {
		fmt.Fprintf(&b, "\nRun '%s help <command>' for more information on a command.\n", r.Name)
	}
	return b.String()
}

// Execute parses args, prints help to stdout or usage errors to stderr, and
// otherwise runs the selected command (a command without Run shows its help).
// It returns the process exit code: 0 on success, the code of an *ExitError
// returned by the command, or 2 on any other error.
func (r *Root) Execute(args []string, lookupEnv func(string) (string, bool), stdout, stderr io.Writer) int {
	ctx, err := r.Parse(args, lookupEnv)
	if err != nil {
		return r.fail(stderr, err)
	}
	target, ok, err := r.HelpTarget(ctx)
	if err != nil {
		return r.fail(stderr, err)
	}
	if !ok && ctx.Command.Run == nil {
		target, ok = ctx.Command, true
	}
	if ok {
		fmt.Fprint(stdout, r.HelpText(target))
		return 0
	}
	if err := ctx.Command.Run(ctx); err != nil {
		return r.fail(stderr, err)
	}
	return 0
}

func (r *Root) fail(stderr io.Writer, err error) int {
	var exit *ExitError
	if errors.As(err, &exit) {
		return exit.Code
	}
	fmt.Fprintf(stderr, "error: %s\n", err)
	var usage *UsageError
	if errors.As(err, &usage) {
		fmt.Fprintf(stderr, "Run '%s help' for usage.\n", r.Name)
	}
	return 2
}

func (r *Root) usage(cmd *Command) string {
	if cmd == &r.Command {
		return r.Name + " [command] [flags]"
	}
	parts := []string{r.Name, cmd.Name, "[flags]"}
	for _, a := range cmd.Args {
		parts = append(parts, argLabel(a))
	}
	return strings.Join(parts, " ")
}

func argLabel(a Arg) string {
	if a.Required {
		return "<" + a.Name + ">"
	}
	return "[" + a.Name + "]"
}

func flagRows(flags []Flag) [][2]string {
	rows := make([][2]string, 0, len(flags))
	for _, f := range flags {
		label := "    --" + f.Name
		if f.Short != "" {
			label = "-" + f.Short + ", --" + f.Name
		}
		if f.Kind == KindString {
			label += " <value>"
		}
		var notes []string
		if f.Kind == KindString && f.Default != "" {
			notes = append(notes, "default "+f.Default)
		}
		if f.Env != "" {
			notes = append(notes, "env "+f.Env)
		}
		help := f.Help
		if len(notes) > 0 {
			help += " (" + strings.Join(notes, ", ") + ")"
		}
		rows = append(rows, [2]string{label, help})
	}
	return rows
}

// writeSection writes an aligned two-column section, skipping empty ones.
func writeSection(b *strings.Builder, title string, rows [][2]string) {
	if len(rows) == 0 {
		return
	}
	width := 0
	for _, row := range rows {
		width = max(width, len(row[0]))
	}
	fmt.Fprintf(b, "\n%s:\n", title)
	for _, row := range rows {
		line := "  " + row[0]
		if row[1] != "" {
			line += strings.Repeat(" ", width-len(row[0])) + "  " + row[1]
		}
		b.WriteString(line + "\n")
	}
}
