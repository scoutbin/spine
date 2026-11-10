package term

import (
	"fmt"
	"strings"
)

const (
	barCells   = 10
	barFilled  = "⣿"
	barEmpty   = "⣀"
	bytesPerMB = 1_000_000
)

var spinnerFrames = [...]string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// SpinnerFrame returns the frame for tick i, wrapping around.
func SpinnerFrame(i int) string {
	return spinnerFrames[i%len(spinnerFrames)]
}

// FormatMB formats a byte count as megabytes (1 MB = 1,000,000 bytes) with one decimal.
func FormatMB(bytes int64) string {
	return fmt.Sprintf("%.1f", float64(bytes)/bytesPerMB)
}

// RenderBar renders the progress bar line, e.g. "⣿⣿⣿⣿⣿⣿⣀⣀⣀⣀ 60%  3.1/5.2 MB".
// done is clamped to [0, total]; total must be positive.
func RenderBar(done, total int64) string {
	done = max(0, min(done, total))
	percent := int(done * 100 / total)
	filled := percent * barCells / 100
	bar := strings.Repeat(barFilled, filled) + strings.Repeat(barEmpty, barCells-filled)
	return fmt.Sprintf("%s %d%%  %s/%s MB", bar, percent, FormatMB(done), FormatMB(total))
}
