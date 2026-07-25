package stack

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Registry holds all available stack definitions
type Registry struct {
	stacks map[string]*Stack
}

// NewRegistry builds a stack registry by deriving stacks from compose/*.yaml
// files in composeDir and layering in the semantics declared in the altctl
// config file at configPath (dependency order, optionality, GPU/timeout,
// feature provide/require -- see StackSemantics).
//
// Stack name = compose filename stem (db.yaml -> "db"); a stack's Services
// are exactly the top-level services: map keys of its own file (includes
// and anchors are not resolved across files). A file with no services (or
// an empty services: {}) is not a stack unless explicitly declared in
// configPath (this is how "base" -- shared resources only -- stays a valid
// dependency target). Files listed under configPath's overlays/excluded are
// never auto-registered.
//
// A compose file with services and no declared semantics is auto-registered
// with defaults (optional, depending on "base" if present) and a notice is
// printed to stderr, so drift between compose/ and .altctl.yaml is visible
// without being fatal. A declared semantics entry whose compose file does
// not exist is a hard error (fail-fast, altctl/CLAUDE.md Critical Rule 9).
func NewRegistry(composeDir, configPath string) (*Registry, error) {
	semantics, err := LoadSemanticsConfig(configPath)
	if err != nil {
		return nil, err
	}
	return newRegistry(composeDir, semantics)
}

// NewRegistryFromSemantics builds a registry from an already-loaded
// SemanticsConfig. It exists mainly for tests that want to exercise
// derivation against a fixture compose dir without also writing a config
// file to disk; production code should use NewRegistry.
func NewRegistryFromSemantics(composeDir string, semantics *SemanticsConfig) (*Registry, error) {
	if semantics == nil {
		semantics = &SemanticsConfig{}
	}
	return newRegistry(composeDir, semantics)
}

func newRegistry(composeDir string, semantics *SemanticsConfig) (*Registry, error) {
	skip := stringSet(semantics.Overlays)
	for k, v := range stringSet(semantics.Excluded) {
		skip[k] = v
	}

	discovered, err := discoverComposeStacks(composeDir, skip)
	if err != nil {
		return nil, err
	}

	r := &Registry{stacks: make(map[string]*Stack, len(discovered)+len(semantics.Stacks))}

	// Declared stacks: their compose file must exist, but may have an empty
	// services: map (e.g. "base"). Fail fast if it doesn't exist at all.
	for name, sem := range semantics.Stacks {
		file := name + ".yaml"
		path := filepath.Join(composeDir, file)
		services, statErr := composeFileServicesIfExists(path, composeDir, name)
		if statErr != nil {
			return nil, statErr
		}
		r.stacks[name] = &Stack{
			Name:             name,
			Description:      sem.Description,
			ComposeFile:      file,
			Services:         services,
			DependsOn:        sem.DependsOn,
			Profile:          sem.Profile,
			Optional:         sem.Optional,
			RequiresGPU:      sem.RequiresGPU,
			Timeout:          time.Duration(sem.StartupTimeout),
			Provides:         sem.Provides,
			RequiresFeatures: sem.RequiresFeatures,
		}
	}

	_, hasBase := r.stacks["base"]

	// Auto-register any discovered compose file that wasn't already covered
	// by a declared stack above.
	for _, cs := range discovered {
		if _, declared := semantics.Stacks[cs.Stem]; declared {
			continue
		}
		fmt.Fprintf(os.Stderr,
			"notice: stack %q auto-registered from %s (no semantics declared in altctl config; using defaults)\n",
			cs.Stem, filepath.Join(composeDir, cs.File))

		st := &Stack{
			Name:        cs.Stem,
			ComposeFile: cs.File,
			Services:    cs.Services,
			Optional:    true,
		}
		if hasBase && cs.Stem != "base" {
			st.DependsOn = []string{"base"}
		}
		r.stacks[cs.Stem] = st
	}

	// Mark every stack (declared and auto-registered alike) with whether
	// its compose file is reachable through compose.yaml's `include:`
	// graph -- see Stack.AggregateCovered and internal/stack/aggregate.go.
	// A missing or unreadable aggregate file degrades to "nothing is
	// covered" rather than failing registry construction outright (see
	// aggregateCoveredFiles), except for a genuine parse error on an
	// aggregate file that does exist, which is a real config problem worth
	// surfacing fail-fast (Critical Rule 9).
	covered, err := aggregateCoveredFiles(composeDir)
	if err != nil {
		return nil, fmt.Errorf("determining aggregate-covered stacks: %w", err)
	}
	for _, st := range r.stacks {
		st.AggregateCovered = covered[st.ComposeFile]
	}

	return r, nil
}

// composeFileServicesIfExists resolves a declared stack's compose file
// path, returning its services (possibly empty) or an error that names the
// declaring stack when the file is missing entirely.
func composeFileServicesIfExists(path, composeDir, stackName string) ([]string, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("stack %q is declared in altctl config but has no matching compose file %s in %s", stackName, stackName+".yaml", composeDir)
		}
		return nil, fmt.Errorf("checking compose file for stack %q: %w", stackName, err)
	}
	return composeFileServices(path)
}

// Get returns a stack by name
func (r *Registry) Get(name string) (*Stack, bool) {
	s, ok := r.stacks[name]
	return s, ok
}

// All returns all registered stacks
func (r *Registry) All() []*Stack {
	result := make([]*Stack, 0, len(r.stacks))
	for _, s := range r.stacks {
		result = append(result, s)
	}
	// Sort by name for consistent ordering
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// Names returns all stack names
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.stacks))
	for name := range r.stacks {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// DefaultStacks returns stacks that should be started by default
func (r *Registry) DefaultStacks() []*Stack {
	var defaults []*Stack
	for _, s := range r.stacks {
		if s.IsDefault() {
			defaults = append(defaults, s)
		}
	}
	return defaults
}

// OptionalStacks returns stacks that are optional
func (r *Registry) OptionalStacks() []*Stack {
	var optional []*Stack
	for _, s := range r.stacks {
		if s.Optional {
			optional = append(optional, s)
		}
	}
	return optional
}

// FindByService returns the stack containing the given service,
// deterministically. Before this existed, FindByService iterated a Go map
// directly, so a service declared in more than one stack's Services (e.g.
// "alt-backend" in both core.yaml and the local-dev override dev.yaml)
// resolved to a different stack on roughly half of all calls -- silently
// nondeterministic (C4). Iteration now goes through r.All(), which is
// sorted by name, and ambiguity is broken deterministically:
//
//  1. If the service appears in exactly one stack, return it.
//  2. If it appears in more than one, prefer a stack inside the aggregate
//     include: graph (Stack.AggregateCovered) over one outside it --
//     that's the stack `docker compose -f compose/compose.yaml up
//     <service>` would actually touch under the aggregate-file-first
//     strategy lifecycle commands use (see cmd/compose_target.go). E.g.
//     "alt-backend" resolves to "core", never "dev".
//  3. If step 2 still leaves more than one candidate (today: impossible --
//     compose itself rejects two stacks declaring the same service name
//     inside one project, so two AggregateCovered stacks sharing a service
//     name can't coexist; two non-AggregateCovered stacks sharing one
//     could, in principle, e.g. two future isolated dev overlays), return
//     an error naming every candidate stack instead of guessing.
func (r *Registry) FindByService(service string) (*Stack, error) {
	var candidates []*Stack
	for _, s := range r.All() {
		for _, svc := range s.Services {
			if svc == service {
				candidates = append(candidates, s)
				break
			}
		}
	}

	switch len(candidates) {
	case 0:
		return nil, nil
	case 1:
		return candidates[0], nil
	}

	var aggregateCandidates []*Stack
	for _, s := range candidates {
		if s.AggregateCovered {
			aggregateCandidates = append(aggregateCandidates, s)
		}
	}
	switch len(aggregateCandidates) {
	case 1:
		return aggregateCandidates[0], nil
	case 0:
		return nil, ambiguousServiceError(service, candidates)
	default:
		return nil, ambiguousServiceError(service, aggregateCandidates)
	}
}

// ambiguousServiceError builds the error FindByService returns when a
// service name can't be resolved to exactly one stack even after preferring
// aggregate-covered stacks.
func ambiguousServiceError(service string, candidates []*Stack) error {
	names := make([]string, len(candidates))
	for i, s := range candidates {
		names[i] = s.Name
	}
	return fmt.Errorf(
		"service %q is declared in more than one stack (%s) and could not be resolved deterministically; disambiguate by passing the stack name instead",
		service, strings.Join(names, ", "))
}

// Register adds or updates a stack in the registry
func (r *Registry) Register(s *Stack) {
	r.stacks[s.Name] = s
}
