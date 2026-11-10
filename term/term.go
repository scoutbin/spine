// Package term decides whether progress output and color are shown and renders
// the spinner, progress bar and result lines. Everything is written to stderr.
package term

import (
	"os"
)

// Inputs are the facts the progress and color decision is based on.
type Inputs struct {
	StdoutTTY  bool
	StderrTTY  bool
	JSON       bool
	NoColor    bool   // the --no-color flag
	NoColorEnv string // value of NO_COLOR; non-empty disables color (no-color.org)
}

// Mode is the outcome of Decide.
type Mode struct {
	Progress bool
	Color    bool
}

// Decide turns progress on only when stdout and stderr are both terminals and
// --json is not set. Color needs progress and no opt-out.
func Decide(in Inputs) Mode {
	progress := in.StdoutTTY && in.StderrTTY && !in.JSON
	color := progress && !in.NoColor && in.NoColorEnv == ""
	return Mode{Progress: progress, Color: color}
}

// Detect inspects the real streams and the environment. When both are
// terminals but ANSI cannot be enabled on stderr (older Windows consoles),
// progress and color are turned off.
func Detect(stdout, stderr *os.File, jsonOutput, noColor bool) Mode {
	in := Inputs{
		StdoutTTY:  IsTerminal(stdout),
		StderrTTY:  IsTerminal(stderr),
		JSON:       jsonOutput,
		NoColor:    noColor,
		NoColorEnv: os.Getenv("NO_COLOR"),
	}
	if in.StdoutTTY && in.StderrTTY && !enableVT(stderr) {
		return Mode{}
	}
	return Decide(in)
}

// IsTerminal reports whether f is a character device.
func IsTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
