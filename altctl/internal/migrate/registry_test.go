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

func TestVolumeRegistry_AllAreTar(t *testing.T) {
	r := NewVolumeRegistry()

	// All volumes should now be tar type
	for _, v := range r.All() {
		if v.BackupType != BackupTypeTar {
			t.Errorf("Volume %s should be Tar type, got %s", v.Name, v.BackupType.String())
		}
	}
}

func TestVolumeRegistry_Tar(t *testing.T) {
	r := NewVolumeRegistry()
	tarVolumes := r.Tar()

	// Should have all 9 volumes as tar
	if len(tarVolumes) != 9 {
		t.Errorf("Expected 9 tar volumes, got %d", len(tarVolumes))
	}

	expectedNames := map[string]bool{
		"db_data_17":               true,
		"kratos_db_data":           true,
		"recap_db_data":            true,
		"rag_db_data":              true,
		"meili_data":               true,
		"clickhouse_data":          true,
		"news_creator_models":      true,
		"rask_log_aggregator_data": true,
		"oauth_token_data":         true,
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

func TestVolumeRegistry_All(t *testing.T) {
	r := NewVolumeRegistry()
	all := r.All()

	// Should have 9 total volumes (4 PostgreSQL + 5 tar)
	if len(all) != 9 {
		t.Errorf("Expected 9 total volumes, got %d", len(all))
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
