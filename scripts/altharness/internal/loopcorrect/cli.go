package loopcorrect

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// Run executes the `alt-harness loop-correct` CLI. args excludes argv[0].
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if ctx == nil {
		writeln(stderr, "error: nil context")
		return exitUsage
	}
	if len(args) == 0 {
		writeln(stderr, "error: subcommand is required (choose from scan, validate, record, scaffold-corpus)")
		writeUsage(stderr)
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		writeUsage(stdout)
		return exitOK
	case "scan":
		return cmdScan(ctx, args[1:], stdout, stderr)
	case "validate":
		return cmdValidate(ctx, args[1:], stdout, stderr)
	case "record":
		return cmdRecord(ctx, args[1:], stdout, stderr)
	case "scaffold-corpus":
		return cmdScaffoldCorpus(ctx, args[1:], stdout, stderr)
	default:
		writef(stderr, "error: unknown subcommand %q (choose from scan, validate, record, scaffold-corpus)\n", args[0])
		return exitUsage
	}
}

func writeUsage(w io.Writer) {
	writeStr(w, `LOOP-07 /correct cadence engine for Alt.

usage:
  alt-harness loop-correct <subcommand> [flags]

subcommands:
  scan            [--decisions-file PATH | --runs-dir DIR] [--branch NAME] [--threshold 2] [--repo-root DIR] [--format text|json]
  validate        <disposition.json> [--repo-root DIR] [--format text|json]
  record          <disposition.json> [--branch NAME] [--decisions-file PATH] [--runs-dir DIR] [--repo-root DIR] [--format text|json]
  scaffold-corpus --class-id <id> --title <title> --language <lang> --category <cat> [--write] [--repo-root DIR] [--format text|json]
`)
}

type parsedFlags struct {
	values      map[string]string
	bools       map[string]bool
	positionals []string
	help        bool
}

func parseFlags(args []string, valueFlags, boolFlags map[string]bool) (parsedFlags, error) {
	out := parsedFlags{
		values: map[string]string{},
		bools:  map[string]bool{},
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "-h" || a == "--help" {
			out.help = true
			return out, nil
		}
		if a == "--" {
			out.positionals = append(out.positionals, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			out.positionals = append(out.positionals, a)
			continue
		}
		name := a
		val := ""
		hasEq := false
		if eq := strings.IndexByte(a, '='); eq >= 0 {
			name = a[:eq]
			val = a[eq+1:]
			hasEq = true
		}
		if boolFlags[name] {
			if hasEq {
				return out, fmt.Errorf("argument %s does not accept a value", name)
			}
			out.bools[name] = true
			continue
		}
		if valueFlags[name] {
			if !hasEq {
				if i+1 >= len(args) {
					return out, fmt.Errorf("argument %s: expected one argument", name)
				}
				i++
				val = args[i]
			}
			out.values[name] = val
			continue
		}
		return out, fmt.Errorf("unrecognized arguments: %s", a)
	}
	return out, nil
}

func parseFormatFlag(p parsedFlags) (string, error) {
	f, ok := p.values["--format"]
	if !ok || strings.TrimSpace(f) == "" {
		return "text", nil
	}
	if f != "text" && f != "json" {
		return "", fmt.Errorf("argument --format: invalid choice %q (choose from text, json)", f)
	}
	return f, nil
}
