package diagnostic_usecase

import (
	"context"
	"fmt"
	"time"

	"deploy-cli/domain"
	"deploy-cli/port/kubectl_port"
	"deploy-cli/port/logger_port"
	"deploy-cli/usecase/infrastructure_usecase"
)

// PrerequisitesDiagnosticUsecase handles comprehensive environment diagnosis and automated fixes
type PrerequisitesDiagnosticUsecase struct {
	kubectlPort        kubectl_port.KubectlPort
	namespaceEnsure    *infrastructure_usecase.NamespaceEnsureUsecase
	storageClassEnsure *infrastructure_usecase.StorageClassEnsureUsecase
	logger             logger_port.LoggerPort
}

// NewPrerequisitesDiagnosticUsecase creates a new prerequisites diagnostic usecase
func NewPrerequisitesDiagnosticUsecase(
	kubectlPort kubectl_port.KubectlPort,
	namespaceEnsure *infrastructure_usecase.NamespaceEnsureUsecase,
	storageClassEnsure *infrastructure_usecase.StorageClassEnsureUsecase,
	logger logger_port.LoggerPort,
) *PrerequisitesDiagnosticUsecase {
	return &PrerequisitesDiagnosticUsecase{
		kubectlPort:        kubectlPort,
		namespaceEnsure:    namespaceEnsure,
		storageClassEnsure: storageClassEnsure,
		logger:             logger,
	}
}

// DiagnosticReport represents a comprehensive diagnostic report
type DiagnosticReport struct {
	OverallStatus   string                   `json:"overall_status"`
	Kubernetes      KubernetesStatus         `json:"kubernetes"`
	Namespaces      []NamespaceStatus        `json:"namespaces"`
	StorageClasses  []StorageClassStatus     `json:"storage_classes"`
	RBAC            RBACStatus               `json:"rbac"`
	Recommendations []string                 `json:"recommendations"`
	ExecutionTime   time.Duration            `json:"execution_time"`
	Environment     string                   `json:"environment"`
	Timestamp       time.Time                `json:"timestamp"`
}

// KubernetesStatus represents the status of Kubernetes connection
type KubernetesStatus struct {
	Status       string `json:"status"`
	Version      string `json:"version,omitempty"`
	Message      string `json:"message"`
	NodesCount   int    `json:"nodes_count"`
	ClusterReady bool   `json:"cluster_ready"`
}

// NamespaceStatus represents the status of a namespace
type NamespaceStatus struct {
	Name        string `json:"name"`
	Exists      bool   `json:"exists"`
	Status      string `json:"status"`
	Accessible  bool   `json:"accessible"`
	AutoCreated bool   `json:"auto_created"`
	Error       string `json:"error,omitempty"`
}

// StorageClassStatus represents the status of storage classes
type StorageClassStatus struct {
	Name        string `json:"name"`
	Exists      bool   `json:"exists"`
	Provisioner string `json:"provisioner,omitempty"`
	IsDefault   bool   `json:"is_default"`
	Status      string `json:"status"`
}

// RBACStatus represents the status of RBAC permissions
type RBACStatus struct {
	Status           string   `json:"status"`
	CanCreateSecrets bool     `json:"can_create_secrets"`
	CanCreateNS      bool     `json:"can_create_namespaces"`
	CanListPods      bool     `json:"can_list_pods"`
	Permissions      []string `json:"permissions"`
	Restrictions     []string `json:"restrictions"`
}

// DiagnoseAndFix 包括的診断と自動修復
func (u *PrerequisitesDiagnosticUsecase) DiagnoseAndFix(ctx context.Context, env domain.Environment) (*DiagnosticReport, error) {
	startTime := time.Now()
	report := &DiagnosticReport{
		OverallStatus: "診断中",
		Environment:   env.String(),
		Timestamp:     startTime,
	}

	u.logger.InfoWithContext("包括的環境診断を開始", map[string]interface{}{
		"environment": env.String(),
	})

	// 1. Kubernetes接続診断
	report.Kubernetes = u.diagnoseKubernetesConnection(ctx)
	if report.Kubernetes.Status != "正常" {
		report.OverallStatus = "重大な問題"
		report.ExecutionTime = time.Since(startTime)
		return report, fmt.Errorf("Kubernetes接続に問題があります: %s", report.Kubernetes.Message)
	}

	// 2. 名前空間診断・自動修復
	report.Namespaces = u.diagnoseAndFixNamespaces(ctx, env)

	// 3. StorageClass診断・自動修復
	report.StorageClasses = u.diagnoseAndFixStorageClasses(ctx)

	// 4. RBAC権限診断
	report.RBAC = u.diagnoseRBACPermissions(ctx)

	// 5. 総合判定と推奨事項
	report.OverallStatus = u.calculateOverallStatus(report)
	report.Recommendations = u.generateRecommendations(report)
	report.ExecutionTime = time.Since(startTime)

	u.logger.InfoWithContext("包括的環境診断が完了", map[string]interface{}{
		"overall_status": report.OverallStatus,
		"execution_time": report.ExecutionTime.String(),
	})

	return report, nil
}

// diagnoseKubernetesConnection diagnoses Kubernetes cluster connection
func (u *PrerequisitesDiagnosticUsecase) diagnoseKubernetesConnection(ctx context.Context) KubernetesStatus {
	status := KubernetesStatus{
		Status:       "診断中",
		ClusterReady: false,
	}

	u.logger.DebugWithContext("Kubernetes接続診断開始", map[string]interface{}{})

	// kubectl version check
	version, err := u.kubectlPort.Version(ctx)
	if err != nil {
		status.Status = "エラー"
		status.Message = fmt.Sprintf("kubectl実行に失敗: %v", err)
		return status
	}
	status.Version = version

	// Cluster node check
	nodes, err := u.kubectlPort.GetNodes(ctx)
	if err != nil {
		status.Status = "エラー"
		status.Message = fmt.Sprintf("クラスターノード取得に失敗: %v", err)
		return status
	}

	status.NodesCount = len(nodes)
	if len(nodes) == 0 {
		status.Status = "警告"
		status.Message = "利用可能なノードが見つかりません"
		return status
	}

	// Check node readiness
	readyNodes := 0
	for _, node := range nodes {
		if node.Status == "Ready" {
			readyNodes++
		}
	}

	if readyNodes == 0 {
		status.Status = "エラー"
		status.Message = "Ready状態のノードがありません"
		return status
	}

	if readyNodes < len(nodes) {
		status.Status = "警告"
		status.Message = fmt.Sprintf("%d/%d ノードがReady状態です", readyNodes, len(nodes))
		status.ClusterReady = true
		return status
	}

	status.Status = "正常"
	status.Message = fmt.Sprintf("クラスターは正常です（%d ノード、全てReady）", len(nodes))
	status.ClusterReady = true

	return status
}

// diagnoseAndFixNamespaces diagnoses and fixes namespace issues
func (u *PrerequisitesDiagnosticUsecase) diagnoseAndFixNamespaces(ctx context.Context, env domain.Environment) []NamespaceStatus {
	u.logger.InfoWithContext("名前空間診断・自動修復開始", map[string]interface{}{
		"environment": env.String(),
	})

	requiredNamespaces := domain.GetNamespacesForEnvironment(env)
	var statuses []NamespaceStatus

	for _, nsName := range requiredNamespaces {
		status := NamespaceStatus{
			Name:        nsName,
			Exists:      false,
			Accessible:  false,
			AutoCreated: false,
			Status:      "診断中",
		}

		// Check if namespace exists
		err := u.kubectlPort.GetNamespace(ctx, nsName)
		if err != nil {
			if u.isNotFoundError(err) {
				u.logger.InfoWithContext("名前空間が存在しません。自動作成を試行", map[string]interface{}{
					"namespace": nsName,
				})

				// Attempt auto-creation
				if createErr := u.namespaceEnsure.EnsureNamespaceExists(ctx, nsName); createErr != nil {
					status.Status = "作成失敗"
					status.Error = createErr.Error()
					u.logger.ErrorWithContext("名前空間自動作成に失敗", map[string]interface{}{
						"namespace": nsName,
						"error":     createErr.Error(),
					})
				} else {
					status.Exists = true
					status.AutoCreated = true
					status.Status = "自動作成済み"
					u.logger.InfoWithContext("名前空間を自動作成しました", map[string]interface{}{
						"namespace": nsName,
					})
				}
			} else {
				status.Status = "アクセスエラー"
				status.Error = err.Error()
			}
		} else {
			status.Exists = true
			status.Status = "存在"
		}

		// Check accessibility if namespace exists
		if status.Exists {
			if accessErr := u.namespaceEnsure.ValidateNamespaceAccess(ctx, nsName); accessErr != nil {
				status.Accessible = false
				status.Status = "アクセス不可"
				if status.Error == "" {
					status.Error = accessErr.Error()
				}
			} else {
				status.Accessible = true
				if status.Status == "存在" {
					status.Status = "正常"
				}
			}
		}

		statuses = append(statuses, status)
	}

	return statuses
}

// diagnoseAndFixStorageClasses diagnoses and fixes storage class issues
func (u *PrerequisitesDiagnosticUsecase) diagnoseAndFixStorageClasses(ctx context.Context) []StorageClassStatus {
	u.logger.InfoWithContext("StorageClass診断・自動修復開始", map[string]interface{}{})

	var statuses []StorageClassStatus

	// Get all storage classes
	storageClasses, err := u.kubectlPort.GetStorageClasses(ctx)
	if err != nil {
		u.logger.WarnWithContext("StorageClass一覧取得に失敗", map[string]interface{}{
			"error": err.Error(),
		})
		return statuses
	}

	// Check for standard storage class
	standardExists := false
	for _, sc := range storageClasses {
		status := StorageClassStatus{
			Name:        sc.Name,
			Exists:      true,
			Provisioner: sc.Provisioner,
			IsDefault:   sc.Name == "standard",
			Status:      "正常",
		}

		if sc.Name == "standard" {
			standardExists = true
		}

		statuses = append(statuses, status)
	}

	// If standard doesn't exist, try to fix it
	if !standardExists {
		u.logger.InfoWithContext("標準StorageClassが見つかりません。自動設定を試行", map[string]interface{}{})

		if fixErr := u.storageClassEnsure.EnsureDefaultStorageClass(ctx); fixErr != nil {
			u.logger.WarnWithContext("StorageClass自動設定に失敗", map[string]interface{}{
				"error": fixErr.Error(),
			})
		}

		// Add standard storage class status (may not exist physically but handled gracefully)
		standardStatus := StorageClassStatus{
			Name:      "standard",
			Exists:    false,
			IsDefault: true,
			Status:    "警告（代替StorageClassを使用）",
		}
		statuses = append(statuses, standardStatus)
	}

	return statuses
}

// diagnoseRBACPermissions diagnoses RBAC permissions
func (u *PrerequisitesDiagnosticUsecase) diagnoseRBACPermissions(ctx context.Context) RBACStatus {
	status := RBACStatus{
		Status:      "診断中",
		Permissions: []string{},
		Restrictions: []string{},
	}

	u.logger.DebugWithContext("RBAC権限診断開始", map[string]interface{}{})

	// Test namespace creation permission
	testNS := "rbac-test-" + fmt.Sprintf("%d", time.Now().Unix())
	if err := u.kubectlPort.CreateNamespace(ctx, testNS); err != nil {
		status.CanCreateNS = false
		status.Restrictions = append(status.Restrictions, "名前空間作成権限なし")
	} else {
		status.CanCreateNS = true
		status.Permissions = append(status.Permissions, "名前空間作成可能")
		// Clean up test namespace
		u.kubectlPort.DeleteNamespace(ctx, testNS)
	}

	// Test secret listing permission (using default namespace)
	if err := u.kubectlPort.ListSecrets(ctx, "default"); err != nil {
		status.CanCreateSecrets = false
		status.Restrictions = append(status.Restrictions, "Secret操作権限制限あり")
	} else {
		status.CanCreateSecrets = true
		status.Permissions = append(status.Permissions, "Secret操作可能")
	}

	// Test pod listing permission
	if _, err := u.kubectlPort.GetPods(ctx, "default", ""); err != nil {
		status.CanListPods = false
		status.Restrictions = append(status.Restrictions, "Pod一覧取得権限なし")
	} else {
		status.CanListPods = true
		status.Permissions = append(status.Permissions, "Pod一覧取得可能")
	}

	// Determine overall RBAC status
	if status.CanCreateNS && status.CanCreateSecrets && status.CanListPods {
		status.Status = "正常"
	} else if status.CanCreateSecrets && status.CanListPods {
		status.Status = "部分的制限"
	} else {
		status.Status = "権限不足"
	}

	return status
}

// calculateOverallStatus calculates the overall system status
func (u *PrerequisitesDiagnosticUsecase) calculateOverallStatus(report *DiagnosticReport) string {
	if report.Kubernetes.Status == "エラー" {
		return "重大な問題"
	}

	errorCount := 0
	warningCount := 0

	// Check namespace statuses
	for _, ns := range report.Namespaces {
		if ns.Status == "作成失敗" || ns.Status == "アクセスエラー" {
			errorCount++
		} else if ns.Status == "アクセス不可" {
			warningCount++
		}
	}

	// Check RBAC status
	if report.RBAC.Status == "権限不足" {
		errorCount++
	} else if report.RBAC.Status == "部分的制限" {
		warningCount++
	}

	// Check Kubernetes status
	if report.Kubernetes.Status == "警告" {
		warningCount++
	}

	if errorCount > 0 {
		return "問題あり"
	} else if warningCount > 0 {
		return "警告あり"
	} else {
		return "正常"
	}
}

// generateRecommendations generates recommendations based on the diagnostic report
func (u *PrerequisitesDiagnosticUsecase) generateRecommendations(report *DiagnosticReport) []string {
	var recommendations []string

	// Kubernetes recommendations
	if report.Kubernetes.Status == "エラー" {
		recommendations = append(recommendations, "Kubernetesクラスターへの接続を確認してください")
		recommendations = append(recommendations, "kubectlの設定ファイル（~/.kube/config）を確認してください")
	} else if report.Kubernetes.Status == "警告" {
		recommendations = append(recommendations, "一部のノードが利用できません。クラスターの健全性を確認してください")
	}

	// Namespace recommendations
	failedNamespaces := 0
	for _, ns := range report.Namespaces {
		if ns.Status == "作成失敗" || ns.Status == "アクセスエラー" {
			failedNamespaces++
		}
	}
	if failedNamespaces > 0 {
		recommendations = append(recommendations, "名前空間の作成に失敗しています。RBAC権限を確認してください")
		recommendations = append(recommendations, "--auto-create-namespacesフラグを使用して自動作成を有効にしてください")
	}

	// StorageClass recommendations
	standardExists := false
	for _, sc := range report.StorageClasses {
		if sc.Name == "standard" && sc.Exists {
			standardExists = true
			break
		}
	}
	if !standardExists {
		recommendations = append(recommendations, "標準StorageClass「standard」が見つかりません")
		recommendations = append(recommendations, "利用可能なStorageClassを確認し、適切な設定を行ってください")
	}

	// RBAC recommendations
	if report.RBAC.Status == "権限不足" {
		recommendations = append(recommendations, "RBAC権限が不足しています。クラスター管理者に権限付与を依頼してください")
		recommendations = append(recommendations, "最低限、名前空間とSecretの作成・管理権限が必要です")
	} else if report.RBAC.Status == "部分的制限" {
		recommendations = append(recommendations, "一部のRBAC権限に制限があります。完全な自動化には追加権限が必要です")
	}

	// General recommendations
	if len(recommendations) == 0 {
		recommendations = append(recommendations, "環境は正常です。デプロイメントを開始できます")
		recommendations = append(recommendations, "継続的な監視のため、--continuous-monitoringフラグの使用を検討してください")
	} else {
		recommendations = append(recommendations, "問題を修正後、診断を再実行することを推奨します")
		recommendations = append(recommendations, "./deploy-cli diagnose --environment "+report.Environment+" で再診断できます")
	}

	return recommendations
}

// isNotFoundError checks if the error indicates a not found condition
func (u *PrerequisitesDiagnosticUsecase) isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	
	errorStr := err.Error()
	return contains(errorStr, "not found") || 
		   contains(errorStr, "NotFound") || 
		   contains(errorStr, "does not exist")
}

// contains checks if a string contains a substring (case-insensitive helper)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && 
		   (s == substr || 
		    (len(s) > len(substr) && 
		     findSubstring(s, substr)))
}

// findSubstring finds substring in string (simple implementation)
func findSubstring(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if toLowerCase(s[i+j]) != toLowerCase(substr[j]) {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// toLowerCase converts character to lowercase
func toLowerCase(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}