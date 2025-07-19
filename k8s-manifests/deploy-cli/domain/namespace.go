package domain

import "fmt"

// Namespace represents a Kubernetes namespace
type Namespace struct {
	Name        string
	Environment Environment
}

// NewNamespace creates a new namespace
func NewNamespace(name string, env Environment) *Namespace {
	return &Namespace{
		Name:        name,
		Environment: env,
	}
}

// String returns the string representation of the namespace
func (n *Namespace) String() string {
	return n.Name
}

// DetermineNamespace determines the namespace for a chart in the given environment
func DetermineNamespace(chartName string, env Environment) string {
	switch env {
	case Development:
		return "alt-dev"
	case Staging:
		return "alt-staging"
	case Production:
		return determineProductionNamespace(chartName)
	default:
		return fmt.Sprintf("alt-%s", env)
	}
}

// determineProductionNamespace determines the production namespace for a chart
func determineProductionNamespace(chartName string) string {
	switch chartName {
	case "alt-backend", "alt-frontend", "pre-processor", "search-indexer", "tag-generator", "news-creator", "rask-log-aggregator":
		return "alt-apps"
	case "postgres", "auth-postgres", "kratos-postgres", "clickhouse":
		return "alt-database"
	case "meilisearch":
		return "alt-search"
	case "auth-service", "kratos":
		return "alt-auth"
	case "nginx", "nginx-external":
		return "alt-ingress"
	case "monitoring":
		return "alt-observability"
	case "common-secrets", "common-config", "common-ssl":
		return "alt-apps" // Deploy common charts to alt-apps to match service deployments
	case "migrate":
		return "alt-database" // Deploy migrate to alt-database to access postgres secrets
	case "backup":
		return "alt-database" // Deploy backup to alt-database for database access
	default:
		return "alt-production"
	}
}

// GetProductionNamespaces returns all production namespaces
func GetProductionNamespaces() []string {
	return []string{
		"alt-apps",
		"alt-database",
		"alt-search",
		"alt-auth",
		"alt-ingress",
		"alt-observability",
		"alt-production",
	}
}

// GetNamespacesForEnvironment returns all namespaces for the given environment
func GetNamespacesForEnvironment(env Environment) []string {
	switch env {
	case Development:
		return []string{"alt-dev"}
	case Staging:
		return []string{"alt-staging"}
	case Production:
		return GetProductionNamespaces()
	default:
		return []string{fmt.Sprintf("alt-%s", env)}
	}
}
