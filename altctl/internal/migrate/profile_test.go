package migrate

import (
	"testing"
)

func TestResolveVolumes_ProfileDB(t *testing.T) {
	r := NewVolumeRegistry()
	vols, err := ResolveVolumes(r, ProfileDB, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(vols) != 6 {
		t.Errorf("ProfileDB should return 6 volumes, got %d", len(vols))
	}

	for _, v := range vols {
		if v.BackupType != BackupTypePostgreSQL {
			t.Errorf("ProfileDB should only include PostgreSQL volumes, got %s (%s)", v.Name, v.BackupType)
		}
		if v.Category != CategoryCritical {
			t.Errorf("ProfileDB should only include critical volumes, got %s (%s)", v.Name, v.Category)
		}
	}
}

func TestResolveVolumes_ProfileEssential(t *testing.T) {
	r := NewVolumeRegistry()
	vols, err := ResolveVolumes(r, ProfileEssential, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// critical(6) + data(3) + search(1) = 10
	if len(vols) != 10 {
		t.Errorf("ProfileEssential should return 10 volumes, got %d", len(vols))
	}

	for _, v := range vols {
		if v.Category == CategoryMetrics || v.Category == CategoryModels {
			t.Errorf("ProfileEssential should not include %s category volume %s", v.Category, v.Name)
		}
	}
}

func TestResolveVolumes_ProfileAll(t *testing.T) {
	r := NewVolumeRegistry()
	vols, err := ResolveVolumes(r, ProfileAll, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(vols) != 14 {
		t.Errorf("ProfileAll should return 14 volumes, got %d", len(vols))
	}
}

func TestResolveVolumes_ExcludeByName(t *testing.T) {
	r := NewVolumeRegistry()
	vols, err := ResolveVolumes(r, ProfileAll, nil, []string{"clickhouse_data"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(vols) != 13 {
		t.Errorf("Expected 13 volumes after excluding 1, got %d", len(vols))
	}

	for _, v := range vols {
		if v.Name == "clickhouse_data" {
			t.Error("clickhouse_data should be excluded")
		}
	}
}

func TestResolveVolumes_IncludeByName(t *testing.T) {
	r := NewVolumeRegistry()
	vols, err := ResolveVolumes(r, ProfileAll, []string{"db_data_17"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(vols) != 1 {
		t.Errorf("Expected 1 volume with include filter, got %d", len(vols))
	}

	if vols[0].Name != "db_data_17" {
		t.Errorf("Expected db_data_17, got %s", vols[0].Name)
	}
}

func TestResolveVolumes_IncludeUnknownName(t *testing.T) {
	r := NewVolumeRegistry()
	_, err := ResolveVolumes(r, ProfileAll, []string{"nonexistent_volume"}, nil)
	if err == nil {
		t.Error("Expected error for unknown include volume name")
	}
}

func TestResolveVolumes_ExcludeOverridesProfile(t *testing.T) {
	r := NewVolumeRegistry()
	vols, err := ResolveVolumes(r, ProfileDB, nil, []string{"db_data_17"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(vols) != 5 {
		t.Errorf("Expected 5 volumes after excluding 1 from ProfileDB, got %d", len(vols))
	}

	for _, v := range vols {
		if v.Name == "db_data_17" {
			t.Error("db_data_17 should be excluded")
		}
	}
}

func TestResolveVolumes_IncludeMultiple(t *testing.T) {
	r := NewVolumeRegistry()
	vols, err := ResolveVolumes(r, ProfileAll, []string{"db_data_17", "kratos_db_data"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(vols) != 2 {
		t.Errorf("Expected 2 volumes, got %d", len(vols))
	}
}

func TestResolveVolumes_IncludeOutsideProfile(t *testing.T) {
	r := NewVolumeRegistry()
	// Include a metrics volume with ProfileDB — it should be filtered out
	// because include intersects with profile results
	vols, err := ResolveVolumes(r, ProfileDB, []string{"clickhouse_data"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(vols) != 0 {
		t.Errorf("Expected 0 volumes (include outside profile scope), got %d", len(vols))
	}
}

func TestProfileCategories(t *testing.T) {
	tests := []struct {
		profile    BackupProfile
		wantCount  int
	}{
		{ProfileDB, 1},        // [critical]
		{ProfileEssential, 3}, // [critical, data, search]
		{ProfileAll, 5},       // all 5 categories
	}

	for _, tt := range tests {
		t.Run(string(tt.profile), func(t *testing.T) {
			cats := profileCategories(tt.profile)
			if len(cats) != tt.wantCount {
				t.Errorf("profileCategories(%s) returned %d categories, want %d", tt.profile, len(cats), tt.wantCount)
			}
		})
	}
}
