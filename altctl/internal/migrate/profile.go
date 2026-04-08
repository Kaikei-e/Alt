package migrate

import "fmt"

// BackupProfile defines a named set of volume categories to back up
type BackupProfile string

const (
	// ProfileDB backs up only PostgreSQL databases (fastest)
	ProfileDB BackupProfile = "db"
	// ProfileEssential backs up critical + data + search (no metrics/models)
	ProfileEssential BackupProfile = "essential"
	// ProfileAll backs up every registered volume
	ProfileAll BackupProfile = "all"
)

// profileCategories returns the volume categories for a given profile
func profileCategories(profile BackupProfile) []VolumeCategory {
	switch profile {
	case ProfileDB:
		return []VolumeCategory{CategoryCritical}
	case ProfileEssential:
		return []VolumeCategory{CategoryCritical, CategoryData, CategorySearch}
	case ProfileAll:
		return []VolumeCategory{CategoryCritical, CategoryData, CategorySearch, CategoryMetrics, CategoryModels}
	default:
		// Treat unknown profiles as "all" for forward compatibility
		return []VolumeCategory{CategoryCritical, CategoryData, CategorySearch, CategoryMetrics, CategoryModels}
	}
}

// ResolveVolumes filters the registry by profile, then applies include/exclude overrides.
//
// Resolution order:
//  1. Get volumes matching the profile's categories
//  2. If include is non-empty, intersect (keep only named volumes)
//  3. If exclude is non-empty, subtract (remove named volumes)
//
// Returns an error if any name in include does not exist in the registry.
func ResolveVolumes(registry *VolumeRegistry, profile BackupProfile, include, exclude []string) ([]VolumeSpec, error) {
	// Validate include names exist in registry
	for _, name := range include {
		if _, ok := registry.Get(name); !ok {
			return nil, fmt.Errorf("unknown volume in --include: %s", name)
		}
	}

	// Step 1: Get volumes by profile categories
	cats := profileCategories(profile)
	volumes := registry.ByCategory(cats...)

	// Step 2: Apply include filter (intersection)
	if len(include) > 0 {
		includeSet := make(map[string]bool, len(include))
		for _, name := range include {
			includeSet[name] = true
		}
		var filtered []VolumeSpec
		for _, v := range volumes {
			if includeSet[v.Name] {
				filtered = append(filtered, v)
			}
		}
		volumes = filtered
	}

	// Step 3: Apply exclude filter (subtraction)
	if len(exclude) > 0 {
		excludeSet := make(map[string]bool, len(exclude))
		for _, name := range exclude {
			excludeSet[name] = true
		}
		var filtered []VolumeSpec
		for _, v := range volumes {
			if !excludeSet[v.Name] {
				filtered = append(filtered, v)
			}
		}
		volumes = filtered
	}

	return volumes, nil
}
