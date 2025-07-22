package ssl_usecase

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// CertificateLifecycleManager manages SSL certificate lifecycle across namespaces
// SSL証明書のライフサイクルをnamespace横断で管理する
type CertificateLifecycleManager struct {
	certManager CertManager
	rotator     CertificateRotator
	distributor CrossNamespaceCertDistributor
	validator   CertificateValidator
	alerter     CertificateAlerter
	logger      *slog.Logger
}

// CertManager handles certificate generation and management
type CertManager interface {
	GenerateCertificate(ctx context.Context, spec CertificateSpec) (*Certificate, error)
	GetCertificate(ctx context.Context, namespace, name string) (*Certificate, error)
	ListCertificates(ctx context.Context, namespace string) ([]*Certificate, error)
	DeleteCertificate(ctx context.Context, namespace, name string) error
}

// CertificateRotator handles automatic certificate rotation
type CertificateRotator interface {
	RotateCertificate(ctx context.Context, cert *Certificate) (*Certificate, error)
	ScheduleRotation(ctx context.Context, cert *Certificate, rotateAt time.Time) error
	GetRotationSchedule(ctx context.Context) ([]RotationSchedule, error)
}

// CrossNamespaceCertDistributor distributes certificates across namespaces
type CrossNamespaceCertDistributor interface {
	DistributeCertificate(ctx context.Context, cert *Certificate, targetNamespaces []string) error
	SyncCertificateAcrossNamespaces(ctx context.Context, certName string) error
	ValidateDistribution(ctx context.Context, certName string) error
}

// CertificateValidator validates certificate integrity and expiration
type CertificateValidator interface {
	ValidateCertificate(ctx context.Context, cert *Certificate) error
	CheckExpiration(ctx context.Context, cert *Certificate) (*ExpirationStatus, error)
	PerformHealthCheck(ctx context.Context, namespace, certName string) error
}

// CertificateAlerter handles certificate expiration alerts
type CertificateAlerter interface {
	SendExpirationAlert(ctx context.Context, cert *Certificate, daysUntilExpiry int) error
	SendRotationAlert(ctx context.Context, cert *Certificate, status string) error
	ConfigureAlertThresholds(thresholds AlertThresholds) error
}

// Certificate represents an SSL certificate
type Certificate struct {
	Name         string
	Namespace    string
	CommonName   string
	DNSNames     []string
	Certificate  []byte
	PrivateKey   []byte
	CACert       []byte
	IssuedAt     time.Time
	ExpiresAt    time.Time
	Issuer       string
	SerialNumber string
	Fingerprint  string
	KeyUsage     []string
}

// CertificateSpec defines certificate generation requirements
type CertificateSpec struct {
	Name        string
	Namespace   string
	CommonName  string
	DNSNames    []string
	IPAddresses []string
	ValidFor    time.Duration
	KeySize     int
	Algorithm   string
	Issuer      string
}

// RotationSchedule represents a scheduled certificate rotation
type RotationSchedule struct {
	CertificateName string
	Namespace       string
	CurrentExpiry   time.Time
	ScheduledRotation time.Time
	RotationReason  string
	Status          string
}

// ExpirationStatus contains certificate expiration information
type ExpirationStatus struct {
	IsExpired       bool
	DaysUntilExpiry int
	ExpiresAt       time.Time
	RiskLevel       string // "low", "medium", "high", "critical"
}

// AlertThresholds defines when to send alerts
type AlertThresholds struct {
	CriticalDays int // Send critical alert
	WarningDays  int // Send warning alert
	InfoDays     int // Send info alert
}

// NewCertificateLifecycleManager creates new instance
func NewCertificateLifecycleManager(
	certManager CertManager,
	rotator CertificateRotator,
	distributor CrossNamespaceCertDistributor,
	validator CertificateValidator,
	alerter CertificateAlerter,
	logger *slog.Logger,
) *CertificateLifecycleManager {
	return &CertificateLifecycleManager{
		certManager: certManager,
		rotator:     rotator,
		distributor: distributor,
		validator:   validator,
		alerter:     alerter,
		logger:      logger,
	}
}

// ManageCertificateLifecycle orchestrates complete certificate lifecycle
func (clm *CertificateLifecycleManager) ManageCertificateLifecycle(
	ctx context.Context,
	namespaces []string,
) error {
	clm.logger.Info("Starting certificate lifecycle management",
		"namespaces", namespaces)

	// Phase 1: Certificate health check across all namespaces
	if err := clm.performHealthChecks(ctx, namespaces); err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}

	// Phase 2: Check for certificates needing rotation
	if err := clm.checkAndScheduleRotations(ctx, namespaces); err != nil {
		return fmt.Errorf("rotation scheduling failed: %w", err)
	}

	// Phase 3: Synchronize certificates across namespaces
	if err := clm.synchronizeCertificates(ctx, namespaces); err != nil {
		return fmt.Errorf("certificate synchronization failed: %w", err)
	}

	// Phase 4: Send expiration alerts
	if err := clm.processExpirationAlerts(ctx, namespaces); err != nil {
		return fmt.Errorf("alert processing failed: %w", err)
	}

	clm.logger.Info("Certificate lifecycle management completed successfully")
	return nil
}

// CreateAndDistributeCertificate creates certificate and distributes to target namespaces
func (clm *CertificateLifecycleManager) CreateAndDistributeCertificate(
	ctx context.Context,
	spec CertificateSpec,
	targetNamespaces []string,
) (*Certificate, error) {
	clm.logger.Info("Creating and distributing certificate",
		"name", spec.Name,
		"namespace", spec.Namespace,
		"targets", targetNamespaces)

	// Generate certificate
	cert, err := clm.certManager.GenerateCertificate(ctx, spec)
	if err != nil {
		return nil, fmt.Errorf("certificate generation failed: %w", err)
	}

	// Validate generated certificate
	if err := clm.validator.ValidateCertificate(ctx, cert); err != nil {
		return nil, fmt.Errorf("certificate validation failed: %w", err)
	}

	// Distribute to target namespaces
	if err := clm.distributor.DistributeCertificate(ctx, cert, targetNamespaces); err != nil {
		return nil, fmt.Errorf("certificate distribution failed: %w", err)
	}

	// Schedule automatic rotation
	rotateAt := cert.ExpiresAt.Add(-30 * 24 * time.Hour) // 30 days before expiry
	if err := clm.rotator.ScheduleRotation(ctx, cert, rotateAt); err != nil {
		clm.logger.Warn("Failed to schedule rotation", "error", err)
	}

	clm.logger.Info("Certificate created and distributed successfully",
		"name", cert.Name,
		"expires_at", cert.ExpiresAt,
		"distributed_to", targetNamespaces)

	return cert, nil
}

// performHealthChecks validates certificates across all namespaces
func (clm *CertificateLifecycleManager) performHealthChecks(
	ctx context.Context,
	namespaces []string,
) error {
	clm.logger.Info("Performing certificate health checks", "namespaces", namespaces)

	for _, namespace := range namespaces {
		certs, err := clm.certManager.ListCertificates(ctx, namespace)
		if err != nil {
			clm.logger.Warn("Failed to list certificates",
				"namespace", namespace,
				"error", err)
			continue
		}

		for _, cert := range certs {
			if err := clm.validator.PerformHealthCheck(ctx, namespace, cert.Name); err != nil {
				clm.logger.Warn("Certificate health check failed",
					"namespace", namespace,
					"certificate", cert.Name,
					"error", err)
			}
		}
	}

	return nil
}

// checkAndScheduleRotations checks for certificates needing rotation
func (clm *CertificateLifecycleManager) checkAndScheduleRotations(
	ctx context.Context,
	namespaces []string,
) error {
	clm.logger.Info("Checking certificates for rotation", "namespaces", namespaces)

	for _, namespace := range namespaces {
		certs, err := clm.certManager.ListCertificates(ctx, namespace)
		if err != nil {
			continue
		}

		for _, cert := range certs {
			status, err := clm.validator.CheckExpiration(ctx, cert)
			if err != nil {
				continue
			}

			// Schedule rotation if certificate expires soon
			if status.DaysUntilExpiry <= 30 {
				rotateAt := time.Now().Add(24 * time.Hour) // Rotate tomorrow
				if err := clm.rotator.ScheduleRotation(ctx, cert, rotateAt); err != nil {
					clm.logger.Warn("Failed to schedule rotation",
						"certificate", cert.Name,
						"error", err)
				} else {
					clm.logger.Info("Scheduled certificate rotation",
						"certificate", cert.Name,
						"namespace", namespace,
						"rotate_at", rotateAt)
				}
			}
		}
	}

	return nil
}

// synchronizeCertificates ensures certificates are consistent across namespaces
func (clm *CertificateLifecycleManager) synchronizeCertificates(
	ctx context.Context,
	namespaces []string,
) error {
	clm.logger.Info("Synchronizing certificates across namespaces")

	// Get list of shared certificates
	sharedCerts := []string{"server-ssl-secret", "ca-secret", "client-ssl-secret"}

	for _, certName := range sharedCerts {
		if err := clm.distributor.SyncCertificateAcrossNamespaces(ctx, certName); err != nil {
			clm.logger.Warn("Failed to sync certificate",
				"certificate", certName,
				"error", err)
		}
	}

	return nil
}

// processExpirationAlerts sends alerts for certificates approaching expiration
func (clm *CertificateLifecycleManager) processExpirationAlerts(
	ctx context.Context,
	namespaces []string,
) error {
	clm.logger.Info("Processing expiration alerts")

	for _, namespace := range namespaces {
		certs, err := clm.certManager.ListCertificates(ctx, namespace)
		if err != nil {
			continue
		}

		for _, cert := range certs {
			status, err := clm.validator.CheckExpiration(ctx, cert)
			if err != nil {
				continue
			}

			// Send alerts based on risk level
			switch status.RiskLevel {
			case "critical":
				if err := clm.alerter.SendExpirationAlert(ctx, cert, status.DaysUntilExpiry); err != nil {
					clm.logger.Warn("Failed to send critical alert", "error", err)
				}
			case "high":
				if err := clm.alerter.SendExpirationAlert(ctx, cert, status.DaysUntilExpiry); err != nil {
					clm.logger.Warn("Failed to send high alert", "error", err)
				}
			}
		}
	}

	return nil
}

// RotateExpiredCertificates rotates all expired or soon-to-expire certificates
func (clm *CertificateLifecycleManager) RotateExpiredCertificates(
	ctx context.Context,
	namespaces []string,
) error {
	clm.logger.Info("Rotating expired certificates", "namespaces", namespaces)

	rotated := 0
	for _, namespace := range namespaces {
		certs, err := clm.certManager.ListCertificates(ctx, namespace)
		if err != nil {
			continue
		}

		for _, cert := range certs {
			status, err := clm.validator.CheckExpiration(ctx, cert)
			if err != nil {
				continue
			}

			// Rotate if critical or expired
			if status.RiskLevel == "critical" || status.IsExpired {
				newCert, err := clm.rotator.RotateCertificate(ctx, cert)
				if err != nil {
					clm.logger.Error("Certificate rotation failed",
						"certificate", cert.Name,
						"namespace", namespace,
						"error", err)
					continue
				}

				// Send rotation success alert
				if err := clm.alerter.SendRotationAlert(ctx, newCert, "success"); err != nil {
					clm.logger.Warn("Failed to send rotation alert", "error", err)
				}

				rotated++
				clm.logger.Info("Certificate rotated successfully",
					"certificate", cert.Name,
					"namespace", namespace,
					"new_expiry", newCert.ExpiresAt)
			}
		}
	}

	clm.logger.Info("Certificate rotation completed", "rotated_count", rotated)
	return nil
}

// GetCertificateReport generates comprehensive certificate status report
func (clm *CertificateLifecycleManager) GetCertificateReport(
	ctx context.Context,
	namespaces []string,
) (*CertificateReport, error) {
	report := &CertificateReport{
		GeneratedAt: time.Now(),
		Namespaces:  namespaces,
	}

	for _, namespace := range namespaces {
		certs, err := clm.certManager.ListCertificates(ctx, namespace)
		if err != nil {
			continue
		}

		for _, cert := range certs {
			status, err := clm.validator.CheckExpiration(ctx, cert)
			if err != nil {
				continue
			}

			certStatus := CertificateStatus{
				Certificate: cert,
				Expiration:  status,
				Namespace:   namespace,
			}

			report.Certificates = append(report.Certificates, certStatus)

			// Update summary statistics
			switch status.RiskLevel {
			case "critical":
				report.Summary.CriticalCount++
			case "high":
				report.Summary.HighRiskCount++
			case "medium":
				report.Summary.MediumRiskCount++
			default:
				report.Summary.HealthyCount++
			}
		}
	}

	report.Summary.TotalCount = len(report.Certificates)
	return report, nil
}

// CertificateReport contains comprehensive certificate status
type CertificateReport struct {
	GeneratedAt   time.Time
	Namespaces    []string
	Certificates  []CertificateStatus
	Summary       CertificateSummary
}

// CertificateStatus represents status of a single certificate
type CertificateStatus struct {
	Certificate *Certificate
	Expiration  *ExpirationStatus
	Namespace   string
}

// CertificateSummary provides summary statistics
type CertificateSummary struct {
	TotalCount      int
	HealthyCount    int
	MediumRiskCount int
	HighRiskCount   int
	CriticalCount   int
}