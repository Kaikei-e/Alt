package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alt-project/adrdag/internal/adr"
	"github.com/alt-project/adrdag/internal/graph"
)

type finding struct {
	Severity string `json:"severity"`
	Rule     string `json:"rule"`
	ID       string `json:"id"`
	Detail   string `json:"detail"`
}

type checkReport struct {
	ADRCount  int       `json:"adr_count"`
	EdgeCount int       `json:"edge_count"`
	OK        bool      `json:"ok"`
	Findings  []finding `json:"findings"`
}

// runCheck validates the DAG in adr_graph.py's cmd_check order:
// dangling refs, cycle, empty stubs, status drift (all ERROR), then
// orphan-superseded WARNs. Text output is line-for-line identical.
func runCheck(adrs map[string]adr.ADR) checkReport {
	g := adr.SupersedesGraph(adrs)
	reverse := graph.BuildReverse(g)
	report := checkReport{ADRCount: len(adrs), Findings: []finding{}}
	for _, targets := range g {
		report.EdgeCount += len(targets)
	}

	for _, newID := range adr.SortedIDs(adrs) {
		for _, oldID := range g[newID] {
			if _, known := adrs[oldID]; !known {
				report.Findings = append(report.Findings, finding{
					Severity: "error", Rule: "dangling_ref", ID: newID,
					Detail: fmt.Sprintf("ERROR: %s supersedes unknown ADR %s", newID, oldID),
				})
			}
		}
	}

	if cycle := graph.FindCycle(g); cycle != nil {
		report.Findings = append(report.Findings, finding{
			Severity: "error", Rule: "cycle", ID: cycle[0],
			Detail: "ERROR: cycle detected in supersedes graph: " + strings.Join(cycle, " --> "),
		})
	}

	for _, id := range adr.SortedIDs(adrs) {
		if adrs[id].EmptySupersedesStub {
			report.Findings = append(report.Findings, finding{
				Severity: "error", Rule: "empty_stub", ID: id,
				Detail: fmt.Sprintf("ERROR: %s has empty supersedes stub (omit the key or use a real id list)", id),
			})
		}
	}

	for _, id := range adr.SortedIDs(adrs) {
		if adrs[id].Status == "accepted" && len(reverse[id]) > 0 {
			report.Findings = append(report.Findings, finding{
				Severity: "error", Rule: "status_drift", ID: id,
				Detail: fmt.Sprintf("ERROR: %s status=accepted but superseded by %s (set status: superseded)",
					id, strings.Join(reverse[id], ", ")),
			})
		}
	}

	for _, id := range adr.SortedIDs(adrs) {
		if adrs[id].Status == "superseded" && len(reverse[id]) == 0 {
			report.Findings = append(report.Findings, finding{
				Severity: "warning", Rule: "orphan_superseded", ID: id,
				Detail: fmt.Sprintf("WARN: %s status=superseded with no inbound supersedes (withdrawn/deprecated? do not invent an edge)", id),
			})
		}
	}

	report.OK = true
	for _, f := range report.Findings {
		if f.Severity == "error" {
			report.OK = false
		}
	}
	return report
}

func newCheckCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Validate the supersedes DAG (cycles, dangling refs, empty stubs, status drift)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("format")
			if err := validFormat(format, "text", "json"); err != nil {
				return err
			}
			adrs, err := loadCorpus(cmd)
			if err != nil {
				return err
			}
			report := runCheck(adrs)

			if format == "json" {
				out, err := json.MarshalIndent(report, "", "  ")
				if err != nil {
					return ioErr(err)
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(out))
			} else {
				for _, f := range report.Findings {
					fmt.Fprintln(cmd.ErrOrStderr(), f.Detail)
				}
				if report.OK {
					fmt.Fprintf(cmd.OutOrStdout(), "OK: %d ADRs, %d supersedes edges, no cycles, status aligned\n",
						report.ADRCount, report.EdgeCount)
				}
			}
			if !report.OK {
				return &cliError{code: exitFailure}
			}
			return nil
		},
	}
	cmd.Flags().String("format", "text", "output format: text|json")
	return cmd
}
