package output

import (
	"fmt"
	"strings"
)

// CommandHints maps command names to related commands users might want to run next
var CommandHints = map[string][]string{
	"up":              {"status", "logs <service>"},
	"down":            {"status"},
	"build":           {"up"},
	"status":          {"logs <service>", "up", "down"},
	"list":            {"up", "config"},
	"config":          {"list", "up"},
	"logs":            {"status"},
	"restart":         {"status", "logs <service>"},
	"deploy":          {"status", "logs <service>", "down"},
	"migrate backup":  {"migrate verify", "migrate list", "migrate status"},
	"migrate restore": {"migrate verify", "status"},
	"migrate status":  {"migrate backup", "migrate list"},
}

// PrintHints prints "See also" hints for a command. No-op in quiet mode or if command has no hints.
func (p *Printer) PrintHints(command string) {
	if p.quiet {
		return
	}
	hints, ok := CommandHints[command]
	if !ok || len(hints) == 0 {
		return
	}

	cmds := make([]string, len(hints))
	for i, h := range hints {
		cmds[i] = "altctl " + h
	}
	fmt.Fprintf(p.out, "\nSee also: %s\n", strings.Join(cmds, ", "))
}
