package lang

import (
	"context"
	"io"

	"alt/scripts/altharness/internal/lint"
)

func init() {
	lint.RegisterKindDispatcher(dispatchKind)
}

func dispatchKind(ctx context.Context, kind, target string, stdout, stderr io.Writer) (int, bool) {
	args := []string{target}
	switch kind {
	case "go":
		return RunGo(ctx, args, stdout, stderr), true
	case "python":
		return RunPython(ctx, args, stdout, stderr), true
	case "rust":
		return RunRust(ctx, args, stdout, stderr), true
	case "deno":
		return RunDeno(ctx, args, stdout, stderr), true
	case "frontend":
		return RunFrontend(ctx, args, stdout, stderr), true
	case "adr":
		return RunAdr(ctx, args, stdout, stderr), true
	case "proto":
		return RunProto(ctx, args, stdout, stderr), true
	case "compose":
		return RunCompose(ctx, args, stdout, stderr), true
	case "observability":
		return RunObservability(ctx, args, stdout, stderr), true
	case "apparmor":
		return RunApparmor(ctx, args, stdout, stderr), true
	case "e2e":
		return RunE2E(ctx, args, stdout, stderr), true
	case "registry":
		return RunRegistry(ctx, args, stdout, stderr), true
	default:
		return 0, false
	}
}
