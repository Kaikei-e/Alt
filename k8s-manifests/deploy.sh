#!/bin/bash
# deploy.sh
# Kustomizeを使用したKubernetesデプロイメントの統合スクリプト

set -euo pipefail

# カラー出力の定義
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
MAGENTA='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# デフォルト値の設定
ENVIRONMENT="${1:-development}"
ACTION="${2:-apply}"
DRY_RUN="${3:-false}"
KUSTOMIZE_BUILD_DIR="/tmp/alt-k8s-build"
BACKUP_DIR="k8s/backups/$(date +%Y%m%d-%H%M%S)"

# 使用方法を表示する関数
usage() {
    echo -e "${CYAN}使用方法: $0 [環境] [アクション] [ドライラン]${NC}"
    echo -e "${CYAN}  環境: development | staging | production (デフォルト: development)${NC}"
    echo -e "${CYAN}  アクション: apply | delete | rollback | status (デフォルト: apply)${NC}"
    echo -e "${CYAN}  ドライラン: true | false (デフォルト: false)${NC}"
    echo -e "${CYAN}例:${NC}"
    echo -e "${CYAN}  $0 production apply true  # 本番環境へのドライラン${NC}"
    echo -e "${CYAN}  $0 staging apply           # ステージング環境へのデプロイ${NC}"
    echo -e "${CYAN}  $0 development status      # 開発環境の状態確認${NC}"
    exit 1
}

# 環境の検証
validate_environment() {
    case "$ENVIRONMENT" in
        development|staging|production)
            echo -e "${GREEN}環境: ${ENVIRONMENT}${NC}"
            ;;
        *)
            echo -e "${RED}エラー: 無効な環境 '${ENVIRONMENT}'${NC}"
            usage
            ;;
    esac
}

# 前提条件の確認
check_prerequisites() {
    echo -e "${BLUE}前提条件を確認中...${NC}"

    # kubectl の確認
    if ! command -v kubectl &> /dev/null; then
        echo -e "${RED}エラー: kubectl がインストールされていません${NC}"
        exit 1
    fi

    # kustomize の確認
    if ! command -v kustomize &> /dev/null; then
        echo -e "${YELLOW}警告: kustomize がインストールされていません。kubectl内蔵のkustomizeを使用します${NC}"
        KUSTOMIZE_CMD="kubectl kustomize"
    else
        KUSTOMIZE_CMD="kustomize build"
    fi

    # クラスター接続の確認
    if ! kubectl cluster-info &> /dev/null; then
        echo -e "${RED}エラー: Kubernetesクラスターに接続できません${NC}"
        exit 1
    fi

    echo -e "${GREEN}✓ すべての前提条件が満たされています${NC}"
}

# ストレージディレクトリの作成
create_storage_directories() {
    echo -e "${BLUE}ストレージディレクトリを作成中...${NC}"

    if [ "$DRY_RUN" = "true" ]; then
        echo -e "${YELLOW}ドライラン: ストレージディレクトリの作成をスキップ${NC}"
    else
        # 必要なディレクトリを作成
        sudo mkdir -p /mnt/data/postgres
        sudo mkdir -p /mnt/data/clickhouse
        sudo mkdir -p /mnt/data/meilisearch
        sudo mkdir -p /mnt/data/news-creator-models

        # パーミッションを設定
        sudo chmod 777 /mnt/data/postgres
        sudo chmod 777 /mnt/data/clickhouse
        sudo chmod 777 /mnt/data/meilisearch
        sudo chmod 777 /mnt/data/news-creator-models

        echo -e "${GREEN}✓ ストレージディレクトリの作成が完了しました${NC}"
    fi
}

# 名前空間の作成
create_namespaces() {
    echo -e "${BLUE}名前空間を作成中...${NC}"

    if [ "$DRY_RUN" = "true" ]; then
        echo -e "${YELLOW}ドライラン: kubectl apply -f k8s/namespaces.yaml${NC}"
    else
        kubectl apply -f k8s/namespaces.yaml
    fi
}

# シークレットの検証
validate_secrets() {
    echo -e "${BLUE}シークレットを検証中...${NC}"

    NAMESPACE="alt-${ENVIRONMENT}"
    if [ "$ENVIRONMENT" = "production" ]; then
        NAMESPACE="alt-production"
    fi

    if [ -f "k8s/scripts/validate-secrets.sh" ]; then
        if [ "$DRY_RUN" = "false" ]; then
            bash k8s/scripts/validate-secrets.sh ${NAMESPACE} || {
                echo -e "${YELLOW}警告: 一部のシークレットが見つかりません。シークレットの作成をスキップしますか？ (y/N)${NC}"
                read -r response
                if [[ ! "$response" =~ ^([yY][eE][sS]|[yY])$ ]]; then
                    exit 1
                fi
            }
        fi
    fi
}

# Kustomizeビルドの実行
build_manifests() {
    echo -e "${BLUE}マニフェストをビルド中...${NC}"

    mkdir -p ${KUSTOMIZE_BUILD_DIR}
    BUILD_FILE="${KUSTOMIZE_BUILD_DIR}/${ENVIRONMENT}-manifest.yaml"

    echo -e "${CYAN}実行: ${KUSTOMIZE_CMD} k8s/overlays/${ENVIRONMENT}${NC}"

    if [ "$KUSTOMIZE_CMD" = "kubectl kustomize" ]; then
        kubectl kustomize k8s/overlays/${ENVIRONMENT} > ${BUILD_FILE}
    else
        kustomize build k8s/overlays/${ENVIRONMENT} > ${BUILD_FILE}
    fi

    # ビルド結果の検証
    if [ ! -s ${BUILD_FILE} ]; then
        echo -e "${RED}エラー: マニフェストのビルドに失敗しました${NC}"
        exit 1
    fi

    echo -e "${GREEN}✓ マニフェストのビルドが完了しました: ${BUILD_FILE}${NC}"

    # マニフェストの概要を表示
    echo -e "${CYAN}ビルドされたリソース:${NC}"
    if command -v yq &> /dev/null; then
        yq eval '.kind + "/" + .metadata.name + " (" + .metadata.namespace + ")"' ${BUILD_FILE} 2>/dev/null | sort | uniq || echo "  リソース情報の解析に失敗しました"
    else
        echo "  リソース一覧の表示には yq が必要です"
        echo "  インストール: sudo wget https://github.com/mikefarah/yq/releases/latest/download/yq_linux_amd64 -O /usr/bin/yq && sudo chmod +x /usr/bin/yq"
    fi
}

# デプロイメントの実行
deploy_application() {
    echo -e "${BLUE}アプリケーションをデプロイ中...${NC}"

    BUILD_FILE="${KUSTOMIZE_BUILD_DIR}/${ENVIRONMENT}-manifest.yaml"

    if [ "$DRY_RUN" = "true" ]; then
        echo -e "${YELLOW}ドライランモード: 実際のデプロイは実行されません${NC}"
        kubectl apply -f ${BUILD_FILE} --dry-run=client
    else
        # 現在の状態をバックアップ
        backup_current_state

        # デプロイメントの実行
        echo -e "${CYAN}リソースをデプロイ中...${NC}"
        kubectl apply -f ${BUILD_FILE}

        echo -e "${CYAN}デプロイメントの完了を待機中...${NC}"
        sleep 30
    fi
}

# 現在の状態をバックアップ
backup_current_state() {
    echo -e "${BLUE}現在の状態をバックアップ中...${NC}"

    mkdir -p ${BACKUP_DIR}

    NAMESPACE="alt-${ENVIRONMENT}"
    if [ "$ENVIRONMENT" = "production" ]; then
        # 本番環境の全名前空間をバックアップ
        for ns in alt-apps alt-database alt-search alt-observability alt-ingress; do
            kubectl get all,configmap,secret,pvc -n ${ns} -o yaml > ${BACKUP_DIR}/${ns}-backup.yaml 2>/dev/null || true
        done
    else
        kubectl get all,configmap,secret,pvc -n ${NAMESPACE} -o yaml > ${BACKUP_DIR}/${NAMESPACE}-backup.yaml 2>/dev/null || true
    fi

    echo -e "${GREEN}✓ バックアップ完了: ${BACKUP_DIR}${NC}"
}

# デプロイメントの状態確認
check_deployment_status() {
    echo -e "${BLUE}デプロイメントの状態を確認中...${NC}"

    NAMESPACE="alt-${ENVIRONMENT}"
    if [ "$ENVIRONMENT" = "production" ]; then
        NAMESPACES=(alt-apps alt-database alt-search alt-observability alt-ingress)
    else
        NAMESPACES=(${NAMESPACE})
    fi

    for ns in "${NAMESPACES[@]}"; do
        echo -e "${CYAN}名前空間: ${ns}${NC}"

        # Deploymentの状態
        echo -e "${MAGENTA}Deployments:${NC}"
        kubectl get deployment -n ${ns} -o wide 2>/dev/null || echo "  デプロイメントなし"

        # StatefulSetの状態
        echo -e "${MAGENTA}StatefulSets:${NC}"
        kubectl get statefulset -n ${ns} -o wide 2>/dev/null || echo "  StatefulSetなし"

        # Podの状態
        echo -e "${MAGENTA}Pods:${NC}"
        kubectl get pods -n ${ns} -o wide 2>/dev/null || echo "  Podなし"

        # Serviceの状態
        echo -e "${MAGENTA}Services:${NC}"
        kubectl get service -n ${ns} 2>/dev/null || echo "  サービスなし"

        echo ""
    done

    # ヘルスチェック
    perform_health_check
}

# ヘルスチェックの実行
perform_health_check() {
    echo -e "${BLUE}ヘルスチェックを実行中...${NC}"

    NAMESPACE="alt-${ENVIRONMENT}"
    if [ "$ENVIRONMENT" = "production" ]; then
        NAMESPACE="alt-apps"
    fi

    # バックエンドのヘルスチェック
    if kubectl get service alt-backend -n ${NAMESPACE} &> /dev/null; then
        echo -e "${CYAN}バックエンドのヘルスチェック:${NC}"
        BACKEND_POD=$(kubectl get pod -n ${NAMESPACE} -l app.kubernetes.io/name=alt-backend -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
        if [ ! -z "$BACKEND_POD" ]; then
            kubectl exec -n ${NAMESPACE} ${BACKEND_POD} -- wget -qO- http://localhost:9000/v1/health 2>/dev/null && \
                echo -e "${GREEN}✓ バックエンドは正常です${NC}" || \
                echo -e "${RED}✗ バックエンドのヘルスチェックに失敗${NC}"
        fi
    fi

    # フロントエンドのヘルスチェック
    if kubectl get service alt-frontend -n ${NAMESPACE} &> /dev/null; then
        echo -e "${CYAN}フロントエンドのヘルスチェック:${NC}"
        FRONTEND_POD=$(kubectl get pod -n ${NAMESPACE} -l app.kubernetes.io/name=alt-frontend -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
        if [ ! -z "$FRONTEND_POD" ]; then
            kubectl exec -n ${NAMESPACE} ${FRONTEND_POD} -- wget -qO- http://localhost:3000 2>/dev/null && \
                echo -e "${GREEN}✓ フロントエンドは正常です${NC}" || \
                echo -e "${RED}✗ フロントエンドのヘルスチェックに失敗${NC}"
        fi
    fi
}

# ロールバックの実行
rollback_deployment() {
    echo -e "${BLUE}デプロイメントをロールバック中...${NC}"

    # 最新のバックアップを探す
    LATEST_BACKUP=$(ls -t k8s/backups/*/alt-*-backup.yaml 2>/dev/null | head -1)

    if [ -z "$LATEST_BACKUP" ]; then
        echo -e "${RED}エラー: バックアップが見つかりません${NC}"
        exit 1
    fi

    echo -e "${YELLOW}最新のバックアップ: ${LATEST_BACKUP}${NC}"
    echo -e "${YELLOW}このバックアップにロールバックしますか？ (y/N)${NC}"
    read -r response

    if [[ "$response" =~ ^([yY][eE][sS]|[yY])$ ]]; then
        kubectl apply -f ${LATEST_BACKUP}
        echo -e "${GREEN}✓ ロールバックが完了しました${NC}"
    else
        echo -e "${YELLOW}ロールバックがキャンセルされました${NC}"
    fi
}

# クリーンアップ
cleanup_deployment() {
    echo -e "${BLUE}デプロイメントを削除中...${NC}"

    BUILD_FILE="${KUSTOMIZE_BUILD_DIR}/${ENVIRONMENT}-manifest.yaml"

    if [ -f ${BUILD_FILE} ]; then
        if [ "$DRY_RUN" = "true" ]; then
            echo -e "${YELLOW}ドライラン: kubectl delete -f ${BUILD_FILE}${NC}"
        else
            echo -e "${YELLOW}警告: すべてのリソースが削除されます。続行しますか？ (y/N)${NC}"
            read -r response
            if [[ "$response" =~ ^([yY][eE][sS]|[yY])$ ]]; then
                kubectl delete -f ${BUILD_FILE}
                echo -e "${GREEN}✓ リソースの削除が完了しました${NC}"
            else
                echo -e "${YELLOW}削除がキャンセルされました${NC}"
            fi
        fi
    else
        echo -e "${RED}エラー: ビルドファイルが見つかりません${NC}"
        exit 1
    fi
}

# メイン処理
main() {
    echo -e "${MAGENTA}========================================${NC}"
    echo -e "${MAGENTA}ALTプラットフォーム Kubernetesデプロイメント${NC}"
    echo -e "${MAGENTA}========================================${NC}"

    validate_environment
    check_prerequisites

    case "$ACTION" in
        apply)
            create_storage_directories
            create_namespaces
            validate_secrets
            build_manifests
            deploy_application
            if [ "$DRY_RUN" = "false" ]; then
                sleep 5
                check_deployment_status
            fi
            ;;
        delete)
            build_manifests
            cleanup_deployment
            ;;
        rollback)
            rollback_deployment
            ;;
        status)
            check_deployment_status
            ;;
        *)
            echo -e "${RED}エラー: 無効なアクション '${ACTION}'${NC}"
            usage
            ;;
    esac

    echo -e "${MAGENTA}========================================${NC}"
    echo -e "${GREEN}処理が完了しました！${NC}"
}

# スクリプトの実行
main