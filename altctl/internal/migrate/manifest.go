package migrate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const (
	// ManifestVersion is the current manifest format version
	ManifestVersion = "1.0"
	// ManifestFilename is the standard manifest filename
	ManifestFilename = "manifest.json"
)

// Manifest describes a complete backup
type Manifest struct {
	Version       string         `json:"version"`
	CreatedAt     time.Time      `json:"created_at"`
	AltctlVersion string         `json:"altctl_version"`
	Volumes       []VolumeBackup `json:"volumes"`
	Checksum      string         `json:"checksum,omitempty"`
}

// VolumeBackup describes a single volume's backup
type VolumeBackup struct {
	Name       string     `json:"name"`
	Type       BackupType `json:"type"`
	TypeString string     `json:"type_string"`
	Filename   string     `json:"filename"`
	Size       int64      `json:"size"`
	Checksum   string     `json:"checksum"`
	Service    string     `json:"service"`
	BackedUpAt time.Time  `json:"backed_up_at"`
}

// MarshalJSON implements custom JSON marshaling for BackupType
func (t BackupType) MarshalJSON() ([]byte, error) {
	return json.Marshal(int(t))
}

// UnmarshalJSON implements custom JSON unmarshaling for BackupType
func (t *BackupType) UnmarshalJSON(data []byte) error {
	var i int
	if err := json.Unmarshal(data, &i); err != nil {
		return err
	}
	*t = BackupType(i)
	return nil
}

// NewManifest creates a new manifest with default values
func NewManifest(altctlVersion string) *Manifest {
	return &Manifest{
		Version:       ManifestVersion,
		CreatedAt:     time.Now().UTC(),
		AltctlVersion: altctlVersion,
		Volumes:       []VolumeBackup{},
	}
}

// AddVolume adds a volume backup entry to the manifest
func (m *Manifest) AddVolume(vb VolumeBackup) {
	vb.TypeString = vb.Type.String()
	if vb.BackedUpAt.IsZero() {
		vb.BackedUpAt = time.Now().UTC()
	}
	m.Volumes = append(m.Volumes, vb)
}

// ComputeChecksum calculates the overall manifest checksum
func (m *Manifest) ComputeChecksum() string {
	h := sha256.New()
	for _, v := range m.Volumes {
		h.Write([]byte(v.Name))
		h.Write([]byte(v.Checksum))
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// Finalize computes the checksum and marks the manifest as complete
func (m *Manifest) Finalize() {
	m.Checksum = m.ComputeChecksum()
}

// Save writes the manifest to a file
func (m *Manifest) Save(path string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing manifest: %w", err)
	}

	return nil
}

// LoadManifest reads a manifest from a file
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	return &m, nil
}

// Verify checks that all backup files exist and have correct checksums
func (m *Manifest) Verify(backupDir string) error {
	for _, v := range m.Volumes {
		filePath := filepath.Join(backupDir, v.Filename)

		// Check file exists
		info, err := os.Stat(filePath)
		if err != nil {
			return fmt.Errorf("volume %s: file not found: %w", v.Name, err)
		}

		// Check file size
		if info.Size() != v.Size {
			return fmt.Errorf("volume %s: size mismatch (expected %d, got %d)",
				v.Name, v.Size, info.Size())
		}

		// Check checksum
		checksum, err := FileChecksum(filePath)
		if err != nil {
			return fmt.Errorf("volume %s: checksum error: %w", v.Name, err)
		}
		if checksum != v.Checksum {
			return fmt.Errorf("volume %s: checksum mismatch", v.Name)
		}
	}

	// Verify overall checksum
	if m.Checksum != "" && m.Checksum != m.ComputeChecksum() {
		return fmt.Errorf("manifest checksum mismatch")
	}

	return nil
}

// FileChecksum calculates SHA256 checksum of a file
func FileChecksum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// GetVolume returns a volume backup by name
func (m *Manifest) GetVolume(name string) (VolumeBackup, bool) {
	for _, v := range m.Volumes {
		if v.Name == name {
			return v, true
		}
	}
	return VolumeBackup{}, false
}

// BackupDir returns the expected backup directory path
func BackupDir(baseDir string) string {
	timestamp := time.Now().Format("20060102_150405")
	return filepath.Join(baseDir, timestamp)
}
