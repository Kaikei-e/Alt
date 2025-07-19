package domain

import (
	"fmt"
	"time"
)

// SecretType represents the type of secret
type SecretType string

const (
	DatabaseSecret SecretType = "Opaque"
	SSLSecret      SecretType = "kubernetes.io/tls"
	APISecret      SecretType = "Opaque"
)

// Secret represents a Kubernetes secret
type Secret struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Type        string            `json:"type"`
	Data        map[string]string `json:"data"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// NewSecret creates a new secret
func NewSecret(name, namespace string, secretType SecretType) *Secret {
	return &Secret{
		Name:        name,
		Namespace:   namespace,
		Type:        string(secretType),
		Data:        make(map[string]string),
		Labels:      make(map[string]string),
		Annotations: make(map[string]string),
	}
}

// AddData adds data to the secret
func (s *Secret) AddData(key, value string) {
	s.Data[key] = value
}

// AddStandardLabels adds standard management labels to the secret
func (s *Secret) AddStandardLabels(chartName, component string) {
	if s.Labels == nil {
		s.Labels = make(map[string]string)
	}

	s.Labels["app.kubernetes.io/managed-by"] = "deploy-cli"
	s.Labels["app.kubernetes.io/name"] = chartName
	s.Labels["app.kubernetes.io/component"] = component
	s.Labels["app.kubernetes.io/part-of"] = "alt"
	s.Labels["deploy-cli/auto-generated"] = "true"
}

// AddStandardAnnotations adds standard management annotations to the secret
func (s *Secret) AddStandardAnnotations() {
	if s.Annotations == nil {
		s.Annotations = make(map[string]string)
	}

	s.Annotations["deploy-cli/created-at"] = time.Now().Format(time.RFC3339)
	s.Annotations["deploy-cli/version"] = "v1.0.0"
}

// GetData returns the data for the given key
func (s *Secret) GetData(key string) (string, bool) {
	value, exists := s.Data[key]
	return value, exists
}

// DatabaseSecretConfig represents database secret configuration
type DatabaseSecretConfig struct {
	Name      string
	Username  string
	Password  string
	KeyName   string
	Namespace string
}

// NewDatabaseSecretConfig creates a new database secret configuration
func NewDatabaseSecretConfig(name, username, password, keyName, namespace string) *DatabaseSecretConfig {
	return &DatabaseSecretConfig{
		Name:      name,
		Username:  username,
		Password:  password,
		KeyName:   keyName,
		Namespace: namespace,
	}
}

// GetDefaultDatabaseConfigs returns default database secret configurations
func GetDefaultDatabaseConfigs() []DatabaseSecretConfig {
	return []DatabaseSecretConfig{
		{
			Name:      "postgres-secrets",
			Username:  "alt_db_user",
			Password:  "ProductionPassword123",
			KeyName:   "postgres-password",
			Namespace: "alt-database",
		},
		{
			Name:      "auth-postgres-secrets",
			Username:  "auth_db_user",
			Password:  "AuthProdPassword456",
			KeyName:   "postgres-password",
			Namespace: "alt-database",
		},
		{
			Name:      "kratos-postgres-secrets",
			Username:  "kratos_db_user",
			Password:  "KratosProdPassword789",
			KeyName:   "postgres-password",
			Namespace: "alt-database",
		},
		{
			Name:      "clickhouse-secrets",
			Username:  "clickhouse_user",
			Password:  "analytics_secure_password",
			KeyName:   "clickhouse-password",
			Namespace: "alt-database",
		},
	}
}

// GetMeiliSearchSecretConfig returns MeiliSearch secret configuration
func GetMeiliSearchSecretConfig() DatabaseSecretConfig {
	return DatabaseSecretConfig{
		Name:      "meilisearch-secrets",
		Username:  "",
		Password:  "",
		KeyName:   "master-key",
		Namespace: "alt-search",
	}
}

// SSLSecretConfig represents SSL secret configuration
type SSLSecretConfig struct {
	Name        string
	Namespace   string
	CertFile    string
	KeyFile     string
	CAFile      string
	ServiceName string
}

// NewSSLSecretConfig creates a new SSL secret configuration
func NewSSLSecretConfig(serviceName, namespace, certFile, keyFile, caFile string) *SSLSecretConfig {
	return &SSLSecretConfig{
		Name:        fmt.Sprintf("%s-ssl-certs-prod", serviceName),
		Namespace:   namespace,
		CertFile:    certFile,
		KeyFile:     keyFile,
		CAFile:      caFile,
		ServiceName: serviceName,
	}
}

// GetDefaultSSLConfigs returns default SSL secret configurations
func GetDefaultSSLConfigs(sslDir string) []SSLSecretConfig {
	return []SSLSecretConfig{
		{
			Name:        "postgres-ssl-certs-prod",
			Namespace:   "alt-database",
			CertFile:    fmt.Sprintf("%s/postgres.crt", sslDir),
			KeyFile:     fmt.Sprintf("%s/postgres.key", sslDir),
			CAFile:      fmt.Sprintf("%s/ca.crt", sslDir),
			ServiceName: "postgres",
		},
		{
			Name:        "auth-postgres-ssl-certs-prod",
			Namespace:   "alt-database",
			CertFile:    fmt.Sprintf("%s/auth-postgres.crt", sslDir),
			KeyFile:     fmt.Sprintf("%s/auth-postgres.key", sslDir),
			CAFile:      fmt.Sprintf("%s/ca.crt", sslDir),
			ServiceName: "auth-postgres",
		},
		{
			Name:        "kratos-postgres-ssl-certs-prod",
			Namespace:   "alt-database",
			CertFile:    fmt.Sprintf("%s/kratos-postgres.crt", sslDir),
			KeyFile:     fmt.Sprintf("%s/kratos-postgres.key", sslDir),
			CAFile:      fmt.Sprintf("%s/ca.crt", sslDir),
			ServiceName: "kratos-postgres",
		},
		{
			Name:        "clickhouse-ssl-certs-prod",
			Namespace:   "alt-database",
			CertFile:    fmt.Sprintf("%s/clickhouse.crt", sslDir),
			KeyFile:     fmt.Sprintf("%s/clickhouse.key", sslDir),
			CAFile:      fmt.Sprintf("%s/ca.crt", sslDir),
			ServiceName: "clickhouse",
		},
		{
			Name:        "meilisearch-ssl-certs-prod",
			Namespace:   "alt-search",
			CertFile:    fmt.Sprintf("%s/meilisearch.crt", sslDir),
			KeyFile:     fmt.Sprintf("%s/meilisearch.key", sslDir),
			CAFile:      fmt.Sprintf("%s/ca.crt", sslDir),
			ServiceName: "meilisearch",
		},
		{
			Name:        "alt-backend-ssl-certs-prod",
			Namespace:   "alt-apps",
			CertFile:    fmt.Sprintf("%s/alt-backend.crt", sslDir),
			KeyFile:     fmt.Sprintf("%s/alt-backend.key", sslDir),
			CAFile:      fmt.Sprintf("%s/ca.crt", sslDir),
			ServiceName: "alt-backend",
		},
		{
			Name:        "alt-frontend-ssl-certs-prod",
			Namespace:   "alt-apps",
			CertFile:    fmt.Sprintf("%s/alt-frontend.crt", sslDir),
			KeyFile:     fmt.Sprintf("%s/alt-frontend.key", sslDir),
			CAFile:      fmt.Sprintf("%s/ca.crt", sslDir),
			ServiceName: "alt-frontend",
		},
		{
			Name:        "kratos-ssl-certs-prod",
			Namespace:   "alt-auth",
			CertFile:    fmt.Sprintf("%s/kratos.crt", sslDir),
			KeyFile:     fmt.Sprintf("%s/kratos.key", sslDir),
			CAFile:      fmt.Sprintf("%s/ca.crt", sslDir),
			ServiceName: "kratos",
		},
		{
			Name:        "nginx-ssl-certs-prod",
			Namespace:   "alt-ingress",
			CertFile:    fmt.Sprintf("%s/nginx.crt", sslDir),
			KeyFile:     fmt.Sprintf("%s/nginx.key", sslDir),
			CAFile:      fmt.Sprintf("%s/ca.crt", sslDir),
			ServiceName: "nginx",
		},
	}
}

// SecretValidationResult represents the result of secret validation
type SecretValidationResult struct {
	Environment Environment      `json:"environment"`
	Conflicts   []SecretConflict `json:"conflicts"`
	Warnings    []string         `json:"warnings"`
	Valid       bool             `json:"valid"`
}

// SecretConflict represents a secret ownership or distribution conflict
type SecretConflict struct {
	ResourceType     string       `json:"resource_type,omitempty"` // Added to support all resource types
	SecretName       string       `json:"secret_name"`
	SecretNamespace  string       `json:"secret_namespace"`
	ReleaseName      string       `json:"release_name"`
	ReleaseNamespace string       `json:"release_namespace"`
	ConflictType     ConflictType `json:"conflict_type"`
	Description      string       `json:"description"`
}

// ConflictType represents the type of secret conflict
type ConflictType string

const (
	// ConflictTypeCrossNamespace indicates a secret owned by a release in a different namespace
	ConflictTypeCrossNamespace ConflictType = "cross_namespace"
	// ConflictTypeDuplicateOwnership indicates multiple releases claiming ownership
	ConflictTypeDuplicateOwnership ConflictType = "duplicate_ownership"
	// ConflictTypeMissingSecret indicates an expected secret is missing
	ConflictTypeMissingSecret ConflictType = "missing_secret"
	// ConflictTypeMetadataConflict indicates Helm metadata annotation conflicts
	ConflictTypeMetadataConflict ConflictType = "metadata_conflict"
	// ConflictTypeResourceConflict indicates Kubernetes resource metadata conflicts
	ConflictTypeResourceConflict ConflictType = "resource_conflict"
)

// String returns the string representation of ConflictType
func (c ConflictType) String() string {
	return string(c)
}

// SecretDistribution defines how secrets should be distributed across namespaces
type SecretDistribution struct {
	SecretName  string   `json:"secret_name"`
	Namespaces  []string `json:"namespaces"`
	Required    bool     `json:"required"`
	Description string   `json:"description"`
}

// SecretInfo represents information about a Kubernetes secret
type SecretInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Owner     string `json:"owner,omitempty"`
	Type      string `json:"type,omitempty"`
	Age       string `json:"age,omitempty"`
}

// SecretDistributionValidation represents the validation result for secret distribution
type SecretDistributionValidation struct {
	Environment     Environment `json:"environment"`
	TotalSecrets    int         `json:"total_secrets"`
	ValidSecrets    int         `json:"valid_secrets"`
	MissingSecrets  []string    `json:"missing_secrets"`
	ConflictSecrets []string    `json:"conflict_secrets"`
	Issues          []string    `json:"issues"`
	IsValid         bool        `json:"is_valid"`
}

// SecretConfig represents secret configuration for an environment
type SecretConfig struct {
	Environment   Environment          `json:"environment"`
	Distributions []SecretDistribution `json:"distributions"`
}

// NewSecretConfig creates a new secret configuration for the given environment
func NewSecretConfig(environment Environment) *SecretConfig {
	return &SecretConfig{
		Environment:   environment,
		Distributions: getDefaultSecretDistributions(environment),
	}
}

// getDefaultSecretDistributions returns default secret distributions for environment
func getDefaultSecretDistributions(environment Environment) []SecretDistribution {
	switch environment {
	case Production:
		return []SecretDistribution{
			{
				SecretName:  "huggingface-secret",
				Namespaces:  []string{"alt-auth", "alt-apps"},
				Required:    true,
				Description: "Hugging Face API token for ML services",
			},
			{
				SecretName:  "meilisearch-secrets",
				Namespaces:  []string{"alt-search"},
				Required:    true,
				Description: "Meilisearch API keys and master key",
			},
			{
				SecretName:  "postgres-secrets",
				Namespaces:  []string{"alt-database"},
				Required:    true,
				Description: "PostgreSQL database credentials",
			},
			{
				SecretName:  "auth-postgres-secrets",
				Namespaces:  []string{"alt-database"},
				Required:    true,
				Description: "Auth service PostgreSQL credentials",
			},
			{
				SecretName:  "auth-service-secrets",
				Namespaces:  []string{"alt-auth"},
				Required:    true,
				Description: "Auth service configuration secrets",
			},
			{
				SecretName:  "backend-secrets",
				Namespaces:  []string{"alt-apps"},
				Required:    true,
				Description: "Backend service configuration secrets",
			},
			{
				SecretName:  "clickhouse-secrets",
				Namespaces:  []string{"alt-database"},
				Required:    true,
				Description: "ClickHouse database credentials",
			},
		}
	case Staging:
		return []SecretDistribution{
			{
				SecretName:  "huggingface-secret",
				Namespaces:  []string{"alt-staging"},
				Required:    true,
				Description: "Hugging Face API token for staging",
			},
			{
				SecretName:  "meilisearch-secrets",
				Namespaces:  []string{"alt-staging"},
				Required:    true,
				Description: "Meilisearch secrets for staging",
			},
			{
				SecretName:  "postgres-secrets",
				Namespaces:  []string{"alt-staging"},
				Required:    true,
				Description: "PostgreSQL secrets for staging",
			},
		}
	case Development:
		return []SecretDistribution{
			{
				SecretName:  "huggingface-secret",
				Namespaces:  []string{"alt-dev"},
				Required:    false,
				Description: "Hugging Face API token for development",
			},
			{
				SecretName:  "meilisearch-secrets",
				Namespaces:  []string{"alt-dev"},
				Required:    true,
				Description: "Meilisearch secrets for development",
			},
			{
				SecretName:  "postgres-secrets",
				Namespaces:  []string{"alt-dev"},
				Required:    true,
				Description: "PostgreSQL secrets for development",
			},
		}
	default:
		return []SecretDistribution{}
	}
}
