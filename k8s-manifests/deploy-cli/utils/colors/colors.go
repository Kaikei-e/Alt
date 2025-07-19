package colors

import (
	"github.com/fatih/color"
)

var (
	// Red color for errors
	Red = color.New(color.FgRed).SprintFunc()

	// Green color for success
	Green = color.New(color.FgGreen).SprintFunc()

	// Yellow color for warnings
	Yellow = color.New(color.FgYellow).SprintFunc()

	// Blue color for info
	Blue = color.New(color.FgBlue).SprintFunc()

	// Cyan color for secondary info
	Cyan = color.New(color.FgCyan).SprintFunc()

	// Bold colors
	RedBold    = color.New(color.FgRed, color.Bold).SprintFunc()
	GreenBold  = color.New(color.FgGreen, color.Bold).SprintFunc()
	YellowBold = color.New(color.FgYellow, color.Bold).SprintFunc()
	BlueBold   = color.New(color.FgBlue, color.Bold).SprintFunc()
	CyanBold   = color.New(color.FgCyan, color.Bold).SprintFunc()
)

// PrintSuccess prints a success message
func PrintSuccess(msg string) {
	color.Green("✓ %s", msg)
}

// PrintError prints an error message
func PrintError(msg string) {
	color.Red("✗ %s", msg)
}

// PrintWarning prints a warning message
func PrintWarning(msg string) {
	color.Yellow("⚠ %s", msg)
}

// PrintInfo prints an info message
func PrintInfo(msg string) {
	color.Blue("▶ %s", msg)
}

// PrintSubInfo prints a sub-info message
func PrintSubInfo(msg string) {
	color.Cyan("  ↪ %s", msg)
}

// PrintStep prints a step message
func PrintStep(msg string) {
	color.Cyan("▶ %s", msg)
}

// PrintProgress prints a progress message
func PrintProgress(msg string) {
	color.Blue("  ↻ %s", msg)
}

// DisableColor disables colored output
func DisableColor() {
	color.NoColor = true
}
