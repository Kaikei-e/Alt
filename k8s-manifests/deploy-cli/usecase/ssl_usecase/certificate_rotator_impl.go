package ssl_usecase

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// certificateRotatorImpl implements CertificateRotator interface
type certificateRotatorImpl struct {
	certManager CertManager
	scheduler   *rotationScheduler
	logger      *slog.Logger
}

// rotationScheduler manages scheduled certificate rotations
type rotationScheduler struct {
	schedules map[string]RotationSchedule
	mutex     sync.RWMutex
	ticker    *time.Ticker
	stopCh    chan struct{}
	rotator   *certificateRotatorImpl
}

// NewCertificateRotator creates new CertificateRotator instance
func NewCertificateRotator(certManager CertManager, logger *slog.Logger) CertificateRotator {
	rotator := &certificateRotatorImpl{
		certManager: certManager,
		logger:      logger,
	}

	scheduler := &rotationScheduler{
		schedules: make(map[string]RotationSchedule),
		ticker:    time.NewTicker(1 * time.Hour), // Check every hour
		stopCh:    make(chan struct{}),
		rotator:   rotator,
	}

	rotator.scheduler = scheduler

	// Start background scheduler
	go scheduler.run()

	return rotator
}

// RotateCertificate performs immediate certificate rotation
func (cr *certificateRotatorImpl) RotateCertificate(ctx context.Context, cert *Certificate) (*Certificate, error) {
	cr.logger.Info("Rotating certificate",
		"name", cert.Name,
		"namespace", cert.Namespace,
		"current_expiry", cert.ExpiresAt)

	// Create new certificate spec based on existing certificate
	spec := CertificateSpec{
		Name:        cert.Name,
		Namespace:   cert.Namespace,
		CommonName:  cert.CommonName,
		DNSNames:    cert.DNSNames,
		ValidFor:    365 * 24 * time.Hour, // 1 year validity
		KeySize:     2048,
		Algorithm:   "RSA",
		Issuer:      "certificate-lifecycle-manager",
	}

	// Generate new certificate
	newCert, err := cr.certManager.GenerateCertificate(ctx, spec)
	if err != nil {
		return nil, fmt.Errorf("failed to generate new certificate: %w", err)
	}

	// Update rotation schedule entry to mark as completed
	cr.scheduler.markRotationCompleted(cert.Name, cert.Namespace)

	cr.logger.Info("Certificate rotation completed",
		"name", newCert.Name,
		"namespace", newCert.Namespace,
		"new_expiry", newCert.ExpiresAt,
		"validity_duration", newCert.ExpiresAt.Sub(newCert.IssuedAt))

	return newCert, nil
}

// ScheduleRotation schedules certificate rotation at specified time
func (cr *certificateRotatorImpl) ScheduleRotation(ctx context.Context, cert *Certificate, rotateAt time.Time) error {
	cr.logger.Info("Scheduling certificate rotation",
		"name", cert.Name,
		"namespace", cert.Namespace,
		"rotate_at", rotateAt,
		"current_expiry", cert.ExpiresAt)

	schedule := RotationSchedule{
		CertificateName:   cert.Name,
		Namespace:         cert.Namespace,
		CurrentExpiry:     cert.ExpiresAt,
		ScheduledRotation: rotateAt,
		RotationReason:    cr.determineRotationReason(cert, rotateAt),
		Status:            "scheduled",
	}

	cr.scheduler.addSchedule(schedule)

	cr.logger.Info("Certificate rotation scheduled",
		"name", cert.Name,
		"namespace", cert.Namespace,
		"reason", schedule.RotationReason)

	return nil
}

// GetRotationSchedule returns all scheduled rotations
func (cr *certificateRotatorImpl) GetRotationSchedule(ctx context.Context) ([]RotationSchedule, error) {
	cr.logger.Debug("Getting rotation schedule")

	schedules := cr.scheduler.getAllSchedules()

	cr.logger.Debug("Retrieved rotation schedule",
		"count", len(schedules))

	return schedules, nil
}

// determineRotationReason determines why certificate needs rotation
func (cr *certificateRotatorImpl) determineRotationReason(cert *Certificate, rotateAt time.Time) string {
	now := time.Now()
	daysUntilExpiry := int(cert.ExpiresAt.Sub(now).Hours() / 24)

	switch {
	case daysUntilExpiry <= 0:
		return "expired"
	case daysUntilExpiry <= 7:
		return "expires_within_week"
	case daysUntilExpiry <= 30:
		return "expires_within_month"
	case rotateAt.Before(cert.ExpiresAt.Add(-30*24*time.Hour)):
		return "proactive_rotation"
	default:
		return "scheduled_maintenance"
	}
}

// rotationScheduler methods

// run starts the background rotation scheduler
func (rs *rotationScheduler) run() {
	rs.rotator.logger.Info("Starting certificate rotation scheduler")

	for {
		select {
		case <-rs.ticker.C:
			rs.processScheduledRotations()
		case <-rs.stopCh:
			rs.rotator.logger.Info("Stopping certificate rotation scheduler")
			return
		}
	}
}

// processScheduledRotations processes due rotations
func (rs *rotationScheduler) processScheduledRotations() {
	rs.mutex.RLock()
	defer rs.mutex.RUnlock()

	now := time.Now()
	
	for key, schedule := range rs.schedules {
		if schedule.Status == "scheduled" && now.After(schedule.ScheduledRotation) {
			rs.rotator.logger.Info("Processing scheduled rotation",
				"certificate", schedule.CertificateName,
				"namespace", schedule.Namespace,
				"scheduled_time", schedule.ScheduledRotation)

			go rs.executeRotation(key, schedule)
		}
	}
}

// executeRotation executes a scheduled rotation
func (rs *rotationScheduler) executeRotation(key string, schedule RotationSchedule) {
	ctx := context.Background()

	// Mark as in progress
	rs.updateScheduleStatus(key, "in_progress")

	// Get current certificate
	cert, err := rs.rotator.certManager.GetCertificate(ctx, schedule.Namespace, schedule.CertificateName)
	if err != nil {
		rs.rotator.logger.Error("Failed to get certificate for rotation",
			"certificate", schedule.CertificateName,
			"namespace", schedule.Namespace,
			"error", err)
		rs.updateScheduleStatus(key, "failed")
		return
	}

	// Perform rotation
	newCert, err := rs.rotator.RotateCertificate(ctx, cert)
	if err != nil {
		rs.rotator.logger.Error("Certificate rotation failed",
			"certificate", schedule.CertificateName,
			"namespace", schedule.Namespace,
			"error", err)
		rs.updateScheduleStatus(key, "failed")
		return
	}

	rs.rotator.logger.Info("Scheduled certificate rotation completed",
		"certificate", newCert.Name,
		"namespace", newCert.Namespace,
		"new_expiry", newCert.ExpiresAt)

	rs.updateScheduleStatus(key, "completed")
}

// addSchedule adds a new rotation schedule
func (rs *rotationScheduler) addSchedule(schedule RotationSchedule) {
	rs.mutex.Lock()
	defer rs.mutex.Unlock()

	key := fmt.Sprintf("%s/%s", schedule.Namespace, schedule.CertificateName)
	rs.schedules[key] = schedule
}

// updateScheduleStatus updates schedule status
func (rs *rotationScheduler) updateScheduleStatus(key, status string) {
	rs.mutex.Lock()
	defer rs.mutex.Unlock()

	if schedule, exists := rs.schedules[key]; exists {
		schedule.Status = status
		rs.schedules[key] = schedule
	}
}

// markRotationCompleted marks rotation as completed
func (rs *rotationScheduler) markRotationCompleted(certName, namespace string) {
	key := fmt.Sprintf("%s/%s", namespace, certName)
	rs.updateScheduleStatus(key, "completed")
}

// getAllSchedules returns all schedules
func (rs *rotationScheduler) getAllSchedules() []RotationSchedule {
	rs.mutex.RLock()
	defer rs.mutex.RUnlock()

	schedules := make([]RotationSchedule, 0, len(rs.schedules))
	for _, schedule := range rs.schedules {
		schedules = append(schedules, schedule)
	}

	return schedules
}

// stop stops the rotation scheduler
func (rs *rotationScheduler) stop() {
	close(rs.stopCh)
	rs.ticker.Stop()
}