package recovery_usecase

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"deploy-cli/domain"
	"deploy-cli/port/logger_port"
	"deploy-cli/usecase/infrastructure_usecase"
	"deploy-cli/usecase/secret_usecase"
)

// AutoRecoveryUsecase handles automatic error recovery for common deployment issues
type AutoRecoveryUsecase struct {
	namespaceEnsure    *infrastructure_usecase.NamespaceEnsureUsecase
	storageClassEnsure *infrastructure_usecase.StorageClassEnsureUsecase
	secretUsecase      *secret_usecase.SecretUsecase
	logger             logger_port.LoggerPort
}

// NewAutoRecoveryUsecase creates a new auto recovery usecase
func NewAutoRecoveryUsecase(
	namespaceEnsure *infrastructure_usecase.NamespaceEnsureUsecase,
	storageClassEnsure *infrastructure_usecase.StorageClassEnsureUsecase,
	secretUsecase *secret_usecase.SecretUsecase,
	logger logger_port.LoggerPort,
) *AutoRecoveryUsecase {
	return &AutoRecoveryUsecase{
		namespaceEnsure:    namespaceEnsure,
		storageClassEnsure: storageClassEnsure,
		secretUsecase:      secretUsecase,
		logger:             logger,
	}
}

// ErrorInfo represents information about a detected error
type ErrorInfo struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Namespace   string `json:"namespace,omitempty"`
	Resource    string `json:"resource,omitempty"`
	Error       error  `json:"-"`
}

// RecoveryResult represents the result of an auto-recovery attempt
type RecoveryResult struct {
	Success     bool   `json:"success"`
	Action      string `json:"action"`
	Description string `json:"description"`
	Error       error  `json:"-"`
}

// RecoverFromError エラーの種類に応じた自動復旧
func (u *AutoRecoveryUsecase) RecoverFromError(ctx context.Context, err error) (*RecoveryResult, error) {
	u.logger.InfoWithContext("エラー自動復旧を開始", map[string]interface{}{
		"error": err.Error(),
	})
	
	errorInfo := u.analyzeError(err)
	
	switch errorInfo.Type {
	case "NamespaceNotFound":
		return u.recoverNamespaceNotFound(ctx, errorInfo)
		
	case "StorageClassNotFound":
		return u.recoverStorageClassNotFound(ctx, errorInfo)
		
	case "SecretNotFound":
		return u.recoverSecretNotFound(ctx, errorInfo)
		
	case "InsufficientPermissions":
		return u.handleInsufficientPermissions(ctx, errorInfo)
		
	default:
		u.logger.WarnWithContext("未対応のエラータイプです", map[string]interface{}{
			"error_type": errorInfo.Type,
			"error":      err.Error(),
		})
		return &RecoveryResult{
			Success:     false,
			Action:      "NoAction",
			Description: fmt.Sprintf("未対応のエラータイプ: %s", errorInfo.Type),
			Error:       err,
		}, nil
	}
}

// analyzeError analyzes an error and categorizes it
func (u *AutoRecoveryUsecase) analyzeError(err error) *ErrorInfo {
	if err == nil {
		return &ErrorInfo{Type: "Unknown", Description: "No error provided"}
	}
	
	errorStr := strings.ToLower(err.Error())
	
	// 名前空間不存在エラーの検出
	if strings.Contains(errorStr, "namespace") && 
	   (strings.Contains(errorStr, "not found") || strings.Contains(errorStr, "notfound")) {
		namespace := u.extractNamespaceFromError(err.Error())
		return &ErrorInfo{
			Type:        "NamespaceNotFound",
			Description: fmt.Sprintf("名前空間 '%s' が存在しません", namespace),
			Namespace:   namespace,
			Error:       err,
		}
	}
	
	// StorageClass不存在エラーの検出
	if strings.Contains(errorStr, "storageclass") && 
	   (strings.Contains(errorStr, "not found") || strings.Contains(errorStr, "notfound")) {
		return &ErrorInfo{
			Type:        "StorageClassNotFound",
			Description: "StorageClass 'standard' が存在しません",
			Resource:    "standard",
			Error:       err,
		}
	}
	
	// Secret不存在エラーの検出
	if strings.Contains(errorStr, "secret") && 
	   (strings.Contains(errorStr, "not found") || strings.Contains(errorStr, "notfound")) {
		secretInfo := u.extractSecretInfoFromError(err.Error())
		return &ErrorInfo{
			Type:        "SecretNotFound",
			Description: fmt.Sprintf("Secret '%s' が名前空間 '%s' に存在しません", secretInfo.Name, secretInfo.Namespace),
			Namespace:   secretInfo.Namespace,
			Resource:    secretInfo.Name,
			Error:       err,
		}
	}
	
	// 権限不足エラーの検出
	if strings.Contains(errorStr, "forbidden") || strings.Contains(errorStr, "unauthorized") {
		return &ErrorInfo{
			Type:        "InsufficientPermissions",
			Description: "Kubernetesクラスターへの権限が不足しています",
			Error:       err,
		}
	}
	
	return &ErrorInfo{
		Type:        "Unknown",
		Description: "未知のエラータイプ",
		Error:       err,
	}
}

// recoverNamespaceNotFound recovers from namespace not found errors
func (u *AutoRecoveryUsecase) recoverNamespaceNotFound(ctx context.Context, errorInfo *ErrorInfo) (*RecoveryResult, error) {
	u.logger.InfoWithContext("名前空間不存在エラーの自動修復中", map[string]interface{}{
		"namespace": errorInfo.Namespace,
	})
	
	if err := u.namespaceEnsure.EnsureNamespaceExists(ctx, errorInfo.Namespace); err != nil {
		return &RecoveryResult{
			Success:     false,
			Action:      "CreateNamespace",
			Description: fmt.Sprintf("名前空間 '%s' の作成に失敗", errorInfo.Namespace),
			Error:       err,
		}, err
	}
	
	u.logger.InfoWithContext("名前空間不存在エラーの自動修復が成功", map[string]interface{}{
		"namespace": errorInfo.Namespace,
	})
	
	return &RecoveryResult{
		Success:     true,
		Action:      "CreateNamespace",
		Description: fmt.Sprintf("名前空間 '%s' を正常に作成しました", errorInfo.Namespace),
	}, nil
}

// recoverStorageClassNotFound recovers from storage class not found errors
func (u *AutoRecoveryUsecase) recoverStorageClassNotFound(ctx context.Context, errorInfo *ErrorInfo) (*RecoveryResult, error) {
	u.logger.InfoWithContext("StorageClass不存在エラーの自動修復中", map[string]interface{}{
		"storage_class": errorInfo.Resource,
	})
	
	if err := u.storageClassEnsure.EnsureDefaultStorageClass(ctx); err != nil {
		// StorageClassエラーは警告として処理
		u.logger.WarnWithContext("StorageClass修復は警告として処理", map[string]interface{}{
			"storage_class": errorInfo.Resource,
			"error":         err.Error(),
		})
	}
	
	return &RecoveryResult{
		Success:     true, // 警告として成功扱い
		Action:      "ValidateStorageClass",
		Description: "StorageClass問題を警告として処理しました",
	}, nil
}

// recoverSecretNotFound recovers from secret not found errors
func (u *AutoRecoveryUsecase) recoverSecretNotFound(ctx context.Context, errorInfo *ErrorInfo) (*RecoveryResult, error) {
	u.logger.InfoWithContext("Secret不存在エラーの自動修復中", map[string]interface{}{
		"secret":    errorInfo.Resource,
		"namespace": errorInfo.Namespace,
	})
	
	// この実装では基本的なSecretリカバリのみ対応
	// より詳細なSecret自動生成は secretUsecase で実装
	u.logger.InfoWithContext("Secret自動生成は別のユースケースで処理されます", map[string]interface{}{
		"secret":    errorInfo.Resource,
		"namespace": errorInfo.Namespace,
	})
	
	return &RecoveryResult{
		Success:     false,
		Action:      "SecretRecovery",
		Description: "Secret自動生成は専用ユースケースで処理する必要があります",
		Error:       errorInfo.Error,
	}, nil
}

// handleInsufficientPermissions handles permission-related errors
func (u *AutoRecoveryUsecase) handleInsufficientPermissions(ctx context.Context, errorInfo *ErrorInfo) (*RecoveryResult, error) {
	u.logger.ErrorWithContext("権限不足エラーが検出されました", map[string]interface{}{
		"error": errorInfo.Error.Error(),
	})
	
	return &RecoveryResult{
		Success:     false,
		Action:      "CheckPermissions",
		Description: "Kubernetesクラスターへの権限を確認してください",
		Error:       errorInfo.Error,
	}, errorInfo.Error
}

// RecoverFromMultipleErrors 複数のエラーからの自動復旧
func (u *AutoRecoveryUsecase) RecoverFromMultipleErrors(ctx context.Context, errors []error) ([]*RecoveryResult, error) {
	u.logger.InfoWithContext("複数エラーの自動復旧を開始", map[string]interface{}{
		"error_count": len(errors),
	})
	
	var results []*RecoveryResult
	var finalError error
	successCount := 0
	
	for i, err := range errors {
		u.logger.InfoWithContext("エラーの個別処理中", map[string]interface{}{
			"index": i + 1,
			"total": len(errors),
			"error": err.Error(),
		})
		
		result, recoverErr := u.RecoverFromError(ctx, err)
		results = append(results, result)
		
		if result.Success {
			successCount++
		} else if recoverErr != nil {
			finalError = recoverErr
		}
	}
	
	u.logger.InfoWithContext("複数エラーの自動復旧が完了", map[string]interface{}{
		"total_errors":      len(errors),
		"successful_recovery": successCount,
		"failed_recovery":     len(errors) - successCount,
	})
	
	return results, finalError
}

// extractNamespaceFromError extracts namespace name from error message
func (u *AutoRecoveryUsecase) extractNamespaceFromError(errorMsg string) string {
	// パターン: namespaces "namespace-name" not found
	re := regexp.MustCompile(`namespaces?\s+"([^"]+)"\s+not found`)
	matches := re.FindStringSubmatch(errorMsg)
	if len(matches) > 1 {
		return matches[1]
	}
	
	// パターン: namespace namespace-name not found
	re = regexp.MustCompile(`namespace\s+([^\s]+)\s+not found`)
	matches = re.FindStringSubmatch(errorMsg)
	if len(matches) > 1 {
		return matches[1]
	}
	
	return "unknown"
}

// SecretInfo represents information about a secret
type SecretInfo struct {
	Name      string
	Namespace string
}

// extractSecretInfoFromError extracts secret information from error message
func (u *AutoRecoveryUsecase) extractSecretInfoFromError(errorMsg string) SecretInfo {
	secretInfo := SecretInfo{
		Name:      "unknown",
		Namespace: "unknown",
	}
	
	// パターン: secret "secret-name" in namespace "namespace-name" not found
	re := regexp.MustCompile(`secret\s+"([^"]+)"\s+.*namespace\s+"([^"]+)"\s+not found`)
	matches := re.FindStringSubmatch(errorMsg)
	if len(matches) > 2 {
		secretInfo.Name = matches[1]
		secretInfo.Namespace = matches[2]
		return secretInfo
	}
	
	// パターン: secrets "secret-name" not found
	re = regexp.MustCompile(`secrets?\s+"([^"]+)"\s+not found`)
	matches = re.FindStringSubmatch(errorMsg)
	if len(matches) > 1 {
		secretInfo.Name = matches[1]
	}
	
	return secretInfo
}

// IsRecoverableError checks if an error is recoverable
func (u *AutoRecoveryUsecase) IsRecoverableError(err error) bool {
	if err == nil {
		return false
	}
	
	errorInfo := u.analyzeError(err)
	
	switch errorInfo.Type {
	case "NamespaceNotFound", "StorageClassNotFound":
		return true
	case "SecretNotFound":
		return true // 部分的に対応
	case "InsufficientPermissions":
		return false // 自動復旧不可
	default:
		return false
	}
}

// GetRecoveryStrategies returns available recovery strategies for an error
func (u *AutoRecoveryUsecase) GetRecoveryStrategies(err error) []string {
	if err == nil {
		return []string{}
	}
	
	errorInfo := u.analyzeError(err)
	
	switch errorInfo.Type {
	case "NamespaceNotFound":
		return []string{
			"名前空間の自動作成",
			"名前空間存在確認",
		}
	case "StorageClassNotFound":
		return []string{
			"利用可能なStorageClassの確認",
			"代替StorageClassの使用",
			"警告として処理継続",
		}
	case "SecretNotFound":
		return []string{
			"Secret存在確認",
			"Secret自動生成（制限あり）",
		}
	case "InsufficientPermissions":
		return []string{
			"権限確認の推奨",
			"管理者への連絡",
		}
	default:
		return []string{"自動復旧戦略なし"}
	}
}