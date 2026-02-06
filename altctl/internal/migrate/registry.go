// Package migrate provides backup and restore functionality for Docker Compose volumes
package migrate

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

// VolumeSpec defines a volume to be backed up
type VolumeSpec struct {
	Name        string     // Docker volume name
	Service     string     // Docker Compose service name
	BackupType  BackupType // Backup strategy
	Description string     // Human-readable description

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

// Get returns a volume by name
func (r *VolumeRegistry) Get(name string) (VolumeSpec, bool) {
	for _, v := range r.volumes {
		if v.Name == name {
			return v, true
		}
	}
	return VolumeSpec{}, false
}

// defaultVolumes contains all Alt platform persistent volumes
var defaultVolumes = []VolumeSpec{
	// PostgreSQL databases (logical backup via pg_dump)
	{
		Name:        "db_data_17",
		Service:     "db",
		BackupType:  BackupTypePostgreSQL,
		Description: "Main application database (PostgreSQL 17)",
		DBName:      "alt",
		DBUser:      "alt_db_user",
		DBPort:      5432,
	},
	{
		Name:        "kratos_db_data",
		Service:     "kratos-db",
		BackupType:  BackupTypePostgreSQL,
		Description: "Kratos identity database (PostgreSQL 16)",
		DBName:      "kratos",
		DBUser:      "kratos_user",
		DBPort:      5432,
	},
	{
		Name:        "recap_db_data",
		Service:     "recap-db",
		BackupType:  BackupTypePostgreSQL,
		Description: "Recap worker database (PostgreSQL 18)",
		DBName:      "recap",
		DBUser:      "recap_user",
		DBPort:      5432,
	},
	{
		Name:        "rag_db_data",
		Service:     "rag-db",
		BackupType:  BackupTypePostgreSQL,
		Description: "RAG orchestrator database (PostgreSQL)",
		DBName:      "rag_db",
		DBUser:      "rag_user",
		DBPort:      5432,
	},
	// Other volumes
	{
		Name:        "meili_data",
		Service:     "meilisearch",
		BackupType:  BackupTypeTar,
		Description: "Meilisearch search index data",
	},
	{
		Name:        "clickhouse_data",
		Service:     "clickhouse",
		BackupType:  BackupTypeTar,
		Description: "ClickHouse analytics database",
	},
	{
		Name:        "news_creator_models",
		Service:     "news-creator",
		BackupType:  BackupTypeTar,
		Description: "Ollama LLM models",
	},
	{
		Name:        "rask_log_aggregator_data",
		Service:     "rask-log-aggregator",
		BackupType:  BackupTypeTar,
		Description: "Rask log aggregator data",
	},
	{
		Name:        "oauth_token_data",
		Service:     "auth-token-manager",
		BackupType:  BackupTypeTar,
		Description: "OAuth2 token storage",
	},
	{
		Name:        "redis-streams-data",
		Service:     "redis-streams",
		BackupType:  BackupTypeTar,
		Description: "Redis Streams message queue data",
	},
	{
		Name:        "prometheus_data",
		Service:     "prometheus",
		BackupType:  BackupTypeTar,
		Description: "Prometheus monitoring data",
	},
	{
		Name:        "grafana_data",
		Service:     "grafana",
		BackupType:  BackupTypeTar,
		Description: "Grafana dashboards and configuration",
	},
}
