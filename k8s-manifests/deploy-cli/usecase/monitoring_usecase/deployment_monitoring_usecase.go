package monitoring_usecase

import (
	"context"
	"fmt"
	"time"

	"deploy-cli/domain"
	"deploy-cli/port/kubectl_port"
	"deploy-cli/port/logger_port"
	"deploy-cli/usecase/recovery_usecase"
)

// DeploymentMonitoringUsecase handles real-time monitoring and automatic recovery
type DeploymentMonitoringUsecase struct {
	kubectlPort   kubectl_port.KubectlPort
	autoRecovery  *recovery_usecase.AutoRecoveryUsecase
	healthChecker *HealthCheckerUsecase
	logger        logger_port.LoggerPort
	metrics       *MetricsCollector
}

// NewDeploymentMonitoringUsecase creates a new deployment monitoring usecase
func NewDeploymentMonitoringUsecase(
	kubectlPort kubectl_port.KubectlPort,
	autoRecovery *recovery_usecase.AutoRecoveryUsecase,
	healthChecker *HealthCheckerUsecase,
	logger logger_port.LoggerPort,
	metrics *MetricsCollector,
) *DeploymentMonitoringUsecase {
	return &DeploymentMonitoringUsecase{
		kubectlPort:   kubectlPort,
		autoRecovery:  autoRecovery,
		healthChecker: healthChecker,
		logger:        logger,
		metrics:       metrics,
	}
}

// HealthCheckerUsecase represents health checking functionality
type HealthCheckerUsecase struct {
	kubectlPort kubectl_port.KubectlPort
	logger      logger_port.LoggerPort
}

// NewHealthCheckerUsecase creates a new health checker usecase
func NewHealthCheckerUsecase(
	kubectlPort kubectl_port.KubectlPort,
	logger logger_port.LoggerPort,
) *HealthCheckerUsecase {
	return &HealthCheckerUsecase{
		kubectlPort: kubectlPort,
		logger:      logger,
	}
}

// MetricsCollector represents metrics collection functionality
type MetricsCollector struct {
	logger logger_port.LoggerPort
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(logger logger_port.LoggerPort) *MetricsCollector {
	return &MetricsCollector{
		logger: logger,
	}
}

// HealthReport represents a comprehensive health report
type HealthReport struct {
	OverallStatus   string                  `json:"overall_status"`
	Timestamp       time.Time               `json:"timestamp"`
	Environment     string                  `json:"environment"`
	NamespaceHealth []NamespaceHealthStatus `json:"namespace_health"`
	ServiceHealth   []ServiceHealthStatus   `json:"service_health"`
	StorageHealth   []StorageHealthStatus   `json:"storage_health"`
	NetworkHealth   NetworkHealthStatus     `json:"network_health"`
	Issues          []HealthIssue           `json:"issues"`
	Recommendations []string                `json:"recommendations"`
}

// NamespaceHealthStatus represents namespace health status
type NamespaceHealthStatus struct {
	Name      string   `json:"name"`
	Status    string   `json:"status"`
	PodCount  int      `json:"pod_count"`
	ReadyPods int      `json:"ready_pods"`
	Issues    []string `json:"issues"`
}

// ServiceHealthStatus represents service health status
type ServiceHealthStatus struct {
	Name        string    `json:"name"`
	Namespace   string    `json:"namespace"`
	Type        string    `json:"type"`
	Status      string    `json:"status"`
	Replicas    int       `json:"replicas"`
	ReadyReps   int       `json:"ready_replicas"`
	Restarts    int       `json:"restarts"`
	LastRestart time.Time `json:"last_restart,omitempty"`
	Issues      []string  `json:"issues"`
}

// StorageHealthStatus represents storage health status
type StorageHealthStatus struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Status   string   `json:"status"`
	Capacity string   `json:"capacity"`
	Usage    string   `json:"usage,omitempty"`
	Issues   []string `json:"issues"`
}

// NetworkHealthStatus represents network health status
type NetworkHealthStatus struct {
	IngressStatus  string   `json:"ingress_status"`
	ServiceMeshOK  bool     `json:"service_mesh_ok"`
	ConnectivityOK bool     `json:"connectivity_ok"`
	ExternalAccess bool     `json:"external_access"`
	Issues         []string `json:"issues"`
}

// HealthIssue represents a detected health issue
type HealthIssue struct {
	Type        string    `json:"type"`
	Severity    string    `json:"severity"`
	Component   string    `json:"component"`
	Description string    `json:"description"`
	Error       error     `json:"-"`
	DetectedAt  time.Time `json:"detected_at"`
	Recoverable bool      `json:"recoverable"`
}

// MonitoringConfig represents monitoring configuration
type MonitoringConfig struct {
	Interval            time.Duration `json:"interval"`
	AutoRecoveryEnabled bool          `json:"auto_recovery_enabled"`
	NotificationEnabled bool          `json:"notification_enabled"`
	MaxRetries          int           `json:"max_retries"`
	CriticalThreshold   int           `json:"critical_threshold"`
	WarningThreshold    int           `json:"warning_threshold"`
}

// ContinuousMonitoring 継続的監視と自動修復
func (u *DeploymentMonitoringUsecase) ContinuousMonitoring(ctx context.Context, options *domain.DeploymentOptions) {
	config := u.getMonitoringConfig(options.Environment)

	u.logger.InfoWithContext("継続的監視を開始", map[string]interface{}{
		"environment":   options.Environment.String(),
		"interval":      config.Interval.String(),
		"auto_recovery": config.AutoRecoveryEnabled,
	})

	ticker := time.NewTicker(config.Interval)
	defer ticker.Stop()

	consecutiveFailures := 0
	lastHealthy := time.Now()

	for {
		select {
		case <-ticker.C:
			err := u.performMonitoringCycle(ctx, options, config)
			if err != nil {
				consecutiveFailures++
				u.logger.ErrorWithContext("監視サイクルでエラーが発生", map[string]interface{}{
					"error":                err.Error(),
					"consecutive_failures": consecutiveFailures,
				})

				// Escalate if too many consecutive failures
				if consecutiveFailures >= config.CriticalThreshold {
					u.handleCriticalFailure(ctx, options, consecutiveFailures)
				}
			} else {
				if consecutiveFailures > 0 {
					u.logger.InfoWithContext("監視状態が回復しました", map[string]interface{}{
						"consecutive_failures": consecutiveFailures,
						"downtime":             time.Since(lastHealthy).String(),
					})
				}
				consecutiveFailures = 0
				lastHealthy = time.Now()
			}

		case <-ctx.Done():
			u.logger.InfoWithContext("継続的監視を終了", map[string]interface{}{
				"reason": "context cancelled",
			})
			return
		}
	}
}

// performMonitoringCycle performs a single monitoring cycle
func (u *DeploymentMonitoringUsecase) performMonitoringCycle(ctx context.Context, options *domain.DeploymentOptions, config MonitoringConfig) error {
	u.logger.DebugWithContext("監視サイクル開始", map[string]interface{}{
		"environment": options.Environment.String(),
	})

	// 1. Comprehensive health check
	healthReport, err := u.healthChecker.ComprehensiveHealthCheck(ctx, options)
	if err != nil {
		return fmt.Errorf("ヘルスチェックに失敗: %w", err)
	}

	// 2. Record metrics
	u.recordHealthMetrics(healthReport)

	// 3. Detect issues
	issues := u.detectIssues(healthReport)
	if len(issues) == 0 {
		u.metrics.RecordHealthyState()
		u.logger.DebugWithContext("監視サイクル完了（問題なし）", map[string]interface{}{
			"overall_status": healthReport.OverallStatus,
		})
		return nil
	}

	// 4. Categorize issues by severity
	criticalIssues, warnings := u.categorizeIssues(issues)

	u.logger.InfoWithContext("問題を検出しました", map[string]interface{}{
		"total_issues":    len(issues),
		"critical_issues": len(criticalIssues),
		"warnings":        len(warnings),
	})

	// 5. Auto-recovery for critical issues if enabled
	if config.AutoRecoveryEnabled && len(criticalIssues) > 0 {
		err := u.performAutoRecovery(ctx, criticalIssues)
		if err != nil {
			u.logger.ErrorWithContext("自動復旧でエラーが発生", map[string]interface{}{
				"error": err.Error(),
			})
		}
	}

	// 6. Handle warnings
	u.handleWarnings(ctx, warnings)

	return nil
}

// ComprehensiveHealthCheck performs a comprehensive health check
func (h *HealthCheckerUsecase) ComprehensiveHealthCheck(ctx context.Context, options *domain.DeploymentOptions) (*HealthReport, error) {
	report := &HealthReport{
		Timestamp:   time.Now(),
		Environment: options.Environment.String(),
		Issues:      []HealthIssue{},
	}

	h.logger.DebugWithContext("包括的ヘルスチェック開始", map[string]interface{}{
		"environment": options.Environment.String(),
	})

	// 1. Check namespace health
	report.NamespaceHealth = h.checkNamespaceHealth(ctx, options.Environment)

	// 2. Check service health
	report.ServiceHealth = h.checkServiceHealth(ctx, options.Environment)

	// 3. Check storage health
	report.StorageHealth = h.checkStorageHealth(ctx)

	// 4. Check network health
	report.NetworkHealth = h.checkNetworkHealth(ctx, options.Environment)

	// 5. Determine overall status
	report.OverallStatus = h.calculateOverallHealthStatus(report)

	// 6. Generate recommendations
	report.Recommendations = h.generateHealthRecommendations(report)

	return report, nil
}

// checkNamespaceHealth checks the health of required namespaces
func (h *HealthCheckerUsecase) checkNamespaceHealth(ctx context.Context, env domain.Environment) []NamespaceHealthStatus {
	var healthStatuses []NamespaceHealthStatus
	requiredNamespaces := domain.GetNamespacesForEnvironment(env)

	for _, nsName := range requiredNamespaces {
		status := NamespaceHealthStatus{
			Name:   nsName,
			Status: "checking",
			Issues: []string{},
		}

		// Get pods in namespace
		pods, err := h.kubectlPort.GetPods(ctx, nsName, "")
		if err != nil {
			status.Status = "error"
			status.Issues = append(status.Issues, fmt.Sprintf("Pod取得エラー: %v", err))
		} else {
			status.PodCount = len(pods)
			readyCount := 0

			for _, pod := range pods {
				if pod.Status == "Running" && pod.Ready == "True" {
					readyCount++
				} else if pod.Status == "Failed" || pod.Status == "Error" {
					status.Issues = append(status.Issues, fmt.Sprintf("Pod %s が異常状態: %s", pod.Name, pod.Status))
				}
			}

			status.ReadyPods = readyCount
			if readyCount == status.PodCount && status.PodCount > 0 {
				status.Status = "healthy"
			} else if readyCount > 0 {
				status.Status = "partially_healthy"
			} else {
				status.Status = "unhealthy"
			}
		}

		healthStatuses = append(healthStatuses, status)
	}

	return healthStatuses
}

// checkServiceHealth checks the health of deployed services
func (h *HealthCheckerUsecase) checkServiceHealth(ctx context.Context, env domain.Environment) []ServiceHealthStatus {
	var healthStatuses []ServiceHealthStatus
	requiredNamespaces := domain.GetNamespacesForEnvironment(env)

	for _, nsName := range requiredNamespaces {
		// Check deployments
		deployments, err := h.kubectlPort.GetDeployments(ctx, nsName)
		if err != nil {
			h.logger.WarnWithContext("Deployment取得に失敗", map[string]interface{}{
				"namespace": nsName,
				"error":     err.Error(),
			})
			continue
		}

		for _, deployment := range deployments {
			status := ServiceHealthStatus{
				Name:      deployment.Name,
				Namespace: nsName,
				Type:      "Deployment",
				Issues:    []string{},
			}

			// Parse replica information
			status.Replicas = deployment.Replicas
			status.ReadyReps = deployment.ReadyReplicas

			if status.ReadyReps == status.Replicas && status.Replicas > 0 {
				status.Status = "healthy"
			} else if status.ReadyReps > 0 {
				status.Status = "partially_healthy"
				status.Issues = append(status.Issues, fmt.Sprintf("一部のレプリカが準備完了していません (%d/%d)", status.ReadyReps, status.Replicas))
			} else {
				status.Status = "unhealthy"
				status.Issues = append(status.Issues, "利用可能なレプリカがありません")
			}

			healthStatuses = append(healthStatuses, status)
		}

		// Check stateful sets
		statefulSets, err := h.kubectlPort.GetStatefulSets(ctx, nsName)
		if err != nil {
			h.logger.WarnWithContext("StatefulSet取得に失敗", map[string]interface{}{
				"namespace": nsName,
				"error":     err.Error(),
			})
			continue
		}

		for _, sts := range statefulSets {
			status := ServiceHealthStatus{
				Name:      sts.Name,
				Namespace: nsName,
				Type:      "StatefulSet",
				Issues:    []string{},
			}

			status.Replicas = sts.Replicas
			status.ReadyReps = sts.ReadyReplicas

			if status.ReadyReps == status.Replicas && status.Replicas > 0 {
				status.Status = "healthy"
			} else if status.ReadyReps > 0 {
				status.Status = "partially_healthy"
				status.Issues = append(status.Issues, fmt.Sprintf("一部のレプリカが準備完了していません (%d/%d)", status.ReadyReps, status.Replicas))
			} else {
				status.Status = "unhealthy"
				status.Issues = append(status.Issues, "利用可能なレプリカがありません")
			}

			healthStatuses = append(healthStatuses, status)
		}
	}

	return healthStatuses
}

// checkStorageHealth checks the health of storage resources
func (h *HealthCheckerUsecase) checkStorageHealth(ctx context.Context) []StorageHealthStatus {
	var healthStatuses []StorageHealthStatus

	// Check persistent volumes
	pvs, err := h.kubectlPort.GetPersistentVolumes(ctx)
	if err != nil {
		h.logger.WarnWithContext("PersistentVolume取得に失敗", map[string]interface{}{
			"error": err.Error(),
		})
		return healthStatuses
	}

	for _, pv := range pvs {
		status := StorageHealthStatus{
			Name:     pv.Name,
			Type:     "PersistentVolume",
			Capacity: pv.Capacity,
			Issues:   []string{},
		}

		switch pv.Status {
		case "Available", "Bound":
			status.Status = "healthy"
		case "Released":
			status.Status = "warning"
			status.Issues = append(status.Issues, "PVがReleased状態です")
		case "Failed":
			status.Status = "unhealthy"
			status.Issues = append(status.Issues, "PVがFailed状態です")
		default:
			status.Status = "unknown"
			status.Issues = append(status.Issues, fmt.Sprintf("不明なPV状態: %s", pv.Status))
		}

		healthStatuses = append(healthStatuses, status)
	}

	return healthStatuses
}

// checkNetworkHealth checks the health of network resources
func (h *HealthCheckerUsecase) checkNetworkHealth(ctx context.Context, env domain.Environment) NetworkHealthStatus {
	status := NetworkHealthStatus{
		IngressStatus:  "checking",
		ServiceMeshOK:  true,
		ConnectivityOK: true,
		ExternalAccess: false,
		Issues:         []string{},
	}

	// Simple connectivity test (basic implementation)
	nodes, err := h.kubectlPort.GetNodes(ctx)
	if err != nil {
		status.ConnectivityOK = false
		status.Issues = append(status.Issues, fmt.Sprintf("ノード接続テストに失敗: %v", err))
	} else if len(nodes) == 0 {
		status.ConnectivityOK = false
		status.Issues = append(status.Issues, "利用可能なノードがありません")
	}

	// Check ingress status (simplified)
	ingressNamespace := "alt-ingress"
	pods, err := h.kubectlPort.GetPods(ctx, ingressNamespace, "")
	if err != nil {
		status.IngressStatus = "error"
		status.Issues = append(status.Issues, fmt.Sprintf("Ingress状態確認に失敗: %v", err))
	} else {
		readyIngress := 0
		for _, pod := range pods {
			if pod.Status == "Running" && pod.Ready == "True" {
				readyIngress++
			}
		}

		if readyIngress > 0 {
			status.IngressStatus = "healthy"
			status.ExternalAccess = true
		} else {
			status.IngressStatus = "unhealthy"
			status.Issues = append(status.Issues, "Ingressポッドが利用できません")
		}
	}

	return status
}

// calculateOverallHealthStatus calculates the overall health status
func (h *HealthCheckerUsecase) calculateOverallHealthStatus(report *HealthReport) string {
	criticalIssues := 0
	warnings := 0

	// Check namespace health
	for _, ns := range report.NamespaceHealth {
		if ns.Status == "error" || ns.Status == "unhealthy" {
			criticalIssues++
		} else if ns.Status == "partially_healthy" {
			warnings++
		}
	}

	// Check service health
	for _, svc := range report.ServiceHealth {
		if svc.Status == "unhealthy" {
			criticalIssues++
		} else if svc.Status == "partially_healthy" {
			warnings++
		}
	}

	// Check storage health
	for _, storage := range report.StorageHealth {
		if storage.Status == "unhealthy" {
			criticalIssues++
		} else if storage.Status == "warning" {
			warnings++
		}
	}

	// Check network health
	if !report.NetworkHealth.ConnectivityOK {
		criticalIssues++
	}
	if report.NetworkHealth.IngressStatus == "error" || report.NetworkHealth.IngressStatus == "unhealthy" {
		warnings++
	}

	if criticalIssues > 0 {
		return "critical"
	} else if warnings > 0 {
		return "warning"
	} else {
		return "healthy"
	}
}

// generateHealthRecommendations generates health recommendations
func (h *HealthCheckerUsecase) generateHealthRecommendations(report *HealthReport) []string {
	var recommendations []string

	if report.OverallStatus == "healthy" {
		recommendations = append(recommendations, "システムは正常に動作しています")
		return recommendations
	}

	// Namespace recommendations
	for _, ns := range report.NamespaceHealth {
		if ns.Status == "unhealthy" {
			recommendations = append(recommendations, fmt.Sprintf("名前空間 %s の問題を調査してください", ns.Name))
		}
	}

	// Service recommendations
	unhealthyServices := 0
	for _, svc := range report.ServiceHealth {
		if svc.Status == "unhealthy" {
			unhealthyServices++
		}
	}
	if unhealthyServices > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d個のサービスが異常状態です。ログを確認してください", unhealthyServices))
	}

	// Storage recommendations
	for _, storage := range report.StorageHealth {
		if storage.Status == "unhealthy" {
			recommendations = append(recommendations, fmt.Sprintf("ストレージ %s に問題があります", storage.Name))
		}
	}

	// Network recommendations
	if !report.NetworkHealth.ConnectivityOK {
		recommendations = append(recommendations, "ネットワーク接続に問題があります。クラスター設定を確認してください")
	}

	return recommendations
}

// getMonitoringConfig returns monitoring configuration for the environment
func (u *DeploymentMonitoringUsecase) getMonitoringConfig(env domain.Environment) MonitoringConfig {
	switch env {
	case domain.Production:
		return MonitoringConfig{
			Interval:            30 * time.Second,
			AutoRecoveryEnabled: true,
			NotificationEnabled: true,
			MaxRetries:          3,
			CriticalThreshold:   3,
			WarningThreshold:    5,
		}
	case domain.Staging:
		return MonitoringConfig{
			Interval:            1 * time.Minute,
			AutoRecoveryEnabled: true,
			NotificationEnabled: false,
			MaxRetries:          2,
			CriticalThreshold:   2,
			WarningThreshold:    3,
		}
	case domain.Development:
		return MonitoringConfig{
			Interval:            2 * time.Minute,
			AutoRecoveryEnabled: false,
			NotificationEnabled: false,
			MaxRetries:          1,
			CriticalThreshold:   5,
			WarningThreshold:    10,
		}
	default:
		return MonitoringConfig{
			Interval:            1 * time.Minute,
			AutoRecoveryEnabled: false,
			NotificationEnabled: false,
			MaxRetries:          2,
			CriticalThreshold:   3,
			WarningThreshold:    5,
		}
	}
}

// recordHealthMetrics records health metrics
func (u *DeploymentMonitoringUsecase) recordHealthMetrics(report *HealthReport) {
	u.metrics.RecordHealthReport(report)
}

// detectIssues detects issues from the health report
func (u *DeploymentMonitoringUsecase) detectIssues(report *HealthReport) []HealthIssue {
	var issues []HealthIssue

	// Detect namespace issues
	for _, ns := range report.NamespaceHealth {
		if ns.Status == "error" || ns.Status == "unhealthy" {
			issues = append(issues, HealthIssue{
				Type:        "NamespaceUnhealthy",
				Severity:    "critical",
				Component:   ns.Name,
				Description: fmt.Sprintf("名前空間 %s が異常状態です", ns.Name),
				DetectedAt:  time.Now(),
				Recoverable: true,
			})
		}
	}

	// Detect service issues
	for _, svc := range report.ServiceHealth {
		if svc.Status == "unhealthy" {
			issues = append(issues, HealthIssue{
				Type:        "ServiceUnhealthy",
				Severity:    "critical",
				Component:   fmt.Sprintf("%s/%s", svc.Namespace, svc.Name),
				Description: fmt.Sprintf("サービス %s が異常状態です", svc.Name),
				DetectedAt:  time.Now(),
				Recoverable: true,
			})
		}
	}

	return issues
}

// categorizeIssues categorizes issues by severity
func (u *DeploymentMonitoringUsecase) categorizeIssues(issues []HealthIssue) ([]HealthIssue, []HealthIssue) {
	var critical, warnings []HealthIssue

	for _, issue := range issues {
		if issue.Severity == "critical" {
			critical = append(critical, issue)
		} else {
			warnings = append(warnings, issue)
		}
	}

	return critical, warnings
}

// performAutoRecovery performs automatic recovery for critical issues
func (u *DeploymentMonitoringUsecase) performAutoRecovery(ctx context.Context, issues []HealthIssue) error {
	u.logger.InfoWithContext("自動復旧を開始", map[string]interface{}{
		"issues_count": len(issues),
	})

	successCount := 0
	for _, issue := range issues {
		if !issue.Recoverable {
			continue
		}

		u.logger.InfoWithContext("問題の自動復旧を試行", map[string]interface{}{
			"type":        issue.Type,
			"component":   issue.Component,
			"description": issue.Description,
		})

		result, err := u.autoRecovery.RecoverFromError(ctx, issue.Error)
		if err != nil {
			u.logger.ErrorWithContext("自動復旧に失敗", map[string]interface{}{
				"issue": issue.Description,
				"error": err.Error(),
			})
			u.metrics.RecordRecoveryFailure(issue.Type)
		} else {
			u.logger.InfoWithContext("自動復旧が成功", map[string]interface{}{
				"issue":       issue.Description,
				"action":      result.Action,
				"description": result.Description,
			})
			u.metrics.RecordRecoverySuccess(issue.Type)
			successCount++
		}
	}

	u.logger.InfoWithContext("自動復旧処理が完了", map[string]interface{}{
		"total_issues":        len(issues),
		"successful_recovery": successCount,
		"failed_recovery":     len(issues) - successCount,
	})

	return nil
}

// handleWarnings handles warning-level issues
func (u *DeploymentMonitoringUsecase) handleWarnings(ctx context.Context, warnings []HealthIssue) {
	if len(warnings) == 0 {
		return
	}

	u.logger.WarnWithContext("警告レベルの問題を検出", map[string]interface{}{
		"warning_count": len(warnings),
	})

	for _, warning := range warnings {
		u.logger.WarnWithContext("警告", map[string]interface{}{
			"type":        warning.Type,
			"component":   warning.Component,
			"description": warning.Description,
		})
	}
}

// handleCriticalFailure handles critical monitoring failures
func (u *DeploymentMonitoringUsecase) handleCriticalFailure(ctx context.Context, options *domain.DeploymentOptions, consecutiveFailures int) {
	u.logger.ErrorWithContext("重大な監視障害を検出", map[string]interface{}{
		"consecutive_failures": consecutiveFailures,
		"environment":          options.Environment.String(),
	})

	// Record critical failure
	u.metrics.RecordCriticalFailure(consecutiveFailures)

	// This could trigger additional alerting or escalation procedures
}

// RecordHealthyState records a healthy monitoring state
func (m *MetricsCollector) RecordHealthyState() {
	m.logger.DebugWithContext("健全状態を記録", map[string]interface{}{
		"timestamp": time.Now().Unix(),
	})
}

// RecordRecoverySuccess records a successful recovery
func (m *MetricsCollector) RecordRecoverySuccess(issueType string) {
	m.logger.InfoWithContext("復旧成功を記録", map[string]interface{}{
		"issue_type": issueType,
		"timestamp":  time.Now().Unix(),
	})
}

// RecordRecoveryFailure records a failed recovery
func (m *MetricsCollector) RecordRecoveryFailure(issueType string) {
	m.logger.WarnWithContext("復旧失敗を記録", map[string]interface{}{
		"issue_type": issueType,
		"timestamp":  time.Now().Unix(),
	})
}

// RecordHealthReport records a health report
func (m *MetricsCollector) RecordHealthReport(report *HealthReport) {
	m.logger.DebugWithContext("ヘルスレポートを記録", map[string]interface{}{
		"overall_status": report.OverallStatus,
		"timestamp":      report.Timestamp.Unix(),
		"environment":    report.Environment,
	})
}

// RecordCriticalFailure records a critical failure
func (m *MetricsCollector) RecordCriticalFailure(consecutiveFailures int) {
	m.logger.ErrorWithContext("重大な障害を記録", map[string]interface{}{
		"consecutive_failures": consecutiveFailures,
		"timestamp":            time.Now().Unix(),
	})
}
