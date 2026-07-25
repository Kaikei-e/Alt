package migrate

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// excludedDBVolumes lists *_db_data / *-db-data volumes found in
// compose/*.yaml that are intentionally NOT registered in defaultVolumes,
// with the reason why. Every db-shaped volume must appear either in
// NewVolumeRegistry() or here -- see TestVolumeUniverse_EveryDBVolumeIsRegisteredOrExcluded.
// This is the guard the acolyte_db_data bug (silently unprotected by
// `altctl migrate backup --profile all`) should have caught.
var excludedDBVolumes = map[string]string{
	"dev_db_data":  "compose/dev.yaml is a local dev-only overlay stack (ephemeral, disposable by design); not part of any backup profile",
	"pact_db_data": "compose/pact.yaml is CI-only (Pact Broker for contract testing); ephemeral, not backed up",
}

// composeVolumesDoc decodes only the top-level "volumes:" key of a compose
// YAML document as a raw yaml.Node, mirroring internal/stack's
// composeFileDoc technique (services:) and internal/setup's
// composeSecretsDoc technique (secrets:) -- we only need the mapping's
// keys, not full anchor/alias resolution.
type composeVolumesDoc struct {
	Volumes yaml.Node `yaml:"volumes"`
}

// composeFileVolumeNames reads a compose YAML file and returns the keys of
// its top-level volumes: mapping.
func composeFileVolumeNames(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc composeVolumesDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if doc.Volumes.Kind != yaml.MappingNode {
		return nil, nil
	}
	var names []string
	for i := 0; i+1 < len(doc.Volumes.Content); i += 2 {
		key := doc.Volumes.Content[i]
		if key.Kind == yaml.ScalarNode {
			names = append(names, key.Value)
		}
	}
	return names, nil
}

// repoComposeDir resolves compose/ from this test file's own location
// (runtime.Caller), independent of `go test`'s working directory.
func repoComposeDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// this file: <repo>/altctl/internal/migrate/registry_sync_test.go
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(thisFile))))
	dir := filepath.Join(repoRoot, "compose")
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("resolved compose dir does not exist at %s: %v", dir, err)
	}
	return dir
}

// isDBVolumeName reports whether a volume name looks like a PostgreSQL
// database volume by the *_db_data / *-db-data naming convention every
// database volume in this repo follows (db_data_17, kratos_db_data,
// acolyte_db_data, knowledge-sovereign-db-data, ...).
func isDBVolumeName(name string) bool {
	return strings.Contains(name, "db_data") || strings.Contains(name, "db-data")
}

// TestVolumeUniverse_EveryDBVolumeIsRegisteredOrExcluded derives the full
// volume-name universe from every compose/*.yaml's top-level volumes:
// block and asserts each *_db_data / *-db-data volume is either registered
// in NewVolumeRegistry() (so `altctl migrate backup` protects it) or
// explicitly listed in excludedDBVolumes with a reason. This is the sync
// test that would have caught acolyte_db_data being silently unprotected:
// the next new database can no longer land in compose/*.yaml without this
// test forcing a decision (register it, or document why not).
func TestVolumeUniverse_EveryDBVolumeIsRegisteredOrExcluded(t *testing.T) {
	dir := repoComposeDir(t)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading compose dir: %v", err)
	}

	seen := make(map[string]bool)
	var dbVolumes []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		names, err := composeFileVolumeNames(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		for _, vn := range names {
			if !isDBVolumeName(vn) || seen[vn] {
				continue
			}
			seen[vn] = true
			dbVolumes = append(dbVolumes, vn)
		}
	}

	if len(dbVolumes) == 0 {
		t.Fatal("found zero *_db_data volumes across compose/*.yaml -- the scan is almost certainly broken")
	}
	sort.Strings(dbVolumes)

	r := NewVolumeRegistry()

	for _, name := range dbVolumes {
		if _, ok := r.Get(name); ok {
			continue
		}
		if reason, ok := excludedDBVolumes[name]; ok && reason != "" {
			continue
		}
		t.Errorf("volume %q (from compose/*.yaml) is neither registered in NewVolumeRegistry() nor listed in excludedDBVolumes with a reason -- "+
			"altctl migrate backup would silently skip its data. Register it, or add an excludedDBVolumes entry explaining why not.", name)
	}
}

// TestExcludedDBVolumes_StillReferencedByCompose keeps excludedDBVolumes
// itself from drifting: an excluded entry that no longer exists anywhere
// in compose/*.yaml (stack removed) is stale and should be deleted so the
// exclusion list doesn't accumulate cruft that masks future mistakes.
func TestExcludedDBVolumes_StillReferencedByCompose(t *testing.T) {
	dir := repoComposeDir(t)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading compose dir: %v", err)
	}

	found := make(map[string]bool)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		names, err := composeFileVolumeNames(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		for _, vn := range names {
			found[vn] = true
		}
	}

	for name := range excludedDBVolumes {
		if !found[name] {
			t.Errorf("excludedDBVolumes[%q] is stale: no compose/*.yaml volumes: block declares it anymore", name)
		}
	}
}
