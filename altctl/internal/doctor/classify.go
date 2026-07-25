package doctor

import (
	"fmt"
	"sort"
	"strings"
)

// classifyService turns a service's `docker compose ps` entry into a state
// label. entry is nil when the service is expected (declared in a stack in
// scope) but docker compose ps returned nothing for it at all -- "missing".
// The second return value is false for services that are simply healthy and
// running, i.e. not a Finding-worthy problem.
//
// M1: the default case used to catch every unmatched compose State --
// including "paused" and "dead", both real values `docker compose ps`
// reports -- and mark it StateHealthy with isProblem=false. A paused or
// crashed-and-undead container was therefore invisible to `altctl doctor`.
// The default now mirrors internal/health/waiter.go's evaluateOne polarity:
// a state is only treated as healthy when it's explicitly recognized as
// such (running/up, or a clean exit(0) for one-shot containers); anything
// else -- including states this table hasn't been taught about yet -- is a
// problem, not an assumed-fine default.
func classifyService(entry *psEntry) (label string, severity Severity, isProblem bool) {
	if entry == nil {
		return StateMissing, SeverityWarning, true
	}

	state := strings.ToLower(entry.State)
	health := strings.ToLower(entry.Health)

	switch {
	case health == "unhealthy":
		return StateUnhealthy, SeverityError, true
	case strings.Contains(state, "restart"):
		return StateRestarting, SeverityError, true
	case state == "paused":
		return StatePaused, SeverityWarning, true
	case state == "dead":
		return StateDead, SeverityError, true
	case state == "exited" && entry.ExitCode != 0:
		return StateExitedNonZero, SeverityError, true
	case state == "exited":
		// ExitCode == 0: a one-shot container (migrator/init job) that
		// exited cleanly, matching internal/health/waiter.go's Ready rule
		// for one-shot containers -- not a problem.
		return StateHealthy, "", false
	case health == "starting" || state == "created":
		return StateStarting, SeverityInfo, true
	case state == "running":
		// health == "" (no healthcheck declared) or "healthy" both mean
		// Ready here, mirroring internal/health/waiter.go's evaluateOne.
		return StateHealthy, "", false
	default:
		return StateUnknown, SeverityWarning, true
	}
}

// describeState renders a short human phrase for a classification label,
// used when building "X is waiting on Y (<describeState>)" root-cause
// messages.
func describeState(label string) string {
	switch label {
	case StateMissing:
		return "missing"
	case StateUnhealthy:
		return "unhealthy"
	case StateRestarting:
		return "restarting"
	case StateExitedNonZero:
		return "exited non-zero"
	case StateStarting:
		return "still starting"
	case StatePaused:
		return "paused"
	case StateDead:
		return "dead"
	case StateUnknown:
		return "in an unrecognized state"
	default:
		return label
	}
}

// findRootCause walks a service's depends_on chain looking for the deepest
// ancestor that is itself a problem (missing or in a bad ps state). If A
// depends on B and B is unhealthy, B is reported as the root cause of A's
// finding instead of (or in addition to) A itself. Dependency iteration is
// sorted for deterministic output; a visited set guards against cycles.
func findRootCause(svc string, cfgMap map[string]composeServiceConfig, psMap map[string]psEntry, visited map[string]bool) string {
	if visited[svc] {
		return svc
	}
	visited[svc] = true

	cfg, ok := cfgMap[svc]
	if !ok || len(cfg.DependsOn) == 0 {
		return svc
	}

	deps := make([]string, 0, len(cfg.DependsOn))
	for dep := range cfg.DependsOn {
		deps = append(deps, dep)
	}
	sort.Strings(deps)

	for _, dep := range deps {
		entry, present := psMap[dep]
		var entryPtr *psEntry
		if present {
			entryPtr = &entry
		}
		_, _, isProblem := classifyService(entryPtr)
		if !present || isProblem {
			return findRootCause(dep, cfgMap, psMap, visited)
		}
	}
	return svc
}

// buildMessage renders the human-readable summary for a service finding,
// pointing at the root cause when one was identified.
func buildMessage(svc, label, rootCause, rootCauseLabel string) string {
	if rootCause != "" && rootCause != svc {
		return fmt.Sprintf("%s is %s -- waiting on %s (%s), fix that first", svc, describeState(label), rootCause, describeState(rootCauseLabel))
	}
	switch label {
	case StateMissing:
		return fmt.Sprintf("%s is expected but has no container", svc)
	case StateUnhealthy:
		return fmt.Sprintf("%s is unhealthy", svc)
	case StateRestarting:
		return fmt.Sprintf("%s is restarting (crash-looping)", svc)
	case StateExitedNonZero:
		return fmt.Sprintf("%s exited non-zero", svc)
	case StateStarting:
		return fmt.Sprintf("%s is still starting", svc)
	case StatePaused:
		return fmt.Sprintf("%s is paused", svc)
	case StateDead:
		return fmt.Sprintf("%s is dead (failed to stop/remove cleanly)", svc)
	case StateUnknown:
		return fmt.Sprintf("%s is in an unrecognized state", svc)
	default:
		return fmt.Sprintf("%s: %s", svc, label)
	}
}

// prescriptionFor returns concrete next-command suggestions for a
// classified finding. target is the root cause when one was identified,
// otherwise the service itself -- prescriptions should point at whatever
// actually needs fixing.
func prescriptionFor(label, target string) []string {
	switch label {
	case StateMissing:
		return []string{
			fmt.Sprintf("altctl logs %s", target),
			fmt.Sprintf("docker compose -f compose/compose.yaml -p alt up -d %s", target),
		}
	case StateUnhealthy, StateRestarting:
		return []string{
			fmt.Sprintf("altctl logs %s -f", target),
			fmt.Sprintf("docker compose -f compose/compose.yaml -p alt up -d --force-recreate %s", target),
		}
	case StateExitedNonZero:
		return []string{
			fmt.Sprintf("altctl logs %s", target),
			fmt.Sprintf("docker compose -f compose/compose.yaml -p alt up -d --force-recreate %s", target),
		}
	case StateStarting:
		return []string{
			"altctl status",
			fmt.Sprintf("altctl logs %s -f", target),
		}
	case StatePaused:
		return []string{
			fmt.Sprintf("docker compose -f compose/compose.yaml -p alt unpause %s", target),
			"altctl status",
		}
	case StateDead:
		return []string{
			fmt.Sprintf("altctl logs %s", target),
			fmt.Sprintf("docker compose -f compose/compose.yaml -p alt up -d --force-recreate %s", target),
		}
	case StateUnknown:
		return []string{
			"altctl status",
			fmt.Sprintf("altctl logs %s", target),
		}
	default:
		return nil
	}
}

// healthyDependsOnFindings implements the known failure pattern from
// docs/services/altctl.md: `depends_on: {condition: service_healthy}`
// pointing at a service with no healthcheck: block at all is a landmine
// that makes `up --wait` hang or abort, independent of whether anything is
// currently running. It's evaluated statically from compose config, so it
// surfaces even when every container is currently stopped.
func healthyDependsOnFindings(stackName string, services []string, cfgMap map[string]composeServiceConfig) []Finding {
	var findings []Finding
	for _, svc := range services {
		cfg, ok := cfgMap[svc]
		if !ok {
			continue
		}
		deps := make([]string, 0, len(cfg.DependsOn))
		for dep := range cfg.DependsOn {
			deps = append(deps, dep)
		}
		sort.Strings(deps)
		for _, dep := range deps {
			rel := cfg.DependsOn[dep]
			if rel.Condition != "service_healthy" {
				continue
			}
			depCfg, known := cfgMap[dep]
			if known && depCfg.HasHealthcheck() {
				continue
			}
			findings = append(findings, Finding{
				Severity: SeverityWarning,
				Category: "config",
				Stack:    stackName,
				Service:  svc,
				Message:  fmt.Sprintf("%s depends_on %s with condition: service_healthy, but %s has no healthcheck defined", svc, dep, dep),
				Detail:   "docker compose up --wait will hang or abort waiting for a health status that can never arrive (docs/services/altctl.md known failure pattern, [[000809]])",
				Prescription: []string{
					fmt.Sprintf("add a healthcheck: block to %s in compose/*.yaml, or relax %s's depends_on condition", dep, svc),
				},
			})
		}
	}
	return findings
}
