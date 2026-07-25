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
	case state == "exited" && entry.ExitCode != 0:
		return StateExitedNonZero, SeverityError, true
	case health == "starting" || state == "created":
		return StateStarting, SeverityInfo, true
	default:
		return StateHealthy, "", false
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
