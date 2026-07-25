package doctor

import (
	"fmt"
	"strings"
)

// RenderText renders the report as a human-readable grouped summary:
// healthy stacks get one line, problem stacks get a detailed breakdown per
// finding including evidence and prescription. It is deliberately
// plain-text / color-free so it's trivially unit-testable; cmd/doctor.go is
// responsible for any terminal styling around it.
func (r *Report) RenderText() string {
	var b strings.Builder

	if !r.DockerReachable {
		b.WriteString("DOCKER DAEMON UNREACHABLE\n")
		b.WriteString(renderFindings(r.Preflight, "  "))
		return b.String()
	}

	fmt.Fprintf(&b, "altctl doctor -- %s\n", r.GeneratedAt.Format("2006-01-02 15:04:05 MST"))
	fmt.Fprintf(&b, "scope: %s\n", strings.Join(r.Scope, ", "))

	if len(r.Preflight) > 0 {
		b.WriteString("\nEnvironment:\n")
		b.WriteString(renderFindings(r.Preflight, "  "))
	}

	b.WriteString("\nStacks:\n")
	for _, sr := range r.Stacks {
		if sr.Healthy {
			optional := ""
			if sr.Optional {
				optional = " (optional)"
			}
			fmt.Fprintf(&b, "  [OK] %s%s -- %d service(s) healthy\n", sr.Name, optional, sr.ServiceCount)
			continue
		}
		fmt.Fprintf(&b, "  [PROBLEM] %s -- %d finding(s)\n", sr.Name, len(sr.Findings))
		b.WriteString(renderFindings(sr.Findings, "    "))
	}

	if !r.HasProblems() {
		b.WriteString("\nNo problems found.\n")
	}

	return b.String()
}

func renderFindings(findings []Finding, indent string) string {
	var b strings.Builder
	for _, f := range findings {
		fmt.Fprintf(&b, "%s- [%s] %s\n", indent, strings.ToUpper(string(f.Severity)), f.Message)
		if f.Detail != "" {
			fmt.Fprintf(&b, "%s    %s\n", indent, f.Detail)
		}
		for _, line := range f.Evidence {
			fmt.Fprintf(&b, "%s    | %s\n", indent, line)
		}
		for _, cmd := range f.Prescription {
			fmt.Fprintf(&b, "%s    -> %s\n", indent, cmd)
		}
	}
	return b.String()
}
