#!/bin/bash
# build-images.sh
# Dockerイメージのビルドとcontainerdへのインポートを行うスクリプト

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
BUILD_ALL="${1:-true}"
SERVICES="${2:-}"

# サービス定義
declare -A SERVICE_CONFIGS=(
    ["alt-backend"]="alt-backend/Dockerfile.backend"
    ["alt-frontend"]="alt-frontend/Dockerfile.frontend"
    ["pre-processor"]="pre-processor/Dockerfile"
    ["news-creator"]="news-creator/Dockerfile.creator"
    ["search-indexer"]="search-indexer/Dockerfile.search-indexer"
    ["tag-generator"]="tag-generator/Dockerfile.tag-generator"
    ["migrate"]="migrate/Dockerfile.migrate"
    ["rask-log-aggregator"]="rask-log-aggregator/Dockerfile.rask-log-aggregator"
    ["rask-log-forwarder"]="rask-log-forwarder/app/Dockerfile.rask-log-forwarder"
)

# 使用方法を表示する関数
usage() {
    echo -e "${CYAN}使用方法: $0 [BUILD_ALL] [SERVICES]${NC}"
    echo -e "${CYAN}  BUILD_ALL: true | false (デフォルト: true)${NC}"
    echo -e "${CYAN}  SERVICES: カンマ区切りのサービス名 (例: alt-backend,pre-processor)${NC}"
    echo -e "${CYAN}例:${NC}"
    echo -e "${CYAN}  $0 true                    # 全サービスをビルド${NC}"
    echo -e "${CYAN}  $0 false alt-backend       # alt-backendのみビルド${NC}"
    echo -e "${CYAN}  $0 false alt-backend,pre-processor  # 指定したサービスのみビルド${NC}"
    exit 1
}

# 前提条件の確認
check_prerequisites() {
    echo -e "${BLUE}前提条件を確認中...${NC}"

    # Docker の確認
    if ! command -v docker &> /dev/null; then
        echo -e "${RED}エラー: Docker がインストールされていません${NC}"
        exit 1
    fi

    # containerd の確認
    if ! command -v ctr &> /dev/null; then
        echo -e "${RED}エラー: containerd (ctr) がインストールされていません${NC}"
        exit 1
    fi

    # Docker デーモンの確認
    if ! docker info &> /dev/null; then
        echo -e "${RED}エラー: Docker デーモンに接続できません${NC}"
        exit 1
    fi

    echo -e "${GREEN}✓ すべての前提条件が満たされています${NC}"
}

# サービスディレクトリの確認
validate_service_directories() {
    echo -e "${BLUE}サービスディレクトリを確認中...${NC}"

    for service in "${!SERVICE_CONFIGS[@]}"; do
        dockerfile_path="${SERVICE_CONFIGS[$service]}"
        service_dir=$(dirname "$dockerfile_path")

        if [ ! -d "$service_dir" ]; then
            echo -e "${YELLOW}警告: サービスディレクトリが見つかりません: ${service_dir}${NC}"
            unset SERVICE_CONFIGS["$service"]
        elif [ ! -f "$dockerfile_path" ]; then
            echo -e "${YELLOW}警告: Dockerfileが見つかりません: ${dockerfile_path}${NC}"
            unset SERVICE_CONFIGS["$service"]
        else
            echo -e "${GREEN}✓ ${service}: ${dockerfile_path}${NC}"
        fi
    done

    if [ ${#SERVICE_CONFIGS[@]} -eq 0 ]; then
        echo -e "${RED}エラー: 有効なサービスが見つかりません${NC}"
        exit 1
    fi
}

# Dockerイメージのビルド
build_image() {
    local service="$1"
    local dockerfile_path="${SERVICE_CONFIGS[$service]}"
    local service_dir=$(dirname "$dockerfile_path")
    local dockerfile_name=$(basename "$dockerfile_path")

    echo -e "${BLUE}ビルド中: ${service}${NC}"
    echo -e "${CYAN}  ディレクトリ: ${service_dir}${NC}"
    echo -e "${CYAN}  Dockerfile: ${dockerfile_name}${NC}"

    # サービスディレクトリに移動
    cd "$service_dir"

    # Dockerイメージをビルド
    if docker build -f "$dockerfile_name" -t "${service}:production" .; then
        echo -e "${GREEN}✓ ${service} のビルドが完了しました${NC}"

        # containerdにインポート
        echo -e "${CYAN}  containerdにインポート中...${NC}"
        if docker save "${service}:production" | sudo ctr -n k8s.io images import -; then
            echo -e "${GREEN}✓ ${service} のインポートが完了しました${NC}"
        else
            echo -e "${RED}✗ ${service} のインポートに失敗しました${NC}"
            return 1
        fi
    else
        echo -e "${RED}✗ ${service} のビルドに失敗しました${NC}"
        return 1
    fi

    # 元のディレクトリに戻る
    cd - > /dev/null
}

# 全サービスのビルド
build_all_services() {
    echo -e "${MAGENTA}全サービスのビルドを開始します...${NC}"

    local failed_services=()

    for service in "${!SERVICE_CONFIGS[@]}"; do
        if ! build_image "$service"; then
            failed_services+=("$service")
        fi
        echo ""
    done

    if [ ${#failed_services[@]} -gt 0 ]; then
        echo -e "${RED}以下のサービスのビルドに失敗しました:${NC}"
        for service in "${failed_services[@]}"; do
            echo -e "${RED}  - ${service}${NC}"
        done
        return 1
    else
        echo -e "${GREEN}✓ すべてのサービスのビルドが完了しました${NC}"
    fi
}

# 指定サービスのビルド
build_specific_services() {
    echo -e "${MAGENTA}指定されたサービスのビルドを開始します...${NC}"

    IFS=',' read -ra SERVICES_ARRAY <<< "$SERVICES"
    local failed_services=()

    for service in "${SERVICES_ARRAY[@]}"; do
        service=$(echo "$service" | xargs)  # 空白を削除

        if [[ -n "${SERVICE_CONFIGS[$service]:-}" ]]; then
            if ! build_image "$service"; then
                failed_services+=("$service")
            fi
        else
            echo -e "${YELLOW}警告: 不明なサービス '${service}' をスキップします${NC}"
        fi
        echo ""
    done

    if [ ${#failed_services[@]} -gt 0 ]; then
        echo -e "${RED}以下のサービスのビルドに失敗しました:${NC}"
        for service in "${failed_services[@]}"; do
            echo -e "${RED}  - ${service}${NC}"
        done
        return 1
    else
        echo -e "${GREEN}✓ 指定されたサービスのビルドが完了しました${NC}"
    fi
}

# ビルド済みイメージの確認
list_built_images() {
    echo -e "${BLUE}ビルド済みイメージの確認:${NC}"

    for service in "${!SERVICE_CONFIGS[@]}"; do
        if docker images | grep -q "${service}:production"; then
            echo -e "${GREEN}✓ ${service}:production${NC}"
        else
            echo -e "${YELLOW}✗ ${service}:production (未ビルド)${NC}"
        fi
    done
}

# containerd内のイメージ確認
list_containerd_images() {
    echo -e "${BLUE}containerd内のイメージ確認:${NC}"

    for service in "${!SERVICE_CONFIGS[@]}"; do
        if sudo ctr -n k8s.io images ls | grep -q "${service}:production"; then
            echo -e "${GREEN}✓ ${service}:production (containerd)${NC}"
        else
            echo -e "${YELLOW}✗ ${service}:production (containerd内に未発見)${NC}"
        fi
    done
}

# メイン処理
main() {
    echo -e "${MAGENTA}========================================${NC}"
    echo -e "${MAGENTA}ALTプラットフォーム Dockerイメージビルド${NC}"
    echo -e "${MAGENTA}========================================${NC}"

    check_prerequisites
    validate_service_directories

    if [ "$BUILD_ALL" = "true" ]; then
        build_all_services
    else
        if [ -z "$SERVICES" ]; then
            echo -e "${RED}エラー: サービスが指定されていません${NC}"
            usage
        fi
        build_specific_services
    fi

    echo ""
    list_built_images
    echo ""
    list_containerd_images

    echo -e "${MAGENTA}========================================${NC}"
    echo -e "${GREEN}ビルド処理が完了しました！${NC}"
    echo -e "${CYAN}次のステップ: kubectl apply -f k8s-manifests/ でデプロイしてください${NC}"
}

# スクリプトの実行
main