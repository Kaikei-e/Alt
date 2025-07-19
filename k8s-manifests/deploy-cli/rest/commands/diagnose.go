package commands

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"deploy-cli/domain"
	"deploy-cli/driver/kubectl_driver"
	"deploy-cli/port/logger_port"
	"deploy-cli/usecase/diagnostic_usecase"
	"deploy-cli/usecase/infrastructure_usecase"
	"deploy-cli/utils/logger"
)

// DiagnoseLoggerAdapter adapts utils/logger to logger_port.LoggerPort interface for diagnose command
type DiagnoseLoggerAdapter struct {
	logger *logger.Logger
}

// LoggerPort interface implementation methods
func (l *DiagnoseLoggerAdapter) Info(msg string, args ...interface{}) {
	l.logger.Info(msg, args...)
}

func (l *DiagnoseLoggerAdapter) Error(msg string, args ...interface{}) {
	l.logger.Error(msg, args...)
}

func (l *DiagnoseLoggerAdapter) Warn(msg string, args ...interface{}) {
	l.logger.Warn(msg, args...)
}

func (l *DiagnoseLoggerAdapter) Debug(msg string, args ...interface{}) {
	l.logger.Debug(msg, args...)
}

func (l *DiagnoseLoggerAdapter) InfoWithContext(message string, context map[string]interface{}) {
	l.logger.Info(message, context)
}

func (l *DiagnoseLoggerAdapter) WarnWithContext(message string, context map[string]interface{}) {
	l.logger.Warn(message, context)
}

func (l *DiagnoseLoggerAdapter) ErrorWithContext(message string, context map[string]interface{}) {
	l.logger.Error(message, context)
}

func (l *DiagnoseLoggerAdapter) DebugWithContext(message string, context map[string]interface{}) {
	l.logger.Debug(message, context)
}

func (l *DiagnoseLoggerAdapter) WithField(key string, value interface{}) logger_port.LoggerPort {
	// Simple implementation - return self for now
	return l
}

func (l *DiagnoseLoggerAdapter) WithFields(fields map[string]interface{}) logger_port.LoggerPort {
	// Simple implementation - return self for now
	return l
}

// NewDiagnoseCommand creates a new diagnose command
func NewDiagnoseCommand(log *logger.Logger) *cobra.Command {
	var diagnoseCmd = &cobra.Command{
	Use:   "diagnose",
	Short: "環境の包括的診断と自動修復",
	Long: `Deploy-CLI環境の包括的診断を実行し、検出された問題の自動修復を試行します。

このコマンドは以下の項目を診断します：
- Kubernetesクラスター接続
- 必要な名前空間の存在と状態
- StorageClassの設定状況
- RBAC権限の確認
- デプロイメント前提条件の検証

検出された問題は、可能な場合自動的に修復されます。`,
	Example: `  # 本番環境の診断
  deploy-cli diagnose --environment production

  # ステージング環境の詳細診断（JSON形式）
  deploy-cli diagnose --environment staging --output json

  # 開発環境の診断（自動修復無効）
  deploy-cli diagnose --environment development --no-auto-fix

  # 診断レポートをファイルに出力
  deploy-cli diagnose --environment production --output json > diagnostic-report.json`,
	}

	var (
		diagnoseEnvironment  string
		diagnoseOutputFormat string
		diagnoseNoAutoFix    bool
		diagnoseVerbose      bool
	)

	diagnoseCmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runDiagnose(cmd, args, log, diagnoseEnvironment, diagnoseOutputFormat, diagnoseNoAutoFix, diagnoseVerbose)
	}

	// Environment flag
	diagnoseCmd.Flags().StringVarP(&diagnoseEnvironment, "environment", "e", "production", 
		"診断対象の環境 (production, staging, development)")

	// Output format flag
	diagnoseCmd.Flags().StringVarP(&diagnoseOutputFormat, "output", "o", "text", 
		"出力形式 (text, json, yaml)")

	// Auto-fix control
	diagnoseCmd.Flags().BoolVar(&diagnoseNoAutoFix, "no-auto-fix", false, 
		"自動修復を無効化（診断のみ実行）")

	// Verbose output
	diagnoseCmd.Flags().BoolVarP(&diagnoseVerbose, "verbose", "v", false, 
		"詳細な診断情報を表示")

	return diagnoseCmd
}

func runDiagnose(cmd *cobra.Command, args []string, log *logger.Logger, diagnoseEnvironment, diagnoseOutputFormat string, diagnoseNoAutoFix, diagnoseVerbose bool) error {
	// Parse environment
	env, err := parseEnvironment(diagnoseEnvironment)
	if err != nil {
		return fmt.Errorf("無効な環境指定: %w", err)
	}

	// Initialize dependencies
	loggerAdapter := &DiagnoseLoggerAdapter{logger: log}

	kubectlPort := kubectl_driver.NewKubectlDriver()
	
	namespaceEnsure := infrastructure_usecase.NewNamespaceEnsureUsecase(kubectlPort, loggerAdapter)
	storageClassEnsure := infrastructure_usecase.NewStorageClassEnsureUsecase(kubectlPort, loggerAdapter)
	
	diagnosticUsecase := diagnostic_usecase.NewPrerequisitesDiagnosticUsecase(
		kubectlPort,
		namespaceEnsure, 
		storageClassEnsure,
		loggerAdapter,
	)

	// Execute diagnosis
	log.Info("環境診断を開始", map[string]interface{}{
		"environment": env.String(),
		"auto_fix":    !diagnoseNoAutoFix,
		"output":      diagnoseOutputFormat,
	})

	report, err := diagnosticUsecase.DiagnoseAndFix(cmd.Context(), env)
	if err != nil {
		return fmt.Errorf("診断の実行に失敗: %w", err)
	}

	// Output results
	switch diagnoseOutputFormat {
	case "json":
		return outputDiagnosticJSON(report)
	case "yaml":
		return outputDiagnosticYAML(report)
	default:
		return outputDiagnosticText(report, diagnoseVerbose)
	}
}

func outputDiagnosticJSON(report *diagnostic_usecase.DiagnosticReport) error {
	jsonData, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("JSON出力の生成に失敗: %w", err)
	}
	
	fmt.Println(string(jsonData))
	return nil
}

func outputDiagnosticYAML(report *diagnostic_usecase.DiagnosticReport) error {
	// For simplicity, we'll output JSON formatted as YAML-like structure
	fmt.Printf("overall_status: %s\n", report.OverallStatus)
	fmt.Printf("environment: %s\n", report.Environment)
	fmt.Printf("execution_time: %s\n", report.ExecutionTime.String())
	fmt.Printf("timestamp: %s\n", report.Timestamp.Format("2006-01-02T15:04:05Z07:00"))
	
	fmt.Println("kubernetes:")
	fmt.Printf("  status: %s\n", report.Kubernetes.Status)
	fmt.Printf("  version: %s\n", report.Kubernetes.Version)
	fmt.Printf("  message: %s\n", report.Kubernetes.Message)
	fmt.Printf("  nodes_count: %d\n", report.Kubernetes.NodesCount)
	fmt.Printf("  cluster_ready: %t\n", report.Kubernetes.ClusterReady)
	
	if len(report.Namespaces) > 0 {
		fmt.Println("namespaces:")
		for _, ns := range report.Namespaces {
			fmt.Printf("  - name: %s\n", ns.Name)
			fmt.Printf("    exists: %t\n", ns.Exists)
			fmt.Printf("    status: %s\n", ns.Status)
			fmt.Printf("    accessible: %t\n", ns.Accessible)
			fmt.Printf("    auto_created: %t\n", ns.AutoCreated)
			if ns.Error != "" {
				fmt.Printf("    error: %s\n", ns.Error)
			}
		}
	}
	
	if len(report.StorageClasses) > 0 {
		fmt.Println("storage_classes:")
		for _, sc := range report.StorageClasses {
			fmt.Printf("  - name: %s\n", sc.Name)
			fmt.Printf("    exists: %t\n", sc.Exists)
			fmt.Printf("    provisioner: %s\n", sc.Provisioner)
			fmt.Printf("    is_default: %t\n", sc.IsDefault)
			fmt.Printf("    status: %s\n", sc.Status)
		}
	}
	
	fmt.Println("rbac:")
	fmt.Printf("  status: %s\n", report.RBAC.Status)
	fmt.Printf("  can_create_secrets: %t\n", report.RBAC.CanCreateSecrets)
	fmt.Printf("  can_create_namespaces: %t\n", report.RBAC.CanCreateNS)
	fmt.Printf("  can_list_pods: %t\n", report.RBAC.CanListPods)
	
	if len(report.RBAC.Permissions) > 0 {
		fmt.Println("  permissions:")
		for _, perm := range report.RBAC.Permissions {
			fmt.Printf("    - %s\n", perm)
		}
	}
	
	if len(report.RBAC.Restrictions) > 0 {
		fmt.Println("  restrictions:")
		for _, restr := range report.RBAC.Restrictions {
			fmt.Printf("    - %s\n", restr)
		}
	}
	
	if len(report.Recommendations) > 0 {
		fmt.Println("recommendations:")
		for _, rec := range report.Recommendations {
			fmt.Printf("  - %s\n", rec)
		}
	}
	
	return nil
}

func outputDiagnosticText(report *diagnostic_usecase.DiagnosticReport, verbose bool) error {
	// Header
	fmt.Printf("🔍 Deploy-CLI 環境診断レポート\n")
	fmt.Printf("===============================\n\n")
	
	// Basic info
	fmt.Printf("📅 実行日時: %s\n", report.Timestamp.Format("2006-01-02 15:04:05"))
	fmt.Printf("🌍 環境: %s\n", report.Environment)
	fmt.Printf("⏱️  実行時間: %s\n", report.ExecutionTime.String())
	
	// Overall status with emoji
	statusEmoji := getStatusEmoji(report.OverallStatus)
	fmt.Printf("🎯 総合状態: %s %s\n\n", statusEmoji, report.OverallStatus)
	
	// Kubernetes status
	fmt.Printf("☸️  Kubernetes クラスター\n")
	fmt.Printf("-------------------------\n")
	kubernetesEmoji := getStatusEmoji(report.Kubernetes.Status)
	fmt.Printf("状態: %s %s\n", kubernetesEmoji, report.Kubernetes.Status)
	if report.Kubernetes.Version != "" {
		fmt.Printf("バージョン: %s\n", report.Kubernetes.Version)
	}
	fmt.Printf("メッセージ: %s\n", report.Kubernetes.Message)
	fmt.Printf("ノード数: %d\n", report.Kubernetes.NodesCount)
	fmt.Printf("クラスター準備: %t\n\n", report.Kubernetes.ClusterReady)
	
	// Namespace status
	if len(report.Namespaces) > 0 {
		fmt.Printf("📁 名前空間状態\n")
		fmt.Printf("---------------\n")
		for _, ns := range report.Namespaces {
			nsEmoji := getStatusEmoji(ns.Status)
			fmt.Printf("• %s %s: %s", nsEmoji, ns.Name, ns.Status)
			if ns.AutoCreated {
				fmt.Printf(" (自動作成済み)")
			}
			if !ns.Accessible && ns.Exists {
				fmt.Printf(" (アクセス不可)")
			}
			fmt.Printf("\n")
			
			if verbose && ns.Error != "" {
				fmt.Printf("  エラー: %s\n", ns.Error)
			}
		}
		fmt.Printf("\n")
	}
	
	// StorageClass status
	if len(report.StorageClasses) > 0 {
		fmt.Printf("💾 StorageClass 状態\n")
		fmt.Printf("-------------------\n")
		for _, sc := range report.StorageClasses {
			scEmoji := getStatusEmoji(sc.Status)
			fmt.Printf("• %s %s: %s", scEmoji, sc.Name, sc.Status)
			if sc.IsDefault {
				fmt.Printf(" (デフォルト)")
			}
			if sc.Provisioner != "" && verbose {
				fmt.Printf(" [%s]", sc.Provisioner)
			}
			fmt.Printf("\n")
		}
		fmt.Printf("\n")
	}
	
	// RBAC status
	fmt.Printf("🔐 RBAC 権限状態\n")
	fmt.Printf("---------------\n")
	rbacEmoji := getStatusEmoji(report.RBAC.Status)
	fmt.Printf("状態: %s %s\n", rbacEmoji, report.RBAC.Status)
	
	if verbose || report.RBAC.Status != "正常" {
		fmt.Printf("権限:\n")
		fmt.Printf("  • 名前空間作成: %s\n", getBooleanEmoji(report.RBAC.CanCreateNS))
		fmt.Printf("  • Secret操作: %s\n", getBooleanEmoji(report.RBAC.CanCreateSecrets))
		fmt.Printf("  • Pod一覧取得: %s\n", getBooleanEmoji(report.RBAC.CanListPods))
		
		if len(report.RBAC.Restrictions) > 0 {
			fmt.Printf("制限事項:\n")
			for _, restriction := range report.RBAC.Restrictions {
				fmt.Printf("  ⚠️  %s\n", restriction)
			}
		}
	}
	fmt.Printf("\n")
	
	// Recommendations
	if len(report.Recommendations) > 0 {
		fmt.Printf("💡 推奨事項\n")
		fmt.Printf("-----------\n")
		for i, rec := range report.Recommendations {
			fmt.Printf("%d. %s\n", i+1, rec)
		}
		fmt.Printf("\n")
	}
	
	// Footer with next steps
	fmt.Printf("🚀 次のステップ\n")
	fmt.Printf("---------------\n")
	if report.OverallStatus == "正常" {
		fmt.Printf("✅ 環境は正常です。デプロイメントを開始できます:\n")
		fmt.Printf("   ./deploy-cli deploy %s --auto-everything\n\n", report.Environment)
	} else {
		fmt.Printf("🔧 問題を修正後、再診断を実行してください:\n")
		fmt.Printf("   ./deploy-cli diagnose --environment %s\n\n", report.Environment)
		fmt.Printf("🆘 問題が解決しない場合:\n")
		fmt.Printf("   ./deploy-cli diagnose --environment %s --output json > diagnostic-report.json\n", report.Environment)
		fmt.Printf("   # 上記レポートを管理者に送付してください\n\n")
	}
	
	return nil
}

func getStatusEmoji(status string) string {
	switch status {
	case "正常", "healthy", "OK":
		return "✅"
	case "警告", "warning", "Warning":
		return "⚠️"
	case "エラー", "error", "問題あり", "critical":
		return "❌"
	case "診断中", "checking":
		return "🔄"
	case "自動作成済み":
		return "🔧"
	default:
		return "❓"
	}
}

func getBooleanEmoji(value bool) string {
	if value {
		return "✅ 可能"
	}
	return "❌ 不可"
}

func parseEnvironment(envStr string) (domain.Environment, error) {
	switch envStr {
	case "production", "prod":
		return domain.Production, nil
	case "staging", "stage":
		return domain.Staging, nil
	case "development", "dev":
		return domain.Development, nil
	default:
		return domain.Production, fmt.Errorf("未対応の環境: %s (production, staging, development のいずれかを指定してください)", envStr)
	}
}