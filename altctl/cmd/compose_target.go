package cmd

import (
	"sort"

	"github.com/alt-project/altctl/internal/stack"
)

// stackInvocation is the -f file list / --profile list / service name list
// that a resolved set of stacks maps to for a single `docker compose`
// invocation.
//
// This is the C3 fix: per-stack `-f` subsets are structurally broken --
// `docker compose -f base.yaml -f db.yaml -f pgbouncer.yaml -f auth.yaml -f
// sovereign.yaml -f core.yaml config` (i.e. exactly what `altctl up core`
// used to build) is rejected by real `docker compose` ("service
// 'pki-agent-acolyte-orchestrator' depends on undefined service
// 'acolyte-orchestrator': invalid compose project") because core.yaml
// transitively `include:`s pki.yaml, whose pki-agent sidecars `depends_on`
// services scattered across many other stacks (recap-subworker,
// tag-generator, acolyte-orchestrator, ...). Even a *single* stack's own
// file can fail alone for the same reason (`-f core.yaml` alone: "migrate
// depends on undefined service db" -- migrate's depends_on reaches outside
// core.yaml's own include: closure entirely).
//
// compose/compose.yaml (the top-level `include:` aggregate) is exactly what
// a human running `docker compose` directly already uses, and is
// internally consistent by construction (see compose/compose.yaml's own
// header comment). Lifecycle commands (up/restart/rebuild/logs/exec) use it
// as the sole `-f` argument whenever every stack involved is reachable
// through it (stack.Stack.AggregateCovered), and scope the operation by
// naming services explicitly -- compose auto-starts each named service's
// transitive depends_on, so the file list doesn't need to be "complete" for
// anything beyond the aggregate itself.
//
// Three stacks sit outside the aggregate's include: graph entirely (dev,
// frontend-dev, load-test -- local-dev-only overlays; see
// internal/doctor/scope.go's isolated-stacks logic, which hit this first).
// Two different sub-cases apply to them, both empirically verified against
// real `docker compose ... config`:
//
//   - dev and frontend-dev must NEVER be combined with the aggregate file:
//     compose.yaml pulls in core.yaml, which declares alt-frontend-sv /
//     alt-backend with resource limits that conflict with dev.yaml's /
//     frontend-dev.yaml's own redeclaration of the same service names
//     ("services.alt-frontend-sv: can't set distinct values on 'mem_limit'
//     and 'deploy.resources.limits.memory'"). Each of their own compose
//     files already `include: base.yaml` itself, so the file is
//     self-sufficient alone.
//   - load-test depends on perf (an AggregateCovered stack) and is NOT
//     self-sufficient alone: perf.yaml's k6 service depends_on alt-backend,
//     which is only defined in core.yaml -- nowhere in load-test's own
//     dependency closure (base + perf + load-test). Combining the
//     aggregate with load-test.yaml on top has no such conflict (verified:
//     `docker compose -f compose.yaml -f load-test.yaml config` succeeds),
//     and is exactly the recipe compose/load-test.yaml's own header comment
//     documents.
//
// The rule below generalizes both empirically-verified cases: if resolving
// the requested stacks pulls in an isolated stack, the aggregate is only
// skipped when every non-base member of the resolved set is *also*
// isolated (the dev/frontend-dev case); as soon as a real (non-base)
// AggregateCovered stack shows up alongside an isolated one (the load-test
// case, via its "perf" dependency), the aggregate is used as the base file
// with the isolated stack(s)' own file(s) layered on top. base.yaml itself
// is always AggregateCovered but contributes no services, so its presence
// alone never tips the decision either way.
type stackInvocation struct {
	// Files is the -f argument list.
	Files []string
	// Services is the union of every resolved stack's own Services, in
	// resolve order, deduped.
	Services []string
	// Profiles is the deduped list of `--profile` values needed for any
	// resolved stack that declares one (stack.Stack.Profile), sorted for
	// deterministic argv.
	Profiles []string
	// Aggregate reports whether Files anchors on stack.AggregateComposeFile
	// (true for the common case, false for a pure dev/frontend-dev-style
	// isolated invocation).
	Aggregate bool
}

// buildStackInvocation computes the Files/Services/Profiles for a list of
// stacks (a resolved dependency closure for up/restart, or a single-element
// list for logs/exec, which don't want to pull in dependencies at all).
// Every stack must come from the same registry.
func buildStackInvocation(stacks []*stack.Stack) stackInvocation {
	var inv stackInvocation

	var isolatedFiles []string
	seenIsolatedFile := map[string]bool{}
	nonBaseAggregateMember := false

	seenSvc := map[string]bool{}
	seenProfile := map[string]bool{}

	for _, s := range stacks {
		if s.AggregateCovered {
			if s.Name != "base" {
				nonBaseAggregateMember = true
			}
		} else if s.ComposeFile != "" && !seenIsolatedFile[s.ComposeFile] {
			seenIsolatedFile[s.ComposeFile] = true
			isolatedFiles = append(isolatedFiles, s.ComposeFile)
		}

		for _, svc := range s.Services {
			if seenSvc[svc] {
				continue
			}
			seenSvc[svc] = true
			inv.Services = append(inv.Services, svc)
		}

		if s.Profile != "" && !seenProfile[s.Profile] {
			seenProfile[s.Profile] = true
			inv.Profiles = append(inv.Profiles, s.Profile)
		}
	}

	if len(isolatedFiles) == 0 || nonBaseAggregateMember {
		inv.Aggregate = true
		inv.Files = append([]string{stack.AggregateComposeFile}, isolatedFiles...)
	} else {
		inv.Aggregate = false
		inv.Files = isolatedFiles
	}

	sort.Strings(inv.Profiles)
	return inv
}
