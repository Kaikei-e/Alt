package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
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
	printer := newPrinter()
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
	printer.PrintHints("list")
	return nil
}

func outputDependencyGraph(printer *output.Printer, registry *stack.Registry) error {
	printer.Header("Dependency Graph")

	resolver := stack.NewDependencyResolver(registry)
	graph := resolver.GetDependencyGraph()

	// Build children map (reverse of DependsOn)
	children := make(map[string][]string)
	for _, s := range registry.All() {
		for _, dep := range s.DependsOn {
			children[dep] = append(children[dep], s.Name)
		}
	}
	// Sort children for consistent output
	for k := range children {
		sort.Strings(children[k])
	}

	// Find roots (stacks with no dependencies)
	roots := findRoots(graph)

	// Print tree from each root, tracking visited nodes to avoid duplication
	fmt.Println()
	visited := make(map[string]bool)
	for i, root := range roots {
		isLast := i == len(roots)-1
		printTree(printer, root, children, visited, "", isLast)
	}
	fmt.Println()

	// Detailed dependencies
	printer.Header("Detailed Dependencies")
	for _, s := range registry.All() {
		deps := graph[s.Name]
		if len(deps) == 0 {
			printer.Print("%s: %s", printer.Bold(s.Name), printer.Dim("(no dependencies)"))
		} else {
			printer.Print("%s: %s", printer.Bold(s.Name), strings.Join(deps, " → "))
		}
	}

	return nil
}

// findRoots returns stacks that have no dependencies (tree roots).
func findRoots(graph map[string][]string) []string {
	var roots []string
	for name, deps := range graph {
		if len(deps) == 0 {
			roots = append(roots, name)
		}
	}
	sort.Strings(roots)
	return roots
}

// printTree recursively prints a tree representation of the dependency graph.
// Tracks visited nodes to avoid duplicating subtrees.
func printTree(printer *output.Printer, name string, children map[string][]string, visited map[string]bool, prefix string, isLast bool) {
	isRoot := prefix == "" && !isLast && !visited[name+"_printed"]

	if isRoot {
		printer.Print("%s", printer.Bold(name))
	} else if prefix == "" {
		// Non-first root
		printer.Print("%s", printer.Bold(name))
	} else {
		connector := "├── "
		if isLast {
			connector = "└── "
		}
		printer.Print("%s%s%s", prefix, connector, printer.Bold(name))
	}

	// Skip children if already visited (prevents duplication)
	if visited[name] {
		return
	}
	visited[name] = true

	var childPrefix string
	if prefix == "" {
		// Children of root get standard indent
		childPrefix = "  "
	} else if isLast {
		childPrefix = prefix + "    "
	} else {
		childPrefix = prefix + "│   "
	}

	kids := children[name]
	for i, child := range kids {
		printTree(printer, child, children, visited, childPrefix, i == len(kids)-1)
	}
}
