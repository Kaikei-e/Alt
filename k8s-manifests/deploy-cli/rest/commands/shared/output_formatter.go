// PHASE R3: Shared output formatting utilities
package shared

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v2"

	"deploy-cli/utils/colors"
)

// OutputFormatter provides shared output formatting functionality
type OutputFormatter struct {
	shared *CommandShared
}

// NewOutputFormatter creates a new output formatter
func NewOutputFormatter(shared *CommandShared) *OutputFormatter {
	return &OutputFormatter{
		shared: shared,
	}
}

// FormatResult formats a result according to the specified output format
func (o *OutputFormatter) FormatResult(data interface{}, format string) (string, error) {
	switch strings.ToLower(format) {
	case "json":
		return o.formatJSON(data)
	case "yaml":
		return o.formatYAML(data)
	case "text", "":
		return o.formatText(data)
	default:
		return "", fmt.Errorf("unsupported output format: %s", format)
	}
}

// formatJSON formats data as JSON
func (o *OutputFormatter) formatJSON(data interface{}) (string, error) {
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return string(bytes), nil
}

// formatYAML formats data as YAML
func (o *OutputFormatter) formatYAML(data interface{}) (string, error) {
	bytes, err := yaml.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("failed to marshal YAML: %w", err)
	}
	return string(bytes), nil
}

// formatText formats data as human-readable text
func (o *OutputFormatter) formatText(data interface{}) (string, error) {
	// For text format, we expect the caller to handle formatting
	// This is a fallback for simple string conversion
	return fmt.Sprintf("%+v", data), nil
}

// PrintOperationStart prints a standardized operation start message
func (o *OutputFormatter) PrintOperationStart(operation, environment string, options map[string]interface{}) {
	colors.PrintInfo(fmt.Sprintf("Starting %s for %s environment", operation, environment))
	
	if len(options) > 0 {
		o.printOptions(options)
	}
}

// PrintOperationResult prints a standardized operation result
func (o *OutputFormatter) PrintOperationResult(operation string, success bool, duration time.Duration, details map[string]interface{}) {
	status := "completed"
	if !success {
		status = "failed"
	}

	colors.PrintStep(fmt.Sprintf("%s %s in %s", 
		strings.Title(operation), status, duration.Truncate(time.Millisecond)))

	if success {
		colors.PrintSuccess(fmt.Sprintf("%s completed successfully", strings.Title(operation)))
	} else {
		colors.PrintError(fmt.Sprintf("%s failed", strings.Title(operation)))
	}

	if len(details) > 0 {
		o.printDetails(details)
	}
}

// PrintOperationError prints a standardized error message
func (o *OutputFormatter) PrintOperationError(operation string, err error) {
	colors.PrintError(fmt.Sprintf("%s failed: %v", strings.Title(operation), err))
}

// PrintWarning prints a warning message with consistent formatting
func (o *OutputFormatter) PrintWarning(message string, context map[string]interface{}) {
	colors.PrintWarning(message)
	if len(context) > 0 {
		o.printContext(context)
	}
}

// PrintInfo prints an info message with consistent formatting
func (o *OutputFormatter) PrintInfo(message string, context map[string]interface{}) {
	colors.PrintInfo(message)
	if len(context) > 0 {
		o.printContext(context)
	}
}

// PrintStep prints a step message with consistent formatting
func (o *OutputFormatter) PrintStep(message string) {
	colors.PrintStep(message)
}

// PrintSubInfo prints sub-information with consistent formatting
func (o *OutputFormatter) PrintSubInfo(message string) {
	colors.PrintSubInfo(message)
}

// PrintProgress prints progress information
func (o *OutputFormatter) PrintProgress(message string) {
	colors.PrintProgress(message)
}

// PrintSummary prints a formatted summary of operations
func (o *OutputFormatter) PrintSummary(title string, summary map[string]interface{}) {
	colors.PrintInfo(fmt.Sprintf("=== %s ===", title))
	
	for key, value := range summary {
		colors.PrintSubInfo(fmt.Sprintf("%s: %v", strings.Title(key), value))
	}
}

// PrintTable prints tabular data with headers
func (o *OutputFormatter) PrintTable(headers []string, rows [][]string) {
	if len(headers) == 0 || len(rows) == 0 {
		return
	}

	// Calculate column widths
	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = len(header)
	}

	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	// Print header
	headerRow := ""
	separatorRow := ""
	for i, header := range headers {
		headerRow += fmt.Sprintf("%-*s  ", widths[i], header)
		separatorRow += strings.Repeat("-", widths[i]) + "  "
	}
	
	colors.PrintInfo(strings.TrimRight(headerRow, " "))
	colors.PrintSubInfo(strings.TrimRight(separatorRow, " "))

	// Print rows
	for _, row := range rows {
		rowStr := ""
		for i, cell := range row {
			if i < len(widths) {
				rowStr += fmt.Sprintf("%-*s  ", widths[i], cell)
			}
		}
		colors.PrintSubInfo(strings.TrimRight(rowStr, " "))
	}
}

// PrintList prints a formatted list with bullets
func (o *OutputFormatter) PrintList(title string, items []string, bullet string) {
	if title != "" {
		colors.PrintInfo(title)
	}

	if bullet == "" {
		bullet = "•"
	}

	for _, item := range items {
		colors.PrintSubInfo(fmt.Sprintf("  %s %s", bullet, item))
	}
}

// printOptions prints operation options in a consistent format
func (o *OutputFormatter) printOptions(options map[string]interface{}) {
	for key, value := range options {
		colors.PrintSubInfo(fmt.Sprintf("%s: %v", strings.Title(key), value))
	}
}

// printDetails prints operation details in a consistent format
func (o *OutputFormatter) printDetails(details map[string]interface{}) {
	for key, value := range details {
		colors.PrintSubInfo(fmt.Sprintf("%s: %v", strings.Title(key), value))
	}
}

// printContext prints context information in a consistent format
func (o *OutputFormatter) printContext(context map[string]interface{}) {
	for key, value := range context {
		colors.PrintSubInfo(fmt.Sprintf("  %s: %v", key, value))
	}
}

// FormatDuration formats a duration in a human-readable way
func (o *OutputFormatter) FormatDuration(duration time.Duration) string {
	return duration.Truncate(time.Millisecond).String()
}

// FormatBytesSize formats byte size in human-readable format
func (o *OutputFormatter) FormatBytesSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// FormatPercentage formats a percentage with appropriate precision
func (o *OutputFormatter) FormatPercentage(value float64) string {
	return fmt.Sprintf("%.1f%%", value)
}