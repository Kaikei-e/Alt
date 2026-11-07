package mtlsedge

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"gopkg.in/yaml.v3"
)

var passthroughOrder = []string{
	"DOCKER_HOST",
	"DOCKER_CONTEXT",
	"DOCKER_CONFIG",
	"PATH",
	"HOME",
}

func isAllowedPassthrough(name string) bool {
	return name == "COMPOSE_PROJECT_NAME" || slices.Contains(passthroughOrder, name)
}

type composeRefFile struct {
	Include  []yaml.Node          `yaml:"include"`
	Services map[string]yaml.Node `yaml:"services"`
}

// CollectComposeReferencedVars walks composeRootFile and all compose files reachable
// through include:, scanning every YAML scalar node for compose variable interpolations.
func CollectComposeReferencedVars(composeRootFile string) ([]string, error) {
	absRoot, err := filepath.Abs(composeRootFile)
	if err != nil {
		return nil, fmt.Errorf("resolve compose root path: %w", err)
	}

	visited := make(map[string]bool)
	vars := make(map[string]bool)
	queue := []string{absRoot}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if visited[cur] {
			continue
		}
		visited[cur] = true

		raw, err := os.ReadFile(cur)
		if err != nil {
			return nil, fmt.Errorf("read compose file %s: %w", filepath.Base(cur), err)
		}

		var root yaml.Node
		if err := yaml.Unmarshal(raw, &root); err != nil {
			return nil, fmt.Errorf("parse compose file %s: %w", filepath.Base(cur), err)
		}
		walkScalarVars(&root, vars)

		var doc composeRefFile
		if err := root.Decode(&doc); err != nil {
			return nil, fmt.Errorf("decode compose references %s: %w", filepath.Base(cur), err)
		}

		baseDir := filepath.Dir(cur)
		for _, node := range doc.Include {
			paths, err := extractIncludePaths(node)
			if err != nil {
				return nil, fmt.Errorf("invalid include in %s: %w", filepath.Base(cur), err)
			}
			for _, rel := range paths {
				next := filepath.Clean(filepath.Join(baseDir, rel))
				if !visited[next] {
					queue = append(queue, next)
				}
			}
		}
		for _, svcNode := range doc.Services {
			if hasExtendsFile(svcNode) {
				return nil, fmt.Errorf("extends.file is not supported in %s", filepath.Base(cur))
			}
		}
	}

	out := make([]string, 0, len(vars))
	for k := range vars {
		out = append(out, k)
	}
	sort.Strings(out)
	return out, nil
}

func walkScalarVars(n *yaml.Node, vars map[string]bool) {
	if n == nil {
		return
	}
	if n.Kind == yaml.ScalarNode {
		scanScalarComposeVars(n.Value, vars)
	}
	for _, child := range n.Content {
		walkScalarVars(child, vars)
	}
}

func scanScalarComposeVars(s string, out map[string]bool) {
	for i := 0; i < len(s); {
		if s[i] != '$' {
			i++
			continue
		}
		if i+1 < len(s) && s[i+1] == '$' {
			i += 2
			continue
		}
		if i+1 < len(s) && s[i+1] == '{' {
			if i+2 < len(s) && isVarStart(s[i+2]) {
				end := i + 3
				for end < len(s) && isVarChar(s[end]) {
					end++
				}
				out[s[i+2:end]] = true
				i = end
				continue
			}
			i += 2
			continue
		}
		if i+1 < len(s) && isVarStart(s[i+1]) {
			end := i + 2
			for end < len(s) && isVarChar(s[end]) {
				end++
			}
			out[s[i+1:end]] = true
			i = end
			continue
		}
		i++
	}
}

func isVarStart(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || b == '_'
}

func isVarChar(b byte) bool {
	return isVarStart(b) || (b >= '0' && b <= '9')
}

func hasExtendsFile(svcNode yaml.Node) bool {
	if svcNode.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(svcNode.Content); i += 2 {
		kNode, vNode := svcNode.Content[i], svcNode.Content[i+1]
		if kNode.Kind != yaml.ScalarNode || kNode.Value != "extends" {
			continue
		}
		if vNode.Kind != yaml.MappingNode {
			return false
		}
		for j := 0; j+1 < len(vNode.Content); j += 2 {
			if vNode.Content[j].Kind == yaml.ScalarNode && vNode.Content[j].Value == "file" {
				return true
			}
		}
	}
	return false
}

func extractIncludePaths(node yaml.Node) ([]string, error) {
	switch node.Kind {
	case yaml.ScalarNode:
		if node.Value != "" {
			return []string{node.Value}, nil
		}
	case yaml.MappingNode:
		var m struct {
			Path yaml.Node `yaml:"path"`
		}
		if err := node.Decode(&m); err != nil {
			return nil, err
		}
		switch m.Path.Kind {
		case yaml.ScalarNode:
			if m.Path.Value != "" {
				return []string{m.Path.Value}, nil
			}
		case yaml.SequenceNode:
			var paths []string
			if err := m.Path.Decode(&paths); err != nil {
				return nil, err
			}
			for _, p := range paths {
				if p == "" {
					return nil, fmt.Errorf("empty include path")
				}
			}
			if len(paths) > 0 {
				return paths, nil
			}
		}
	}
	return nil, fmt.Errorf("invalid include entry")
}

// FreezeEnvironment performs the environment freeze:
//   - Fails closed with shell_env_override:<NAME> reasons if any variable referenced
//     in reachable compose files is set in parentEnviron (except the 6 allowed passthrough names).
//   - Constructs the frozen child environment containing only DOCKER_HOST,
//     DOCKER_CONTEXT, DOCKER_CONFIG, PATH, HOME (when present) and COMPOSE_PROJECT_NAME=alt.
func FreezeEnvironment(composeRootFile string, parentEnviron []string) (frozenEnv []string, overrideReasons []string, err error) {
	referenced, err := CollectComposeReferencedVars(composeRootFile)
	if err != nil {
		return nil, nil, err
	}

	parentMap := parseEnvSlice(parentEnviron)

	overrideReasons = make([]string, 0)
	for _, varName := range referenced {
		if isAllowedPassthrough(varName) {
			continue
		}
		if _, ok := parentMap[varName]; ok {
			overrideReasons = append(overrideReasons, "shell_env_override:"+varName)
		}
	}

	frozenEnv = make([]string, 0, len(passthroughOrder)+1)
	for _, key := range passthroughOrder {
		if v, ok := parentMap[key]; ok {
			frozenEnv = append(frozenEnv, key+"="+v)
		}
	}
	frozenEnv = append(frozenEnv, "COMPOSE_PROJECT_NAME="+ComposeProjectName)

	return frozenEnv, overrideReasons, nil
}
