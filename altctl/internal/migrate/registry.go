// Package migrate provides backup and restore functionality for Docker Compose volumes
package migrate

import "strings"

// BackupType represents the backup strategy for a volume
type BackupType int

const (
	// BackupTypePostgreSQL uses pg_dump for logical backup
	BackupTypePostgreSQL BackupType = iota
	// BackupTypeTar uses tar for raw volume backup
	BackupTypeTar
)

func (t BackupType) String() string {
	switch t {
	case BackupTypePostgreSQL:
		return "postgresql"
	case BackupTypeTar:
		return "tar"
	default:
		return "unknown"
	}
}

// VolumeCategory classifies volumes by data importance and recoverability
type VolumeCategory int

const (
	// CategoryCritical is for PostgreSQL databases — data loss is unacceptable
	CategoryCritical VolumeCategory = iota + 1
	// CategoryData is for operational data that would cause impact if lost
	CategoryData
	// CategorySearch is for search indexes that can be rebuilt
	CategorySearch
	// CategoryMetrics is for monitoring data that can be re-collected
	CategoryMetrics
	// CategoryModels is for ML models that can be re-downloaded
	CategoryModels
)

func (c VolumeCategory) String() string {
	switch c {
	case CategoryCritical:
		return "critical"
	case CategoryData:
		return "data"
	case CategorySearch:
		return "search"
	case CategoryMetrics:
		return "metrics"
	case CategoryModels:
		return "models"
	default:
		return "unknown"
	}
}

// VolumeSpec defines a volume to be backed up
type VolumeSpec struct {
	Name        string         // Docker volume name
	Service     string         // Docker Compose service name
	BackupType  BackupType     // Backup strategy
	Category    VolumeCategory // Data importance classification
	Description string         // Human-readable description

	// PostgreSQL-specific fields
	DBName string // Database name
	DBUser string // Database user
	DBPort int    // Internal container port (usually 5432)

	// Environment variable names for configuration
	DBNameEnv     string
	DBUserEnv     string
	DBPasswordEnv string
}

// VolumeRegistry holds the list of volumes to backup
type VolumeRegistry struct {
	volumes []VolumeSpec
}

// NewVolumeRegistry creates a new registry with default Alt platform volumes
func NewVolumeRegistry() *VolumeRegistry {
	return &VolumeRegistry{
		volumes: defaultVolumes,
	}
}

// All returns all registered volumes
func (r *VolumeRegistry) All() []VolumeSpec {
	return r.volumes
}

// PostgreSQL returns only PostgreSQL volumes
func (r *VolumeRegistry) PostgreSQL() []VolumeSpec {
	var result []VolumeSpec
	for _, v := range r.volumes {
		if v.BackupType == BackupTypePostgreSQL {
			result = append(result, v)
		}
	}
	return result
}

// Tar returns only tar-based volumes
func (r *VolumeRegistry) Tar() []VolumeSpec {
	var result []VolumeSpec
	for _, v := range r.volumes {
		if v.BackupType == BackupTypeTar {
			result = append(result, v)
		}
	}
	return result
}

// ByCategory returns volumes matching any of the given categories
func (r *VolumeRegistry) ByCategory(cats ...VolumeCategory) []VolumeSpec {
	catSet := make(map[VolumeCategory]bool, len(cats))
	for _, c := range cats {
		catSet[c] = true
	}
	var result []VolumeSpec
	for _, v := range r.volumes {
		if catSet[v.Category] {
			result = append(result, v)
		}
	}
	return result
}

// Get returns a volume by name, with hyphen/underscore normalization fallback
// for backward compatibility with old manifests
func (r *VolumeRegistry) Get(name string) (VolumeSpec, bool) {
	for _, v := range r.volumes {
		if v.Name == name {
			return v, true
		}
	}
	// Fallback: try normalizing hyphens <-> underscores
	normalized := strings.ReplaceAll(name, "_", "-")
	if normalized == name {
		normalized = strings.ReplaceAll(name, "-", "_")
	}
	for _, v := range r.volumes {
		if v.Name == normalized {
			return v, true
		}
	}
	return VolumeSpec{}, false
}

// defaultVolumes contains all Alt platform persistent volumes
var defaultVolumes = []VolumeSpec{
	// PostgreSQL databases — CategoryCritical (logical backup via pg_dump)
	{
		Name:        "db_data_17",
		Service:     "db",
		BackupType:  BackupTypePostgreSQL,
		Category:    CategoryCritical,
		Description: "Main application database (PostgreSQL 17)",
		DBName:      "alt",
		DBUser:      "alt_db_user",
		DBPort:      5432,
	},
	{
		Name:        "kratos_db_data",
		Service:     "kratos-db",
		BackupType:  BackupTypePostgreSQL,
		Category:    CategoryCritical,
		Description: "Kratos identity database (PostgreSQL 16)",
		DBName:      "kratos",
		DBUser:      "kratos_user",
		DBPort:      5432,
	},
	{
		Name:        "recap_db_data",
		Service:     "recap-db",
		BackupType:  BackupTypePostgreSQL,
		Category:    CategoryCritical,
		Description: "Recap worker database (PostgreSQL 18)",
		DBName:      "recap",
		DBUser:      "recap_user",
		DBPort:      5432,
	},
	{
		Name:        "rag_db_data",
		Service:     "rag-db",
		BackupType:  BackupTypePostgreSQL,
		Category:    CategoryCritical,
		Description: "RAG orchestrator database (PostgreSQL)",
		DBName:      "rag_db",
		DBUser:      "rag_user",
		DBPort:      5432,
	},
	{
		Name:        "knowledge-sovereign-db-data",
		Service:     "knowledge-sovereign-db",
		BackupType:  BackupTypePostgreSQL,
		Category:    CategoryCritical,
		Description: "Knowledge Sovereign database (PostgreSQL 16)",
		DBName:      "knowledge_sovereign",
		DBUser:      "sovereign",
		DBPort:      5432,
	},
	{
		Name:        "pre_processor_db_data",
		Service:     "pre-processor-db",
		BackupType:  BackupTypePostgreSQL,
		Category:    CategoryCritical,
		Description: "Pre-processor dedicated database (PostgreSQL 17)",
		DBName:      "pre_processor",
		DBUser:      "pp_user",
		DBPort:      5432,
	},
	// Operational data — CategoryData
	{
		Name:        "oauth_token_data",
		Service:     "auth-token-manager",
		BackupType:  BackupTypeTar,
		Category:    CategoryData,
		Description: "OAuth2 token storage",
	},
	{
		Name:        "redis-streams-data",
		Service:     "redis-streams",
		BackupType:  BackupTypeTar,
		Category:    CategoryData,
		Description: "Redis Streams message queue data",
	},
	{
		Name:        "rask_log_aggregator_data",
		Service:     "rask-log-aggregator",
		BackupType:  BackupTypeTar,
		Category:    CategoryData,
		Description: "Rask log aggregator data",
	},
	// Search index — CategorySearch (rebuildable)
	{
		Name:        "meili_data",
		Service:     "meilisearch",
		BackupType:  BackupTypeTar,
		Category:    CategorySearch,
		Description: "Meilisearch search index data",
	},
	// Metrics — CategoryMetrics (re-collectable)
	{
		Name:        "clickhouse_data",
		Service:     "clickhouse",
		BackupType:  BackupTypeTar,
		Category:    CategoryMetrics,
		Description: "ClickHouse analytics database",
	},
	{
		Name:        "prometheus_data",
		Service:     "prometheus",
		BackupType:  BackupTypeTar,
		Category:    CategoryMetrics,
		Description: "Prometheus monitoring data",
	},
	{
		Name:        "grafana_data",
		Service:     "grafana",
		BackupType:  BackupTypeTar,
		Category:    CategoryMetrics,
		Description: "Grafana dashboards and configuration",
	},
	// Models — CategoryModels (re-downloadable)
	{
		Name:        "news_creator_models",
		Service:     "news-creator",
		BackupType:  BackupTypeTar,
		Category:    CategoryModels,
		Description: "Ollama LLM models",
	},
}
