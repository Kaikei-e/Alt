package stack

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// composeStackFile is a single compose/*.yaml file that is a candidate to
// become a Stack: its filename stem (the would-be stack name), the filename
// itself, and the top-level services: map keys found inside it.
type composeStackFile struct {
	Stem     string
	File     string
	Services []string
}

// composeFileDoc decodes only the top-level "services:" key of a compose
// YAML document, as a raw yaml.Node. Using a Node (rather than decoding into
// map[string]interface{}) means we never need to resolve YAML anchors,
// aliases, or merge keys (<<: *anchor) that compose files use heavily (e.g.
// compose/pki.yaml's per-service `<<: *pki-agent`) -- we only need the
// scalar keys of the mapping, which are available without walking aliased
// content.
type composeFileDoc struct {
	Services yaml.Node `yaml:"services"`
}

// composeFileServices reads a compose YAML file and returns the keys of its
// top-level services: mapping, sorted. A file with no services: key, or an
// empty services: {}, returns an empty (nil) slice, not an error -- this is
// how base.yaml (shared resources only) and compose.yaml (the include-only
// aggregate) are distinguished from real stacks.
func composeFileServices(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading compose file %s: %w", path, err)
	}

	var doc composeFileDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing compose file %s: %w", path, err)
	}

	if doc.Services.Kind != yaml.MappingNode {
		return nil, nil
	}

	names := make([]string, 0, len(doc.Services.Content)/2)
	for i := 0; i+1 < len(doc.Services.Content); i += 2 {
		key := doc.Services.Content[i]
		if key.Kind == yaml.ScalarNode {
			names = append(names, key.Value)
		}
	}
	sort.Strings(names)
	return names, nil
}

// discoverComposeStacks scans dir for *.yaml/*.yml files and returns one
// composeStackFile per file that is a stack candidate: skipped files are
// those named in skip (overlays and explicitly excluded files) and those
// whose services: map is empty or absent (e.g. base.yaml, compose.yaml).
// Results are sorted by stem for deterministic ordering.
func discoverComposeStacks(dir string, skip map[string]bool) ([]composeStackFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading compose directory %s: %w", dir, err)
	}

	var stacks []composeStackFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		if skip[name] {
			continue
		}

		services, err := composeFileServices(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		if len(services) == 0 {
			continue
		}

		stem := strings.TrimSuffix(strings.TrimSuffix(name, ".yaml"), ".yml")
		stacks = append(stacks, composeStackFile{Stem: stem, File: name, Services: services})
	}

	sort.Slice(stacks, func(i, j int) bool { return stacks[i].Stem < stacks[j].Stem })
	return stacks, nil
}
