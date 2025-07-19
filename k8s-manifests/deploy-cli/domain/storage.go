package domain

import (
	"fmt"
)

// StorageClass represents a Kubernetes storage class
type StorageClass struct {
	Name        string
	Provisioner string
	Parameters  map[string]string
}

// PersistentVolume represents a Kubernetes persistent volume
type PersistentVolume struct {
	Name         string
	Capacity     string
	StorageClass string
	HostPath     string
	AccessModes  []string
	Status       string
}

// NewPersistentVolume creates a new persistent volume
func NewPersistentVolume(name, capacity, storageClass, hostPath string) *PersistentVolume {
	return &PersistentVolume{
		Name:         name,
		Capacity:     capacity,
		StorageClass: storageClass,
		HostPath:     hostPath,
		AccessModes:  []string{"ReadWriteOnce"},
		Status:       "Available",
	}
}

// StorageConfig represents storage configuration
type StorageConfig struct {
	RequiredStorageClasses []string
	CapacityLimits         map[string]string
	PersistentVolumes      []PersistentVolume
	DataPaths              []string
}

// NewStorageConfig creates a new storage configuration
func NewStorageConfig() *StorageConfig {
	return &StorageConfig{
		RequiredStorageClasses: []string{
			"standard",
		},
		CapacityLimits: map[string]string{
			"postgres":              "8Gi",
			"auth-postgres":         "5Gi",
			"kratos-postgres":       "5Gi",
			"clickhouse":            "8Gi",
			"meilisearch":           "8Gi",
			"meilisearch-dumps":     "3Gi",
			"meilisearch-snapshots": "3Gi",
		},
		PersistentVolumes: []PersistentVolume{
			*NewPersistentVolume("postgres-pv", "8Gi", "local-storage", "/home/koko/Documents/dev/Alt/pv-data/postgres"),
			*NewPersistentVolume("auth-postgres-pv", "5Gi", "local-storage", "/home/koko/Documents/dev/Alt/pv-data/auth-postgres"),
			*NewPersistentVolume("kratos-postgres-pv", "5Gi", "local-storage", "/home/koko/Documents/dev/Alt/pv-data/kratos-postgres"),
			*NewPersistentVolume("clickhouse-pv", "8Gi", "local-storage", "/home/koko/Documents/dev/Alt/pv-data/clickhouse"),
			*NewPersistentVolume("meilisearch-pv", "8Gi", "local-storage", "/home/koko/Documents/dev/Alt/pv-data/meilisearch"),
			*NewPersistentVolume("meilisearch-dumps-pv", "3Gi", "local-storage", "/home/koko/Documents/dev/Alt/pv-data/meilisearch-dumps"),
			*NewPersistentVolume("meilisearch-snapshots-pv", "3Gi", "local-storage", "/home/koko/Documents/dev/Alt/pv-data/meilisearch-snapshots"),
		},
		DataPaths: []string{
			"/mnt/data/postgres",
			"/mnt/data/clickhouse",
			"/mnt/data/meilisearch",
			"/home/koko/Documents/dev/Alt/pv-data/postgres",
			"/home/koko/Documents/dev/Alt/pv-data/auth-postgres",
			"/home/koko/Documents/dev/Alt/pv-data/kratos-postgres",
		},
	}
}

// GetRequiredPVs returns the list of required persistent volumes
func (s *StorageConfig) GetRequiredPVs() []string {
	return []string{
		"postgres-pv",
		"auth-postgres-pv",
		"kratos-postgres-pv",
		"clickhouse-pv",
		"meilisearch-pv",
	}
}

// GetCapacityLimit returns the capacity limit for the given service
func (s *StorageConfig) GetCapacityLimit(service string) (string, error) {
	if limit, exists := s.CapacityLimits[service]; exists {
		return limit, nil
	}
	return "", fmt.Errorf("capacity limit not found for service: %s", service)
}

// GetTotalCapacity returns the total capacity required
func (s *StorageConfig) GetTotalCapacity() int {
	capacities := map[string]int{
		"postgres":              8,
		"auth-postgres":         5,
		"kratos-postgres":       5,
		"clickhouse":            8,
		"meilisearch":           8,
		"meilisearch-dumps":     3,
		"meilisearch-snapshots": 3,
	}

	total := 0
	for _, capacity := range capacities {
		total += capacity
	}
	return total
}

// GetMaxCapacityLimit returns the maximum capacity limit (40Gi)
func (s *StorageConfig) GetMaxCapacityLimit() int {
	return 40
}

// ValidateCapacity validates that total capacity is within limits
func (s *StorageConfig) ValidateCapacity() error {
	total := s.GetTotalCapacity()
	max := s.GetMaxCapacityLimit()

	if total > max {
		return fmt.Errorf("total storage capacity %dGi exceeds maximum limit %dGi", total, max)
	}
	return nil
}

// GetStandardStorageClass returns the standard storage class for OSS environment
func (s *StorageConfig) GetStandardStorageClass() string {
	return "local-storage"
}

// GetPVFiles returns the list of PV files to apply
func (s *StorageConfig) GetPVFiles() []string {
	return []string{
		"../postgres-pv.yaml",
		"../auth-postgres-pv.yaml",
		"../kratos-postgres-pv.yaml",
		"../clickhouse-pv.yaml",
		"../meilisearch-pv.yaml",
		"../meilisearch-dumps-pv.yaml",
		"../meilisearch-snapshots-pv.yaml",
	}
}
