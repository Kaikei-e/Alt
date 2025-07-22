// PHASE R3: Maintenance output formatting with focused responsibility
package maintenance

import (
	"fmt"
	"time"

	"deploy-cli/domain"
	"deploy-cli/rest/commands/shared"
	"deploy-cli/utils/colors"
)

// MaintenanceOutput provides maintenance-specific output formatting
type MaintenanceOutput struct {
	shared *shared.CommandShared
}

// NewMaintenanceOutput creates a new maintenance output formatter
func NewMaintenanceOutput(shared *shared.CommandShared) *MaintenanceOutput {
	return &MaintenanceOutput{
		shared: shared,
	}
}

// Cleanup output methods

// PrintCleanupStart prints cleanup operation start message
func (m *MaintenanceOutput) PrintCleanupStart(env domain.Environment, options *CleanupOptions) {
	colors.PrintInfo(fmt.Sprintf("Starting cleanup operations for %s environment", env))
	
	if options.DryRun {
		colors.PrintWarning("DRY RUN MODE - No actual changes will be made")
	}
	
	resourceTypes := options.GetSelectedResourceTypes()
	if len(resourceTypes) > 0 {
		colors.PrintSubInfo(fmt.Sprintf("Resource types: %v", resourceTypes))
	}
	
	if options.Force {
		colors.PrintWarning("FORCE MODE - Skipping confirmation prompts")
	}
}

// PrintCleanupResults prints cleanup operation results
func (m *MaintenanceOutput) PrintCleanupResults(result *CleanupResult) {
	colors.PrintStep(fmt.Sprintf("Cleanup completed in %s", result.Duration.Truncate(time.Millisecond)))
	
	if result.Success {
		colors.PrintSuccess("All cleanup operations completed successfully")
	} else {
		colors.PrintError("Some cleanup operations failed")
	}
	
	for _, operation := range result.Operations {
		m.printCleanupOperation(operation)
	}
	
	colors.PrintSubInfo(fmt.Sprintf("Environment: %s | Operations: %d | Duration: %s",
		result.Environment, len(result.Operations), result.Duration.Truncate(time.Millisecond)))
}

// PrintCleanupError prints cleanup error message
func (m *MaintenanceOutput) PrintCleanupError(err error) {
	colors.PrintError(fmt.Sprintf("Cleanup failed: %v", err))
}

// printCleanupOperation prints individual cleanup operation result
func (m *MaintenanceOutput) printCleanupOperation(operation CleanupOperation) {
	status := "✓"
	if !operation.Success {
		status = "✗"
	}
	
	duration := operation.Duration.Truncate(time.Millisecond)
	colors.PrintSubInfo(fmt.Sprintf("  %s %s: %s (took %s)", status, operation.Type, operation.Message, duration))
}

// Troubleshoot output methods

// PrintTroubleshootStart prints troubleshoot operation start message
func (m *MaintenanceOutput) PrintTroubleshootStart(env domain.Environment, options *TroubleshootOptions) {
	colors.PrintInfo(fmt.Sprintf("Starting troubleshooting for %s environment", env))
	
	if options.Interactive {
		colors.PrintSubInfo("Interactive mode enabled - will prompt for confirmations")
	}
	
	if options.AutoFix {
		colors.PrintWarning("Auto-fix mode enabled - will automatically apply safe fixes")
	}
	
	if len(options.Categories) > 0 {
		colors.PrintSubInfo(fmt.Sprintf("Focus areas: %v", options.Categories))
	}
}

// PrintTroubleshootResults prints troubleshoot operation results
func (m *MaintenanceOutput) PrintTroubleshootResults(result *TroubleshootResult) {
	colors.PrintStep(fmt.Sprintf("Troubleshooting completed in %s", result.Duration.Truncate(time.Millisecond)))
	
	if len(result.Issues) == 0 {
		colors.PrintSuccess("No issues detected - system appears healthy")
	} else {
		colors.PrintWarning(fmt.Sprintf("Detected %d issues", len(result.Issues)))
		
		for _, issue := range result.Issues {
			m.printTroubleshootIssue(issue)
		}
	}
	
	if len(result.Fixes) > 0 {
		colors.PrintInfo(fmt.Sprintf("Applied %d fixes", len(result.Fixes)))
		
		for _, fix := range result.Fixes {
			m.printTroubleshootFix(fix)
		}
	}
}

// PrintTroubleshootError prints troubleshoot error message
func (m *MaintenanceOutput) PrintTroubleshootError(err error) {
	colors.PrintError(fmt.Sprintf("Troubleshooting failed: %v", err))
}

// printTroubleshootIssue prints individual troubleshoot issue
func (m *MaintenanceOutput) printTroubleshootIssue(issue TroubleshootIssue) {
	colors.PrintSubInfo(fmt.Sprintf("  Issue: %s [%s] - %s", issue.Title, issue.Severity, issue.Description))
}

// printTroubleshootFix prints individual troubleshoot fix
func (m *MaintenanceOutput) printTroubleshootFix(fix TroubleshootFix) {
	status := "✓"
	if !fix.Success {
		status = "✗"
	}
	
	colors.PrintSubInfo(fmt.Sprintf("  %s Fix: %s - %s", status, fix.Action, fix.Description))
}

// Emergency output methods

// PrintEmergencyStart prints emergency operation start message
func (m *MaintenanceOutput) PrintEmergencyStart(env domain.Environment, options *EmergencyOptions) {
	colors.PrintError(fmt.Sprintf("EMERGENCY OPERATION: %s for %s environment", options.Operation, env))
	colors.PrintWarning("This is a destructive operation - proceed with caution")
	
	if options.SafeMode {
		colors.PrintInfo("Safe mode enabled - reduced blast radius")
	}
	
	if options.Component != "" {
		colors.PrintSubInfo(fmt.Sprintf("Target component: %s", options.Component))
	}
}

// PrintEmergencyResults prints emergency operation results
func (m *MaintenanceOutput) PrintEmergencyResults(result *EmergencyResult) {
	colors.PrintStep(fmt.Sprintf("Emergency operation '%s' completed in %s", result.Operation, result.Duration.Truncate(time.Millisecond)))
	
	if result.Success {
		colors.PrintSuccess("Emergency operation completed successfully")
	} else {
		colors.PrintError("Emergency operation failed or completed with errors")
	}
	
	for _, operation := range result.Operations {
		m.printEmergencyOperation(operation)
	}
}

// PrintEmergencyError prints emergency error message
func (m *MaintenanceOutput) PrintEmergencyError(err error) {
	colors.PrintError(fmt.Sprintf("Emergency operation failed: %v", err))
}

// printEmergencyOperation prints individual emergency operation result
func (m *MaintenanceOutput) printEmergencyOperation(operation EmergencyOperation) {
	status := "✓"
	if !operation.Success {
		status = "✗"
	}
	
	duration := operation.Duration.Truncate(time.Millisecond)
	message := operation.Step
	if operation.Error != "" {
		message = fmt.Sprintf("%s (error: %s)", operation.Step, operation.Error)
	}
	
	colors.PrintSubInfo(fmt.Sprintf("  %s %s (took %s)", status, message, duration))
}

// Diagnose output methods

// PrintDiagnoseStart prints diagnose operation start message
func (m *MaintenanceOutput) PrintDiagnoseStart(env domain.Environment, options *DiagnoseOptions) {
	colors.PrintInfo(fmt.Sprintf("Starting %s diagnostics for %s environment", options.Level, env))
	
	if len(options.Areas) > 0 {
		colors.PrintSubInfo(fmt.Sprintf("Diagnostic areas: %v", options.Areas))
	}
	
	if options.AutoFix {
		colors.PrintWarning("Auto-fix enabled - will apply recommended fixes")
	}
	
	if options.OutputFile != "" {
		colors.PrintSubInfo(fmt.Sprintf("Results will be exported to: %s (%s format)", options.OutputFile, options.Format))
	}
}

// PrintDiagnoseResults prints diagnose operation results
func (m *MaintenanceOutput) PrintDiagnoseResults(result *DiagnoseResult) {
	colors.PrintStep(fmt.Sprintf("%s diagnostics completed in %s", result.Level, result.Duration.Truncate(time.Millisecond)))
	
	// Print check summary
	successCount := 0
	for _, check := range result.Checks {
		if check.Status == "pass" {
			successCount++
		}
	}
	
	colors.PrintInfo(fmt.Sprintf("Checks: %d passed, %d failed", successCount, len(result.Checks)-successCount))
	
	if len(result.Issues) > 0 {
		colors.PrintWarning(fmt.Sprintf("Issues detected: %d", len(result.Issues)))
		for _, issue := range result.Issues {
			m.printDiagnosticIssue(issue)
		}
	} else {
		colors.PrintSuccess("No issues detected - system appears healthy")
	}
	
	if len(result.Fixes) > 0 {
		colors.PrintInfo(fmt.Sprintf("Applied fixes: %d", len(result.Fixes)))
		for _, fix := range result.Fixes {
			m.printDiagnosticFix(fix)
		}
	}
}

// PrintDiagnoseError prints diagnose error message
func (m *MaintenanceOutput) PrintDiagnoseError(err error) {
	colors.PrintError(fmt.Sprintf("Diagnostics failed: %v", err))
}

// printDiagnosticIssue prints individual diagnostic issue
func (m *MaintenanceOutput) printDiagnosticIssue(issue DiagnosticIssue) {
	colors.PrintSubInfo(fmt.Sprintf("  [%s] %s: %s", issue.Severity, issue.Title, issue.Description))
	if issue.Recommendation != "" {
		colors.PrintSubInfo(fmt.Sprintf("    → %s", issue.Recommendation))
	}
}

// printDiagnosticFix prints individual diagnostic fix
func (m *MaintenanceOutput) printDiagnosticFix(fix DiagnosticFix) {
	status := "✓"
	if !fix.Success {
		status = "✗"
	}
	
	colors.PrintSubInfo(fmt.Sprintf("  %s %s: %s", status, fix.Action, fix.Description))
}

// Repair output methods

// PrintRepairStart prints repair operation start message
func (m *MaintenanceOutput) PrintRepairStart(env domain.Environment, options *RepairOptions) {
	colors.PrintInfo(fmt.Sprintf("Starting automated repairs for %s environment", env))
	
	if len(options.Types) > 0 {
		colors.PrintSubInfo(fmt.Sprintf("Repair types: %v", options.Types))
	}
	
	if options.Interactive {
		colors.PrintSubInfo("Interactive mode enabled - will prompt for confirmations")
	}
	
	if options.Aggressive {
		colors.PrintWarning("Aggressive mode enabled - will attempt riskier repairs")
	}
	
	if options.DryRun {
		colors.PrintWarning("DRY RUN MODE - No actual repairs will be made")
	}
}

// PrintRepairResults prints repair operation results
func (m *MaintenanceOutput) PrintRepairResults(result *RepairResult) {
	colors.PrintStep(fmt.Sprintf("Repair operations completed in %s", result.Duration.Truncate(time.Millisecond)))
	
	if result.Success {
		colors.PrintSuccess("All repair operations completed successfully")
	} else {
		colors.PrintError("Some repair operations failed")
	}
	
	for _, repair := range result.Repairs {
		m.printRepairOperation(repair)
	}
	
	// Print summary
	totalFound := 0
	totalRepaired := 0
	for _, repair := range result.Repairs {
		totalFound += repair.ItemsFound
		totalRepaired += repair.ItemsRepaired
	}
	
	colors.PrintSubInfo(fmt.Sprintf("Summary: %d items found, %d repaired, %d operations", 
		totalFound, totalRepaired, len(result.Repairs)))
}

// PrintRepairError prints repair error message
func (m *MaintenanceOutput) PrintRepairError(err error) {
	colors.PrintError(fmt.Sprintf("Repair operations failed: %v", err))
}

// printRepairOperation prints individual repair operation result
func (m *MaintenanceOutput) printRepairOperation(repair RepairOperation) {
	status := "✓"
	if !repair.Success {
		status = "✗"
	}
	
	duration := repair.Duration.Truncate(time.Millisecond)
	colors.PrintSubInfo(fmt.Sprintf("  %s %s: %s (took %s)", status, repair.Type, repair.Message, duration))
}