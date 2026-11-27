package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"alt/scripts/altharness/internal/canary"
	"alt/scripts/altharness/internal/codegen"
	"alt/scripts/altharness/internal/corpus"
	"alt/scripts/altharness/internal/generated"
	"alt/scripts/altharness/internal/guards"
	"alt/scripts/altharness/internal/hygiene"
	"alt/scripts/altharness/internal/lang"
	"alt/scripts/altharness/internal/ledgers"
	"alt/scripts/altharness/internal/lint"
	"alt/scripts/altharness/internal/meta"
	"alt/scripts/altharness/internal/packet"
	"alt/scripts/altharness/internal/pki"
	"alt/scripts/altharness/internal/preflight"
	"alt/scripts/altharness/internal/pytestrand"
	"alt/scripts/altharness/internal/pyver"
	"alt/scripts/altharness/internal/ratchet"
	"alt/scripts/altharness/internal/screener"
	"alt/scripts/altharness/internal/selfcheck"
	"alt/scripts/altharness/internal/suppress"
	"alt/scripts/altharness/internal/workflow"
)

func init() {
	guards.PreflightRunner = preflight.Run
}

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: alt-harness <command> [args]")
		return 2
	}
	switch args[0] {
	case "lint":
		repo, err := resolveRepoRoot()
		if err != nil {
			fmt.Fprintln(stderr, err.Error())
			return 1
		}
		if err := os.Chdir(repo); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return 1
		}
		return lint.Run(ctx, repo, args[1:], stdout, stderr)
	case "lang":
		return runLang(ctx, args[1:], stdout, stderr)
	case "layers":
		return lang.RunLayersWithStdin(ctx, args[1:], stdin, stdout, stderr)
	case "host-final":
		return lang.RunHostFinal(ctx, args[1:], stdout, stderr)
	case "link-naming":
		return lang.RunLinkNaming(ctx, args[1:], stdout, stderr)
	case "sync-go":
		return codegen.Run(ctx, args[1:], stdout, stderr)
	case "pki":
		return pki.Run(ctx, args[1:], stdout, stderr)
	case "pyver":
		return pyver.Run(ctx, args[1:], stdout, stderr)
	case "workflow":
		return workflow.Run(ctx, args[1:], stdout, stderr)
	case "hygiene":
		return hygiene.Run(ctx, args[1:], stdout, stderr)
	case "ratchet":
		return ratchet.Run(ctx, args[1:], stdout, stderr)
	case "suppressions":
		return suppress.RunCheck(ctx, args[1:], stdout, stderr)
	case "suppression-diff":
		return suppress.RunDiff(ctx, args[1:], stdout, stderr)
	case "generated":
		return generated.Run(ctx, args[1:], stdout, stderr)
	case "screener":
		return screener.Run(ctx, args[1:], stdout, stderr)
	case "packet":
		return packet.Run(ctx, args[1:], stdout, stderr)
	case "corpus":
		return corpus.Run(ctx, args[1:], stdout, stderr)
	case "selfcheck":
		return selfcheck.Run(ctx, args[1:], stdout, stderr)
	case "canary":
		return canary.RunAudit(ctx, args[1:], stdout, stderr)
	case "entrypoint-canary":
		return canary.RunEntrypoint(ctx, args[1:], stdout, stderr)
	case "postmortem":
		return ledgers.RunPostmortem(ctx, args[1:], stdout, stderr)
	case "migration-gate":
		return ledgers.RunMigration(ctx, args[1:], stdout, stderr)
	case "adr-gate":
		return ledgers.RunAdr(ctx, args[1:], stdout, stderr)
	case "guard-exec":
		return guards.RunExec(ctx, args[1:], stdin, stdout, stderr)
	case "guard-scope":
		return guards.RunScope(ctx, args[1:], stdin, stdout, stderr)
	case "guard-ops":
		return guards.RunOps(ctx, args[1:], stdin, stdout, stderr)
	case "guard-protected":
		return guards.RunProtected(ctx, args[1:], stdin, stdout, stderr)
	case "guard-event-time":
		return guards.RunEventTime(ctx, args[1:], stdin, stdout, stderr)
	case "guard-adr-read":
		return guards.RunAdrRead(ctx, args[1:], stdin, stdout, stderr)
	case "harness-run":
		return guards.RunHarness(ctx, args[1:], stdin, stdout, stderr)
	case "agy-hook":
		return guards.RunAgyHook(ctx, args[1:], stdin, stdout, stderr)
	case "preflight":
		return preflight.Run(ctx, args[1:], stdin, stdout, stderr)
	case "pytest-rand":
		return pytestrand.Run(ctx, args[1:], stdout, stderr)
	case "gate":
		if len(args) != 2 {
			fmt.Fprintln(stderr, "usage: alt-harness gate <repo|harness|both>")
			return 2
		}
		repo, err := resolveRepoRoot()
		if err != nil {
			fmt.Fprintln(stderr, err.Error())
			return 1
		}
		if err := os.Chdir(repo); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return 1
		}
		steps, err := meta.Catalog(ctx, repo, args[1])
		if err != nil {
			fmt.Fprintln(stderr, err.Error())
			return 2
		}
		code, err := meta.Run(ctx, steps, meta.Options{Repo: repo, Stdout: stdout, Stderr: stderr, Timeout: 15 * time.Minute})
		if err != nil {
			fmt.Fprintln(stderr, err.Error())
			return 1
		}
		return code
	case "help", "-h", "--help":
		fmt.Fprintln(stdout, "alt-harness commands: lint lang layers host-final link-naming sync-go pki pyver workflow hygiene ratchet suppressions suppression-diff generated screener packet corpus selfcheck canary entrypoint-canary postmortem migration-gate adr-gate guard-exec guard-scope guard-ops guard-protected guard-event-time guard-adr-read harness-run agy-hook preflight pytest-rand gate")
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		return 2
	}
}

func runLang(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: alt-harness lang <go|python|rust|deno|frontend|adr|proto|compose|observability|apparmor|e2e|registry> [args]")
		return 2
	}
	rest := args[1:]
	switch args[0] {
	case "go":
		return lang.RunGo(ctx, rest, stdout, stderr)
	case "python":
		return lang.RunPython(ctx, rest, stdout, stderr)
	case "rust":
		return lang.RunRust(ctx, rest, stdout, stderr)
	case "deno":
		return lang.RunDeno(ctx, rest, stdout, stderr)
	case "frontend":
		return lang.RunFrontend(ctx, rest, stdout, stderr)
	case "adr":
		return lang.RunAdr(ctx, rest, stdout, stderr)
	case "proto":
		return lang.RunProto(ctx, rest, stdout, stderr)
	case "compose":
		return lang.RunCompose(ctx, rest, stdout, stderr)
	case "observability":
		return lang.RunObservability(ctx, rest, stdout, stderr)
	case "apparmor":
		return lang.RunApparmor(ctx, rest, stdout, stderr)
	case "e2e":
		return lang.RunE2E(ctx, rest, stdout, stderr)
	case "registry":
		return lang.RunRegistry(ctx, rest, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown language runner %q\n", args[0])
		return 2
	}
}

func resolveRepoRoot() (string, error) {
	isReg := func(path string) bool {
		st, err := os.Stat(path)
		return err == nil && !st.IsDir()
	}
	hasRepoMarker := func(dir string) bool {
		return isReg(filepath.Join(dir, "scripts", "lint", "run.sh")) ||
			isReg(filepath.Join(dir, "scripts", "altharness", "alt-harness"))
	}
	walkUp := func(start string) string {
		dir := start
		for {
			if hasRepoMarker(dir) {
				return dir
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				return ""
			}
			dir = parent
		}
	}
	wd, err := os.Getwd()
	if err == nil {
		if found := walkUp(wd); found != "" {
			return found, nil
		}
		if out, gerr := exec.Command("git", "rev-parse", "--show-toplevel").Output(); gerr == nil {
			top := strings.TrimSpace(string(out))
			if top != "" && hasRepoMarker(top) {
				return top, nil
			}
		}
	}
	if exe, eerr := os.Executable(); eerr == nil {
		if found := walkUp(filepath.Dir(exe)); found != "" {
			return found, nil
		}
		if resolved, rerr := filepath.EvalSymlinks(exe); rerr == nil {
			if found := walkUp(filepath.Dir(resolved)); found != "" {
				return found, nil
			}
		}
	}
	if err != nil {
		return "", err
	}
	return wd, nil
}
