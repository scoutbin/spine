// Package cli is a small command/flag framework: a root command with global
// flags, subcommands with local flags and positional args, and env fallbacks.
package cli

import "fmt"

type Kind int

const (
	KindBool Kind = iota
	KindString
)

// Flag is a bool or string flag. Default applies to string flags only; bool
// flags default to false. A bool Env fallback counts as set when the variable
// is non-empty (no-color.org); a string Env fallback is used when non-empty.
type Flag struct {
	Name    string // long name without dashes
	Short   string // optional single letter without dash
	Kind    Kind
	Default string
	Env     string
	Help    string
}

type Arg struct {
	Name     string
	Required bool
	Help     string
}

// Command declares one command. Required args must precede optional ones.
type Command struct {
	Name    string
	Summary string
	Flags   []Flag
	Args    []Arg
	Run     func(*Context) error
}

// Root is the top-level command. Globals are accepted before or after the
// command. The embedded Command's Flags are valid only when no subcommand is
// given. The built-in --help/-h global flag is always present.
type Root struct {
	Command
	Globals  []Flag
	Commands []*Command
}

var helpFlag = Flag{Name: "help", Short: "h", Kind: KindBool, Help: "Show help"}

// GlobalFlags returns the built-in help flag followed by Globals.
func (r *Root) GlobalFlags() []Flag {
	return append([]Flag{helpFlag}, r.Globals...)
}

// Context holds the parsed values for the selected command.
type Context struct {
	Command    *Command
	values     map[string]string
	kinds      map[string]Kind
	positional []string
}

// Bool returns a bool flag value. It panics on an unknown or non-bool flag.
func (c *Context) Bool(name string) bool {
	c.mustHave(name, KindBool)
	return c.values[name] == "true"
}

// String returns a string flag value. It panics on an unknown or non-string flag.
func (c *Context) String(name string) string {
	c.mustHave(name, KindString)
	return c.values[name]
}

// Args returns the positional arguments.
func (c *Context) Args() []string { return c.positional }

// Arg returns the positional argument declared as name, or "" if absent.
// It panics if the command declares no such arg.
func (c *Context) Arg(name string) string {
	for i, a := range c.Command.Args {
		if a.Name == name {
			if i < len(c.positional) {
				return c.positional[i]
			}
			return ""
		}
	}
	panic(fmt.Sprintf("cli: command %q has no arg %q", c.Command.Name, name))
}

func (c *Context) mustHave(name string, kind Kind) {
	if k, ok := c.kinds[name]; !ok || k != kind {
		panic(fmt.Sprintf("cli: command %q has no flag %q of that kind", c.Command.Name, name))
	}
}

// ExitError lets a command choose its exit code. Execute returns Code and
// prints nothing for it, so use it for outcomes that are not failures.
type ExitError struct{ Code int }

func (e *ExitError) Error() string { return fmt.Sprintf("exit code %d", e.Code) }

type ErrorKind int

const (
	UnknownFlag ErrorKind = iota + 1
	MissingValue
	InvalidValue
	UnknownCommand
	TooManyArgs
	MissingArgs
)

// UsageError is returned for every user-facing parse failure; main maps it
// to exit code 2.
type UsageError struct {
	Kind ErrorKind
	Msg  string
}

func (e *UsageError) Error() string { return e.Msg }

func usageErr(kind ErrorKind, format string, a ...any) error {
	return &UsageError{Kind: kind, Msg: fmt.Sprintf(format, a...)}
}
