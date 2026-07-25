package stack

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// duration is time.Duration with YAML decoding of Go duration strings
// ("1200s", "5m"), matching what Viper/mapstructure already accepts
// elsewhere in .altctl.yaml. gopkg.in/yaml.v3 has no built-in string ->
// time.Duration conversion, so this type supplies one via UnmarshalYAML.
type duration time.Duration

func (d *duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	if s == "" {
		*d = 0
		return nil
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	*d = duration(parsed)
	return nil
}

// StackSemantics holds the parts of a stack's definition that cannot be
// derived from its compose file's services: keys -- dependency ordering,
// optionality, GPU requirements, startup timeout, and the feature
// provide/require relationships used by DependencyResolver / FeatureResolver.
//
// DependsOn's meaning under the aggregate-file-first lifecycle strategy
// (see cmd/compose_target.go's buildStackInvocation in the altctl CLI
// package): it is NOT a completeness proof of every container-level
// `depends_on:` a stack's services actually declare in compose/*.yaml --
// docker compose itself already auto-starts each named service's own
// transitive depends_on regardless of what's listed here. What DependsOn
// controls is (a) dependency order for DependencyResolver.Resolve, and (b)
// the Ready-wait target set: `altctl up core` only waits for
// base/db/pgbouncer/auth/sovereign/core's own services to become Ready,
// even though core's alt-backend actually depends_on services in "workers"
// and "mq" too (started implicitly by compose, not tracked by this wait --
// run `altctl doctor` for the full picture). core.DependsOn deliberately
// does NOT list "workers"/"mq" to get those services waited-for, because
// workers.DependsOn already includes "core" -- adding the reverse edge
// would make DependencyResolver's topological sort see a cycle that real
// `docker compose` (which resolves the whole aggregate's service graph
// directly, no stack-level ordering at all) has no trouble with. Keep
// DependsOn's edges acyclic and treat it as "what else this stack needs
// waited-for, chosen to keep the resolver's DAG acyclic" -- not "everything
// this stack's containers actually depend_on."
type StackSemantics struct {
	Description      string    `yaml:"description"`
	DependsOn        []string  `yaml:"depends_on"`
	Optional         bool      `yaml:"optional"`
	RequiresGPU      bool      `yaml:"requires_gpu"`
	StartupTimeout   duration  `yaml:"startup_timeout"`
	Profile          string    `yaml:"profile"`
	Provides         []Feature `yaml:"provides"`
	RequiresFeatures []Feature `yaml:"requires_features"`
}

// SemanticsConfig is the subset of .altctl.yaml the stack registry reads:
// per-stack semantics plus the list of compose files that are overlays
// (modify an existing stack's services rather than define a new one) or
// otherwise explicitly excluded from stack discovery.
type SemanticsConfig struct {
	Stacks   map[string]StackSemantics `yaml:"stacks"`
	Overlays []string                  `yaml:"overlays"`
	Excluded []string                  `yaml:"excluded"`
}

// LoadSemanticsConfig reads and parses the stack-registry-relevant portion
// of an altctl config file at path. A missing file (including path == "")
// is not an error -- it yields a zero-value SemanticsConfig, so a compose
// directory can still be used with pure auto-derived defaults for every
// stack.
func LoadSemanticsConfig(path string) (*SemanticsConfig, error) {
	if path == "" {
		return &SemanticsConfig{}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &SemanticsConfig{}, nil
		}
		return nil, fmt.Errorf("reading altctl config %s: %w", path, err)
	}

	var cfg SemanticsConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing altctl config %s: %w", path, err)
	}
	return &cfg, nil
}

func stringSet(items []string) map[string]bool {
	set := make(map[string]bool, len(items))
	for _, item := range items {
		set[item] = true
	}
	return set
}
