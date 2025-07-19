package infrastructure_usecase

import (
	"context"
	"fmt"
	"strings"

	"deploy-cli/port/kubectl_port"
	"deploy-cli/port/logger_port"
)

// StorageClassEnsureUsecase handles StorageClass validation and automatic configuration
type StorageClassEnsureUsecase struct {
	kubectlPort kubectl_port.KubectlPort
	logger      logger_port.LoggerPort
}

// NewStorageClassEnsureUsecase creates a new storage class ensure usecase
func NewStorageClassEnsureUsecase(
	kubectlPort kubectl_port.KubectlPort,
	logger logger_port.LoggerPort,
) *StorageClassEnsureUsecase {
	return &StorageClassEnsureUsecase{
		kubectlPort: kubectlPort,
		logger:      logger,
	}
}

// StorageClassStatus represents the status of storage classes
type StorageClassStatus struct {
	StandardExists bool     `json:"standard_exists"`
	Available      []string `json:"available"`
	Default        string   `json:"default"`
	Status         string   `json:"status"`
	Message        string   `json:"message"`
}

// EnsureDefaultStorageClass デフォルトStorageClassの確認・設定
func (u *StorageClassEnsureUsecase) EnsureDefaultStorageClass(ctx context.Context) error {
	u.logger.InfoWithContext("デフォルトStorageClassの確認中", map[string]interface{}{
		"target_storage_class": "standard",
	})
	
	// 利用可能なStorageClassを取得
	storageClasses, err := u.kubectlPort.GetStorageClasses(ctx)
	if err != nil {
		u.logger.WarnWithContext("StorageClass一覧の取得に失敗、警告として処理継続", map[string]interface{}{
			"error": err.Error(),
		})
		return nil // エラーとせず警告として処理
	}
	
	if len(storageClasses) == 0 {
		u.logger.WarnWithContext("利用可能なStorageClassが見つかりません", map[string]interface{}{
			"message": "StorageClassが設定されていない環境での実行",
		})
		return nil // エラーとせず警告として処理
	}
	
	// "standard" StorageClassの存在確認
	standardExists := false
	var availableClasses []string
	
	for _, sc := range storageClasses {
		availableClasses = append(availableClasses, sc.Name)
		if sc.Name == "standard" {
			standardExists = true
		}
	}
	
	u.logger.InfoWithContext("StorageClass状況", map[string]interface{}{
		"standard_exists":    standardExists,
		"available_classes":  availableClasses,
		"total_count":        len(storageClasses),
	})
	
	if standardExists {
		u.logger.InfoWithContext("標準StorageClass「standard」が利用可能です", map[string]interface{}{
			"storage_class": "standard",
		})
		return nil
	}
	
	// "standard"が存在しない場合の対応
	defaultClass := availableClasses[0]
	u.logger.InfoWithContext("標準StorageClassが見つかりません。代替クラスを使用します", map[string]interface{}{
		"missing_class":     "standard",
		"alternative_class": defaultClass,
		"available_classes": availableClasses,
	})
	
	return nil // 警告として処理し、デプロイメントは継続
}

// GetStorageClassStatus StorageClass状況の詳細取得
func (u *StorageClassEnsureUsecase) GetStorageClassStatus(ctx context.Context) (*StorageClassStatus, error) {
	u.logger.DebugWithContext("StorageClass状況の詳細確認", map[string]interface{}{})
	
	status := &StorageClassStatus{
		StandardExists: false,
		Available:      []string{},
		Status:         "Unknown",
	}
	
	storageClasses, err := u.kubectlPort.GetStorageClasses(ctx)
	if err != nil {
		status.Status = "Error"
		status.Message = fmt.Sprintf("StorageClass一覧取得エラー: %v", err)
		return status, nil // エラーを返さず状況を返す
	}
	
	if len(storageClasses) == 0 {
		status.Status = "NoStorageClasses"
		status.Message = "利用可能なStorageClassが見つかりません"
		return status, nil
	}
	
	for _, sc := range storageClasses {
		status.Available = append(status.Available, sc.Name)
		if sc.Name == "standard" {
			status.StandardExists = true
		}
	}
	
	if status.StandardExists {
		status.Status = "OK"
		status.Default = "standard"
		status.Message = "標準StorageClass「standard」が利用可能"
	} else {
		status.Status = "Warning"
		status.Default = status.Available[0]
		status.Message = fmt.Sprintf("「standard」は見つかりませんが、「%s」が利用可能", status.Default)
	}
	
	return status, nil
}

// ValidateStorageClassRequirements StorageClass要件の検証
func (u *StorageClassEnsureUsecase) ValidateStorageClassRequirements(ctx context.Context) error {
	u.logger.InfoWithContext("StorageClass要件の検証開始", map[string]interface{}{})
	
	status, err := u.GetStorageClassStatus(ctx)
	if err != nil {
		return fmt.Errorf("StorageClass状況確認に失敗: %w", err)
	}
	
	switch status.Status {
	case "OK":
		u.logger.InfoWithContext("StorageClass要件を満たしています", map[string]interface{}{
			"standard_available": true,
		})
		return nil
		
	case "Warning":
		u.logger.WarnWithContext("StorageClass要件の部分的不適合", map[string]interface{}{
			"issue":             "standard StorageClassが見つからない",
			"alternative":       status.Default,
			"available_classes": status.Available,
		})
		return nil // 警告として処理
		
	case "NoStorageClasses":
		u.logger.WarnWithContext("StorageClassが設定されていません", map[string]interface{}{
			"message": "動的プロビジョニングが利用できない可能性があります",
		})
		return nil // 警告として処理
		
	case "Error":
		u.logger.ErrorWithContext("StorageClass確認エラー", map[string]interface{}{
			"error": status.Message,
		})
		return nil // デプロイメントを止めずに警告として処理
		
	default:
		u.logger.WarnWithContext("未知のStorageClass状況", map[string]interface{}{
			"status": status.Status,
		})
		return nil
	}
}

// CreateStorageClassIfNeeded 必要に応じてStorageClassを作成
func (u *StorageClassEnsureUsecase) CreateStorageClassIfNeeded(ctx context.Context) error {
	u.logger.InfoWithContext("StorageClass自動作成の検討", map[string]interface{}{})
	
	status, err := u.GetStorageClassStatus(ctx)
	if err != nil {
		return fmt.Errorf("StorageClass状況確認に失敗: %w", err)
	}
	
	if status.StandardExists {
		u.logger.InfoWithContext("「standard」StorageClassは既に存在します", map[string]interface{}{})
		return nil
	}
	
	if len(status.Available) == 0 {
		// 基本的なStorageClassを作成
		u.logger.InfoWithContext("基本的なStorageClassを作成します", map[string]interface{}{
			"storage_class": "standard",
		})
		
		return u.createDefaultStorageClass(ctx)
	}
	
	u.logger.InfoWithContext("他のStorageClassが利用可能なため、作成をスキップ", map[string]interface{}{
		"available_classes": status.Available,
	})
	
	return nil
}

// createDefaultStorageClass creates a basic default storage class
func (u *StorageClassEnsureUsecase) createDefaultStorageClass(ctx context.Context) error {
	u.logger.InfoWithContext("デフォルトStorageClassの作成中", map[string]interface{}{})
	
	// 基本的なStorageClass定義
	storageClass := kubectl_port.KubernetesStorageClass{
		Name:        "standard",
		Provisioner: "kubernetes.io/no-provisioner", // ローカルStorageを想定
		Parameters: map[string]string{
			"type": "local",
		},
	}
	
	if err := u.kubectlPort.CreateStorageClass(ctx, storageClass); err != nil {
		u.logger.WarnWithContext("StorageClass作成に失敗、警告として継続", map[string]interface{}{
			"storage_class": "standard",
			"error":         err.Error(),
		})
		return nil // エラーとせず警告として処理
	}
	
	u.logger.InfoWithContext("デフォルトStorageClassを正常に作成", map[string]interface{}{
		"storage_class": "standard",
		"provisioner":   storageClass.Provisioner,
	})
	
	return nil
}

// isStorageClassNotFoundError checks if the error indicates a storage class not found condition
func (u *StorageClassEnsureUsecase) isStorageClassNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	
	errorStr := strings.ToLower(err.Error())
	return strings.Contains(errorStr, "storageclass") && 
		   (strings.Contains(errorStr, "not found") || 
		    strings.Contains(errorStr, "notfound"))
}