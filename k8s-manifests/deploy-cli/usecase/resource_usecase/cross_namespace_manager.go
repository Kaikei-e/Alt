package resource_usecase

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"deploy-cli/domain"
	"deploy-cli/port/kubectl_port"
)

// CrossNamespaceResourceManager manages resources across multiple namespaces
// 複数のnamespace間でのリソース管理を行う
type CrossNamespaceResourceManager struct {
	kubectl    kubectl_port.KubectlPort
	validator  ResourceConflictDetector
	resolver   ConflictResolver
	tracker    NamespaceResourceTracker
	logger     *slog.Logger
}

// MultiNamespaceDeploymentPlan defines deployment plan across namespaces
type MultiNamespaceDeploymentPlan struct {
	Environment       domain.Environment
	TargetNamespaces  []string
	SharedResources   []SharedResource
	ChartDeployments  []NamespaceChartDeployment
	DependencyGraph   map[string][]string
}

// SharedResource represents a resource shared across namespaces
type SharedResource struct {
	Type        string // "ConfigMap", "Secret", "ClusterRole", etc.
	Name        string
	OwnerChart  string
	Namespaces  []string
	Priority    int // Higher priority wins conflicts
}

// NamespaceChartDeployment represents chart deployment in specific namespace
type NamespaceChartDeployment struct {
	Chart     domain.Chart
	Namespace string
	Priority  int
	DependsOn []string
}

// ResourceConflictDetector detects potential resource conflicts
type ResourceConflictDetector interface {
	DetectConflicts(ctx context.Context, plan MultiNamespaceDeploymentPlan) ([]ResourceConflict, error)
	ValidateOwnership(ctx context.Context, resource SharedResource) error
}

// ConflictResolver resolves resource conflicts
type ConflictResolver interface {
	ResolveConflicts(ctx context.Context, conflicts []ResourceConflict) error
	CreateSharedResources(ctx context.Context, resources []SharedResource) error
}

// NamespaceResourceTracker tracks resource state across namespaces
type NamespaceResourceTracker interface {
	TrackResource(ctx context.Context, namespace, resourceType, name string) error
	GetResourceOwnership(ctx context.Context, namespace, resourceType, name string) (*ResourceOwnership, error)
	ListConflictingResources(ctx context.Context) ([]ResourceConflict, error)
}

// ResourceConflict represents a detected conflict
type ResourceConflict struct {
	ResourceType   string
	ResourceName   string
	ConflictType   string // "ownership", "duplicate", "version"
	SourceChart    string
	TargetChart    string
	Namespaces     []string
	Severity       string // "critical", "warning", "info"
	Resolution     string
}

// ResourceOwnership represents resource ownership information
type ResourceOwnership struct {
	Namespace     string
	ResourceType  string
	ResourceName  string
	OwnerChart    string
	OwnerRelease  string
	CreatedAt     time.Time
	LastModified  time.Time
	Annotations   map[string]string
}

// NewCrossNamespaceResourceManager creates new instance
func NewCrossNamespaceResourceManager(
	kubectl kubectl_port.KubectlPort,
	logger *slog.Logger,
) *CrossNamespaceResourceManager {
	return &CrossNamespaceResourceManager{
		kubectl:   kubectl,
		validator: NewResourceConflictDetector(kubectl, logger),
		resolver:  NewConflictResolver(kubectl, logger),
		tracker:   NewNamespaceResourceTracker(kubectl, logger),
		logger:    logger,
	}
}

// ManageMultiNamespaceDeployment orchestrates deployment across multiple namespaces
func (cnrm *CrossNamespaceResourceManager) ManageMultiNamespaceDeployment(
	ctx context.Context,
	deploymentPlan MultiNamespaceDeploymentPlan,
) error {
	cnrm.logger.Info("Starting multi-namespace deployment management",
		"environment", deploymentPlan.Environment,
		"namespaces", deploymentPlan.TargetNamespaces,
		"charts", len(deploymentPlan.ChartDeployments))

	// Phase 1: 事前競合検出
	conflicts, err := cnrm.validator.DetectConflicts(ctx, deploymentPlan)
	if err != nil {
		return fmt.Errorf("conflict detection failed: %w", err)
	}

	if len(conflicts) > 0 {
		cnrm.logger.Warn("Resource conflicts detected", "count", len(conflicts))
		for _, conflict := range conflicts {
			cnrm.logger.Warn("Detected conflict",
				"type", conflict.ConflictType,
				"resource", conflict.ResourceName,
				"source", conflict.SourceChart,
				"target", conflict.TargetChart,
				"severity", conflict.Severity)
		}

		// Phase 2: 競合解決
		if err := cnrm.resolver.ResolveConflicts(ctx, conflicts); err != nil {
			return fmt.Errorf("conflict resolution failed: %w", err)
		}
	}

	// Phase 3: 共有リソースの事前デプロイ
	if err := cnrm.resolver.CreateSharedResources(ctx, deploymentPlan.SharedResources); err != nil {
		return fmt.Errorf("shared resource creation failed: %w", err)
	}

	// Phase 4: namespace間依存関係の解決
	if err := cnrm.resolveDependencies(ctx, deploymentPlan); err != nil {
		return fmt.Errorf("dependency resolution failed: %w", err)
	}

	cnrm.logger.Info("Multi-namespace deployment management completed successfully")
	return nil
}

// resolveDependencies resolves dependencies between namespace deployments
func (cnrm *CrossNamespaceResourceManager) resolveDependencies(
	ctx context.Context,
	plan MultiNamespaceDeploymentPlan,
) error {
	cnrm.logger.Info("Resolving namespace dependencies",
		"dependency_graph", plan.DependencyGraph)

	// Topological sort implementation for dependency resolution
	// 依存関係のトポロジカルソートによる解決順序決定
	sortedCharts, err := cnrm.topologicalSort(plan.ChartDeployments, plan.DependencyGraph)
	if err != nil {
		return fmt.Errorf("dependency sorting failed: %w", err)
	}

	cnrm.logger.Info("Dependency resolution completed",
		"deployment_order", len(sortedCharts))

	return nil
}

// topologicalSort sorts chart deployments based on dependencies
func (cnrm *CrossNamespaceResourceManager) topologicalSort(
	deployments []NamespaceChartDeployment,
	dependencies map[string][]string,
) ([]NamespaceChartDeployment, error) {
	// Simple topological sort implementation
	// TODO: Implement full topological sort algorithm
	return deployments, nil
}

// ValidateMultiNamespaceState validates the current state across namespaces
func (cnrm *CrossNamespaceResourceManager) ValidateMultiNamespaceState(
	ctx context.Context,
	namespaces []string,
) error {
	cnrm.logger.Info("Validating multi-namespace state", "namespaces", namespaces)

	conflicts, err := cnrm.tracker.ListConflictingResources(ctx)
	if err != nil {
		return fmt.Errorf("failed to list conflicting resources: %w", err)
	}

	if len(conflicts) > 0 {
		cnrm.logger.Warn("State validation found conflicts", "count", len(conflicts))
		for _, conflict := range conflicts {
			cnrm.logger.Warn("State conflict detected",
				"resource", conflict.ResourceName,
				"type", conflict.ConflictType,
				"severity", conflict.Severity)
		}
		return fmt.Errorf("multi-namespace state validation failed: %d conflicts found", len(conflicts))
	}

	cnrm.logger.Info("Multi-namespace state validation passed")
	return nil
}