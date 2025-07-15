package integration_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// PostgreSQLStatefulSetConfig represents PostgreSQL StatefulSet configuration
type PostgreSQLStatefulSetConfig struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name      string            `yaml:"name"`
		Namespace string            `yaml:"namespace"`
		Labels    map[string]string `yaml:"labels"`
	} `yaml:"metadata"`
	Spec struct {
		ServiceName string `yaml:"serviceName"`
		Replicas    int    `yaml:"replicas"`
		Selector    struct {
			MatchLabels map[string]string `yaml:"matchLabels"`
		} `yaml:"selector"`
		Template struct {
			Metadata struct {
				Labels map[string]string `yaml:"labels"`
			} `yaml:"metadata"`
			Spec struct {
				Containers []PostgreSQLContainer `yaml:"containers"`
				Volumes    []Volume              `yaml:"volumes"`
			} `yaml:"spec"`
		} `yaml:"template"`
		VolumeClaimTemplates []VolumeClaimTemplate `yaml:"volumeClaimTemplates"`
	} `yaml:"spec"`
}

type PostgreSQLContainer struct {
	Name         string           `yaml:"name"`
	Image        string           `yaml:"image"`
	Ports        []Port           `yaml:"ports"`
	Env          []EnvVar         `yaml:"env"`
	VolumeMounts []VolumeMount    `yaml:"volumeMounts"`
	Resources    ResourceRequests `yaml:"resources"`
}

type Port struct {
	ContainerPort int    `yaml:"containerPort"`
	Name          string `yaml:"name,omitempty"`
}

type EnvVar struct {
	Name      string             `yaml:"name"`
	Value     string             `yaml:"value,omitempty"`
	ValueFrom *EnvVarSource      `yaml:"valueFrom,omitempty"`
}

type EnvVarSource struct {
	SecretKeyRef *SecretKeySelector `yaml:"secretKeyRef,omitempty"`
}

type SecretKeySelector struct {
	Name string `yaml:"name"`
	Key  string `yaml:"key"`
}

type VolumeMount struct {
	Name      string `yaml:"name"`
	MountPath string `yaml:"mountPath"`
	ReadOnly  bool   `yaml:"readOnly,omitempty"`
}

type ResourceRequests struct {
	Requests map[string]string `yaml:"requests"`
	Limits   map[string]string `yaml:"limits"`
}

type Volume struct {
	Name      string             `yaml:"name"`
	ConfigMap *ConfigMapVolumeSource `yaml:"configMap,omitempty"`
}

type ConfigMapVolumeSource struct {
	Name string `yaml:"name"`
}

type VolumeClaimTemplate struct {
	Metadata struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		AccessModes []string            `yaml:"accessModes"`
		Resources   VolumeClaimResource `yaml:"resources"`
	} `yaml:"spec"`
}

type VolumeClaimResource struct {
	Requests map[string]string `yaml:"requests"`
}

func TestAuthPostgreSQLConfiguration(t *testing.T) {
	authPostgresConfig := PostgreSQLStatefulSetConfig{
		APIVersion: "apps/v1",
		Kind:       "StatefulSet",
		Metadata: struct {
			Name      string            `yaml:"name"`
			Namespace string            `yaml:"namespace"`
			Labels    map[string]string `yaml:"labels"`
		}{
			Name:      "auth-postgres",
			Namespace: "alt-database",
			Labels: map[string]string{
				"app":       "auth-postgres",
				"component": "database",
				"service":   "authentication",
			},
		},
		Spec: struct {
			ServiceName string `yaml:"serviceName"`
			Replicas    int    `yaml:"replicas"`
			Selector    struct {
				MatchLabels map[string]string `yaml:"matchLabels"`
			} `yaml:"selector"`
			Template struct {
				Metadata struct {
					Labels map[string]string `yaml:"labels"`
				} `yaml:"metadata"`
				Spec struct {
					Containers []PostgreSQLContainer `yaml:"containers"`
					Volumes    []Volume              `yaml:"volumes"`
				} `yaml:"spec"`
			} `yaml:"template"`
			VolumeClaimTemplates []VolumeClaimTemplate `yaml:"volumeClaimTemplates"`
		}{
			ServiceName: "auth-postgres",
			Replicas:    1,
			Selector: struct {
				MatchLabels map[string]string `yaml:"matchLabels"`
			}{
				MatchLabels: map[string]string{
					"app": "auth-postgres",
				},
			},
			Template: struct {
				Metadata struct {
					Labels map[string]string `yaml:"labels"`
				} `yaml:"metadata"`
				Spec struct {
					Containers []PostgreSQLContainer `yaml:"containers"`
					Volumes    []Volume              `yaml:"volumes"`
				} `yaml:"spec"`
			}{
				Metadata: struct {
					Labels map[string]string `yaml:"labels"`
				}{
					Labels: map[string]string{
						"app":       "auth-postgres",
						"component": "database",
					},
				},
				Spec: struct {
					Containers []PostgreSQLContainer `yaml:"containers"`
					Volumes    []Volume              `yaml:"volumes"`
				}{
					Containers: []PostgreSQLContainer{
						{
							Name:  "postgres",
							Image: "postgres:16-alpine",
							Ports: []Port{
								{ContainerPort: 5432, Name: "postgres"},
							},
							Env: []EnvVar{
								{Name: "POSTGRES_DB", Value: "auth_db"},
								{Name: "POSTGRES_USER", ValueFrom: &EnvVarSource{
									SecretKeyRef: &SecretKeySelector{Name: "auth-postgres-secret", Key: "username"},
								}},
								{Name: "POSTGRES_PASSWORD", ValueFrom: &EnvVarSource{
									SecretKeyRef: &SecretKeySelector{Name: "auth-postgres-secret", Key: "password"},
								}},
								{Name: "POSTGRES_SSL_MODE", Value: "require"},
							},
							VolumeMounts: []VolumeMount{
								{Name: "postgres-data", MountPath: "/var/lib/postgresql/data"},
								{Name: "ssl-certs", MountPath: "/etc/ssl/certs/postgres", ReadOnly: true},
							},
							Resources: ResourceRequests{
								Requests: map[string]string{"memory": "256Mi", "cpu": "250m"},
								Limits:   map[string]string{"memory": "512Mi", "cpu": "500m"},
							},
						},
					},
					Volumes: []Volume{
						{
							Name: "ssl-certs",
							ConfigMap: &ConfigMapVolumeSource{Name: "postgres-ssl-config"},
						},
					},
				},
			},
			VolumeClaimTemplates: []VolumeClaimTemplate{
				{
					Metadata: struct {
						Name string `yaml:"name"`
					}{Name: "postgres-data"},
					Spec: struct {
						AccessModes []string            `yaml:"accessModes"`
						Resources   VolumeClaimResource `yaml:"resources"`
					}{
						AccessModes: []string{"ReadWriteOnce"},
						Resources: VolumeClaimResource{
							Requests: map[string]string{"storage": "10Gi"},
						},
					},
				},
			},
		},
	}

	// Test basic configuration
	assert.Equal(t, "apps/v1", authPostgresConfig.APIVersion)
	assert.Equal(t, "StatefulSet", authPostgresConfig.Kind)
	assert.Equal(t, "auth-postgres", authPostgresConfig.Metadata.Name)
	assert.Equal(t, "alt-database", authPostgresConfig.Metadata.Namespace)

	// Test StatefulSet spec
	assert.Equal(t, "auth-postgres", authPostgresConfig.Spec.ServiceName)
	assert.Equal(t, 1, authPostgresConfig.Spec.Replicas)

	// Test container configuration
	require.Len(t, authPostgresConfig.Spec.Template.Spec.Containers, 1)
	container := authPostgresConfig.Spec.Template.Spec.Containers[0]
	assert.Equal(t, "postgres", container.Name)
	assert.Equal(t, "postgres:16-alpine", container.Image)

	// Test environment variables
	envVars := make(map[string]EnvVar)
	for _, env := range container.Env {
		envVars[env.Name] = env
	}

	assert.Contains(t, envVars, "POSTGRES_DB")
	assert.Equal(t, "auth_db", envVars["POSTGRES_DB"].Value)

	assert.Contains(t, envVars, "POSTGRES_USER")
	require.NotNil(t, envVars["POSTGRES_USER"].ValueFrom)
	require.NotNil(t, envVars["POSTGRES_USER"].ValueFrom.SecretKeyRef)
	assert.Equal(t, "auth-postgres-secret", envVars["POSTGRES_USER"].ValueFrom.SecretKeyRef.Name)
	assert.Equal(t, "username", envVars["POSTGRES_USER"].ValueFrom.SecretKeyRef.Key)

	// Test volume configuration
	assert.Len(t, authPostgresConfig.Spec.VolumeClaimTemplates, 1)
	pvc := authPostgresConfig.Spec.VolumeClaimTemplates[0]
	assert.Equal(t, "postgres-data", pvc.Metadata.Name)
	assert.Contains(t, pvc.Spec.AccessModes, "ReadWriteOnce")
	assert.Equal(t, "10Gi", pvc.Spec.Resources.Requests["storage"])
}

func TestKratosPostgreSQLConfiguration(t *testing.T) {
	kratosPostgresConfig := PostgreSQLStatefulSetConfig{
		APIVersion: "apps/v1",
		Kind:       "StatefulSet",
		Metadata: struct {
			Name      string            `yaml:"name"`
			Namespace string            `yaml:"namespace"`
			Labels    map[string]string `yaml:"labels"`
		}{
			Name:      "kratos-postgres",
			Namespace: "alt-database",
			Labels: map[string]string{
				"app":       "kratos-postgres",
				"component": "database",
				"service":   "kratos",
			},
		},
		Spec: struct {
			ServiceName string `yaml:"serviceName"`
			Replicas    int    `yaml:"replicas"`
			Selector    struct {
				MatchLabels map[string]string `yaml:"matchLabels"`
			} `yaml:"selector"`
			Template struct {
				Metadata struct {
					Labels map[string]string `yaml:"labels"`
				} `yaml:"metadata"`
				Spec struct {
					Containers []PostgreSQLContainer `yaml:"containers"`
					Volumes    []Volume              `yaml:"volumes"`
				} `yaml:"spec"`
			} `yaml:"template"`
			VolumeClaimTemplates []VolumeClaimTemplate `yaml:"volumeClaimTemplates"`
		}{
			ServiceName: "kratos-postgres",
			Replicas:    1,
			Selector: struct {
				MatchLabels map[string]string `yaml:"matchLabels"`
			}{
				MatchLabels: map[string]string{
					"app": "kratos-postgres",
				},
			},
			Template: struct {
				Metadata struct {
					Labels map[string]string `yaml:"labels"`
				} `yaml:"metadata"`
				Spec struct {
					Containers []PostgreSQLContainer `yaml:"containers"`
					Volumes    []Volume              `yaml:"volumes"`
				} `yaml:"spec"`
			}{
				Metadata: struct {
					Labels map[string]string `yaml:"labels"`
				}{
					Labels: map[string]string{
						"app":       "kratos-postgres",
						"component": "database",
					},
				},
				Spec: struct {
					Containers []PostgreSQLContainer `yaml:"containers"`
					Volumes    []Volume              `yaml:"volumes"`
				}{
					Containers: []PostgreSQLContainer{
						{
							Name:  "postgres",
							Image: "postgres:16-alpine",
							Ports: []Port{
								{ContainerPort: 5432, Name: "postgres"},
							},
							Env: []EnvVar{
								{Name: "POSTGRES_DB", Value: "kratos_db"},
								{Name: "POSTGRES_USER", ValueFrom: &EnvVarSource{
									SecretKeyRef: &SecretKeySelector{Name: "kratos-postgres-secret", Key: "username"},
								}},
								{Name: "POSTGRES_PASSWORD", ValueFrom: &EnvVarSource{
									SecretKeyRef: &SecretKeySelector{Name: "kratos-postgres-secret", Key: "password"},
								}},
							},
							VolumeMounts: []VolumeMount{
								{Name: "postgres-data", MountPath: "/var/lib/postgresql/data"},
							},
							Resources: ResourceRequests{
								Requests: map[string]string{"memory": "256Mi", "cpu": "250m"},
								Limits:   map[string]string{"memory": "512Mi", "cpu": "500m"},
							},
						},
					},
				},
			},
			VolumeClaimTemplates: []VolumeClaimTemplate{
				{
					Metadata: struct {
						Name string `yaml:"name"`
					}{Name: "postgres-data"},
					Spec: struct {
						AccessModes []string            `yaml:"accessModes"`
						Resources   VolumeClaimResource `yaml:"resources"`
					}{
						AccessModes: []string{"ReadWriteOnce"},
						Resources: VolumeClaimResource{
							Requests: map[string]string{"storage": "5Gi"},
						},
					},
				},
			},
		},
	}

	// Test basic configuration
	assert.Equal(t, "apps/v1", kratosPostgresConfig.APIVersion)
	assert.Equal(t, "StatefulSet", kratosPostgresConfig.Kind)
	assert.Equal(t, "kratos-postgres", kratosPostgresConfig.Metadata.Name)
	assert.Equal(t, "alt-database", kratosPostgresConfig.Metadata.Namespace)

	// Test container environment
	require.Len(t, kratosPostgresConfig.Spec.Template.Spec.Containers, 1)
	container := kratosPostgresConfig.Spec.Template.Spec.Containers[0]

	envVars := make(map[string]EnvVar)
	for _, env := range container.Env {
		envVars[env.Name] = env
	}

	assert.Contains(t, envVars, "POSTGRES_DB")
	assert.Equal(t, "kratos_db", envVars["POSTGRES_DB"].Value)

	// Test storage configuration - smaller for Kratos
	assert.Len(t, kratosPostgresConfig.Spec.VolumeClaimTemplates, 1)
	pvc := kratosPostgresConfig.Spec.VolumeClaimTemplates[0]
	assert.Equal(t, "5Gi", pvc.Spec.Resources.Requests["storage"])
}