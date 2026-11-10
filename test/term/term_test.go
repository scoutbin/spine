package term_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/scoutbin/spine/term"
)

func TestDecide(t *testing.T) {
	testcases := []struct {
		name string
		in   term.Inputs
		want term.Mode
	}{
		{"both tty", term.Inputs{StdoutTTY: true, StderrTTY: true}, term.Mode{Progress: true, Color: true}},
		{"stdout piped", term.Inputs{StderrTTY: true}, term.Mode{}},
		{"stderr piped", term.Inputs{StdoutTTY: true}, term.Mode{}},
		{"both piped", term.Inputs{}, term.Mode{}},
		{"json on tty", term.Inputs{StdoutTTY: true, StderrTTY: true, JSON: true}, term.Mode{}},
		{"no-color flag keeps progress", term.Inputs{StdoutTTY: true, StderrTTY: true, NoColor: true}, term.Mode{Progress: true}},
		{"NO_COLOR keeps progress", term.Inputs{StdoutTTY: true, StderrTTY: true, NoColorEnv: "1"}, term.Mode{Progress: true}},
		{"empty NO_COLOR ignored", term.Inputs{StdoutTTY: true, StderrTTY: true, NoColorEnv: ""}, term.Mode{Progress: true, Color: true}},
		{"json beats no-color", term.Inputs{StdoutTTY: true, StderrTTY: true, JSON: true, NoColor: true}, term.Mode{}},
		{"no tty no color", term.Inputs{NoColor: true}, term.Mode{}},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			if got := term.Decide(tc.in); got != tc.want {
				t.Fatalf("Decide(%+v) = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
}

func TestRenderBar(t *testing.T) {
	testcases := []struct {
		name        string
		done, total int64
		want        string
	}{
		{"zero", 0, 5_200_000, "⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀ 0%  0.0/5.2 MB"},
		{"partial", 3_100_000, 5_200_000, "⣿⣿⣿⣿⣿⣀⣀⣀⣀⣀ 59%  3.1/5.2 MB"},
		{"sixty percent", 3_120_000, 5_200_000, "⣿⣿⣿⣿⣿⣿⣀⣀⣀⣀ 60%  3.1/5.2 MB"},
		{"complete", 5_200_000, 5_200_000, "⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿ 100%  5.2/5.2 MB"},
		{"overshoot clamps", 9_000_000, 5_200_000, "⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿ 100%  5.2/5.2 MB"},
		{"negative clamps", -1, 5_200_000, "⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀ 0%  0.0/5.2 MB"},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			if got := term.RenderBar(tc.done, tc.total); got != tc.want {
				t.Fatalf("RenderBar(%d, %d) = %q, want %q", tc.done, tc.total, got, tc.want)
			}
		})
	}
}

func TestFormatMB(t *testing.T) {
	testcases := []struct {
		bytes int64
		want  string
	}{
		{0, "0.0"},
		{49_999, "0.0"},
		{50_000, "0.1"},
		{1_000_000, "1.0"},
		{3_100_000, "3.1"},
		{123_456_789, "123.5"},
	}
	for _, tc := range testcases {
		if got := term.FormatMB(tc.bytes); got != tc.want {
			t.Errorf("FormatMB(%d) = %q, want %q", tc.bytes, got, tc.want)
		}
	}
}

func TestSpinnerFrame(t *testing.T) {
	testcases := []struct {
		i    int
		want string
	}{
		{0, "⠋"},
		{1, "⠙"},
		{9, "⠏"},
		{10, "⠋"},
		{23, "⠸"},
	}
	for _, tc := range testcases {
		if got := term.SpinnerFrame(tc.i); got != tc.want {
			t.Errorf("SpinnerFrame(%d) = %q, want %q", tc.i, got, tc.want)
		}
	}
}

func TestReporterOutput(t *testing.T) {
	const esc = "\x1b["
	testcases := []struct {
		name       string
		mode       term.Mode
		run        func(term.Reporter)
		wantOutput []string
		wantColor  bool
		wantEmpty  bool
	}{
		{
			name:       "spinner success, color",
			mode:       term.Mode{Progress: true, Color: true},
			run:        func(r term.Reporter) { r.Spin("Checking").Done("Up to date") },
			wantOutput: []string{"⠋", "Checking", "✓", " Up to date\n"},
			wantColor:  true,
		},
		{
			name:       "spinner failure, no color keeps glyphs",
			mode:       term.Mode{Progress: true},
			run:        func(r term.Reporter) { r.Spin("Checking").Fail("Lookup failed") },
			wantOutput: []string{"⠋ Checking", "✗ Lookup failed\n"},
		},
		{
			name: "bar with known total",
			mode: term.Mode{Progress: true},
			run: func(r term.Reporter) {
				b := r.Bar("Downloading", 5_200_000)
				b.Update(3_120_000)
				b.Done("Downloaded")
			},
			wantOutput: []string{"⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀ 0%  0.0/5.2 MB", "✓ Downloaded\n"},
		},
		{
			name:       "unknown total falls back to spinner",
			mode:       term.Mode{Progress: true},
			run:        func(r term.Reporter) { r.Bar("Downloading", 0).Done("Downloaded") },
			wantOutput: []string{"⠋ Downloading", "✓ Downloaded\n"},
		},
		{
			name:       "result replaces the active line",
			mode:       term.Mode{Progress: true},
			run:        func(r term.Reporter) { r.Spin("Verifying").Done("Checksum verified") },
			wantOutput: []string{"\r" + esc + "2K✓ Checksum verified\n"},
		},
		{
			name:      "progress off prints nothing",
			mode:      term.Mode{},
			run:       func(r term.Reporter) { r.Spin("x").Done("y"); r.Bar("x", 10).Fail("z") },
			wantEmpty: true,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			tc.run(term.NewReporter(&buf, tc.mode))
			out := buf.String()
			if tc.wantEmpty {
				if out != "" {
					t.Fatalf("expected no output, got %q", out)
				}
				return
			}
			for _, want := range tc.wantOutput {
				if !strings.Contains(out, want) {
					t.Errorf("output %q missing %q", out, want)
				}
			}
			hasColor := strings.Contains(strings.ReplaceAll(out, esc+"2K", ""), esc)
			if hasColor != tc.wantColor {
				t.Errorf("color present = %v, want %v (output %q)", hasColor, tc.wantColor, out)
			}
			if n := strings.Count(out, "\n"); n != 1 {
				t.Errorf("expected exactly one result line, got %d newlines in %q", n, out)
			}
		})
	}
}

func TestTaskFinishesOnce(t *testing.T) {
	var buf bytes.Buffer
	task := term.NewReporter(&buf, term.Mode{Progress: true}).Spin("x")
	task.Done("first")
	task.Fail("second")
	if out := buf.String(); strings.Contains(out, "second") {
		t.Fatalf("second result printed: %q", out)
	}
}
