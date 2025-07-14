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

# ツールの自動インストール
auto_install_tools() {
    local install_tools=false

    # yq の確認
    if ! command -v yq &> /dev/null; then
        echo -e "${YELLOW}yq がインストールされていません。マニフェスト情報の詳細表示ができません。${NC}"
        echo -e "${CYAN}yq をインストールしますか？ (y/N)${NC}"
        read -r response
        if [[ "$response" =~ ^([yY][eE][sS]|[yY])$ ]]; then
            install_tools=true
            echo -e "${BLUE}yq をインストール中...${NC}"

            # yq のインストール
            if sudo wget -q https://github.com/mikefarah/yq/releases/latest/download/yq_linux_amd64 -O /usr/bin/yq && \
               sudo chmod +x /usr/bin/yq; then
                echo -e "${GREEN}✓ yq のインストールが完了しました${NC}"
            else
                echo -e "${RED}✗ yq のインストールに失敗しました${NC}"
            fi
        fi
    fi

    # jq の確認（JSONパース用）
    if ! command -v jq &> /dev/null; then
        echo -e "${YELLOW}jq がインストールされていません。JSON処理ができません。${NC}"
        echo -e "${CYAN}jq をインストールしますか？ (y/N)${NC}"
        read -r response
        if [[ "$response" =~ ^([yY][eE][sS]|[yY])$ ]]; then
            install_tools=true
            echo -e "${BLUE}jq をインストール中...${NC}"

            # jq のインストール
            if sudo apt-get update -qq && sudo apt-get install -y jq >/dev/null 2>&1; then
                echo -e "${GREEN}✓ jq のインストールが完了しました${NC}"
            else
                echo -e "${RED}✗ jq のインストールに失敗しました${NC}"
            fi
        fi
    fi

    # helm の確認（将来的な拡張のため）
    if ! command -v helm &> /dev/null; then
        echo -e "${YELLOW}helm がインストールされていません。Helmチャートを使用する場合に必要です。${NC}"
        echo -e "${CYAN}helm をインストールしますか？ (y/N)${NC}"
        read -r response
        if [[ "$response" =~ ^([yY][eE][sS]|[yY])$ ]]; then
            install_tools=true
            echo -e "${BLUE}helm をインストール中...${NC}"

            # helm のインストール
            if curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 && \
               chmod 700 get_helm.sh && \
               ./get_helm.sh >/dev/null 2>&1 && \
               rm get_helm.sh; then
                echo -e "${GREEN}✓ helm のインストールが完了しました${NC}"
            else
                echo -e "${RED}✗ helm のインストールに失敗しました${NC}"
                rm -f get_helm.sh
            fi
        fi
    fi

    if [ "$install_tools" = "true" ]; then
        echo -e "${GREEN}✓ ツールのインストール処理が完了しました${NC}"
    fi
}

# ツールバージョンの確認
check_tool_versions() {
    echo -e "${BLUE}インストール済みツールのバージョンを確認中...${NC}"

    # kubectl バージョン
    if command -v kubectl &> /dev/null; then
        local kubectl_version=$(kubectl version --client --short 2>/dev/null | grep -oP 'Client Version: \K.*' || kubectl version --client -o json 2>/dev/null | jq -r '.clientVersion.gitVersion' 2>/dev/null || echo "取得失敗")
        echo -e "${CYAN}  kubectl: ${kubectl_version}${NC}"
    fi

    # kustomize バージョン
    if command -v kustomize &> /dev/null; then
        local kustomize_version=$(kustomize version --short 2>/dev/null | grep -oP 'kustomize/\K.*' || echo "取得失敗")
        echo -e "${CYAN}  kustomize: ${kustomize_version}${NC}"
    else
        echo -e "${CYAN}  kustomize: kubectl内蔵版を使用${NC}"
    fi

    # yq バージョン
    if command -v yq &> /dev/null; then
        local yq_version=$(yq --version 2>/dev/null | grep -oP 'version \K.*' || echo "取得失敗")
        echo -e "${CYAN}  yq: ${yq_version}${NC}"
    else
        echo -e "${CYAN}  yq: インストールされていません${NC}"
    fi

    # jq バージョン
    if command -v jq &> /dev/null; then
        local jq_version=$(jq --version 2>/dev/null || echo "取得失敗")
        echo -e "${CYAN}  jq: ${jq_version}${NC}"
    else
        echo -e "${CYAN}  jq: インストールされていません${NC}"
    fi

    # helm バージョン
    if command -v helm &> /dev/null; then
        local helm_version=$(helm version --short 2>/dev/null | grep -oP 'v\K.*' || echo "取得失敗")
        echo -e "${CYAN}  helm: v${helm_version}${NC}"
    else
        echo -e "${CYAN}  helm: インストールされていません${NC}"
    fi
}

# 前提条件の確認
check_prerequisites() {
    echo -e "${BLUE}前提条件を確認中...${NC}"

    # kubectl の確認
    if ! command -v kubectl &> /dev/null; then
        echo -e "${RED}エラー: kubectl がインストールされていません${NC}"
        echo -e "${CYAN}kubectlをインストールしてから再実行してください${NC}"
        echo -e "${CYAN}インストール方法: https://kubernetes.io/docs/tasks/tools/install-kubectl-linux/${NC}"
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
        echo -e "${CYAN}クラスター接続を確認してください:${NC}"
        echo -e "${CYAN}  kubectl config current-context${NC}"
        echo -e "${CYAN}  kubectl config get-contexts${NC}"
        exit 1
    fi

    # 現在のコンテキスト情報を表示
    local current_context=$(kubectl config current-context 2>/dev/null || echo "不明")
    local current_namespace=$(kubectl config view --minify --output 'jsonpath={..namespace}' 2>/dev/null || echo "default")
    echo -e "${CYAN}  接続先クラスター: ${current_context}${NC}"
    echo -e "${CYAN}  デフォルト名前空間: ${current_namespace}${NC}"

    # ツールのバージョン確認
    check_tool_versions

    # オプションツールの自動インストール
    if [ "$DRY_RUN" = "false" ]; then
        auto_install_tools
    fi

    echo -e "${GREEN}✓ すべての前提条件が満たされています${NC}"
}

# ストレージディレクトリの作成
create_storage_directories() {
    echo -e "${BLUE}ストレージディレクトリを作成中...${NC}"

    if [ "$DRY_RUN" = "true" ]; then
        echo -e "${YELLOW}ドライラン: ストレージディレクトリの作成をスキップ${NC}"
    else
        # ディレクトリが既に存在するかチェック
        if [ -d "/mnt/data/postgres" ] && [ -d "/mnt/data/clickhouse" ] && \
           [ -d "/mnt/data/meilisearch" ] && [ -d "/mnt/data/news-creator-models" ]; then
            echo -e "${GREEN}✓ ストレージディレクトリは既に存在します${NC}"
        else
            echo -e "${CYAN}ストレージディレクトリを作成します。sudoパスワードが必要です...${NC}"
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

    if [ "$ENVIRONMENT" = "development" ]; then
        NAMESPACE="alt-dev"
    elif [ "$ENVIRONMENT" = "production" ]; then
        NAMESPACE="alt-production"
    else
        NAMESPACE="alt-${ENVIRONMENT}"
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
    display_manifest_summary ${BUILD_FILE}

    # 名前空間設定の検証
    if [ "$DRY_RUN" = "false" ]; then
        validate_namespace_configuration ${BUILD_FILE}
    fi
}

# 名前空間設定の検証
validate_namespace_configuration() {
    local manifest_file=$1

    echo -e "${BLUE}名前空間設定を検証中...${NC}"

    if command -v yq &> /dev/null; then
        # 重要なアプリケーションリソースが適切な名前空間にあるかチェック
        local incorrect_ns=false

        # データベース関連リソースが正しい名前空間にあるかチェック
        local db_resources=$(yq eval 'select(. != null and .kind != null and .kind != "---" and (.metadata.name == "db" or .metadata.name == "migrate")) | .metadata.name + ":" + (.metadata.namespace // "default")' ${manifest_file} 2>/dev/null | sort -u)

        if [ ! -z "$db_resources" ]; then
            echo -e "${CYAN}  データベース関連リソース:${NC}"
            echo "$db_resources" | while read resource; do
                local resource_name=$(echo "$resource" | cut -d':' -f1)
                local resource_ns=$(echo "$resource" | cut -d':' -f2)

                echo -e "${CYAN}    ${resource_name}: ${resource_ns}${NC}"

                if [ "$resource_ns" = "default" ]; then
                    echo -e "${YELLOW}    ⚠ ${resource_name}がdefault名前空間に設定されています${NC}"
                    incorrect_ns=true
                fi
            done
        fi

        # アプリケーションリソースが正しい名前空間にあるかチェック
        local app_resources=$(yq eval 'select(. != null and .kind != null and .kind != "---" and (.metadata.name == "alt-backend" or .metadata.name == "alt-frontend" or .metadata.name == "pre-processor")) | .metadata.name + ":" + (.metadata.namespace // "default")' ${manifest_file} 2>/dev/null | sort -u)

        if [ ! -z "$app_resources" ]; then
            echo -e "${CYAN}  アプリケーションリソース:${NC}"
            echo "$app_resources" | while read resource; do
                local resource_name=$(echo "$resource" | cut -d':' -f1)
                local resource_ns=$(echo "$resource" | cut -d':' -f2)

                echo -e "${CYAN}    ${resource_name}: ${resource_ns}${NC}"

                if [ "$resource_ns" = "default" ]; then
                    echo -e "${YELLOW}    ⚠ ${resource_name}がdefault名前空間に設定されています${NC}"
                    incorrect_ns=true
                fi
            done
        fi

        if [ "$incorrect_ns" = "true" ]; then
            echo -e "${RED}⚠ 名前空間設定に問題があります${NC}"
            echo -e "${YELLOW}推奨されるアクション:${NC}"
            echo -e "${CYAN}  1. k8s/overlays/${ENVIRONMENT}/kustomization.yamlでnamespaceが正しく設定されているか確認${NC}"
            echo -e "${CYAN}  2. k8s/base/以下のマニフェストファイルでnamespaceが適切に設定されているか確認${NC}"
            echo -e "${CYAN}  3. kustomize buildコマンドでマニフェストを再ビルド${NC}"

            echo -e "${YELLOW}デプロイを続行しますか？ (y/N)${NC}"
            read -r response
            if [[ ! "$response" =~ ^([yY][eE][sS]|[yY])$ ]]; then
                echo -e "${YELLOW}デプロイを中止しました${NC}"
                return 1
            fi
        else
            echo -e "${GREEN}✓ 名前空間設定は正常です${NC}"
        fi
    else
        echo -e "${YELLOW}yqが利用できないため、名前空間検証をスキップします${NC}"
    fi

    return 0
}

# マニフェストサマリーの表示
display_manifest_summary() {
    local manifest_file=$1

    echo -e "${CYAN}ビルドされたリソース:${NC}"

    if command -v yq &> /dev/null; then
        # yqを使った詳細なリソース情報表示（改良版）

        # 有効なKubernetesリソースのみをカウント（document separatorを除外）
        local resource_count=$(yq eval 'select(. != null and .kind != null and .kind != "---") | .kind' ${manifest_file} 2>/dev/null | wc -l | tr -d '\n')
        echo -e "${BLUE}  総リソース数: ${resource_count}${NC}"

        # リソース種別ごとの集計（改良版）
        echo -e "${BLUE}  リソース種別別:${NC}"
        yq eval 'select(. != null and .kind != null and .kind != "---") | .kind' ${manifest_file} 2>/dev/null | sort | uniq -c | while read count kind; do
            if [ ! -z "$kind" ] && [ "$kind" != "---" ]; then
                echo -e "${CYAN}    ${kind}: ${count}個${NC}"
            fi
        done

        # 名前空間別の集計（改良版）
        echo -e "${BLUE}  名前空間別:${NC}"
        # 明示的に名前空間が指定されているリソースのみを対象とし、defaultは実際にdefaultが指定されている場合のみ
        local explicit_ns=$(yq eval 'select(. != null and .kind != null and .kind != "---" and .metadata.namespace != null) | .metadata.namespace' ${manifest_file} 2>/dev/null | grep -v '^---$' | grep -v '^$' | grep -v '^null$' | sort | uniq -c)
        local implicit_ns=$(yq eval 'select(. != null and .kind != null and .kind != "---" and .metadata.namespace == null) | "default"' ${manifest_file} 2>/dev/null | wc -l | tr -d '\n')

        # 明示的な名前空間の表示
        if [ ! -z "$explicit_ns" ]; then
            echo "$explicit_ns" | while read count ns; do
                if [ ! -z "$ns" ] && [ "$ns" != "null" ]; then
                    echo -e "${CYAN}    ${ns}: ${count}個${NC}"
                fi
            done
        fi

        # 暗黙的にdefaultとなるリソースがあれば表示
        if [ "$implicit_ns" -gt 0 ]; then
            echo -e "${CYAN}    default: ${implicit_ns}個${NC}"
        fi

        # 名前空間の設定が正しくない場合の警告
        local default_count=$(echo "$explicit_ns" | grep -c "default" 2>/dev/null | tr -d '\n' || echo "0")
        if [ "$default_count" -gt 0 ] || [ "$implicit_ns" -gt 30 ]; then
            echo -e "${YELLOW}    ⚠ 警告: 多くのリソースがdefault名前空間にデプロイされようとしています${NC}"
            echo -e "${YELLOW}    kustomizeの名前空間設定を確認してください${NC}"
        fi

        # 何も見つからない場合
        if [ -z "$explicit_ns" ] && [ "$implicit_ns" -eq 0 ]; then
            echo -e "${YELLOW}    名前空間情報を取得できませんでした${NC}"
        fi

        # 主要なリソースの詳細（改良版）
        echo -e "${BLUE}  主要リソース:${NC}"

        # Deploymentsの表示
        local deployments=$(yq eval 'select(.kind == "Deployment") | .kind + "/" + .metadata.name + " (" + (.metadata.namespace // "default") + ")"' ${manifest_file} 2>/dev/null | grep -v '^---$' | grep -v '^$' | grep -v '^null$')
        if [ ! -z "$deployments" ]; then
            echo -e "${MAGENTA}    Deployments:${NC}"
            echo "$deployments" | sort | while read line; do
                if [ ! -z "$line" ] && [ "$line" != "---" ] && [ "$line" != "null" ]; then
                    echo -e "${CYAN}      ${line}${NC}"
                fi
            done
        fi

        # Servicesの表示
        local services=$(yq eval 'select(.kind == "Service") | .kind + "/" + .metadata.name + " (" + (.metadata.namespace // "default") + ")"' ${manifest_file} 2>/dev/null | grep -v '^---$' | grep -v '^$' | grep -v '^null$')
        if [ ! -z "$services" ]; then
            echo -e "${MAGENTA}    Services:${NC}"
            echo "$services" | sort | while read line; do
                if [ ! -z "$line" ] && [ "$line" != "---" ] && [ "$line" != "null" ]; then
                    echo -e "${CYAN}      ${line}${NC}"
                fi
            done
        fi

        # PVCsの表示
        local pvcs=$(yq eval 'select(.kind == "PersistentVolumeClaim") | .kind + "/" + .metadata.name + " (" + (.metadata.namespace // "default") + ")"' ${manifest_file} 2>/dev/null | grep -v '^---$' | grep -v '^$' | grep -v '^null$')
        if [ ! -z "$pvcs" ]; then
            echo -e "${MAGENTA}    PersistentVolumeClaims:${NC}"
            echo "$pvcs" | sort | while read line; do
                if [ ! -z "$line" ] && [ "$line" != "---" ] && [ "$line" != "null" ]; then
                    echo -e "${CYAN}      ${line}${NC}"
                fi
            done
        fi

        # ConfigMapsとSecretsの表示
        local configs=$(yq eval 'select(.kind == "ConfigMap" or .kind == "Secret") | .kind + "/" + .metadata.name + " (" + (.metadata.namespace // "default") + ")"' ${manifest_file} 2>/dev/null | grep -v '^---$' | grep -v '^$' | grep -v '^null$')
        if [ ! -z "$configs" ]; then
            echo -e "${MAGENTA}    ConfigMaps & Secrets:${NC}"
            echo "$configs" | sort | while read line; do
                if [ ! -z "$line" ] && [ "$line" != "---" ] && [ "$line" != "null" ]; then
                    echo -e "${CYAN}      ${line}${NC}"
                fi
            done
        fi

    elif command -v jq &> /dev/null; then
        # jqを使った基本的なリソース情報表示（改良版）
        echo -e "${YELLOW}  yqが利用できないため、基本的な情報のみ表示${NC}"

        # リソース数をカウント
        local resource_count=$(grep -c "^kind:" ${manifest_file} 2>/dev/null || echo "0")
        echo -e "${BLUE}  推定リソース数: ${resource_count}${NC}"

        # 基本的なリソース種別の表示
        echo -e "${BLUE}  検出されたリソース種別:${NC}"
        grep "^kind:" ${manifest_file} 2>/dev/null | sort | uniq -c | while read count kind_line; do
            local kind=$(echo "$kind_line" | sed 's/kind: //')
            echo -e "${CYAN}    ${kind}: ${count}個${NC}"
        done

        # リソース名の表示（基本版）
        echo -e "${BLUE}  主要リソース名:${NC}"
        grep -A1 "^kind: \(Deployment\|Service\|PersistentVolumeClaim\)" ${manifest_file} 2>/dev/null | \
        grep -E "^kind:|^  name:" | paste - - | \
        sed 's/kind: //' | sed 's/  name: / /' | \
        while read kind name; do
            echo -e "${CYAN}    ${kind}/${name}${NC}"
        done

    else
        echo -e "${YELLOW}  リソース一覧の詳細表示には yq または jq が必要です${NC}"
        echo -e "${YELLOW}  基本的な情報:${NC}"

        # ドキュメント区切りを除いたリソース数
        local doc_separators=$(grep -c "^---" ${manifest_file} 2>/dev/null || echo "0")
        local resource_kinds=$(grep -c "^kind:" ${manifest_file} 2>/dev/null || echo "0")

        echo -e "${CYAN}    ドキュメント区切り: ${doc_separators}個${NC}"
        echo -e "${CYAN}    リソース定義: ${resource_kinds}個${NC}"

        # 基本的なリソース種別の検出
        if [ -f "$manifest_file" ]; then
            echo -e "${BLUE}  検出されたリソース:${NC}"
            grep "^kind:" ${manifest_file} 2>/dev/null | sort | uniq -c | while read count kind_line; do
                local kind=$(echo "$kind_line" | sed 's/kind: //')
                echo -e "${CYAN}    ${kind}: ${count}個${NC}"
            done
        fi
    fi

    # ファイルサイズ情報
    if [ -f "$manifest_file" ]; then
        local file_size=$(wc -c < "$manifest_file" 2>/dev/null || echo "0")
        local file_lines=$(wc -l < "$manifest_file" 2>/dev/null || echo "0")
        echo -e "${BLUE}  マニフェストファイル情報:${NC}"
        echo -e "${CYAN}    サイズ: ${file_size} バイト, 行数: ${file_lines}${NC}"
    fi
}

# デプロイメント進捗の表示
show_deployment_progress() {
    local namespace=$1
    local operation=${2:-"デプロイメント"}
    local interval=10
    local max_cycles=30
    local cycle=0

    echo -e "${BLUE}${operation}の進捗を監視中...${NC}"

    # 本番環境の場合は複数の名前空間をチェック
    local check_namespaces=""
    if [ "$ENVIRONMENT" = "production" ]; then
        check_namespaces="alt-apps alt-database alt-search alt-ingress"
    else
        check_namespaces="$namespace"
    fi

    while [ $cycle -lt $max_cycles ]; do
        echo -e "${CYAN}━━━ 進捗チェック ($(date '+%H:%M:%S')) ━━━${NC}"

        # Deployment の進捗 (複数名前空間対応)
        local total_deployments=0
        local total_ready_deployments=0

        for ns in $check_namespaces; do
            local ns_deployments=$(kubectl get deployment -n ${ns} --no-headers 2>/dev/null | wc -l | tr -d '\n')
            local ns_ready_deployments=$(kubectl get deployment -n ${ns} --no-headers 2>/dev/null | awk 'split($2, arr, "/") && arr[1] == arr[2] && arr[2] > 0 { print $1 }' | wc -l | tr -d '\n')
            total_deployments=$((total_deployments + ns_deployments))
            total_ready_deployments=$((total_ready_deployments + ns_ready_deployments))
        done

        if [ $total_deployments -gt 0 ]; then
            echo -e "${CYAN}  Deployments: ${total_ready_deployments}/${total_deployments} 準備完了${NC}"

            # 準備中のDeploymentの詳細 (複数名前空間対応)
            for ns in $check_namespaces; do
                kubectl get deployment -n ${ns} --no-headers 2>/dev/null | awk 'split($2, arr, "/") && (arr[1] != arr[2] || arr[2] == 0) { print $1 }' | while read deploy; do
                    if [ ! -z "$deploy" ]; then
                        local replicas=$(kubectl get deployment ${deploy} -n ${ns} -o jsonpath='{.status.replicas}' 2>/dev/null || echo "0")
                        local ready_replicas=$(kubectl get deployment ${deploy} -n ${ns} -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "0")
                        local available_replicas=$(kubectl get deployment ${deploy} -n ${ns} -o jsonpath='{.status.availableReplicas}' 2>/dev/null || echo "0")
                        echo -e "${YELLOW}    ${deploy}: ${ready_replicas}/${replicas} 準備中 (利用可能: ${available_replicas})${NC}"
                    fi
                done
            done
        fi

        # Pod の状態 (複数名前空間対応)
        local total_pods=0
        local running_pods=0
        local pending_pods=0
        local failed_pods=0

        for ns in $check_namespaces; do
            local ns_total_pods=$(kubectl get pod -n ${ns} --no-headers 2>/dev/null | wc -l | tr -d '\n')
            local ns_running_pods=$(kubectl get pod -n ${ns} --no-headers --field-selector=status.phase=Running 2>/dev/null | wc -l | tr -d '\n')
            local ns_pending_pods=$(kubectl get pod -n ${ns} --no-headers --field-selector=status.phase=Pending 2>/dev/null | wc -l | tr -d '\n')
            local ns_failed_pods=$(kubectl get pod -n ${ns} --no-headers --field-selector=status.phase=Failed 2>/dev/null | wc -l | tr -d '\n')
            total_pods=$((total_pods + ns_total_pods))
            running_pods=$((running_pods + ns_running_pods))
            pending_pods=$((pending_pods + ns_pending_pods))
            failed_pods=$((failed_pods + ns_failed_pods))
        done

        if [ $total_pods -gt 0 ]; then
            echo -e "${CYAN}  Pods: ${running_pods}/${total_pods} 実行中, ${pending_pods} 待機中, ${failed_pods} 失敗${NC}"

            # 問題のあるPodの詳細 (複数名前空間対応)
            for ns in $check_namespaces; do
                kubectl get pod -n ${ns} --no-headers 2>/dev/null | awk '$3 != "Running" && $3 != "Completed" { print $1, $3 }' | while read pod_name pod_status; do
                    if [ ! -z "$pod_name" ]; then
                        local restarts=$(kubectl get pod ${pod_name} -n ${ns} -o jsonpath='{.status.containerStatuses[0].restartCount}' 2>/dev/null || echo "0")
                        echo -e "${YELLOW}    ${pod_name} (${ns}): ${pod_status} (再起動: ${restarts}回)${NC}"
                    fi
                done
            done
        fi

        # すべてのDeploymentが準備完了かチェック
        if [ $total_deployments -eq $total_ready_deployments ] && [ $total_deployments -gt 0 ] && [ $failed_pods -eq 0 ]; then
            echo -e "${GREEN}✓ すべてのリソースが正常に起動しました${NC}"
            return 0
        fi

        cycle=$((cycle + 1))
        if [ $cycle -lt $max_cycles ]; then
            echo -e "${BLUE}  ${interval}秒後に再チェックします... (${cycle}/${max_cycles})${NC}"
            sleep $interval
        fi
    done

    echo -e "${YELLOW}⚠ 進捗監視がタイムアウトしました。手動で状態を確認してください。${NC}"
    return 1
}

# デプロイメントサマリーレポート
generate_deployment_summary() {
    local start_time=$1
    local end_time=$(date +%s)
    local duration=$((end_time - start_time))
    local minutes=$((duration / 60))
    local seconds=$((duration % 60))

    echo -e "${MAGENTA}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${MAGENTA}                    デプロイメントサマリーレポート                    ${NC}"
    echo -e "${MAGENTA}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

    echo -e "${BLUE}実行情報:${NC}"
    echo -e "${CYAN}  環境: ${ENVIRONMENT}${NC}"
    echo -e "${CYAN}  実行時間: ${minutes}分${seconds}秒${NC}"
    echo -e "${CYAN}  完了時刻: $(date '+%Y-%m-%d %H:%M:%S')${NC}"

    if [ "$ENVIRONMENT" = "production" ]; then
        echo -e "${BLUE}本番環境の状態:${NC}"

        # 各名前空間のサマリー
        for ns in alt-apps alt-database alt-search alt-observability alt-ingress; do
            local deployments=$(kubectl get deployment -n ${ns} --no-headers 2>/dev/null | wc -l)
            local ready_deployments=$(kubectl get deployment -n ${ns} --no-headers 2>/dev/null | awk 'split($2, arr, "/") && arr[1] == arr[2] && arr[2] > 0 { print $1 }' | wc -l)
            local total_pods=$(kubectl get pod -n ${ns} --no-headers 2>/dev/null | wc -l)
            local running_pods=$(kubectl get pod -n ${ns} --no-headers --field-selector=status.phase=Running 2>/dev/null | wc -l)

            if [ $deployments -gt 0 ] || [ $total_pods -gt 0 ]; then
                echo -e "${CYAN}  ${ns}: Deployments ${ready_deployments}/${deployments}, Pods ${running_pods}/${total_pods}${NC}"
            fi
        done
    elif [ "$ENVIRONMENT" = "development" ]; then
        local ns="alt-dev"
        local deployments=$(kubectl get deployment -n ${ns} --no-headers 2>/dev/null | wc -l)
        local ready_deployments=$(kubectl get deployment -n ${ns} --no-headers 2>/dev/null | awk 'split($2, arr, "/") && arr[1] == arr[2] && arr[2] > 0 { print $1 }' | wc -l)
        local total_pods=$(kubectl get pod -n ${ns} --no-headers 2>/dev/null | wc -l)
        local running_pods=$(kubectl get pod -n ${ns} --no-headers --field-selector=status.phase=Running 2>/dev/null | wc -l)

        echo -e "${BLUE}環境の状態:${NC}"
        echo -e "${CYAN}  ${ns}: Deployments ${ready_deployments}/${deployments}, Pods ${running_pods}/${total_pods}${NC}"
    else
        local ns="alt-${ENVIRONMENT}"
        local deployments=$(kubectl get deployment -n ${ns} --no-headers 2>/dev/null | wc -l)
        local ready_deployments=$(kubectl get deployment -n ${ns} --no-headers 2>/dev/null | awk 'split($2, arr, "/") && arr[1] == arr[2] && arr[2] > 0 { print $1 }' | wc -l)
        local total_pods=$(kubectl get pod -n ${ns} --no-headers 2>/dev/null | wc -l)
        local running_pods=$(kubectl get pod -n ${ns} --no-headers --field-selector=status.phase=Running 2>/dev/null | wc -l)

        echo -e "${BLUE}環境の状態:${NC}"
        echo -e "${CYAN}  ${ns}: Deployments ${ready_deployments}/${deployments}, Pods ${running_pods}/${total_pods}${NC}"
    fi

    echo -e "${BLUE}次のステップ:${NC}"
    if [ "$ENVIRONMENT" = "production" ]; then
        echo -e "${CYAN}  1. ./deploy.sh production status でシステム全体の状態を確認${NC}"
        echo -e "${CYAN}  2. ヘルスチェックが失敗した場合はログを確認${NC}"
        echo -e "${CYAN}  3. 問題がある場合は ./deploy.sh production rollback でロールバック${NC}"
    elif [ "$ENVIRONMENT" = "development" ]; then
        echo -e "${CYAN}  1. ./deploy.sh development status で状態を確認${NC}"
        echo -e "${CYAN}  2. アプリケーションの動作テストを実行${NC}"
    else
        echo -e "${CYAN}  1. ./deploy.sh ${ENVIRONMENT} status で状態を確認${NC}"
        echo -e "${CYAN}  2. アプリケーションの動作テストを実行${NC}"
    fi

    echo -e "${MAGENTA}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
}

# マイグレーションJobの名前空間を動的に検出する関数
detect_migration_namespace() {
    local preferred_namespace="alt-database"
    local fallback_namespace="default"

    # 最初に期待される名前空間をチェック
    if kubectl get job migrate -n ${preferred_namespace} >/dev/null 2>&1; then
        echo "${preferred_namespace}"
        return 0
    fi

    # フォールバック: デフォルト名前空間をチェック
    if kubectl get job migrate -n ${fallback_namespace} >/dev/null 2>&1; then
        echo -e "${YELLOW}⚠ 警告: マイグレーションJobがデフォルト名前空間に見つかりました${NC}" >&2
        echo -e "${YELLOW}  期待される名前空間: ${preferred_namespace}${NC}" >&2
        echo -e "${YELLOW}  実際の名前空間: ${fallback_namespace}${NC}" >&2
        echo "${fallback_namespace}"
        return 0
    fi

    # 旧いDeployment形式もチェック
    if kubectl get deployment migrate -n ${preferred_namespace} >/dev/null 2>&1; then
        echo -e "${YELLOW}⚠ 警告: 旧いDeployment形式のマイグレーションを検出しました${NC}" >&2
        echo "${preferred_namespace}"
        return 0
    fi

    if kubectl get deployment migrate -n ${fallback_namespace} >/dev/null 2>&1; then
        echo -e "${YELLOW}⚠ 警告: 旧いDeployment形式のマイグレーションがデフォルト名前空間に見つかりました${NC}" >&2
        echo -e "${YELLOW}  期待される名前空間: ${preferred_namespace}${NC}" >&2
        echo -e "${YELLOW}  実際の名前空間: ${fallback_namespace}${NC}" >&2
        echo "${fallback_namespace}"
        return 0
    fi

    # 見つからない場合は優先名前空間を返す
    echo -e "${RED}✗ マイグレーションJob/Deploymentが見つかりません${NC}" >&2
    echo -e "${YELLOW}  チェックした名前空間: ${preferred_namespace}, ${fallback_namespace}${NC}" >&2
    echo "${preferred_namespace}"
    return 1
}

# マイグレーション管理関数
handle_migration_deployment() {
    local namespace=$(detect_migration_namespace)

    echo -e "${BLUE}マイグレーション状態を確認中...${NC}"
    echo -e "${CYAN}  使用する名前空間: ${namespace}${NC}"

    # 失敗したJobをクリーンアップ
    if kubectl get job migrate -n ${namespace} >/dev/null 2>&1; then
        local job_failed=$(kubectl get job migrate -n ${namespace} -o jsonpath='{.status.conditions[?(@.type=="Failed")].status}' 2>/dev/null)
        local failed_count=$(kubectl get job migrate -n ${namespace} -o jsonpath='{.status.failed}' 2>/dev/null || echo "0")
        # 空文字列の場合は0に設定
        failed_count=${failed_count:-0}

        if [ "$job_failed" = "True" ] || [ "$failed_count" -gt "0" ]; then
            echo -e "${YELLOW}失敗したマイグレーションJobを検出しました。クリーンアップを実行します...${NC}"

            # 失敗したJobを削除
            kubectl delete job migrate -n ${namespace} --ignore-not-found=true
            kubectl delete pod -l job-name=migrate -n ${namespace} --ignore-not-found=true

            echo -e "${GREEN}✓ 失敗したマイグレーションJobのクリーンアップが完了しました${NC}"
            sleep 5
        fi
    fi

    # フォールバック: 旧いDeployment形式のクリーンアップ
    if kubectl get deployment migrate -n ${namespace} >/dev/null 2>&1; then
        echo -e "${YELLOW}旧いDeployment形式のマイグレーションを検出しました。クリーンアップします...${NC}"

        kubectl delete deployment migrate -n ${namespace} --ignore-not-found=true
        kubectl delete pod -l io.kompose.service=migrate -n ${namespace} --ignore-not-found=true

        echo -e "${GREEN}✓ 旧いマイグレーションDeploymentのクリーンアップが完了しました${NC}"
        sleep 5
    fi
}

# マイグレーション状態の詳細診断
diagnose_migration_state() {
    local namespace=$(detect_migration_namespace)

    echo -e "${BLUE}マイグレーション状態を診断中...${NC}"
    echo -e "${CYAN}  使用する名前空間: ${namespace}${NC}"

    # マイグレーションJobの状態確認を優先（短時間リトライ）
    local job_found=false
    for i in {1..3}; do
        if kubectl get job migrate -n ${namespace} >/dev/null 2>&1; then
            job_found=true
            break
        fi
        sleep 2
    done

    if [ "$job_found" = "true" ]; then
        local job_status=$(kubectl get job migrate -n ${namespace} -o jsonpath='{.status.conditions[?(@.type=="Complete")].status}' 2>/dev/null)
        local job_failed=$(kubectl get job migrate -n ${namespace} -o jsonpath='{.status.conditions[?(@.type=="Failed")].status}' 2>/dev/null)
        local succeeded=$(kubectl get job migrate -n ${namespace} -o jsonpath='{.status.succeeded}' 2>/dev/null || echo "0")
        local failed=$(kubectl get job migrate -n ${namespace} -o jsonpath='{.status.failed}' 2>/dev/null || echo "0")

        echo -e "${CYAN}  Job状態: Succeeded=${succeeded}, Failed=${failed}, Complete=${job_status}${NC}"

        if [ "$job_status" = "True" ] && [ "$succeeded" = "1" ]; then
            echo -e "${GREEN}  → マイグレーションJobが正常に完了しました${NC}"
            return 0
        elif [ "$job_failed" = "True" ] || [ "$failed" -gt "0" ]; then
            echo -e "${RED}  → マイグレーションJobが失敗しました${NC}"
            return 2
        fi

        # JobのPodを確認
        local migrate_pod=$(kubectl get pod -n ${namespace} -l job-name=migrate -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
        if [ ! -z "$migrate_pod" ]; then
            local pod_phase=$(kubectl get pod ${migrate_pod} -n ${namespace} -o jsonpath='{.status.phase}' 2>/dev/null)
            echo -e "${CYAN}  Job Pod状態: Phase=${pod_phase}${NC}"

            if [ "$pod_phase" = "Succeeded" ]; then
                echo -e "${GREEN}  → マイグレーションPodが正常に完了しました${NC}"
                return 0
            elif [ "$pod_phase" = "Failed" ]; then
                echo -e "${RED}  → マイグレーションPodが失敗しました${NC}"
                return 2
            fi
        fi
    # フォールバック: 旧いDeployment形式のチェック
    elif kubectl get deployment migrate -n ${namespace} >/dev/null 2>&1; then
        echo -e "${YELLOW}  → 旧いDeployment形式のマイグレーションを検出しました${NC}"
        local deployment_status=$(kubectl get deployment migrate -n ${namespace} -o jsonpath='{.status.conditions[?(@.type=="Progressing")].status}' 2>/dev/null)
        local ready_replicas=$(kubectl get deployment migrate -n ${namespace} -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "0")
        local desired_replicas=$(kubectl get deployment migrate -n ${namespace} -o jsonpath='{.spec.replicas}' 2>/dev/null || echo "1")

        echo -e "${CYAN}  Deployment状態: Ready=${ready_replicas}/${desired_replicas}, Progressing=${deployment_status}${NC}"

        # マイグレーションPodの詳細確認
        local migrate_pod=$(kubectl get pod -n ${namespace} -l io.kompose.service=migrate -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
        if [ ! -z "$migrate_pod" ]; then
            local pod_phase=$(kubectl get pod ${migrate_pod} -n ${namespace} -o jsonpath='{.status.phase}' 2>/dev/null)
            local ready_condition=$(kubectl get pod ${migrate_pod} -n ${namespace} -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null)
            local restart_count=$(kubectl get pod ${migrate_pod} -n ${namespace} -o jsonpath='{.status.containerStatuses[0].restartCount}' 2>/dev/null || echo "0")

            echo -e "${CYAN}  Pod状態: Phase=${pod_phase}, Ready=${ready_condition}, Restarts=${restart_count}${NC}"

            # マイグレーションの実行ログを確認
            echo -e "${CYAN}  マイグレーション実行ログ:${NC}"
            local recent_logs=$(kubectl logs ${migrate_pod} -n ${namespace} --tail=10 2>/dev/null)
            if [ ! -z "$recent_logs" ]; then
                echo "$recent_logs" | while read line; do
                    echo -e "${YELLOW}    ${line}${NC}"
                done

                # ログから完了状態を判定
                if echo "$recent_logs" | grep -q "migration.*completed\|migration.*success\|successfully\|finished"; then
                    echo -e "${GREEN}  → マイグレーションは既に完了している可能性があります${NC}"
                    return 0
                elif echo "$recent_logs" | grep -q "error\|failed\|exception"; then
                    echo -e "${RED}  → マイグレーションでエラーが発生しています${NC}"
                    return 2
                fi
            else
                echo -e "${YELLOW}    ログを取得できませんでした${NC}"
            fi

            # データベースの状態確認（実際のマイグレーション完了チェック）
            echo -e "${CYAN}  データベース状態確認:${NC}"
            local db_pod=$(kubectl get pod -n ${namespace} -l io.kompose.service=db -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
            if [ ! -z "$db_pod" ]; then
                # マイグレーションテーブルの存在確認
                local migration_check=$(kubectl exec -n ${namespace} ${db_pod} -- psql -U postgres -t -c "SELECT count(*) FROM information_schema.tables WHERE table_name='schema_migrations';" 2>/dev/null | tr -d ' ')
                if [ "$migration_check" = "1" ]; then
                    echo -e "${GREEN}    → データベースにマイグレーションテーブルが存在します${NC}"
                    return 0
                else
                    echo -e "${YELLOW}    → マイグレーションテーブルが見つかりません${NC}"
                fi
            fi
        else
            echo -e "${YELLOW}  → マイグレーションPodが見つかりません${NC}"
        fi
    else
        echo -e "${YELLOW}  → マイグレーションJobまたはDeploymentが見つかりません${NC}"
        echo -e "${YELLOW}  → 詳細な診断情報:${NC}"
        echo -e "${CYAN}    利用可能な名前空間:${NC}"
        kubectl get namespace | grep alt | while read ns_line; do
            echo -e "${CYAN}      ${ns_line}${NC}"
        done

        echo -e "${CYAN}    ${namespace}名前空間のJobリスト:${NC}"
        local job_list=$(kubectl get job -n ${namespace} 2>/dev/null)
        if [ ! -z "$job_list" ]; then
            echo "$job_list" | while read job_line; do
                echo -e "${CYAN}      ${job_line}${NC}"
            done
        else
            echo -e "${YELLOW}      Jobが見つかりません${NC}"
        fi

        echo -e "${CYAN}    ${namespace}名前空間のDeploymentリスト:${NC}"
        local deploy_list=$(kubectl get deployment -n ${namespace} 2>/dev/null)
        if [ ! -z "$deploy_list" ]; then
            echo "$deploy_list" | while read deploy_line; do
                echo -e "${CYAN}      ${deploy_line}${NC}"
            done
        else
            echo -e "${YELLOW}      Deploymentが見つかりません${NC}"
        fi
    fi

    return 1
}

# マイグレーション完了の待機
wait_for_migration_completion() {
    local namespace=$(detect_migration_namespace)
    local max_wait=120  # 2分に短縮
    local interval=10   # チェック間隔を短縮
    local elapsed=0

    echo -e "${BLUE}マイグレーション完了を待機中...${NC}"
    echo -e "${CYAN}  使用する名前空間: ${namespace}${NC}"

    # Jobの登録を待つ
    echo -e "${CYAN}Jobの登録を待機中...${NC}"
    sleep 5

    # 最初に診断を実行
    local diagnosis_result
    diagnosis_result=$(diagnose_migration_state)
    local diagnosis_code=$?

    echo "$diagnosis_result"

    if [ $diagnosis_code -eq 0 ]; then
        echo -e "${GREEN}✓ マイグレーションは既に完了しています（診断結果）${NC}"
        return 0
    elif [ $diagnosis_code -eq 2 ]; then
        echo -e "${RED}✗ マイグレーションでエラーが検出されました${NC}"
        return 1
    fi

    while [ $elapsed -lt $max_wait ]; do
        # Jobを優先してチェック（短時間リトライ）
        local job_found=false
        for i in {1..2}; do
            if kubectl get job migrate -n ${namespace} >/dev/null 2>&1; then
                job_found=true
                break
            fi
            sleep 1
        done

        if [ "$job_found" = "true" ]; then
            local job_status=$(kubectl get job migrate -n ${namespace} -o jsonpath='{.status.conditions[?(@.type=="Complete")].status}' 2>/dev/null)
            local succeeded=$(kubectl get job migrate -n ${namespace} -o jsonpath='{.status.succeeded}' 2>/dev/null || echo "0")
            local failed=$(kubectl get job migrate -n ${namespace} -o jsonpath='{.status.failed}' 2>/dev/null || echo "0")

            if [ "$job_status" = "True" ] && [ "$succeeded" = "1" ]; then
                echo -e "${GREEN}✓ マイグレーションJobが正常に完了しました${NC}"
                return 0
            elif [ "$failed" -gt "0" ]; then
                echo -e "${RED}✗ マイグレーションJobが失敗しました${NC}"
                return 1
            fi
        # フォールバック: 旧いDeployment形式
        elif kubectl get deployment migrate -n ${namespace} >/dev/null 2>&1; then
            local ready_replicas=$(kubectl get deployment migrate -n ${namespace} -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "0")
            local available_replicas=$(kubectl get deployment migrate -n ${namespace} -o jsonpath='{.status.availableReplicas}' 2>/dev/null || echo "0")

            if [ "$ready_replicas" = "1" ] && [ "$available_replicas" = "1" ]; then
                echo -e "${GREEN}✓ マイグレーションDeploymentが正常に完了しました${NC}"
                return 0
            fi
        fi

        # 診断を再実行して完了状態をチェック
        local current_diagnosis_result
        current_diagnosis_result=$(diagnose_migration_state)
        local current_diagnosis_code=$?

        if [ $current_diagnosis_code -eq 0 ]; then
            echo -e "${GREEN}✓ マイグレーションが正常に完了しました（診断確認）${NC}"
            return 0
        elif [ $current_diagnosis_code -eq 2 ]; then
            echo -e "${RED}✗ マイグレーションでエラーが検出されました${NC}"
            return 1
        fi

        sleep $interval
        elapsed=$((elapsed + interval))

        if [ $((elapsed % 30)) -eq 0 ]; then
            echo -e "${YELLOW}  マイグレーション待機中... (${elapsed}/${max_wait}秒経過)${NC}"
        fi
    done

    echo -e "${RED}✗ マイグレーションの完了待機がタイムアウトしました${NC}"

    # タイムアウト時の詳細情報
    echo -e "${YELLOW}タイムアウト時の状態:${NC}"

    # Jobを優先してチェック
    if kubectl get job migrate -n ${namespace} >/dev/null 2>&1; then
        echo -e "${CYAN}Job状態:${NC}"
        kubectl get job migrate -n ${namespace} 2>/dev/null || echo "    取得失敗"

        local migrate_pod=$(kubectl get pod -n ${namespace} -l job-name=migrate -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
        if [ ! -z "$migrate_pod" ]; then
            echo -e "${CYAN}Job Pod状態:${NC}"
            kubectl get pod ${migrate_pod} -n ${namespace} 2>/dev/null || echo "    取得失敗"

            echo -e "${CYAN}最終ログ:${NC}"
            kubectl logs ${migrate_pod} -n ${namespace} --tail=30 2>/dev/null || echo "    ログ取得失敗"
        fi
    elif kubectl get deployment migrate -n ${namespace} >/dev/null 2>&1; then
        echo -e "${CYAN}Deployment状態:${NC}"
        kubectl get deployment migrate -n ${namespace} 2>/dev/null || echo "    取得失敗"

        local migrate_pod=$(kubectl get pod -n ${namespace} -l io.kompose.service=migrate -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
        if [ ! -z "$migrate_pod" ]; then
            echo -e "${CYAN}Pod状態:${NC}"
            kubectl get pod ${migrate_pod} -n ${namespace} 2>/dev/null || echo "    取得失敗"

            echo -e "${CYAN}最終ログ:${NC}"
            kubectl logs ${migrate_pod} -n ${namespace} --tail=30 2>/dev/null || echo "    ログ取得失敗"
        fi
    fi

    echo -e "${YELLOW}マイグレーションをスキップして継続しますか？ (y/N)${NC}"
    read -r response
    if [[ "$response" =~ ^([yY][eE][sS]|[yY])$ ]]; then
        echo -e "${YELLOW}⚠ マイグレーションをスキップして継続します${NC}"
        return 0
    fi

    return 1
}

# データベースPod検索のヘルパー関数
find_database_pod() {
    local namespace=$1
    local db_pod=""

    # 複数のラベルセレクターを試行
    local selectors=(
        "io.kompose.service=db"
        "app=db"
        "component=database"
        "role=database"
    )

    echo -e "${CYAN}  複数のラベルセレクターでPodを検索中...${NC}" >&2

    for selector in "${selectors[@]}"; do
        echo -e "${CYAN}    試行: ${selector}${NC}" >&2
        db_pod=$(kubectl get pod -n ${namespace} -l "${selector}" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
        if [ ! -z "$db_pod" ]; then
            echo -e "${GREEN}    ✓ Pod発見: ${db_pod} (ラベル: ${selector})${NC}" >&2
            echo "$db_pod"
            return 0
        fi
    done

    # ラベルセレクターで見つからない場合は名前パターンで検索
    echo -e "${CYAN}    名前パターンでPodを検索中...${NC}" >&2
    local name_patterns=("db-" "database-" "postgres-" "postgresql-")

    for pattern in "${name_patterns[@]}"; do
        echo -e "${CYAN}    試行パターン: ${pattern}*${NC}" >&2
        db_pod=$(kubectl get pod -n ${namespace} --no-headers 2>/dev/null | awk '$1 ~ /^'${pattern}'/ { print $1; exit }')
        if [ ! -z "$db_pod" ]; then
            echo -e "${GREEN}    ✓ Pod発見: ${db_pod} (名前パターン: ${pattern}*)${NC}" >&2
            echo "$db_pod"
            return 0
        fi
    done

    # 最後の手段：すべてのPodを表示してユーザーに情報提供
    echo -e "${YELLOW}    自動検出に失敗。名前空間内のPod一覧:${NC}" >&2
    kubectl get pod -n ${namespace} --no-headers 2>/dev/null | while read pod_name status ready restarts age; do
        echo -e "${CYAN}      ${pod_name} (Status: ${status})${NC}" >&2
    done

    return 1
}

# データベース接続確認
verify_database_connection() {
    local namespace="alt-database"
    local max_retries=5
    local retry_delay=10

    echo -e "${BLUE}データベース接続を確認中...${NC}"

    # 名前空間の存在確認
    if ! kubectl get namespace ${namespace} >/dev/null 2>&1; then
        echo -e "${RED}✗ 名前空間 '${namespace}' が見つかりません${NC}"
        echo -e "${YELLOW}  利用可能な名前空間:${NC}"
        kubectl get namespace | grep alt
        return 1
    fi

    # データベースPodの検索
    echo -e "${CYAN}  データベースPodを検索中...${NC}"
    local db_pod=$(find_database_pod ${namespace} 2>/dev/null)

    # Pod名の正規化（空白や改行、その他の文字を除去）
    db_pod=$(echo "$db_pod" | tr -d '\n\r\t' | xargs | head -1)

    # Pod名の検証とデバッグ情報
    echo -e "${CYAN}  検索結果: '${db_pod}'${NC}"

    if [ -z "$db_pod" ] || [ "$db_pod" = " " ] || [ "$db_pod" = "null" ]; then
        echo -e "${RED}✗ データベースPodが見つかりません${NC}"
        echo -e "${YELLOW}問題の診断:${NC}"
        echo -e "${CYAN}  1. 名前空間の確認:${NC}"
        kubectl get pod -n ${namespace} 2>/dev/null || echo "    名前空間にアクセスできません"

        echo -e "${CYAN}  2. 他の名前空間でのDB検索:${NC}"
        for ns in alt-apps alt-production default; do
            local alt_pod=$(find_database_pod ${ns} 2>/dev/null | tr -d '\n\r\t' | xargs | head -1)
            if [ ! -z "$alt_pod" ] && [ "$alt_pod" != " " ] && [ "$alt_pod" != "null" ]; then
                echo -e "${YELLOW}    ${ns}名前空間でDB Pod発見: ${alt_pod}${NC}"
            fi
        done

        echo -e "${CYAN}  3. 手動確認コマンド:${NC}"
        echo -e "${CYAN}     kubectl get pod --all-namespaces | grep -i db${NC}"
        echo -e "${CYAN}     kubectl get pod --all-namespaces | grep -i postgres${NC}"

        # 継続するかユーザーに確認
        echo -e "${YELLOW}データベースPodが見つかりませんが、デプロイを続行しますか？ (y/N)${NC}"
        read -r response
        if [[ "$response" =~ ^([yY][eE][sS]|[yY])$ ]]; then
            echo -e "${YELLOW}⚠ データベースチェックをスキップしてデプロイを続行します${NC}"
            return 0
        else
            return 1
        fi
    fi

    # Pod存在確認
    if ! kubectl get pod "${db_pod}" -n ${namespace} >/dev/null 2>&1; then
        echo -e "${RED}✗ Pod '${db_pod}' が名前空間 '${namespace}' に存在しません${NC}"
        echo -e "${CYAN}  実際の名前空間内容:${NC}"
        kubectl get pod -n ${namespace} 2>/dev/null
        return 1
    fi

    # Podの状態確認（正しい構文で）
    local pod_status=$(kubectl get pod "${db_pod}" -n ${namespace} -o jsonpath='{.status.phase}' 2>/dev/null)
    local pod_ready=$(kubectl get pod "${db_pod}" -n ${namespace} -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null)

    echo -e "${CYAN}  Pod状態: ${pod_status}, Ready: ${pod_ready}${NC}"

    if [ "$pod_status" != "Running" ] || [ "$pod_ready" != "True" ]; then
        echo -e "${YELLOW}⚠ データベースPodがまだ準備完了していません${NC}"
        echo -e "${CYAN}  Pod詳細:${NC}"
        kubectl describe pod "${db_pod}" -n ${namespace} | grep -A 5 "Conditions:"

        echo -e "${YELLOW}Podの準備完了を待機しますか？ (y/N)${NC}"
        read -r response
        if [[ "$response" =~ ^([yY][eE][sS]|[yY])$ ]]; then
            echo -e "${CYAN}  Pod準備完了を待機中...${NC}"
            # 正しいkubectl wait構文
            kubectl wait --for=condition=Ready pod "${db_pod}" -n ${namespace} --timeout=120s
            if [ $? -ne 0 ]; then
                echo -e "${RED}✗ Podの準備完了待機がタイムアウトしました${NC}"
                return 1
            fi
        else
            return 1
        fi
    fi

    # データベース接続のリトライ
    echo -e "${CYAN}  PostgreSQL接続テストを実行中...${NC}"
    for ((i=1; i<=max_retries; i++)); do
        echo -e "${CYAN}    接続試行 ${i}/${max_retries}...${NC}"

        if kubectl exec -n ${namespace} ${db_pod} -- pg_isready -U postgres >/dev/null 2>&1; then
            echo -e "${GREEN}✓ データベースへの接続が確認できました${NC}"

            # 追加の接続詳細情報
            local db_info=$(kubectl exec -n ${namespace} ${db_pod} -- psql -U postgres -t -c "SELECT version();" 2>/dev/null | head -1 | tr -d ' ')
            if [ ! -z "$db_info" ]; then
                echo -e "${CYAN}  データベース情報: PostgreSQL起動確認${NC}"
            fi

            return 0
        fi

        if [ $i -lt $max_retries ]; then
            echo -e "${YELLOW}    ${retry_delay}秒後にリトライします...${NC}"
            sleep $retry_delay
        fi
    done

    echo -e "${RED}✗ データベースへの接続に失敗しました${NC}"
    echo -e "${YELLOW}詳細情報:${NC}"
    kubectl describe pod ${db_pod} -n ${namespace} | grep -A 10 "Events:"
    echo -e "${YELLOW}ログ確認:${NC}"
    kubectl logs ${db_pod} -n ${namespace} --tail=10 2>/dev/null || echo "    ログ取得に失敗"

    return 1
}

# エラーリカバリー機構
auto_recovery() {
    local error_type=$1
    local context=$2

    echo -e "${YELLOW}エラーリカバリーを開始します: ${error_type}${NC}"

    case "$error_type" in
        "pod_failure")
            echo -e "${CYAN}Pod失敗のリカバリーを実行中...${NC}"
            # 失敗したPodを再起動
            kubectl delete pod -l io.kompose.service=${context} --all-namespaces --ignore-not-found=true
            sleep 10
            ;;
        "pvc_stuck")
            echo -e "${CYAN}PVCスタックのリカバリーを実行中...${NC}"
            # スタックしたPVCをクリーンアップ
            validate_and_fix_pvc_state
            ;;
        "deployment_timeout")
            echo -e "${CYAN}デプロイメントタイムアウトのリカバリーを実行中...${NC}"
            # デプロイメントをリスタート
            kubectl rollout restart deployment/${context} --ignore-not-found=true
            ;;
        "namespace_missing")
            echo -e "${CYAN}名前空間作成を再実行中...${NC}"
            create_namespaces
            ;;
        *)
            echo -e "${RED}未知のエラータイプ: ${error_type}${NC}"
            return 1
            ;;
    esac

    echo -e "${GREEN}✓ エラーリカバリーが完了しました${NC}"
    return 0
}

# デプロイメントの実行
deploy_application() {
    echo -e "${BLUE}アプリケーションをデプロイ中...${NC}"

    BUILD_FILE="${KUSTOMIZE_BUILD_DIR}/${ENVIRONMENT}-manifest.yaml"

    if [ "$DRY_RUN" = "true" ]; then
        echo -e "${YELLOW}ドライランモード: 実際のデプロイは実行されません${NC}"
        kubectl apply -f ${BUILD_FILE} --dry-run=client
    else
        # 初回デプロイかどうか判定
        check_initial_deployment

        # データベース接続確認
        if [ "$ENVIRONMENT" = "production" ]; then
            if ! verify_database_connection; then
                echo -e "${RED}✗ データベース接続に問題があります。デプロイを中止します。${NC}"
                return 1
            fi
        fi

        # マイグレーション状態の確認とクリーンアップ
        if [ "$ENVIRONMENT" = "production" ]; then
            handle_migration_deployment
        fi

        # 現在の状態をバックアップ
        backup_current_state

        # デプロイメントの実行
        echo -e "${CYAN}リソースをデプロイ中...${NC}"
        if [ "$INITIAL_DEPLOYMENT" = "true" ]; then
            echo -e "${YELLOW}初回デプロイを実行中...${NC}"
            kubectl apply -f ${BUILD_FILE}
        else
            echo -e "${YELLOW}更新デプロイを実行中...${NC}"
            # 更新時は慎重にリソースを適用
            deploy_with_exclusions ${BUILD_FILE}
        fi

        # マイグレーション完了の待機（本番環境のみ）
        if [ "$ENVIRONMENT" = "production" ]; then
            if ! wait_for_migration_completion; then
                echo -e "${RED}✗ マイグレーションに失敗しました。${NC}"

                # エラーリカバリーを試みる
                echo -e "${YELLOW}エラーリカバリーを試みますか？ (y/N)${NC}"
                read -r response
                if [[ "$response" =~ ^([yY][eE][sS]|[yY])$ ]]; then
                    if auto_recovery "pod_failure" "migrate"; then
                        echo -e "${CYAN}リカバリー後のマイグレーションを再試行します...${NC}"
                        sleep 30
                        if ! wait_for_migration_completion; then
                            echo -e "${RED}✗ リカバリー後もマイグレーションに失敗しました。ロールバックを検討してください。${NC}"
                            return 1
                        fi
                    else
                        echo -e "${RED}✗ リカバリーに失敗しました。ロールバックを検討してください。${NC}"
                        return 1
                    fi
                else
                    echo -e "${RED}ロールバックを検討してください。${NC}"
                    return 1
                fi
            fi
        fi

        echo -e "${CYAN}デプロイメントの完了を待機中...${NC}"
        sleep 30
    fi
}

# PVC状態の確認と修復
validate_and_fix_pvc_state() {
    echo -e "${BLUE}PVC状態を検証中...${NC}"

    local pvc_issues=false

    # 主要なPVCの状態チェック
    local pvcs=("db-data:alt-database" "meili-data:alt-search" "clickhouse-data:alt-observability" "news-creator-models:alt-apps")

    for pvc_info in "${pvcs[@]}"; do
        local pvc_name="${pvc_info%:*}"
        local namespace="${pvc_info#*:}"

        echo -e "${CYAN}  ${pvc_name} (${namespace}) をチェック中...${NC}"

        if kubectl get pvc ${pvc_name} -n ${namespace} >/dev/null 2>&1; then
            local pvc_phase=$(kubectl get pvc ${pvc_name} -n ${namespace} -o jsonpath='{.status.phase}' 2>/dev/null)
            local deletion_timestamp=$(kubectl get pvc ${pvc_name} -n ${namespace} -o jsonpath='{.metadata.deletionTimestamp}' 2>/dev/null)

            if [ ! -z "$deletion_timestamp" ]; then
                echo -e "${RED}    ✗ ${pvc_name}は削除中です${NC}"
                pvc_issues=true

                # 削除中のPVCを強制的に削除
                echo -e "${YELLOW}    削除中のPVCを強制クリーンアップします...${NC}"

                # 先にPVCを使用しているPodを終了させる
                local using_pods=$(kubectl get pod --all-namespaces -o jsonpath='{range .items[*]}{.metadata.name}{","}{.metadata.namespace}{","}{.spec.volumes[*].persistentVolumeClaim.claimName}{"\n"}{end}' 2>/dev/null | grep ${pvc_name} | head -5)
                if [ ! -z "$using_pods" ]; then
                    echo -e "${CYAN}    PVCを使用中のPodを終了します...${NC}"
                    echo "$using_pods" | while IFS=',' read pod_name pod_ns pvc_claim; do
                        if [ "$pvc_claim" = "$pvc_name" ]; then
                            echo -e "${CYAN}      Pod: ${pod_name} (${pod_ns})を終了中...${NC}"
                            kubectl delete pod ${pod_name} -n ${pod_ns} --force --grace-period=0 2>/dev/null || true
                        fi
                    done
                    sleep 5
                fi

                kubectl patch pvc ${pvc_name} -n ${namespace} -p '{"metadata":{"finalizers":[]}}' --type=merge 2>/dev/null || true
                kubectl delete pvc ${pvc_name} -n ${namespace} --force --grace-period=0 2>/dev/null || true

                # 関連するPVも削除
                local pv_name=$(kubectl get pv -o jsonpath='{.items[?(@.spec.claimRef.name=="'${pvc_name}'")].metadata.name}' 2>/dev/null)
                if [ ! -z "$pv_name" ]; then
                    echo -e "${YELLOW}    関連するPV ${pv_name} も削除します...${NC}"
                    kubectl patch pv ${pv_name} -p '{"metadata":{"finalizers":[]}}' --type=merge 2>/dev/null || true
                    kubectl delete pv ${pv_name} --force --grace-period=0 2>/dev/null || true
                fi

                # PVCの削除完了を待機
                local wait_count=0
                while kubectl get pvc ${pvc_name} -n ${namespace} >/dev/null 2>&1 && [ $wait_count -lt 60 ]; do
                    echo -e "${CYAN}    PVC削除完了を待機中... (${wait_count}/60)${NC}"
                    sleep 2
                    wait_count=$((wait_count + 1))
                done

                if kubectl get pvc ${pvc_name} -n ${namespace} >/dev/null 2>&1; then
                    echo -e "${RED}    ✗ PVC削除が完了しませんでした。継続します...${NC}"
                else
                    echo -e "${GREEN}    ✓ PVC削除が完了しました${NC}"
                fi

            elif [ "$pvc_phase" != "Bound" ]; then
                echo -e "${YELLOW}    ⚠ ${pvc_name}の状態: ${pvc_phase}${NC}"
                pvc_issues=true
            else
                echo -e "${GREEN}    ✓ ${pvc_name}は正常です (${pvc_phase})${NC}"
            fi
        else
            echo -e "${YELLOW}    - ${pvc_name}は存在しません（初回デプロイ予定）${NC}"
        fi
    done

    if [ "$pvc_issues" = "true" ]; then
        echo -e "${YELLOW}PVC問題が検出されました。10秒待機してから続行します...${NC}"
        sleep 10
    fi

    return 0
}

# ストレージディレクトリとパーミッションの検証
validate_storage_directories() {
    echo -e "${BLUE}ストレージディレクトリの状態を検証中...${NC}"

    local directories=("/mnt/data/postgres" "/mnt/data/clickhouse" "/mnt/data/meilisearch" "/mnt/data/news-creator-models")
    local fix_permissions=false

    for dir in "${directories[@]}"; do
        if [ -d "$dir" ]; then
            local permissions=$(stat -c "%a" "$dir" 2>/dev/null)
            local owner=$(stat -c "%U:%G" "$dir" 2>/dev/null)

            echo -e "${CYAN}  ${dir}: パーミッション=${permissions}, 所有者=${owner}${NC}"

            # パーミッションが777でない場合は修正フラグを立てる
            if [ "$permissions" != "777" ]; then
                echo -e "${YELLOW}    ⚠ パーミッションが不適切です${NC}"
                fix_permissions=true
            fi
        else
            echo -e "${RED}  ✗ ${dir} が存在しません${NC}"
            fix_permissions=true
        fi
    done

    if [ "$fix_permissions" = "true" ]; then
        echo -e "${YELLOW}ストレージディレクトリの問題を修正しますか？ (y/N)${NC}"
        read -r response
        if [[ "$response" =~ ^([yY][eE][sS]|[yY])$ ]]; then
            create_storage_directories
        fi
    else
        echo -e "${GREEN}✓ すべてのストレージディレクトリが正常です${NC}"
    fi
}

# PVC再作成の実行
recreate_problematic_pvcs() {
    local manifest_file=$1

    echo -e "${BLUE}問題のあるPVCを再作成中...${NC}"

    # マニフェストからPVCのみを抽出して適用
    kubectl apply -f ${manifest_file} --dry-run=client -o yaml | \
    grep -A 50 "kind: PersistentVolumeClaim" | \
    kubectl apply -f - 2>/dev/null || true

    # PVCの作成完了を待機
    echo -e "${CYAN}PVC作成の完了を待機中...${NC}"
    sleep 20

    # 作成されたPVCの状態確認
    local pvcs=("db-data:alt-database" "meili-data:alt-search" "clickhouse-data:alt-observability" "news-creator-models:alt-apps")

    for pvc_info in "${pvcs[@]}"; do
        local pvc_name="${pvc_info%:*}"
        local namespace="${pvc_info#*:}"

        if kubectl get pvc ${pvc_name} -n ${namespace} >/dev/null 2>&1; then
            local pvc_phase=$(kubectl get pvc ${pvc_name} -n ${namespace} -o jsonpath='{.status.phase}' 2>/dev/null)
            echo -e "${CYAN}  ${pvc_name}: ${pvc_phase}${NC}"

            if [ "$pvc_phase" = "Bound" ]; then
                echo -e "${GREEN}    ✓ 正常にバインドされました${NC}"
            else
                echo -e "${YELLOW}    ⚠ まだバインドされていません${NC}"
            fi
        fi
    done
}

# 初回デプロイかどうかの判定
check_initial_deployment() {
    echo -e "${BLUE}デプロイメント状況を確認中...${NC}"

    # ストレージディレクトリの検証
    validate_storage_directories

    # PVC状態の検証と修復
    validate_and_fix_pvc_state

    # 主要なPVCが存在し、正常な状態かチェック
    local existing_pvcs=0
    local bound_pvcs=0

    local pvcs=("db-data:alt-database" "meili-data:alt-search")

    for pvc_info in "${pvcs[@]}"; do
        local pvc_name="${pvc_info%:*}"
        local namespace="${pvc_info#*:}"

        if kubectl get pvc ${pvc_name} -n ${namespace} >/dev/null 2>&1; then
            existing_pvcs=$((existing_pvcs + 1))
            local pvc_phase=$(kubectl get pvc ${pvc_name} -n ${namespace} -o jsonpath='{.status.phase}' 2>/dev/null)
            if [ "$pvc_phase" = "Bound" ]; then
                bound_pvcs=$((bound_pvcs + 1))
            fi
        fi
    done

    # StorageClassの存在確認
    if kubectl get storageclass local-storage >/dev/null 2>&1 && \
       [ $existing_pvcs -ge 2 ] && [ $bound_pvcs -ge 2 ]; then
        INITIAL_DEPLOYMENT="false"
        echo -e "${GREEN}既存のデプロイメントを検出しました (更新モード)${NC}"
    else
        INITIAL_DEPLOYMENT="true"
        echo -e "${YELLOW}初回デプロイメントを検出しました${NC}"
        echo -e "${CYAN}必要なStorageClassまたはPVCが見つかりません。初回デプロイを実行します。${NC}"
        echo -e "${CYAN}  既存PVC: ${existing_pvcs}/2, バインド済みPVC: ${bound_pvcs}/2${NC}"
    fi
}

# 更新デプロイ時の除外処理
deploy_with_exclusions() {
    local manifest_file=$1

    echo -e "${CYAN}ConfigMaps, Secrets, Services, Deploymentsを適用中...${NC}"

    # ストレージ関連リソースを除外してマニフェストを分割適用
    kubectl apply -f ${manifest_file} || {

        echo -e "${YELLOW}一部のリソース適用に失敗しました。安全なリソースのみ個別に適用します...${NC}"

        # 一時ファイルに分割して適用
        local temp_dir="/tmp/alt-deploy-$$"
        mkdir -p ${temp_dir}

        # マニフェストからリソース別に分割
        kubectl apply -f ${manifest_file} --dry-run=client -o yaml | \
        csplit --digits=3 --quiet --prefix="${temp_dir}/resource-" - '/^---$/' '{*}' 2>/dev/null || true

        # 各リソースファイルを個別に適用（ストレージ関連を除く）
        for resource_file in ${temp_dir}/resource-*; do
            if [ -f "$resource_file" ]; then
                # PV, PVC, StorageClassを除外
                if ! grep -q "kind: \(PersistentVolume\|PersistentVolumeClaim\|StorageClass\)" "$resource_file"; then
                    kubectl apply -f "$resource_file" 2>/dev/null || true
                fi
            fi
        done

        # 一時ファイルを削除
        rm -rf ${temp_dir}
    }

    echo -e "${GREEN}✓ 主要なアプリケーションリソースの適用が完了しました${NC}"
    echo -e "${CYAN}注意: StorageClass、PV、PVCは既存のものを使用します${NC}"
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
    elif [ "$ENVIRONMENT" = "development" ]; then
        kubectl get all,configmap,secret,pvc -n alt-dev -o yaml > ${BACKUP_DIR}/alt-dev-backup.yaml 2>/dev/null || true
    else
        kubectl get all,configmap,secret,pvc -n ${NAMESPACE} -o yaml > ${BACKUP_DIR}/${NAMESPACE}-backup.yaml 2>/dev/null || true
    fi

    echo -e "${GREEN}✓ バックアップ完了: ${BACKUP_DIR}${NC}"
}

# デプロイメントの状態確認
check_deployment_status() {
    echo -e "${BLUE}デプロイメントの状態を確認中...${NC}"

    if [ "$ENVIRONMENT" = "development" ]; then
        NAMESPACE="alt-dev"
        NAMESPACES=(${NAMESPACE})
    elif [ "$ENVIRONMENT" = "production" ]; then
        NAMESPACE="alt-production"
        NAMESPACES=(alt-apps alt-database alt-search alt-observability alt-ingress)
    else
        NAMESPACE="alt-${ENVIRONMENT}"
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

# ヘルスチェック設定
HEALTH_CHECK_TIMEOUT=30
HEALTH_CHECK_RETRIES=3
HEALTH_CHECK_RETRY_DELAY=5

# 単一サービスのヘルスチェック実行
check_service_health() {
    local service_name=$1
    local namespace=$2
    local health_endpoint=$3
    local port=$4
    local max_retries=${5:-$HEALTH_CHECK_RETRIES}

    echo -e "${CYAN}${service_name}のヘルスチェック:${NC}"

    # Podが存在するかチェック
    local pod_name=$(kubectl get pod -n ${namespace} -l io.kompose.service=${service_name} -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
    if [ -z "$pod_name" ]; then
        echo -e "${RED}✗ ${service_name}のPodが見つかりません${NC}"
        return 1
    fi

    # Podが準備完了状態かチェック
    local pod_ready=$(kubectl get pod ${pod_name} -n ${namespace} -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null)
    if [ "$pod_ready" != "True" ]; then
        echo -e "${YELLOW}⚠ ${service_name}のPodはまだ準備中です${NC}"
        return 1
    fi

    # ヘルスチェック実行（リトライ付き）
    for ((i=1; i<=max_retries; i++)); do
        echo -e "${CYAN}  試行 ${i}/${max_retries}...${NC}"

        # タイムアウト付きでヘルスチェック実行
        if timeout ${HEALTH_CHECK_TIMEOUT} kubectl exec -n ${namespace} ${pod_name} -- \
           wget -qO- --timeout=10 ${health_endpoint} >/dev/null 2>&1; then
            echo -e "${GREEN}✓ ${service_name}は正常です${NC}"
            return 0
        fi

        if [ $i -lt $max_retries ]; then
            echo -e "${YELLOW}  ${HEALTH_CHECK_RETRY_DELAY}秒後にリトライします...${NC}"
            sleep ${HEALTH_CHECK_RETRY_DELAY}
        fi
    done

    echo -e "${RED}✗ ${service_name}のヘルスチェックに失敗（${max_retries}回試行）${NC}"

    # 失敗時の詳細情報を表示
    echo -e "${YELLOW}  Pod状態の詳細:${NC}"
    kubectl describe pod ${pod_name} -n ${namespace} | grep -A 10 "Conditions:"
    echo -e "${YELLOW}  最新のログ:${NC}"
    kubectl logs ${pod_name} -n ${namespace} --tail=10 2>/dev/null || echo "    ログ取得に失敗"

    return 1
}

# マイグレーションジョブのヘルスチェック
check_migration_health() {
    local namespace=$(detect_migration_namespace)

    echo -e "${CYAN}マイグレーションジョブのヘルスチェック:${NC}"
    echo -e "${CYAN}  使用する名前空間: ${namespace}${NC}"

    # マイグレーションジョブの状態をチェック
    local job_status=$(kubectl get job migrate -n ${namespace} -o jsonpath='{.status.conditions[?(@.type=="Complete")].status}' 2>/dev/null)
    local job_succeeded=$(kubectl get job migrate -n ${namespace} -o jsonpath='{.status.succeeded}' 2>/dev/null)

    if [ "$job_status" = "True" ] && [ "$job_succeeded" = "1" ]; then
        echo -e "${GREEN}✓ マイグレーションは正常に完了しています${NC}"
        return 0
    fi

    # 失敗している場合の詳細確認
    echo -e "${RED}✗ マイグレーションに問題があります${NC}"

    # マイグレーションPodの詳細情報
    local migrate_pod=$(kubectl get pod -n ${namespace} -l io.kompose.service=migrate -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
    if [ ! -z "$migrate_pod" ]; then
        echo -e "${YELLOW}  マイグレーションPodの状態:${NC}"
        kubectl get pod ${migrate_pod} -n ${namespace} -o wide

        echo -e "${YELLOW}  マイグレーションログ:${NC}"
        kubectl logs ${migrate_pod} -n ${namespace} --tail=20 2>/dev/null || echo "    ログ取得に失敗"

        # CrashLoopBackOffの場合は前回のログも表示
        local pod_status=$(kubectl get pod ${migrate_pod} -n ${namespace} -o jsonpath='{.status.phase}' 2>/dev/null)
        if [[ "$(kubectl get pod ${migrate_pod} -n ${namespace} -o jsonpath='{.status.containerStatuses[0].state}')" == *"waiting"* ]]; then
            echo -e "${YELLOW}  前回実行時のログ:${NC}"
            kubectl logs ${migrate_pod} -n ${namespace} --previous --tail=20 2>/dev/null || echo "    前回ログなし"
        fi
    fi

    return 1
}

# サービス可用性の事前チェック
check_service_availability() {
    local namespace=$1
    local max_wait=180  # 3分
    local interval=10
    local elapsed=0

    echo -e "${BLUE}サービスの準備状況を確認中...${NC}"

    while [ $elapsed -lt $max_wait ]; do
        local ready_count=0
        local total_count=0

        # 各デプロイメントの準備状況をチェック
        for deployment in $(kubectl get deployment -n ${namespace} -o jsonpath='{.items[*].metadata.name}' 2>/dev/null); do
            total_count=$((total_count + 1))
            local ready_replicas=$(kubectl get deployment ${deployment} -n ${namespace} -o jsonpath='{.status.readyReplicas}' 2>/dev/null)
            local desired_replicas=$(kubectl get deployment ${deployment} -n ${namespace} -o jsonpath='{.spec.replicas}' 2>/dev/null)

            if [ "$ready_replicas" = "$desired_replicas" ] && [ "$ready_replicas" != "0" ]; then
                ready_count=$((ready_count + 1))
            fi
        done

        echo -e "${CYAN}  準備完了: ${ready_count}/${total_count} サービス${NC}"

        if [ $ready_count -eq $total_count ] && [ $total_count -gt 0 ]; then
            echo -e "${GREEN}✓ すべてのサービスが準備完了です${NC}"
            return 0
        fi

        sleep $interval
        elapsed=$((elapsed + interval))

        if [ $((elapsed % 30)) -eq 0 ]; then
            echo -e "${YELLOW}  待機中... (${elapsed}/${max_wait}秒経過)${NC}"
        fi
    done

    echo -e "${RED}✗ タイムアウト: 一部のサービスが準備完了していません${NC}"
    return 1
}

# ヘルスチェックの実行
perform_health_check() {
    echo -e "${BLUE}ヘルスチェックを実行中...${NC}"

    NAMESPACE="alt-${ENVIRONMENT}"
    if [ "$ENVIRONMENT" = "development" ]; then
        NAMESPACE="alt-dev"
    elif [ "$ENVIRONMENT" = "production" ]; then
        NAMESPACE="alt-apps"
    fi

    # サービス可用性の事前チェック
    if ! check_service_availability ${NAMESPACE}; then
        echo -e "${YELLOW}⚠ 一部のサービスがまだ準備中です。ヘルスチェックを継続します...${NC}"
    fi

    # マイグレーションのヘルスチェック（本番環境のみ）
    if [ "$ENVIRONMENT" = "production" ]; then
        if ! check_migration_health; then
            echo -e "${RED}✗ マイグレーションに問題があります。管理者に連絡してください。${NC}"
        fi
    fi

    local health_check_failed=false

    # バックエンドのヘルスチェック
    if kubectl get service alt-backend -n ${NAMESPACE} &> /dev/null; then
        if ! check_service_health "alt-backend" ${NAMESPACE} "http://localhost:9000/v1/health" "9000"; then
            health_check_failed=true
        fi
    fi

    # フロントエンドのヘルスチェック
    if kubectl get service alt-frontend -n ${NAMESPACE} &> /dev/null; then
        if ! check_service_health "alt-frontend" ${NAMESPACE} "http://localhost:3000" "3000"; then
            health_check_failed=true
        fi
    fi

    # プリプロセッサーのヘルスチェック
    if kubectl get service pre-processor -n ${NAMESPACE} &> /dev/null; then
        if ! check_service_health "pre-processor" ${NAMESPACE} "http://localhost:9200/health" "9200"; then
            health_check_failed=true
        fi
    fi

    # 検索インデクサーのヘルスチェック
    if kubectl get service search-indexer -n ${NAMESPACE} &> /dev/null; then
        if ! check_service_health "search-indexer" ${NAMESPACE} "http://localhost:9300/health" "9300"; then
            health_check_failed=true
        fi
    fi

    # ヘルスチェック結果のサマリー
    if [ "$health_check_failed" = "true" ]; then
        echo -e "${RED}⚠ 一部のサービスでヘルスチェックに失敗しました${NC}"
        echo -e "${YELLOW}詳細については上記のログを確認してください${NC}"
        return 1
    else
        echo -e "${GREEN}✓ すべてのヘルスチェックが成功しました${NC}"
        return 0
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
    local start_time=$(date +%s)

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
                # リアルタイム進捗監視
                    if [ "$ENVIRONMENT" = "development" ]; then
        local namespace="alt-dev"
    elif [ "$ENVIRONMENT" = "production" ]; then
        local namespace="alt-apps"
    else
        local namespace="alt-${ENVIRONMENT}"
    fi

                echo -e "${CYAN}デプロイメント進捗の監視を開始します...${NC}"
                show_deployment_progress ${namespace} "デプロイメント"

                # 詳細な状態確認
                sleep 5
                check_deployment_status

                # デプロイメントサマリーの生成
                generate_deployment_summary ${start_time}
            fi
            ;;
        delete)
            build_manifests
            cleanup_deployment
            ;;
        rollback)
            rollback_deployment
            if [ "$DRY_RUN" = "false" ]; then
                generate_deployment_summary ${start_time}
            fi
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

    local end_time=$(date +%s)
    local total_duration=$((end_time - start_time))
    local total_minutes=$((total_duration / 60))
    local total_seconds=$((total_duration % 60))
    echo -e "${BLUE}総実行時間: ${total_minutes}分${total_seconds}秒${NC}"
}

# スクリプトの実行
main