package migrate

import (
	"testing"
)

func TestBackupTypeString(t *testing.T) {
	tests := []struct {
		bt   BackupType
		want string
	}{
		{BackupTypePostgreSQL, "postgresql"},
		{BackupTypeTar, "tar"},
		{BackupType(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.bt.String(); got != tt.want {
				t.Errorf("BackupType.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVolumeCategoryString(t *testing.T) {
	tests := []struct {
		cat  VolumeCategory
		want string
	}{
		{CategoryCritical, "critical"},
		{CategoryData, "data"},
		{CategorySearch, "search"},
		{CategoryMetrics, "metrics"},
		{CategoryModels, "models"},
		{VolumeCategory(0), "unknown"},
		{VolumeCategory(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.cat.String(); got != tt.want {
				t.Errorf("VolumeCategory.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewVolumeRegistry(t *testing.T) {
	r := NewVolumeRegistry()

	if r == nil {
		t.Fatal("NewVolumeRegistry() returned nil")
	}

	all := r.All()
	if len(all) == 0 {
		t.Error("Registry should have default volumes")
	}
}

func TestVolumeRegistry_BackupTypes(t *testing.T) {
	r := NewVolumeRegistry()

	pgNames := map[string]bool{
		"db_data_17":                   true,
		"kratos_db_data":               true,
		"recap_db_data":                true,
		"rag_db_data":                  true,
		"knowledge-sovereign-db-data":  true,
		"pre_processor_db_data":        true,
	}

	for _, v := range r.All() {
		if pgNames[v.Name] {
			if v.BackupType != BackupTypePostgreSQL {
				t.Errorf("Volume %s should be PostgreSQL type, got %s", v.Name, v.BackupType.String())
			}
		} else {
			if v.BackupType != BackupTypeTar {
				t.Errorf("Volume %s should be Tar type, got %s", v.Name, v.BackupType.String())
			}
		}
	}
}

func TestVolumeRegistry_Tar(t *testing.T) {
	r := NewVolumeRegistry()
	tarVolumes := r.Tar()

	// Should have 8 tar volumes (14 total - 6 PG)
	if len(tarVolumes) != 8 {
		t.Errorf("Expected 8 tar volumes, got %d", len(tarVolumes))
	}

	expectedNames := map[string]bool{
		"meili_data":               true,
		"clickhouse_data":          true,
		"news_creator_models":      true,
		"rask_log_aggregator_data": true,
		"oauth_token_data":         true,
		"redis-streams-data":       true,
		"prometheus_data":          true,
		"grafana_data":             true,
	}

	for _, v := range tarVolumes {
		if v.BackupType != BackupTypeTar {
			t.Errorf("Volume %s should be Tar type", v.Name)
		}
		if !expectedNames[v.Name] {
			t.Errorf("Unexpected tar volume: %s", v.Name)
		}
	}
}

func TestVolumeRegistry_Get(t *testing.T) {
	r := NewVolumeRegistry()

	// Test existing volume
	v, ok := r.Get("db_data_17")
	if !ok {
		t.Error("Should find db_data_17")
	}
	if v.Service != "db" {
		t.Errorf("Expected service 'db', got '%s'", v.Service)
	}

	// Test non-existing volume
	_, ok = r.Get("nonexistent")
	if ok {
		t.Error("Should not find nonexistent volume")
	}
}

func TestVolumeRegistry_Get_HyphenUnderscoreCompat(t *testing.T) {
	r := NewVolumeRegistry()

	// Old manifests may use underscores for sovereign volume
	v, ok := r.Get("knowledge_sovereign_db_data")
	if !ok {
		t.Error("Should find sovereign volume via underscore fallback")
	}
	if v.Service != "knowledge-sovereign-db" {
		t.Errorf("Expected service 'knowledge-sovereign-db', got '%s'", v.Service)
	}

	// Direct lookup with hyphens should also work
	v2, ok := r.Get("knowledge-sovereign-db-data")
	if !ok {
		t.Error("Should find sovereign volume by exact name")
	}
	if v.Name != v2.Name {
		t.Error("Both lookups should return the same volume")
	}
}

func TestVolumeRegistry_All(t *testing.T) {
	r := NewVolumeRegistry()
	all := r.All()

	// Should have 14 total volumes (6 PG + 8 tar)
	if len(all) != 14 {
		t.Errorf("Expected 14 total volumes, got %d", len(all))
	}

	// Verify each volume has required fields
	for _, v := range all {
		if v.Name == "" {
			t.Error("Volume name should not be empty")
		}
		if v.Service == "" {
			t.Error("Volume service should not be empty")
		}
		if v.Description == "" {
			t.Error("Volume description should not be empty")
		}
	}
}

func TestVolumeRegistry_AllHaveCategory(t *testing.T) {
	r := NewVolumeRegistry()
	for _, v := range r.All() {
		if v.Category == 0 {
			t.Errorf("Volume %s has no category assigned", v.Name)
		}
	}
}

func TestVolumeRegistry_ByCategory_Critical(t *testing.T) {
	r := NewVolumeRegistry()
	critical := r.ByCategory(CategoryCritical)

	if len(critical) != 6 {
		t.Errorf("Expected 6 critical volumes, got %d", len(critical))
	}

	expectedNames := map[string]bool{
		"db_data_17":                  true,
		"kratos_db_data":              true,
		"recap_db_data":               true,
		"rag_db_data":                 true,
		"knowledge-sovereign-db-data": true,
		"pre_processor_db_data":       true,
	}

	for _, v := range critical {
		if !expectedNames[v.Name] {
			t.Errorf("Unexpected critical volume: %s", v.Name)
		}
		if v.Category != CategoryCritical {
			t.Errorf("Volume %s has category %s, want critical", v.Name, v.Category)
		}
	}
}

func TestVolumeRegistry_ByCategory_Metrics(t *testing.T) {
	r := NewVolumeRegistry()
	metrics := r.ByCategory(CategoryMetrics)

	if len(metrics) != 3 {
		t.Errorf("Expected 3 metrics volumes, got %d", len(metrics))
	}

	expectedNames := map[string]bool{
		"clickhouse_data": true,
		"prometheus_data": true,
		"grafana_data":    true,
	}

	for _, v := range metrics {
		if !expectedNames[v.Name] {
			t.Errorf("Unexpected metrics volume: %s", v.Name)
		}
	}
}

func TestVolumeRegistry_ByCategory_MultiCategories(t *testing.T) {
	r := NewVolumeRegistry()
	result := r.ByCategory(CategoryCritical, CategoryData)

	// 6 critical + 3 data = 9
	if len(result) != 9 {
		t.Errorf("Expected 9 volumes for critical+data, got %d", len(result))
	}

	for _, v := range result {
		if v.Category != CategoryCritical && v.Category != CategoryData {
			t.Errorf("Volume %s has unexpected category %s", v.Name, v.Category)
		}
	}
}
