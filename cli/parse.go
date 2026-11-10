package cli

import (
	"fmt"
	"strconv"
	"strings"
)

// Parse resolves args (without the program name) to a command and its values.
// Precedence per flag: command line, then env var (via lookupEnv, may be
// nil), then default. Global flags work before or after the command; local
// flags only after their command. A string flag's value may not start with
// "-" unless given as --flag=value; "--" ends flag parsing.
func (r *Root) Parse(args []string, lookupEnv func(string) (string, bool)) (*Context, error) {
	if err := r.validate(); err != nil {
		return nil, err
	}

	cmd := &r.Command
	scope := scopeOf(r.GlobalFlags(), r.Flags)
	given := map[string]string{}
	var positional []string
	selecting := true

	addPositional := func(tok string) error {
		if !selecting {
			positional = append(positional, tok)
			return nil
		}
		selecting = false
		sub := r.find(tok)
		if sub == nil {
			return usageErr(UnknownCommand, "unknown command %q", tok)
		}
		for _, f := range r.Flags {
			if _, ok := given[f.Name]; ok {
				return usageErr(UnknownFlag, "flag --%s is only valid without a command", f.Name)
			}
		}
		cmd = sub
		scope = scopeOf(r.GlobalFlags(), sub.Flags)
		return nil
	}

	for i := 0; i < len(args); i++ {
		tok := args[i]
		switch {
		case tok == "--":
			for _, rest := range args[i+1:] {
				if err := addPositional(rest); err != nil {
					return nil, err
				}
			}
			i = len(args)
		case isFlagToken(tok):
			dashes, short := "--", false
			if !strings.HasPrefix(tok, "--") {
				dashes, short = "-", true
			}
			name, val, hasVal := strings.Cut(strings.TrimPrefix(tok, dashes), "=")
			f, ok := lookup(scope, name, short)
			if !ok {
				return nil, usageErr(UnknownFlag, "unknown flag %s%s", dashes, name)
			}
			if f.Kind == KindBool {
				if !hasVal {
					val = "true"
				} else if _, err := strconv.ParseBool(val); err != nil {
					return nil, usageErr(InvalidValue, "invalid value %q for flag --%s", val, f.Name)
				}
			} else if !hasVal {
				if i+1 >= len(args) || isFlagToken(args[i+1]) || args[i+1] == "--" {
					return nil, usageErr(MissingValue, "flag --%s requires a value", f.Name)
				}
				i++
				val = args[i]
			}
			if f.Kind == KindBool {
				b, _ := strconv.ParseBool(val)
				val = strconv.FormatBool(b)
			}
			given[f.Name] = val
		default:
			if err := addPositional(tok); err != nil {
				return nil, err
			}
		}
	}

	ctx := &Context{
		Command:    cmd,
		values:     make(map[string]string, len(scope)),
		kinds:      make(map[string]Kind, len(scope)),
		positional: positional,
	}
	for _, f := range scope {
		ctx.values[f.Name] = resolve(f, given, lookupEnv)
		ctx.kinds[f.Name] = f.Kind
	}

	if ctx.values[helpFlag.Name] == "true" {
		return ctx, nil
	}
	required := 0
	for _, a := range cmd.Args {
		if a.Required {
			required++
		}
	}
	if len(positional) < required {
		return nil, usageErr(MissingArgs, "missing required argument <%s>", cmd.Args[len(positional)].Name)
	}
	if len(positional) > len(cmd.Args) {
		return nil, usageErr(TooManyArgs, "unexpected argument %q", positional[len(cmd.Args)])
	}
	return ctx, nil
}

func (r *Root) find(name string) *Command {
	for _, c := range r.Commands {
		if c.Name == name {
			return c
		}
	}
	if name == helpCommand.Name {
		return helpCommand
	}
	return nil
}

func scopeOf(globals, local []Flag) []Flag {
	return append(append([]Flag{}, globals...), local...)
}

func isFlagToken(tok string) bool { return len(tok) > 1 && tok[0] == '-' && tok != "--" }

func lookup(scope []Flag, name string, short bool) (Flag, bool) {
	if name == "" {
		return Flag{}, false
	}
	for _, f := range scope {
		if (short && f.Short == name) || (!short && f.Name == name) {
			return f, true
		}
	}
	return Flag{}, false
}

func resolve(f Flag, given map[string]string, lookupEnv func(string) (string, bool)) string {
	if v, ok := given[f.Name]; ok {
		return v
	}
	if f.Env != "" && lookupEnv != nil {
		if v, ok := lookupEnv(f.Env); ok && v != "" {
			if f.Kind == KindBool {
				return "true"
			}
			return v
		}
	}
	if f.Kind == KindBool {
		return "false"
	}
	return f.Default
}

// validate rejects definitions that would make parsing ambiguous.
func (r *Root) validate() error {
	for _, cmd := range append([]*Command{&r.Command, helpCommand}, r.Commands...) {
		if cmd.Name == helpCommand.Name && cmd != helpCommand {
			return fmt.Errorf("cli: command name %q is reserved for the built-in help command", cmd.Name)
		}
		local := cmd.Flags
		if cmd == &r.Command {
			local = r.Flags
		}
		names, shorts := map[string]bool{}, map[string]bool{}
		for _, f := range scopeOf(r.GlobalFlags(), local) {
			if f.Name == "" || names[f.Name] {
				return fmt.Errorf("cli: command %q: empty or duplicate flag %q", cmd.Name, f.Name)
			}
			names[f.Name] = true
			if len(f.Short) > 1 || shorts[f.Short] && f.Short != "" {
				return fmt.Errorf("cli: command %q: invalid or duplicate short flag %q", cmd.Name, f.Short)
			}
			shorts[f.Short] = true
			if f.Kind == KindBool && f.Default != "" {
				return fmt.Errorf("cli: flag --%s: bool flags cannot have a default", f.Name)
			}
		}
		optional := false
		for _, a := range cmd.Args {
			if optional && a.Required {
				return fmt.Errorf("cli: command %q: required arg <%s> follows an optional arg", cmd.Name, a.Name)
			}
			optional = optional || !a.Required
		}
	}
	return nil
}
