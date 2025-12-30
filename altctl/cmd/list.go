package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alt-project/altctl/internal/output"
	"github.com/alt-project/altctl/internal/stack"
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List available stacks",
	Long: `List all available stacks with their services and dependencies.

Examples:
  altctl list                  # List all stacks
  altctl list --services       # Include service details
  altctl list --deps           # Show dependency graph
  altctl list --json           # Output as JSON`,
	RunE: runList,
}

func init() {
	rootCmd.AddCommand(listCmd)

	listCmd.Flags().Bool("services", false, "include service details")
	listCmd.Flags().Bool("deps", false, "show dependency graph")
	listCmd.Flags().Bool("json", false, "output as JSON")
}

func runList(cmd *cobra.Command, args []string) error {
	printer := output.NewPrinter(cfg.Output.Colors)
	registry := stack.NewRegistry()

	jsonOutput, _ := cmd.Flags().GetBool("json")
	showServices, _ := cmd.Flags().GetBool("services")
	showDeps, _ := cmd.Flags().GetBool("deps")

	if jsonOutput {
		return outputListJSON(registry)
	}

	if showDeps {
		return outputDependencyGraph(printer, registry)
	}

	return outputStackList(printer, registry, showServices)
}

func outputListJSON(registry *stack.Registry) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(registry.All())
}

func outputStackList(printer *output.Printer, registry *stack.Registry, showServices bool) error {
	printer.Header("Available Stacks")

	table := output.NewTable([]string{"STACK", "DESCRIPTION", "OPTIONAL", "REQUIRES"})

	for _, s := range registry.All() {
		optional := ""
		if s.Optional {
			optional = "yes"
		}

		requires := ""
		if len(s.DependsOn) > 0 {
			requires = strings.Join(s.DependsOn, ", ")
		}

		table.AddRow([]string{
			printer.Bold(s.Name),
			s.Description,
			optional,
			requires,
		})
	}
	table.Render()
	fmt.Println()

	if showServices {
		printer.Header("Stack Services")
		for _, s := range registry.All() {
			if len(s.Services) > 0 {
				printer.Info("%s:", printer.Bold(s.Name))
				for _, svc := range s.Services {
					printer.Print("    • %s", svc)
				}
				fmt.Println()
			}
		}
	}

	// Show default stacks
	printer.Info("Default stacks: %s", strings.Join(cfg.Defaults.Stacks, ", "))
	return nil
}

func outputDependencyGraph(printer *output.Printer, registry *stack.Registry) error {
	printer.Header("Dependency Graph")

	resolver := stack.NewDependencyResolver(registry)
	graph := resolver.GetDependencyGraph()

	// Print ASCII dependency graph
	fmt.Println()
	fmt.Println("                    base")
	fmt.Println("                      │")
	fmt.Println("       ┌──────────────┴──────────────┐")
	fmt.Println("       │                             │")
	fmt.Println("      db                           auth")
	fmt.Println("       │                             │")
	fmt.Println("       └──────────────┬──────────────┘")
	fmt.Println("                      │")
	fmt.Println("                    core")
	fmt.Println("                      │")
	fmt.Println("   ┌──────┬───────────┼───────────┬──────┐")
	fmt.Println("   │      │           │           │      │")
	fmt.Println("  ai   workers     recap        rag  logging")
	fmt.Println()

	// Detailed dependencies
	printer.Header("Detailed Dependencies")
	for name, deps := range graph {
		if len(deps) == 0 {
			printer.Print("%s: %s", printer.Bold(name), printer.Dim("(no dependencies)"))
		} else {
			printer.Print("%s: %s", printer.Bold(name), strings.Join(deps, " → "))
		}
	}

	return nil
}
