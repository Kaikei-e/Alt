package stack

import (
	"sort"
	"time"
)

// Registry holds all available stack definitions
type Registry struct {
	stacks map[string]*Stack
}

// defaultStacks contains the predefined stack configurations
var defaultStacks = []Stack{
	{
		Name:        "base",
		Description: "Shared resources (secrets, networks, volumes)",
		ComposeFile: "base.yaml",
		Services:    []string{}, // No services, only shared resources
		DependsOn:   []string{},
		Optional:    false,
	},
	{
		Name:        "db",
		Description: "Database services (PostgreSQL 17, Meilisearch, ClickHouse)",
		ComposeFile: "db.yaml",
		Services:    []string{"db", "meilisearch", "clickhouse"},
		DependsOn:   []string{"base"},
		Optional:    false,
		Provides:    []Feature{FeatureDatabase},
	},
	{
		Name:        "auth",
		Description: "Authentication services (Kratos, auth-hub)",
		ComposeFile: "auth.yaml",
		Services:    []string{"kratos-db", "kratos-migrate", "kratos", "auth-hub"},
		DependsOn:   []string{"base"},
		Optional:    false,
		Provides:    []Feature{FeatureAuth},
	},
	{
		Name:             "core",
		Description:      "Core application services (nginx, frontend, backend)",
		ComposeFile:      "core.yaml",
		Services:         []string{"nginx", "alt-frontend", "alt-frontend-sv", "alt-backend", "migrate"},
		DependsOn:        []string{"base", "db", "auth"},
		Optional:         false,
		RequiresFeatures: []Feature{FeatureSearch}, // Search UI requires search-indexer from workers stack
	},
	{
		Name:        "ai",
		Description: "AI/LLM services (Ollama, news-creator, pre-processor)",
		ComposeFile: "ai.yaml",
		Services:    []string{"news-creator", "news-creator-volume-init", "pre-processor"},
		DependsOn:   []string{"base", "db", "core"},
		Profile:     "ollama",
		RequiresGPU: true,
		Optional:    true,
		Timeout:     10 * time.Minute, // GPU services need more time
		Provides:    []Feature{FeatureAI},
	},
	{
		Name:        "workers",
		Description: "Background worker services",
		ComposeFile: "workers.yaml",
		Services:    []string{"pre-processor-sidecar", "search-indexer", "tag-generator", "auth-token-manager"},
		DependsOn:   []string{"base", "db", "core"},
		Optional:    false,
		Provides:    []Feature{FeatureSearch}, // search-indexer provides search functionality
	},
	{
		Name:        "recap",
		Description: "Recap services (article summarization)",
		ComposeFile: "recap.yaml",
		Services:    []string{"recap-db", "recap-db-migrator", "recap-worker", "recap-subworker", "dashboard", "recap-evaluator"},
		DependsOn:   []string{"base", "db", "core"},
		Profile:     "recap",
		Optional:    true,
		Provides:    []Feature{FeatureRecap},
	},
	{
		Name:        "logging",
		Description: "Logging infrastructure (rask log forwarders)",
		ComposeFile: "logging.yaml",
		Services: []string{
			"rask-log-aggregator",
			"nginx-logs", "alt-backend-logs", "tag-generator-logs",
			"pre-processor-logs", "search-indexer-logs", "news-creator-logs",
			"meilisearch-logs", "db-logs",
		},
		DependsOn: []string{"base", "db"},
		Profile:   "logging",
		Optional:  true,
		Provides:  []Feature{FeatureLogging},
	},
	{
		Name:        "rag",
		Description: "RAG extension services",
		ComposeFile: "rag.yaml",
		Services:    []string{"rag-db", "rag-db-migrator", "rag-orchestrator"},
		DependsOn:   []string{"base", "db", "core", "workers"},
		Profile:     "rag-extension",
		Optional:    true,
		Provides:    []Feature{FeatureRAG},
	},
	{
		Name:        "perf",
		Description: "E2E performance measurement tool (Deno/Astral)",
		ComposeFile: "perf.yaml",
		Services:    []string{"alt-perf"},
		DependsOn:   []string{"base", "db", "auth", "core"},
		Profile:     "perf",
		Optional:    true,
	},
	{
		Name:        "dev",
		Description: "Development stack (SvelteKit + mock-auth + backend + db)",
		ComposeFile: "dev.yaml",
		Services:    []string{"mock-auth", "alt-frontend-sv", "alt-backend", "db", "migrate"},
		DependsOn:   []string{"base"},
		Profile:     "dev",
		Optional:    true,
	},
	{
		Name:        "frontend-dev",
		Description: "Frontend-only development (mock backend, no database)",
		ComposeFile: "frontend-dev.yaml",
		Services:    []string{"mock-auth", "alt-frontend-sv"},
		DependsOn:   []string{}, // No dependencies - standalone
		Profile:     "frontend-dev",
		Optional:    true,
	},
}

// NewRegistry creates a new stack registry with default stacks
func NewRegistry() *Registry {
	r := &Registry{
		stacks: make(map[string]*Stack),
	}
	for i := range defaultStacks {
		r.stacks[defaultStacks[i].Name] = &defaultStacks[i]
	}
	return r
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

// FindByService returns the stack containing the given service
func (r *Registry) FindByService(service string) *Stack {
	for _, s := range r.stacks {
		for _, svc := range s.Services {
			if svc == service {
				return s
			}
		}
	}
	return nil
}

// Register adds or updates a stack in the registry
func (r *Registry) Register(s *Stack) {
	r.stacks[s.Name] = s
}
