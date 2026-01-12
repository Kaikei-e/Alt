package stack

// Feature represents a capability provided by a stack
type Feature string

const (
	FeatureSearch        Feature = "search"
	FeatureAI            Feature = "ai"
	FeatureRecap         Feature = "recap"
	FeatureRAG           Feature = "rag"
	FeatureLogging       Feature = "logging"
	FeatureAuth          Feature = "auth"
	FeatureDatabase      Feature = "database"
	FeatureObservability Feature = "observability"
	FeatureMQ            Feature = "mq"
	FeatureBFF           Feature = "bff"
)

// WarningSeverity indicates how critical a missing feature is
type WarningSeverity int

const (
	SeverityInfo WarningSeverity = iota
	SeverityWarning
	SeverityCritical
)

// FeatureWarning represents a missing feature dependency
type FeatureWarning struct {
	Stack          string          // Stack that requires the missing feature
	MissingFeature Feature         // Feature that is not available
	ProvidedBy     []string        // Stacks that can provide this feature
	Severity       WarningSeverity // How critical is this missing feature
}

// FeatureResolver handles feature-based dependency analysis
type FeatureResolver struct {
	registry *Registry
}

// NewFeatureResolver creates a new feature resolver
func NewFeatureResolver(registry *Registry) *FeatureResolver {
	return &FeatureResolver{registry: registry}
}

// CheckMissingFeatures analyzes which features are missing for the given stacks
func (r *FeatureResolver) CheckMissingFeatures(stackNames []string) []FeatureWarning {
	var warnings []FeatureWarning

	// Build set of features that will be available
	availableFeatures := make(map[Feature]bool)
	for _, name := range stackNames {
		stack, ok := r.registry.Get(name)
		if !ok {
			continue
		}
		for _, f := range stack.Provides {
			availableFeatures[f] = true
		}
	}

	// Check each stack's required features
	for _, name := range stackNames {
		stack, ok := r.registry.Get(name)
		if !ok {
			continue
		}

		for _, required := range stack.RequiresFeatures {
			if !availableFeatures[required] {
				providers := r.findFeatureProviders(required)
				warnings = append(warnings, FeatureWarning{
					Stack:          name,
					MissingFeature: required,
					ProvidedBy:     providers,
					Severity:       SeverityWarning,
				})
			}
		}
	}

	return warnings
}

// findFeatureProviders returns stacks that provide a given feature
func (r *FeatureResolver) findFeatureProviders(f Feature) []string {
	var providers []string
	for _, stack := range r.registry.All() {
		if stack.ProvidesFeature(f) {
			providers = append(providers, stack.Name)
		}
	}
	return providers
}

// SuggestAdditionalStacks suggests stacks to add for complete functionality
func (r *FeatureResolver) SuggestAdditionalStacks(stackNames []string) []string {
	warnings := r.CheckMissingFeatures(stackNames)

	suggested := make(map[string]bool)
	for _, w := range warnings {
		for _, provider := range w.ProvidedBy {
			suggested[provider] = true
		}
	}

	// Remove stacks already being started
	for _, name := range stackNames {
		delete(suggested, name)
	}

	var result []string
	for name := range suggested {
		result = append(result, name)
	}
	return result
}

// FormatWarnings returns a human-readable string of all warnings
func (r *FeatureResolver) FormatWarnings(warnings []FeatureWarning) string {
	if len(warnings) == 0 {
		return ""
	}

	var result string
	for _, w := range warnings {
		result += "Stack '" + w.Stack + "' requires feature '" + string(w.MissingFeature) + "' which is not available.\n"
		if len(w.ProvidedBy) > 0 {
			result += "  Suggestion: Also start: "
			for i, p := range w.ProvidedBy {
				if i > 0 {
					result += ", "
				}
				result += p
			}
			result += "\n"
		}
	}
	return result
}
