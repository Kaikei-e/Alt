package infrastructure_usecase

import (
	"context"
	"fmt"
	"strings"

	"deploy-cli/domain"
	"deploy-cli/port/kubectl_port"
	"deploy-cli/port/logger_port"
)

// NamespaceEnsureUsecase handles namespace existence validation and automatic creation
type NamespaceEnsureUsecase struct {
	kubectlPort kubectl_port.KubectlPort
	logger      logger_port.LoggerPort
}

// NewNamespaceEnsureUsecase creates a new namespace ensure usecase
func NewNamespaceEnsureUsecase(
	kubectlPort kubectl_port.KubectlPort,
	logger logger_port.LoggerPort,
) *NamespaceEnsureUsecase {
	return &NamespaceEnsureUsecase{
		kubectlPort: kubectlPort,
		logger:      logger,
	}
}

// EnsureNamespaceExists 名前空間の存在確認と自動作成
func (u *NamespaceEnsureUsecase) EnsureNamespaceExists(ctx context.Context, namespace string) error {
	u.logger.InfoWithContext("名前空間の存在確認中", map[string]interface{}{
		"namespace": namespace,
	})
	
	// 名前空間の存在確認
	if err := u.kubectlPort.GetNamespace(ctx, namespace); err != nil {
		if u.isNotFoundError(err) {
			u.logger.InfoWithContext("名前空間が見つかりません。自動作成中...", map[string]interface{}{
				"namespace": namespace,
			})
			
			// 名前空間の自動作成
			if err := u.kubectlPort.CreateNamespace(ctx, namespace); err != nil {
				return fmt.Errorf("名前空間の作成に失敗: %w", err)
			}
			
			u.logger.InfoWithContext("名前空間を正常に作成しました", map[string]interface{}{
				"namespace": namespace,
			})
			return nil
		}
		return fmt.Errorf("名前空間の確認に失敗: %w", err)
	}
	
	u.logger.InfoWithContext("名前空間は既に存在します", map[string]interface{}{
		"namespace": namespace,
	})
	return nil
}

// EnsureAllRequiredNamespaces 必要な全名前空間の確認・作成
func (u *NamespaceEnsureUsecase) EnsureAllRequiredNamespaces(ctx context.Context, env domain.Environment) error {
	requiredNamespaces := domain.GetNamespacesForEnvironment(env)
	
	u.logger.InfoWithContext("必要な名前空間の一括確認開始", map[string]interface{}{
		"environment": env.String(),
		"namespaces":  requiredNamespaces,
	})
	
	var errors []string
	successCount := 0
	
	for _, namespace := range requiredNamespaces {
		if err := u.EnsureNamespaceExists(ctx, namespace); err != nil {
			errorMsg := fmt.Sprintf("名前空間 %s の処理に失敗: %v", namespace, err)
			errors = append(errors, errorMsg)
			u.logger.ErrorWithContext("名前空間処理エラー", map[string]interface{}{
				"namespace": namespace,
				"error":     err.Error(),
			})
		} else {
			successCount++
		}
	}
	
	if len(errors) > 0 {
		u.logger.ErrorWithContext("一部の名前空間処理に失敗", map[string]interface{}{
			"failed_count":   len(errors),
			"success_count":  successCount,
			"total_count":    len(requiredNamespaces),
		})
		return fmt.Errorf("名前空間処理でエラーが発生: %s", strings.Join(errors, "; "))
	}
	
	u.logger.InfoWithContext("全ての必要な名前空間の確認が完了", map[string]interface{}{
		"environment": env.String(),
		"count":       len(requiredNamespaces),
	})
	
	return nil
}

// EnsureNamespaceForService サービス固有の名前空間確認・作成
func (u *NamespaceEnsureUsecase) EnsureNamespaceForService(ctx context.Context, serviceName string, env domain.Environment) error {
	namespace := domain.DetermineNamespace(serviceName, env)
	
	u.logger.InfoWithContext("サービス固有の名前空間確認", map[string]interface{}{
		"service":     serviceName,
		"namespace":   namespace,
		"environment": env.String(),
	})
	
	return u.EnsureNamespaceExists(ctx, namespace)
}

// isNotFoundError checks if the error indicates a "not found" condition
func (u *NamespaceEnsureUsecase) isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	
	errorStr := strings.ToLower(err.Error())
	return strings.Contains(errorStr, "not found") ||
		   strings.Contains(errorStr, "notfound") ||
		   strings.Contains(errorStr, "does not exist")
}

// GetNamespaceStatus 名前空間の状態取得
func (u *NamespaceEnsureUsecase) GetNamespaceStatus(ctx context.Context, namespace string) (*NamespaceStatus, error) {
	u.logger.DebugWithContext("名前空間状態確認", map[string]interface{}{
		"namespace": namespace,
	})
	
	err := u.kubectlPort.GetNamespace(ctx, namespace)
	if err != nil {
		if u.isNotFoundError(err) {
			return &NamespaceStatus{
				Name:   namespace,
				Exists: false,
				Status: "NotFound",
			}, nil
		}
		return nil, fmt.Errorf("名前空間状態確認に失敗: %w", err)
	}
	
	return &NamespaceStatus{
		Name:   namespace,
		Exists: true,
		Status: "Active",
	}, nil
}

// NamespaceStatus represents the status of a namespace
type NamespaceStatus struct {
	Name   string `json:"name"`
	Exists bool   `json:"exists"`
	Status string `json:"status"`
}

// ValidateNamespaceAccess 名前空間へのアクセス権限確認
func (u *NamespaceEnsureUsecase) ValidateNamespaceAccess(ctx context.Context, namespace string) error {
	u.logger.DebugWithContext("名前空間アクセス権限確認", map[string]interface{}{
		"namespace": namespace,
	})
	
	// 簡単なアクセステスト（秘密情報の一覧取得を試行）
	if err := u.kubectlPort.ListSecrets(ctx, namespace); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "forbidden") {
			return fmt.Errorf("名前空間 %s への書き込み権限がありません: %w", namespace, err)
		}
		// その他のエラーは警告として扱う
		u.logger.WarnWithContext("名前空間アクセステストで予期しないエラー", map[string]interface{}{
			"namespace": namespace,
			"error":     err.Error(),
		})
	}
	
	u.logger.DebugWithContext("名前空間アクセス権限確認完了", map[string]interface{}{
		"namespace": namespace,
	})
	
	return nil
}