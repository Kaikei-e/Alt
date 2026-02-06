package migrate

import (
	"fmt"
	"sort"
	"time"
)

// HealthLevel represents backup health
type HealthLevel int

const (
	HealthGood HealthLevel = iota
	HealthWarning
	HealthCritical
)

func (h HealthLevel) String() string {
	switch h {
	case HealthGood:
		return "GOOD"
	case HealthWarning:
		return "WARNING"
	case HealthCritical:
		return "CRITICAL"
	default:
		return "UNKNOWN"
	}
}

// RPOThreshold is the maximum acceptable age for the latest backup
const RPOThreshold = 25 * time.Hour

// BackupStatus contains aggregated status of the backup system
type BackupStatus struct {
	HasBackup       bool
	LatestBackup    string
	LatestTimestamp time.Time
	Age             time.Duration
	ExpectedVolumes int
	ActualVolumes   int
	MissingVolumes  []string
	TotalSize       int64
	ChecksumOK      bool
	Health          HealthLevel
}

// GetBackupStatus scans the backup directory and returns status
func GetBackupStatus(baseDir string) (*BackupStatus, error) {
	registry := NewVolumeRegistry()
	expected := registry.All()

	status := &BackupStatus{
		ExpectedVolumes: len(expected),
		Health:          HealthCritical,
	}

	backups, err := ListBackups(baseDir)
	if err != nil {
		return nil, fmt.Errorf("listing backups: %w", err)
	}

	if len(backups) == 0 {
		return status, nil
	}

	// Find latest backup
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].CreatedAt.After(backups[j].CreatedAt)
	})
	latest := backups[0]

	status.HasBackup = true
	status.LatestBackup = latest.Name
	status.LatestTimestamp = latest.CreatedAt
	status.Age = time.Since(latest.CreatedAt)
	status.ActualVolumes = latest.VolumeCount
	status.TotalSize = latest.TotalSize

	// Check for missing volumes
	backedUp := make(map[string]bool)
	for _, v := range latest.Manifest.Volumes {
		backedUp[v.Name] = true
	}
	for _, spec := range expected {
		if !backedUp[spec.Name] {
			status.MissingVolumes = append(status.MissingVolumes, spec.Name)
		}
	}

	// Verify checksums
	verifyErr := latest.Manifest.Verify(latest.Path)
	status.ChecksumOK = verifyErr == nil

	// Determine health
	status.Health = HealthGood

	if status.Age > RPOThreshold {
		status.Health = HealthWarning
	}

	if len(status.MissingVolumes) > 0 {
		status.Health = HealthWarning
	}

	if !status.ChecksumOK {
		status.Health = HealthWarning
	}

	if !status.HasBackup || status.Age > 2*RPOThreshold {
		status.Health = HealthCritical
	}

	return status, nil
}
