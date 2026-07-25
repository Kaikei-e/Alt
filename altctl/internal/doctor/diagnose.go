package doctor

import (
	"context"
	"fmt"
	"time"

	"github.com/alt-project/altctl/internal/stack"
)

const defaultLogTailLines = 30

// Diagnose runs the full read-only doctor pass: environment preflight,
// docker daemon reachability, service state classification per stack in
// scope, root-cause chaining through depends_on, static config landmine
// checks, and log evidence capture for every problem found.
//
// It never mutates anything -- every docker invocation is `info`, `ps`,
// `config`, or `logs`. An error is only returned for caller mistakes
// (an explicitly named stack that doesn't exist in the registry); every
// other failure mode (daemon unreachable, compose config broken, individual
// log fetch failing) is captured as a Finding in the returned Report
// instead, so a partial diagnosis is still useful.
func Diagnose(ctx context.Context, opts Options) (*Report, error) {
	if opts.LogTailLines <= 0 {
		opts.LogTailLines = defaultLogTailLines
	}

	report := &Report{GeneratedAt: time.Now().UTC()}

	// Validate any explicit stack names up front -- this is the one case
	// that's a genuine caller error, not a diagnosis finding.
	for _, name := range opts.Stacks {
		if _, ok := opts.Registry.Get(name); !ok {
			return nil, fmt.Errorf("unknown stack: %s", name)
		}
	}

	// Static preflight checks: pure filesystem, no docker needed.
	if f, bad := checkDotEnv(opts.ProjectDir); bad {
		report.Preflight = append(report.Preflight, f)
	}
	report.Preflight = append(report.Preflight, checkSecrets(opts.ComposeDir)...)

	// Docker daemon reachability gates everything below it. Report this
	// loudly and distinctly -- never let an unreachable daemon look like
	// "no services running".
	if err := checkDockerDaemon(ctx, opts.Executor); err != nil {
		report.DockerReachable = false
		report.Preflight = append(report.Preflight, Finding{
			Severity: SeverityError,
			Category: "preflight",
			Message:  "docker daemon unreachable",
			Detail:   err.Error(),
			Prescription: []string{
				"start Docker Desktop / the docker service",
				"docker info",
			},
		})
		report.Problems = collectProblems(report.Preflight, nil)
		return report, nil
	}
	report.DockerReachable = true

	psMap := map[string]psEntry{}
	cfgMap := map[string]composeServiceConfig{}
	isolatedFiles := map[string][]string{}

	covered, coverErr := aggregateCoveredFiles(opts.ComposeDir)
	if coverErr != nil {
		report.Preflight = append(report.Preflight, Finding{
			Severity: SeverityWarning,
			Category: "preflight",
			Message:  "could not read compose/compose.yaml's include: list",
			Detail:   coverErr.Error(),
		})
	}

	// Primary probe: the aggregate compose.yaml covers every stack except
	// the local-dev-only ones (dev, frontend-dev, load-test) that
	// compose.yaml deliberately doesn't include.
	primaryFiles := []string{aggregateComposeFile}
	if entries, err := runComposePS(ctx, opts.Executor, opts.ComposeDir, opts.ProjectDir, primaryFiles); err != nil {
		report.Preflight = append(report.Preflight, Finding{
			Severity: SeverityError,
			Category: "preflight",
			Message:  "docker compose status unavailable for the aggregate stack (compose/compose.yaml)",
			Detail:   err.Error(),
			Prescription: []string{
				"docker compose -f compose/compose.yaml config",
				"check the .env / DOCKER_GROUP_ID findings above",
			},
		})
	} else {
		mergePS(psMap, entries)
	}
	if doc, err := runComposeConfig(ctx, opts.Executor, opts.ComposeDir, opts.ProjectDir, primaryFiles); err != nil {
		report.Preflight = append(report.Preflight, Finding{
			Severity: SeverityWarning,
			Category: "preflight",
			Message:  "dependency / healthcheck analysis unavailable for the aggregate stack (compose/compose.yaml)",
			Detail:   err.Error(),
		})
	} else {
		mergeConfig(cfgMap, doc)
	}

	// Stacks the aggregate doesn't cover are only probed when explicitly
	// requested by name -- doing this speculatively for every default-scope
	// run would mean extra subprocess calls for stacks that are almost
	// never running. See scope.go's fileSetForStack doc comment.
	if len(opts.Stacks) > 0 {
		for _, name := range opts.Stacks {
			s, _ := opts.Registry.Get(name)
			if s.ComposeFile == "" || covered[s.ComposeFile] {
				continue
			}
			files, err := fileSetForStack(opts.Registry, s.Name)
			if err != nil {
				report.Preflight = append(report.Preflight, Finding{
					Severity: SeverityWarning,
					Category: "preflight",
					Stack:    s.Name,
					Message:  fmt.Sprintf("could not resolve compose files for stack %q", s.Name),
					Detail:   err.Error(),
				})
				continue
			}
			isolatedFiles[s.Name] = files

			if entries, err := runComposePS(ctx, opts.Executor, opts.ComposeDir, opts.ProjectDir, files); err != nil {
				report.Preflight = append(report.Preflight, Finding{
					Severity: SeverityError,
					Category: "preflight",
					Stack:    s.Name,
					Message:  fmt.Sprintf("docker compose status unavailable for stack %q", s.Name),
					Detail:   err.Error(),
				})
			} else {
				mergePS(psMap, entries)
			}
			if doc, err := runComposeConfig(ctx, opts.Executor, opts.ComposeDir, opts.ProjectDir, files); err == nil {
				mergeConfig(cfgMap, doc)
			}
		}
	}

	runningServices := make(map[string]bool, len(psMap))
	for name := range psMap {
		runningServices[name] = true
	}

	scopeStacks, err := selectScope(opts.Registry, opts.Stacks, runningServices)
	if err != nil {
		return nil, err
	}
	for _, s := range scopeStacks {
		report.Scope = append(report.Scope, s.Name)
	}

	if inScope(scopeStacks, "logging") {
		if f, bad := dockerGroupIDFinding(); bad {
			report.Preflight = append(report.Preflight, f)
		}
	}

	for _, st := range scopeStacks {
		files := primaryFiles
		if f, ok := isolatedFiles[st.Name]; ok {
			files = f
		}
		sr, findings := diagnoseStack(ctx, opts, st, files, psMap, cfgMap)
		report.Stacks = append(report.Stacks, sr)
		report.Problems = append(report.Problems, findings...)
	}

	report.Problems = collectProblems(report.Preflight, report.Problems)
	return report, nil
}

func inScope(stacks []*stack.Stack, name string) bool {
	for _, s := range stacks {
		if s.Name == name {
			return true
		}
	}
	return false
}

func mergePS(dst map[string]psEntry, entries []psEntry) {
	for _, e := range entries {
		if e.Service == "" {
			continue
		}
		dst[e.Service] = e
	}
}

func mergeConfig(dst map[string]composeServiceConfig, doc *composeConfigDoc) {
	if doc == nil {
		return
	}
	for name, cfg := range doc.Services {
		dst[name] = cfg
	}
}

// collectProblems flattens every non-info finding (preflight + service) for
// the Report.Problems / exit-code summary. Info-severity findings (e.g. a
// service still starting up) are informational only and don't fail the run.
func collectProblems(preflight []Finding, stackFindings []Finding) []Finding {
	var problems []Finding
	for _, f := range preflight {
		if f.Severity != SeverityInfo {
			problems = append(problems, f)
		}
	}
	for _, f := range stackFindings {
		if f.Severity != SeverityInfo {
			problems = append(problems, f)
		}
	}
	return problems
}

// diagnoseStack classifies every service in st, resolves root causes,
// checks the static service_healthy-without-healthcheck pattern, and
// fetches log evidence for anything found wrong.
func diagnoseStack(ctx context.Context, opts Options, st *stack.Stack, files []string, psMap map[string]psEntry, cfgMap map[string]composeServiceConfig) (StackReport, []Finding) {
	sr := StackReport{
		Name:         st.Name,
		Optional:     st.Optional,
		ServiceCount: len(st.Services),
		Healthy:      true,
	}

	for _, svc := range st.Services {
		entry, present := psMap[svc]
		var entryPtr *psEntry
		if present {
			entryPtr = &entry
		}
		label, severity, isProblem := classifyService(entryPtr)
		if !isProblem {
			continue
		}

		rootCause := findRootCause(svc, cfgMap, psMap, map[string]bool{})
		rootCauseLabel := label
		if rootCause != svc {
			rcEntry, rcPresent := psMap[rootCause]
			var rcPtr *psEntry
			if rcPresent {
				rcPtr = &rcEntry
			}
			rootCauseLabel, _, _ = classifyService(rcPtr)
		} else {
			rootCause = ""
		}

		target := svc
		if rootCause != "" {
			target = rootCause
		}

		f := Finding{
			Severity:     severity,
			Category:     "service",
			Stack:        st.Name,
			Service:      svc,
			State:        label,
			Message:      buildMessage(svc, label, rootCause, rootCauseLabel),
			RootCause:    rootCause,
			Prescription: prescriptionFor(label, target),
		}

		if label != StateMissing {
			if lines, logErr := runComposeLogs(ctx, opts.Executor, opts.ComposeDir, opts.ProjectDir, files, svc, opts.LogTailLines); logErr != nil {
				f.Detail = "log capture failed: " + logErr.Error()
			} else {
				f.Evidence = lines
			}
		}

		sr.Healthy = false
		sr.Findings = append(sr.Findings, f)
	}

	configFindings := healthyDependsOnFindings(st.Name, st.Services, cfgMap)
	if len(configFindings) > 0 {
		sr.Healthy = false
		sr.Findings = append(sr.Findings, configFindings...)
	}

	return sr, sr.Findings
}
