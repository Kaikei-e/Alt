package domain

// ServiceDependency represents a dependency between services
type ServiceDependency struct {
	ServiceName string `json:"service_name"`
	ServiceType string `json:"service_type"`
	Namespace   string `json:"namespace"`
	Required    bool   `json:"required"`
	Timeout     int    `json:"timeout"` // timeout in seconds
}

// ServiceDependencies defines the runtime dependencies between services
// This is separate from Helm chart dependencies and focuses on actual service communication
var ServiceDependencies = map[string][]ServiceDependency{
	// Migration service needs PostgreSQL to be ready
	"migrate": {
		{
			ServiceName: "postgres",
			ServiceType: "postgresql",
			Namespace:   "alt-database",
			Required:    true,
			Timeout:     300, // 5 minutes
		},
	},

	// Alt backend needs both PostgreSQL and Meilisearch
	"alt-backend": {
		{
			ServiceName: "postgres",
			ServiceType: "postgresql",
			Namespace:   "alt-database",
			Required:    true,
			Timeout:     300,
		},
		{
			ServiceName: "meilisearch",
			ServiceType: "meilisearch",
			Namespace:   "alt-search",
			Required:    true,
			Timeout:     300,
		},
	},

	// Auth service needs Auth PostgreSQL
	"auth-service": {
		{
			ServiceName: "auth-postgres",
			ServiceType: "postgresql",
			Namespace:   "alt-database",
			Required:    true,
			Timeout:     300,
		},
	},

	// Kratos needs Kratos PostgreSQL
	"kratos": {
		{
			ServiceName: "kratos-postgres",
			ServiceType: "postgresql",
			Namespace:   "alt-database",
			Required:    true,
			Timeout:     300,
		},
	},

	// Search indexer needs both PostgreSQL and Meilisearch
	"search-indexer": {
		{
			ServiceName: "postgres",
			ServiceType: "postgresql",
			Namespace:   "alt-database",
			Required:    true,
			Timeout:     300,
		},
		{
			ServiceName: "meilisearch",
			ServiceType: "meilisearch",
			Namespace:   "alt-search",
			Required:    true,
			Timeout:     300,
		},
	},

	// Pre-processor needs PostgreSQL and News Creator
	"pre-processor": {
		{
			ServiceName: "postgres",
			ServiceType: "postgresql",
			Namespace:   "alt-database",
			Required:    true,
			Timeout:     300,
		},
		{
			ServiceName: "news-creator",
			ServiceType: "service",
			Namespace:   "alt-apps",
			Required:    false, // Optional dependency
			Timeout:     180,
		},
	},

	// Tag generator needs PostgreSQL
	"tag-generator": {
		{
			ServiceName: "postgres",
			ServiceType: "postgresql",
			Namespace:   "alt-database",
			Required:    true,
			Timeout:     300,
		},
	},

	// Rask log aggregator needs ClickHouse
	"rask-log-aggregator": {
		{
			ServiceName: "clickhouse",
			ServiceType: "clickhouse",
			Namespace:   "alt-database",
			Required:    true,
			Timeout:     300,
		},
	},

	// Alt frontend depends on backend services for API calls
	"alt-frontend": {
		{
			ServiceName: "alt-backend",
			ServiceType: "application",
			Namespace:   "alt-apps",
			Required:    true,
			Timeout:     180,
		},
		{
			ServiceName: "auth-service",
			ServiceType: "application",
			Namespace:   "alt-auth",
			Required:    true,
			Timeout:     180,
		},
	},

	// Infrastructure services typically don't have dependencies
	"postgres":        {},
	"auth-postgres":   {},
	"kratos-postgres": {},
	"clickhouse":      {},
	"meilisearch":     {},
	"nginx":           {},
	"nginx-external":  {},
	"monitoring":      {},
	"common-ssl":      {},
	"common-secrets":  {},
	"common-config":   {},
	"backup":          {},
	"news-creator":    {},
}

// GetServiceDependencies returns the dependencies for a given service
func GetServiceDependencies(serviceName string) []ServiceDependency {
	if deps, exists := ServiceDependencies[serviceName]; exists {
		return deps
	}
	return []ServiceDependency{}
}

// HasDependencies checks if a service has any dependencies
func HasDependencies(serviceName string) bool {
	deps := GetServiceDependencies(serviceName)
	return len(deps) > 0
}

// GetRequiredDependencies returns only the required dependencies
func GetRequiredDependencies(serviceName string) []ServiceDependency {
	deps := GetServiceDependencies(serviceName)
	var required []ServiceDependency

	for _, dep := range deps {
		if dep.Required {
			required = append(required, dep)
		}
	}

	return required
}

// GetOptionalDependencies returns only the optional dependencies
func GetOptionalDependencies(serviceName string) []ServiceDependency {
	deps := GetServiceDependencies(serviceName)
	var optional []ServiceDependency

	for _, dep := range deps {
		if !dep.Required {
			optional = append(optional, dep)
		}
	}

	return optional
}
