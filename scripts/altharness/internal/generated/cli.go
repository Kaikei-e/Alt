package generated

import (
	"fmt"
	"strings"
)

const usageText = `usage: check_generated_code [-h] [--repo-root REPO_ROOT] [--list-generated]
                            [--check-pins] [--check-regen]
                            [--before BEFORE] [--after AFTER]
                            [--logical-path LOGICAL_PATH] [--base BASE]
                            [--head HEAD] [--only-diff]

Verify generated code pins, regeneration parity, and DO NOT EDIT tamper rules.
`

type cliArgs struct {
	repoRoot      *string
	listGenerated bool
	checkPins     bool
	checkRegen    bool
	before        *string
	after         *string
	logicalPath   *string
	base          *string
	head          *string
	onlyDiff      bool
	help          bool
}

func parseArgs(args []string) (cliArgs, string) {
	var out cliArgs
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			if i+1 < len(args) {
				return cliArgs{}, unrecognized(args[i+1])
			}
			break
		}
		if !strings.HasPrefix(arg, "-") {
			return cliArgs{}, unrecognized(arg)
		}
		name, value, hasValue := splitAssign(arg)
		switch name {
		case "-h", "--help":
			if hasValue {
				return cliArgs{}, unexpectedExplicit(name)
			}
			out.help = true
		case "--list-generated", "--check-pins", "--check-regen", "--only-diff":
			if hasValue {
				return cliArgs{}, unexpectedExplicit(name)
			}
			switch name {
			case "--list-generated":
				out.listGenerated = true
			case "--check-pins":
				out.checkPins = true
			case "--check-regen":
				out.checkRegen = true
			case "--only-diff":
				out.onlyDiff = true
			}
		case "--repo-root", "--before", "--after", "--logical-path", "--base", "--head":
			if !hasValue {
				if i+1 >= len(args) {
					return cliArgs{}, expectedArgument(name)
				}
				i++
				value = args[i]
				hasValue = true
			}
			copied := value
			switch name {
			case "--repo-root":
				out.repoRoot = &copied
			case "--before":
				out.before = &copied
			case "--after":
				out.after = &copied
			case "--logical-path":
				out.logicalPath = &copied
			case "--base":
				out.base = &copied
			case "--head":
				out.head = &copied
			}
		default:
			return cliArgs{}, unrecognized(arg)
		}
	}
	return out, ""
}

func splitAssign(arg string) (name, value string, hasValue bool) {
	if arg == "-" || strings.HasPrefix(arg, "--") && !strings.Contains(arg[2:], "=") && len(arg) > 2 {
		return arg, "", false
	}
	eq := strings.IndexByte(arg, '=')
	if eq <= 0 {
		return arg, "", false
	}
	return arg[:eq], arg[eq+1:], true
}

func unrecognized(arg string) string {
	return "usage: check_generated_code [-h] [--repo-root REPO_ROOT] [--list-generated] [--check-pins] [--check-regen] [--before BEFORE] [--after AFTER] [--logical-path LOGICAL_PATH] [--base BASE] [--head HEAD] [--only-diff]\n" +
		fmt.Sprintf("error: unrecognized arguments: %s\n", arg)
}

func expectedArgument(name string) string {
	return fmt.Sprintf("error: argument %s: expected one argument\n", name)
}

func unexpectedExplicit(name string) string {
	return fmt.Sprintf("error: argument %s: ignored explicit argument\n", name)
}
