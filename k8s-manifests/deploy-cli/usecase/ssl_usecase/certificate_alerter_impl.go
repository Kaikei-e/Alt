package ssl_usecase

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// certificateAlerterImpl implements CertificateAlerter interface
type certificateAlerterImpl struct {
	thresholds AlertThresholds
	logger     *slog.Logger
}

// NewCertificateAlerter creates new CertificateAlerter instance
func NewCertificateAlerter(logger *slog.Logger) CertificateAlerter {
	return &certificateAlerterImpl{
		thresholds: AlertThresholds{
			CriticalDays: 7,  // Alert 7 days before expiry
			WarningDays:  30, // Alert 30 days before expiry
			InfoDays:     90, // Info alert 90 days before expiry
		},
		logger: logger,
	}
}

// SendExpirationAlert sends certificate expiration alert
func (ca *certificateAlerterImpl) SendExpirationAlert(
	ctx context.Context,
	cert *Certificate,
	daysUntilExpiry int,
) error {
	ca.logger.Info("Sending certificate expiration alert",
		"certificate", cert.Name,
		"namespace", cert.Namespace,
		"days_until_expiry", daysUntilExpiry,
		"expires_at", cert.ExpiresAt)

	alertLevel := ca.determineAlertLevel(daysUntilExpiry)
	alertMessage := ca.generateExpirationAlertMessage(cert, daysUntilExpiry, alertLevel)

	// Send alert based on level
	switch alertLevel {
	case "critical":
		return ca.sendCriticalAlert(ctx, alertMessage)
	case "warning":
		return ca.sendWarningAlert(ctx, alertMessage)
	case "info":
		return ca.sendInfoAlert(ctx, alertMessage)
	default:
		return nil
	}
}

// SendRotationAlert sends certificate rotation status alert
func (ca *certificateAlerterImpl) SendRotationAlert(
	ctx context.Context,
	cert *Certificate,
	status string,
) error {
	ca.logger.Info("Sending certificate rotation alert",
		"certificate", cert.Name,
		"namespace", cert.Namespace,
		"status", status)

	alertMessage := ca.generateRotationAlertMessage(cert, status)

	switch status {
	case "success":
		return ca.sendInfoAlert(ctx, alertMessage)
	case "failed":
		return ca.sendCriticalAlert(ctx, alertMessage)
	case "started":
		return ca.sendInfoAlert(ctx, alertMessage)
	default:
		return ca.sendWarningAlert(ctx, alertMessage)
	}
}

// ConfigureAlertThresholds configures alert thresholds
func (ca *certificateAlerterImpl) ConfigureAlertThresholds(thresholds AlertThresholds) error {
	ca.logger.Info("Configuring alert thresholds",
		"critical_days", thresholds.CriticalDays,
		"warning_days", thresholds.WarningDays,
		"info_days", thresholds.InfoDays)

	// Validate thresholds
	if thresholds.CriticalDays >= thresholds.WarningDays {
		return fmt.Errorf("critical threshold (%d) must be less than warning threshold (%d)",
			thresholds.CriticalDays, thresholds.WarningDays)
	}

	if thresholds.WarningDays >= thresholds.InfoDays {
		return fmt.Errorf("warning threshold (%d) must be less than info threshold (%d)",
			thresholds.WarningDays, thresholds.InfoDays)
	}

	ca.thresholds = thresholds
	
	ca.logger.Info("Alert thresholds configured successfully")
	return nil
}

// determineAlertLevel determines alert level based on days until expiry
func (ca *certificateAlerterImpl) determineAlertLevel(daysUntilExpiry int) string {
	switch {
	case daysUntilExpiry <= ca.thresholds.CriticalDays:
		return "critical"
	case daysUntilExpiry <= ca.thresholds.WarningDays:
		return "warning"
	case daysUntilExpiry <= ca.thresholds.InfoDays:
		return "info"
	default:
		return "none"
	}
}

// generateExpirationAlertMessage generates expiration alert message
func (ca *certificateAlerterImpl) generateExpirationAlertMessage(
	cert *Certificate,
	daysUntilExpiry int,
	alertLevel string,
) AlertMessage {
	var severity, subject, body string

	switch alertLevel {
	case "critical":
		severity = "CRITICAL"
		subject = fmt.Sprintf("🚨 CRITICAL: SSL Certificate Expires in %d Days", daysUntilExpiry)
		body = fmt.Sprintf(`
CRITICAL SSL CERTIFICATE EXPIRATION WARNING

Certificate: %s
Namespace: %s
Common Name: %s
Expires: %s
Days Until Expiry: %d

ACTION REQUIRED IMMEDIATELY:
- Certificate will expire in %d days
- Automatic rotation should be scheduled
- Manual intervention may be required

Certificate Details:
- Issuer: %s
- Serial Number: %s
- DNS Names: %v

This is an automated alert from Certificate Lifecycle Manager.
`, cert.Name, cert.Namespace, cert.CommonName, cert.ExpiresAt.Format("2006-01-02 15:04:05 UTC"),
			daysUntilExpiry, daysUntilExpiry, cert.Issuer, cert.SerialNumber, cert.DNSNames)

	case "warning":
		severity = "WARNING"
		subject = fmt.Sprintf("⚠️  WARNING: SSL Certificate Expires in %d Days", daysUntilExpiry)
		body = fmt.Sprintf(`
SSL CERTIFICATE EXPIRATION WARNING

Certificate: %s
Namespace: %s
Common Name: %s
Expires: %s
Days Until Expiry: %d

RECOMMENDED ACTIONS:
- Schedule certificate rotation
- Verify automatic renewal is configured
- Plan maintenance window if needed

Certificate Details:
- Issuer: %s
- Serial Number: %s
- DNS Names: %v

This is an automated alert from Certificate Lifecycle Manager.
`, cert.Name, cert.Namespace, cert.CommonName, cert.ExpiresAt.Format("2006-01-02 15:04:05 UTC"),
			daysUntilExpiry, cert.Issuer, cert.SerialNumber, cert.DNSNames)

	case "info":
		severity = "INFO"
		subject = fmt.Sprintf("ℹ️  INFO: SSL Certificate Expires in %d Days", daysUntilExpiry)
		body = fmt.Sprintf(`
SSL CERTIFICATE EXPIRATION NOTIFICATION

Certificate: %s
Namespace: %s
Common Name: %s
Expires: %s
Days Until Expiry: %d

This is an informational alert. The certificate is approaching expiration but no immediate action is required.

Certificate Details:
- Issuer: %s
- Serial Number: %s
- DNS Names: %v

This is an automated alert from Certificate Lifecycle Manager.
`, cert.Name, cert.Namespace, cert.CommonName, cert.ExpiresAt.Format("2006-01-02 15:04:05 UTC"),
			daysUntilExpiry, cert.Issuer, cert.SerialNumber, cert.DNSNames)
	}

	return AlertMessage{
		Severity:    severity,
		Subject:     subject,
		Body:        body,
		Timestamp:   time.Now(),
		Certificate: cert.Name,
		Namespace:   cert.Namespace,
	}
}

// generateRotationAlertMessage generates rotation alert message
func (ca *certificateAlerterImpl) generateRotationAlertMessage(cert *Certificate, status string) AlertMessage {
	var severity, subject, body string

	switch status {
	case "success":
		severity = "INFO"
		subject = fmt.Sprintf("✅ Certificate Rotation Successful: %s", cert.Name)
		body = fmt.Sprintf(`
SSL CERTIFICATE ROTATION COMPLETED SUCCESSFULLY

Certificate: %s
Namespace: %s
Common Name: %s
New Expiry: %s

The certificate has been successfully rotated and is now valid until %s.

Certificate Details:
- Issuer: %s
- Serial Number: %s
- DNS Names: %v

This is an automated alert from Certificate Lifecycle Manager.
`, cert.Name, cert.Namespace, cert.CommonName, cert.ExpiresAt.Format("2006-01-02 15:04:05 UTC"),
			cert.ExpiresAt.Format("2006-01-02 15:04:05 UTC"), cert.Issuer, cert.SerialNumber, cert.DNSNames)

	case "failed":
		severity = "CRITICAL"
		subject = fmt.Sprintf("🚨 Certificate Rotation Failed: %s", cert.Name)
		body = fmt.Sprintf(`
SSL CERTIFICATE ROTATION FAILED

Certificate: %s
Namespace: %s
Common Name: %s
Current Expiry: %s

IMMEDIATE ACTION REQUIRED:
- Certificate rotation has failed
- Manual intervention is required
- Service availability may be at risk

Certificate Details:
- Issuer: %s
- Serial Number: %s
- DNS Names: %v

Please investigate and resolve the rotation failure immediately.

This is an automated alert from Certificate Lifecycle Manager.
`, cert.Name, cert.Namespace, cert.CommonName, cert.ExpiresAt.Format("2006-01-02 15:04:05 UTC"),
			cert.Issuer, cert.SerialNumber, cert.DNSNames)

	case "started":
		severity = "INFO"
		subject = fmt.Sprintf("🔄 Certificate Rotation Started: %s", cert.Name)
		body = fmt.Sprintf(`
SSL CERTIFICATE ROTATION STARTED

Certificate: %s
Namespace: %s
Common Name: %s
Current Expiry: %s

Certificate rotation process has been initiated. A follow-up notification will be sent upon completion.

Certificate Details:
- Issuer: %s
- Serial Number: %s
- DNS Names: %v

This is an automated alert from Certificate Lifecycle Manager.
`, cert.Name, cert.Namespace, cert.CommonName, cert.ExpiresAt.Format("2006-01-02 15:04:05 UTC"),
			cert.Issuer, cert.SerialNumber, cert.DNSNames)
	}

	return AlertMessage{
		Severity:    severity,
		Subject:     subject,
		Body:        body,
		Timestamp:   time.Now(),
		Certificate: cert.Name,
		Namespace:   cert.Namespace,
	}
}

// sendCriticalAlert sends critical alert
func (ca *certificateAlerterImpl) sendCriticalAlert(ctx context.Context, message AlertMessage) error {
	ca.logger.Error("CRITICAL ALERT",
		"subject", message.Subject,
		"certificate", message.Certificate,
		"namespace", message.Namespace)

	// In real implementation, send to:
	// - PagerDuty/OpsGenie for immediate response
	// - Slack/Teams for team notification
	// - Email to oncall engineers
	// - SMS to critical contacts

	ca.logAlert("CRITICAL", message)
	return nil
}

// sendWarningAlert sends warning alert
func (ca *certificateAlerterImpl) sendWarningAlert(ctx context.Context, message AlertMessage) error {
	ca.logger.Warn("WARNING ALERT",
		"subject", message.Subject,
		"certificate", message.Certificate,
		"namespace", message.Namespace)

	// In real implementation, send to:
	// - Slack/Teams channels
	// - Email to team
	// - JIRA ticket creation

	ca.logAlert("WARNING", message)
	return nil
}

// sendInfoAlert sends info alert
func (ca *certificateAlerterImpl) sendInfoAlert(ctx context.Context, message AlertMessage) error {
	ca.logger.Info("INFO ALERT",
		"subject", message.Subject,
		"certificate", message.Certificate,
		"namespace", message.Namespace)

	// In real implementation, send to:
	// - Monitoring dashboards
	// - Log aggregation systems
	// - Optional team notifications

	ca.logAlert("INFO", message)
	return nil
}

// logAlert logs alert message
func (ca *certificateAlerterImpl) logAlert(level string, message AlertMessage) {
	ca.logger.Info("Certificate alert sent",
		"level", level,
		"certificate", message.Certificate,
		"namespace", message.Namespace,
		"subject", message.Subject,
		"timestamp", message.Timestamp)
}

// AlertMessage represents an alert message
type AlertMessage struct {
	Severity    string
	Subject     string
	Body        string
	Timestamp   time.Time
	Certificate string
	Namespace   string
}