package migrate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alt-project/altctl/internal/fsperm"
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

	if err := os.WriteFile(path, data, fsperm.SecretFile); err != nil {
		return fmt.Errorf("writing manifest: %w", err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		return fmt.Errorf("setting manifest permissions: %w", err)
	}

	return nil
}

// LoadManifest reads a manifest from a file
func LoadManifest(path string) (*Manifest, error) {
	// #nosec G304 -- exported standalone manifest reader for explicit caller-supplied path; backupDir callers use loadManifestInDir
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

// loadManifestInDir reads ManifestFilename strictly within backupDir via
// os.OpenRoot, rejecting symlinks and non-regular files.
func loadManifestInDir(backupDir string) (*Manifest, error) {
	root, err := os.OpenRoot(backupDir)
	if err != nil {
		return nil, fmt.Errorf("opening backup directory %q: %w", backupDir, err)
	}
	defer func() { _ = root.Close() }()

	info, err := root.Lstat(ManifestFilename)
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("reading manifest: symlink %q is not allowed", ManifestFilename)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("reading manifest: %q is not a regular file", ManifestFilename)
	}

	f, err := root.Open(ManifestFilename)
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}
	defer func() { _ = f.Close() }()

	openInfo, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}
	if !openInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("reading manifest: %q is not a regular file", ManifestFilename)
	}

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	return &m, nil
}

// validateRelativePath verifies relPath is a non-empty local relative path
// without traversal components and returns its cleaned relative form.
func validateRelativePath(relPath string) (string, error) {
	if relPath == "" || filepath.IsAbs(relPath) || strings.ContainsRune(relPath, '\x00') {
		return "", fmt.Errorf("invalid relative path %q", relPath)
	}
	for _, part := range strings.Split(filepath.ToSlash(relPath), "/") {
		if part == ".." {
			return "", fmt.Errorf("path %q escapes base directory", relPath)
		}
	}
	clean := filepath.Clean(relPath)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || !filepath.IsLocal(clean) {
		return "", fmt.Errorf("path %q escapes base directory", relPath)
	}
	return clean, nil
}

// rejectSymlinksInRoot walks every path component of cleanRel via root.Lstat
// and rejects any symlink component (including leaf) and any non-regular leaf.
func rejectSymlinksInRoot(root *os.Root, cleanRel string) (os.FileInfo, error) {
	parts := strings.Split(filepath.ToSlash(cleanRel), "/")
	var (
		prefix string
		info   os.FileInfo
		err    error
	)
	for i, part := range parts {
		if i == 0 {
			prefix = part
		} else {
			prefix = filepath.Join(prefix, part)
		}
		info, err = root.Lstat(prefix)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("symlink not allowed at %q", prefix)
		}
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("path %q is not a regular file", cleanRel)
	}
	return info, nil
}

// resolveUnderBase validates relPath lexically, verifies via os.OpenRoot and
// component-wise Lstat that no component is a symlink and the leaf is a regular
// file, and confirms via filepath.EvalSymlinks that the canonical target stays
// strictly under canonical baseDir.
func resolveUnderBase(baseDir, relPath string) (string, error) {
	cleanRel, err := validateRelativePath(relPath)
	if err != nil {
		return "", err
	}

	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("resolving base directory: %w", err)
	}
	target := filepath.Join(absBase, cleanRel)
	lexRel, err := filepath.Rel(absBase, target)
	if err != nil || lexRel == "." || lexRel == ".." || strings.HasPrefix(lexRel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes base directory %q", relPath, baseDir)
	}

	root, err := os.OpenRoot(absBase)
	if err != nil {
		return "", fmt.Errorf("opening base directory %q: %w", baseDir, err)
	}
	defer func() { _ = root.Close() }()

	if _, err := rejectSymlinksInRoot(root, cleanRel); err != nil {
		return "", fmt.Errorf("path %q rejected under %q: %w", relPath, baseDir, err)
	}

	canonicalBase, err := filepath.EvalSymlinks(absBase)
	if err != nil {
		return "", fmt.Errorf("resolving canonical base directory %q: %w", baseDir, err)
	}
	canonicalTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", fmt.Errorf("resolving canonical path %q: %w", relPath, err)
	}
	canonRel, err := filepath.Rel(canonicalBase, canonicalTarget)
	if err != nil || canonRel == "." || canonRel == ".." || strings.HasPrefix(canonRel, ".."+string(filepath.Separator)) || filepath.IsAbs(canonRel) {
		return "", fmt.Errorf("canonical path %q escapes base directory %q", relPath, baseDir)
	}

	return canonicalTarget, nil
}

// Verify checks that all backup files exist inside backupDir without symlink
// traversal, are regular files, and have correct sizes and checksums.
func (m *Manifest) Verify(backupDir string) error {
	root, err := os.OpenRoot(backupDir)
	if err != nil {
		return fmt.Errorf("opening backup directory %q: %w", backupDir, err)
	}
	defer func() { _ = root.Close() }()

	for _, v := range m.Volumes {
		if err := verifyVolumeInRoot(root, v); err != nil {
			return err
		}
	}

	// Verify overall checksum
	if m.Checksum != "" && m.Checksum != m.ComputeChecksum() {
		return fmt.Errorf("manifest checksum mismatch")
	}

	return nil
}

func verifyVolumeInRoot(root *os.Root, v VolumeBackup) error {
	cleanRel, err := validateRelativePath(v.Filename)
	if err != nil {
		return fmt.Errorf("volume %s: %w", v.Name, err)
	}

	if _, err := rejectSymlinksInRoot(root, cleanRel); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("volume %s: file not found: %w", v.Name, err)
		}
		return fmt.Errorf("volume %s: %w", v.Name, err)
	}

	f, err := root.Open(cleanRel)
	if err != nil {
		return fmt.Errorf("volume %s: opening file: %w", v.Name, err)
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("volume %s: stat file: %w", v.Name, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("volume %s: %q is not a regular file", v.Name, v.Filename)
	}

	if info.Size() != v.Size {
		return fmt.Errorf("volume %s: size mismatch (expected %d, got %d)",
			v.Name, v.Size, info.Size())
	}

	checksum, err := readerChecksum(f)
	if err != nil {
		return fmt.Errorf("volume %s: checksum error: %w", v.Name, err)
	}
	if checksum != v.Checksum {
		return fmt.Errorf("volume %s: checksum mismatch", v.Name)
	}

	return nil
}

func readerChecksum(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}

	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// FileChecksum calculates SHA256 checksum of a file
func FileChecksum(path string) (string, error) {
	// #nosec G304 -- exported helper for caller-supplied path; Manifest.Verify hashes an os.Root-opened handle directly
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	return readerChecksum(f)
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
