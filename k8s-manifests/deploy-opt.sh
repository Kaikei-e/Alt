#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

# ───────────────────────────────────────────────────────────
# deploy-opt.sh — Helm-based deployment for Alt services
# Pattern‑B 対応：Helm Charts with environment-specific values
#   TAG_BASE は build-images.sh が出力時に環境変数で渡す想定
#
# 使い方例
#   TAG_BASE=20250715220100-nogit IMAGE_PREFIX=example/project \
#       ./deploy-opt.sh production -r
#   （TAG_BASE を渡さなければデフォルトタグを使用）
#
# 引数
#   <env>               development | staging | production (必須)
#   -d / --dry-run      helm template での dry‑run (適用せず YAML 出力)
#   -r / --restart      deploy 後に rollout restart
#   -n / --namespace    デフォルト alt-<env> を上書き
#   -h / --help         ヘルプ
# 環境変数
#   IMAGE_PREFIX        プライベートレポ (例 example/project) ※必須
#   TAG_BASE            <timestamp>-<sha> 形式 (省略時デフォルトタグ)
# ───────────────────────────────────────────────────────────

# 色
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BLUE='\033[0;34m'; CYAN='\033[0;36m'; NC='\033[0m'

# 変数
ENV=""; DRY=false; DO_RESTART=false; TARGET_NS="";
IMAGE_PREFIX="${IMAGE_PREFIX:-}"  # required for image patch
TAG_BASE="${TAG_BASE:-}"          # optional

usage(){ cat <<EOF
Usage: IMAGE_PREFIX=<repo> [TAG_BASE=<ts-sha>] $0 <env> [options]
  <env>                    development | staging | production
  -d, --dry-run            server-side dry-run (no changes)
  -r, --restart            rollout restart after apply / image patch
  -n, --namespace <ns>     target namespace (default alt-<env>)
  -h, --help               show this help
EOF
exit 1; }

# 引数パース
while [[ $# -gt 0 ]]; do
  case "$1" in
    development|staging|production) ENV="$1"; shift;;
    -d|--dry-run) DRY=true; shift;;
    -r|--restart) DO_RESTART=true; shift;;
    -n|--namespace) TARGET_NS="$2"; shift 2;;
    -h|--help) usage;;
    *) echo -e "${RED}Unknown arg: $1${NC}"; usage;;
  esac
done
[[ -z "$ENV" ]] && usage
[[ -z "$IMAGE_PREFIX" ]] && { echo -e "${RED}IMAGE_PREFIX 必須${NC}"; exit 1; }
TARGET_NS="${TARGET_NS:-alt-$ENV}"

# 依存チェック
command -v helm &>/dev/null || { echo -e "${RED}helm missing${NC}"; exit 1; }
command -v kubectl &>/dev/null || { echo -e "${RED}kubectl missing${NC}"; exit 1; }

# Chart配列（デプロイ順序）
INFRASTRUCTURE_CHARTS=(
  common-config common-ssl common-secrets
  postgres auth-postgres kratos-postgres kratos clickhouse meilisearch nginx nginx-external
  monitoring
)

APPLICATION_CHARTS=(
  alt-backend auth-service pre-processor search-indexer tag-generator
  news-creator rask-log-aggregator alt-frontend
)

OPERATIONAL_CHARTS=(
  migrate backup
)

deploy_charts(){
  echo -e "${BLUE}▶ Deploying Helm charts ($ENV)${NC}"
  local charts_dir="../charts"

  # Infrastructure Chartsから順次デプロイ
  echo -e "${CYAN}▶ Deploying Infrastructure charts${NC}"
  for chart in "${INFRASTRUCTURE_CHARTS[@]}"; do
    deploy_single_chart "$chart" "$charts_dir"
  done

  # Application Charts
  echo -e "${CYAN}▶ Deploying Application charts${NC}"
  for chart in "${APPLICATION_CHARTS[@]}"; do
    deploy_single_chart "$chart" "$charts_dir"
  done

  # Operational Charts
  echo -e "${CYAN}▶ Deploying Operational charts${NC}"
  for chart in "${OPERATIONAL_CHARTS[@]}"; do
    deploy_single_chart "$chart" "$charts_dir"
  done
}

deploy_single_chart(){
  local chart="$1"
  local charts_dir="$2"
  local chart_path="${charts_dir}/${chart}"
  local values_file="${chart_path}/values-${ENV}.yaml"
  local namespace

  # Chart が存在するかチェック
  [[ ! -d "$chart_path" ]] && { echo -e "${YELLOW}⚠ Chart $chart not found (skipped)${NC}"; return; }

  # 環境別values.yamlが存在するかチェック
  [[ ! -f "$values_file" ]] && values_file="${chart_path}/values.yaml"
  [[ ! -f "$values_file" ]] && { echo -e "${YELLOW}⚠ Values file not found for $chart (skipped)${NC}"; return; }

  # namespaceを決定
  namespace=$(determine_namespace "$chart" "$ENV")

  echo -e "${CYAN}  ↪ $chart → $namespace${NC}"

  if $DRY; then
    local image_args=()
    get_image_overrides "$chart" image_args
    if [[ ${#image_args[@]} -gt 0 ]]; then
      helm template "$chart" "$chart_path" \
        -f "$values_file" \
        --namespace "$namespace" \
        "${image_args[@]}" | less -R
    else
      helm template "$chart" "$chart_path" \
        -f "$values_file" \
        --namespace "$namespace" | less -R
    fi
  else
    local wait_args=()
    if should_wait_for_chart "$chart"; then
      wait_args=("--wait" "--timeout=300s")
    fi

    local image_args=()
    get_image_overrides "$chart" image_args
    
    if [[ ${#image_args[@]} -gt 0 ]]; then
      if [[ ${#wait_args[@]} -gt 0 ]]; then
        # Both image overrides and wait args
        helm upgrade --install "$chart" "$chart_path" \
          -f "$values_file" \
          --namespace "$namespace" \
          --create-namespace \
          "${image_args[@]}" \
          "${wait_args[@]}" || {
            echo -e "${RED}✗ deploy failed: $chart${NC}"; return 1;
          }
      else
        # Only image overrides
        helm upgrade --install "$chart" "$chart_path" \
          -f "$values_file" \
          --namespace "$namespace" \
          --create-namespace \
          "${image_args[@]}" || {
            echo -e "${RED}✗ deploy failed: $chart${NC}"; return 1;
          }
      fi
    else
      # No image overrides
      helm upgrade --install "$chart" "$chart_path" \
        -f "$values_file" \
        --namespace "$namespace" \
        --create-namespace \
        "${wait_args[@]}" || {
          echo -e "${RED}✗ deploy failed: $chart${NC}"; return 1;
        }
    fi
    echo -e "${GREEN}✓ $chart deployed${NC}"
  fi
}

determine_namespace(){
  local chart="$1"
  local env="$2"

  case "$env" in
    development) echo "alt-dev" ;;
    staging) echo "alt-staging" ;;
    production)
      case "$chart" in
        alt-backend|alt-frontend|pre-processor|search-indexer|tag-generator|news-creator|rask-log-aggregator) echo "alt-apps" ;;
        postgres|auth-postgres|kratos-postgres|clickhouse) echo "alt-database" ;;
        meilisearch) echo "alt-search" ;;
        auth-service|kratos) echo "alt-auth" ;;
        nginx|nginx-external) echo "alt-ingress" ;;
        monitoring|rask-log-aggregator) echo "alt-observability" ;;
        *) echo "alt-production" ;;
      esac ;;
    *) echo "alt-$env" ;;
  esac
}

should_wait_for_chart(){
  local chart="$1"

  # インフラストラクチャチャートは--waitを使用しない（大きなイメージのプルで時間がかかるため）
  case "$chart" in
    clickhouse|meilisearch|postgres|auth-postgres|kratos-postgres|kratos|nginx-external|monitoring)
      return 1  # don't wait
      ;;
    *)
      return 0  # wait for readiness
      ;;
  esac
}

get_image_overrides(){
  local chart="$1"
  local -n image_args_ref="$2"

  [[ -z "$TAG_BASE" ]] && return 0

  # チャートごとのイメージオーバーライド
  # インフラストラクチャチャート（clickhouse、meilisearch）は公式イメージを使用するため
  # カスタムタグでのオーバーライドは行わない
  case "$chart" in
    alt-backend|auth-service|pre-processor|search-indexer|tag-generator|news-creator|rask-log-aggregator|alt-frontend)
      image_args_ref=("--set" "image.tag=${TAG_BASE}" "--set" "image.repository=${IMAGE_PREFIX}/${chart}")
      ;;
    *)
      # インフラストラクチャチャートや共通Chartはイメージオーバーライドなし
      # clickhouse, meilisearch, postgres等は公式の安定版イメージを使用
      image_args_ref=()
      ;;
  esac
}

rollout_restart_all(){
  $DRY && return 0
  $DO_RESTART || return 0

  echo -e "${CYAN}▶ rollout restart for all deployments${NC}"

  # 全namespaceのリソースを再起動
  local namespaces=()
  case "$ENV" in
    development) namespaces=("alt-dev") ;;
    staging) namespaces=("alt-staging") ;;
    production) namespaces=("alt-apps" "alt-database" "alt-search" "alt-auth" "alt-ingress" "alt-observability" "alt-production") ;;
    *) namespaces=("alt-$ENV") ;;
  esac

  for ns in "${namespaces[@]}"; do
    echo -e "${CYAN}  ↪ namespace: $ns${NC}"
    for kind in deployment statefulset daemonset; do
      mapfile -t res < <(kubectl -n "$ns" get "$kind" -o name 2>/dev/null || true)
      for r in "${res[@]}"; do
        [[ -z "$r" ]] && continue
        echo -e "${CYAN}    ↻ restarting $r${NC}"
        kubectl -n "$ns" rollout restart "$r" || echo -e "${YELLOW}⚠ restart failed on $r (ignored)${NC}"
      done
    done
  done
}

wait_rollout(){
  $DRY && { echo -e "${YELLOW}dry-run: skip rollout status${NC}"; return; }

  echo -e "${CYAN}▶ waiting for all rollouts to complete${NC}"

  # 全namespaceのロールアウト状況を監視
  local namespaces=()
  case "$ENV" in
    development) namespaces=("alt-dev") ;;
    staging) namespaces=("alt-staging") ;;
    production) namespaces=("alt-apps" "alt-database" "alt-search" "alt-auth" "alt-ingress" "alt-observability" "alt-production") ;;
    *) namespaces=("alt-$ENV") ;;
  esac

  for ns in "${namespaces[@]}"; do
    echo -e "${CYAN}  ↪ namespace: $ns${NC}"
    for kind in deployment statefulset daemonset; do
      mapfile -t res < <(kubectl -n "$ns" get "$kind" -o name 2>/dev/null || true)
      for r in "${res[@]}"; do
        [[ -z "$r" ]] && continue
        echo -e "${CYAN}    ↪ $r${NC}"
        kubectl -n "$ns" rollout status "$r" --timeout=300s || {
          echo -e "${RED}✗ rollout failed: $r${NC}"; exit 1;
        }
      done
    done
  done
  echo -e "${GREEN}✓ all rollouts complete${NC}"
}

main(){
  local start=$(date +%s)
  deploy_charts
  rollout_restart_all
  wait_rollout
  echo -e "${GREEN}Helm deployment completed in $(( $(date +%s) - start ))s${NC}"
}

main "$@"
