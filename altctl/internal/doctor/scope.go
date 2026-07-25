package doctor

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/alt-project/altctl/internal/stack"
)

// composeIncludeDoc decodes only the top-level `include:` key of a compose
// YAML file, the same yaml.Node trick internal/stack/compose_discovery.go
// uses for `services:` -- it sidesteps having to resolve anchors/aliases
// elsewhere in the file.
type composeIncludeDoc struct {
	Include yaml.Node `yaml:"include"`
}

// readComposeIncludes returns the list of files named in a compose file's
// top-level `include:` sequence. Every include entry in this repo's
// compose/*.yaml is a bare filename scalar (e.g. `- base.yaml`); the compose
// spec also allows `- path: foo.yaml` mappings, which is handled too for
// robustness even though it isn't exercised today.
func readComposeIncludes(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc composeIncludeDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if doc.Include.Kind != yaml.SequenceNode {
		return nil, nil
	}
	var out []string
	for _, item := range doc.Include.Content {
		switch item.Kind {
		case yaml.ScalarNode:
			out = append(out, item.Value)
		case yaml.MappingNode:
			for i := 0; i+1 < len(item.Content); i += 2 {
				if item.Content[i].Value == "path" {
					out = append(out, item.Content[i+1].Value)
				}
			}
		}
	}
	return out, nil
}

// aggregateCoveredFiles returns the set of compose filenames that
// compose/compose.yaml pulls in transitively (plus compose.yaml and
// base.yaml themselves), i.e. the stacks doctor can query in one shot via
// the aggregate file. Stacks whose file isn't in this set (currently: dev,
// frontend-dev, load-test -- local-dev-only overlays intentionally left out
// of the aggregate) are probed in isolation instead, see fileSetForStack.
func aggregateCoveredFiles(composeDir string) (map[string]bool, error) {
	includes, err := readComposeIncludes(filepath.Join(composeDir, aggregateComposeFile))
	covered := map[string]bool{aggregateComposeFile: true, "base.yaml": true}
	for _, f := range includes {
		covered[f] = true
	}
	if err != nil {
		return covered, err
	}
	return covered, nil
}

// fileSetForStack resolves the minimal, self-consistent -f file list needed
// to query a single stack that the aggregate doesn't cover: the stack's own
// dependency closure (base, its declared depends_on stacks, itself), via
// the same DependencyResolver `altctl up`/`restart` already use. This only
// works safely for stacks that don't transitively include pki.yaml (dev,
// frontend-dev, load-test all only `include: base.yaml`); stacks that do
// must go through the aggregate.
func fileSetForStack(registry *stack.Registry, name string) ([]string, error) {
	resolver := stack.NewDependencyResolver(registry)
	stacks, err := resolver.Resolve([]string{name})
	if err != nil {
		return nil, err
	}
	var files []string
	seen := map[string]bool{}
	for _, st := range stacks {
		if st.ComposeFile == "" || seen[st.ComposeFile] {
			continue
		}
		seen[st.ComposeFile] = true
		files = append(files, st.ComposeFile)
	}
	return files, nil
}

// selectScope resolves which stacks a doctor run should report on.
//
// Explicit requested names narrow the scope to exactly those stacks (order
// preserved, duplicates dropped). With no explicit request, the default
// scope is every non-optional stack plus any optional stack that has at
// least one service present in runningServices (i.e. `docker compose ps`
// returned a container for it, running or not -- presence is evidence the
// stack matters right now).
func selectScope(registry *stack.Registry, requested []string, runningServices map[string]bool) ([]*stack.Stack, error) {
	if len(requested) > 0 {
		var result []*stack.Stack
		seen := map[string]bool{}
		for _, name := range requested {
			if seen[name] {
				continue
			}
			seen[name] = true
			s, ok := registry.Get(name)
			if !ok {
				return nil, &unknownStackError{name: name}
			}
			result = append(result, s)
		}
		return result, nil
	}

	var result []*stack.Stack
	for _, s := range registry.All() {
		if !s.Optional {
			result = append(result, s)
			continue
		}
		for _, svc := range s.Services {
			if runningServices[svc] {
				result = append(result, s)
				break
			}
		}
	}
	return result, nil
}

type unknownStackError struct{ name string }

func (e *unknownStackError) Error() string {
	return "unknown stack: " + e.name
}
