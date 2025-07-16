package helm_port

import (
	"context"
	"time"
)

// HelmPort defines the interface for Helm operations
type HelmPort interface {
	// Template renders chart templates locally
	Template(ctx context.Context, releaseName, chartPath string, options HelmTemplateOptions) (string, error)
	
	// UpgradeInstall installs or upgrades a Helm release
	UpgradeInstall(ctx context.Context, releaseName, chartPath string, options HelmUpgradeOptions) error
	
	// Status returns the status of a Helm release
	Status(ctx context.Context, releaseName, namespace string) (HelmStatus, error)
	
	// List returns list of Helm releases
	List(ctx context.Context, namespace string) ([]HelmRelease, error)
	
	// Uninstall removes a Helm release
	Uninstall(ctx context.Context, releaseName, namespace string) error
}

// HelmTemplateOptions holds options for helm template command
type HelmTemplateOptions struct {
	ValuesFile    string
	Namespace     string
	ImageOverrides map[string]string
	SetValues     map[string]string
}

// HelmUpgradeOptions holds options for helm upgrade command
type HelmUpgradeOptions struct {
	ValuesFile     string
	Namespace      string
	CreateNamespace bool
	Wait           bool
	Timeout        time.Duration
	ImageOverrides map[string]string
	SetValues      map[string]string
}

// HelmStatus represents the status of a Helm release
type HelmStatus struct {
	Name      string
	Namespace string
	Status    string
	Revision  int
	Updated   time.Time
}

// HelmRelease represents a Helm release
type HelmRelease struct {
	Name      string
	Namespace string
	Revision  int
	Status    string
	Chart     string
	AppVersion string
	Updated   time.Time
}