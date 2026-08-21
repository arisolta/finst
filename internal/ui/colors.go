package ui

import "os"

// ANSI Color codes
const (
	Reset       = "\033[0m"
	Bold        = "\033[1m"
	Dim         = "\033[2m"
	Italic      = "\033[3m"
	Underline   = "\033[4m"

	// Foreground Colors
	Black       = "\033[30m"
	Red         = "\033[31m"
	Green       = "\033[32m"
	Yellow      = "\033[33m"
	Blue        = "\033[34m"
	Magenta     = "\033[35m"
	Cyan        = "\033[36m"
	White       = "\033[37m"

	// High Intensity
	BrightBlack   = "\033[90m" // Gray / Slate
	BrightRed     = "\033[91m"
	BrightGreen   = "\033[92m"
	BrightYellow  = "\033[93m" // Amber
	BrightBlue    = "\033[94m"
	BrightMagenta = "\033[95m"
	BrightCyan    = "\033[96m"
	BrightWhite   = "\033[97m"

	// Bloomberg theme aliases
	ColorAmber   = "\033[38;5;214m"
	ColorSlate   = "\033[38;5;244m"
	ColorCyan    = "\033[38;5;44m"
	ColorHeader  = "\033[1;38;5;220m" // Bold Bright Amber
	ColorSection = "\033[1;38;5;75m"  // Bold Soft Blue/Cyan
	ColorBorder  = "\033[38;5;240m"  // Dark Gray
	ColorValue   = "\033[38;5;253m"  // White
	ColorGrowth  = "\033[38;5;78m"   // Light Green
	ColorNeg     = "\033[38;5;203m"  // Light Coral/Red
)

var colorsEnabled = true

func init() {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		colorsEnabled = false
	}
}

// SetColorsEnabled toggles ANSI color formatting.
func SetColorsEnabled(enabled bool) {
	colorsEnabled = enabled
}

func Colorize(color, text string) string {
	if !colorsEnabled {
		return text
	}
	return color + text + Reset
}
