#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

# ───────────────────────────────────────────────────────────
# build-images.sh — SHA256タグ付けビルド & Kindロードスクリプト
# • 各サービスを「<service>:sha256-<hash>」形式でタグ付け
# • <service>:latest タグも追加
# • 自動的にkindクラスターにロード
# • IMAGE_PREFIX不要のシンプル設計
#
# 使い方例:
#   ./build-images.sh all
#   ./build-images.sh alt-backend,alt-frontend
#   ./build-images.sh auth-token-manager
#
# オプション環境変数:
#   KIND_CLUSTER_NAME : kindクラスター名 (デフォルト: alt-prod)
# ───────────────────────────────────────────────────────────

# ----- カラー -----
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BLUE='\033[0;34m'; CYAN='\033[0;36m'; NC='\033[0m'

# ----- グローバル変数 -----
TIMESTAMP="$(date +%Y%m%d%H%M%S)"
GIT_SHA="$(git rev-parse --short=HEAD 2>/dev/null || echo 'nogit')"
KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-alt-prod}"

# ----- サービス → Dockerfile パス -----
declare -A SERVICE_CONFIGS=(
  [alt-backend]="alt-backend/Dockerfile.backend"
  [alt-frontend]="alt-frontend/Dockerfile.frontend"
  [auth-service]="auth-service/Dockerfile"
  [auth-token-manager]="auth-token-manager/Dockerfile"
  [pre-processor]="pre-processor/Dockerfile"
  [pre-processor-sidecar]="pre-processor-sidecar/Dockerfile.pre-processor-sidecar"
  [news-creator]="news-creator/Dockerfile.creator"
  [search-indexer]="search-indexer/Dockerfile.search-indexer"
  [tag-generator]="tag-generator/Dockerfile.tag-generator-balanced"
  [migrate]="migrate/Dockerfile.migrate"
  [rask-log-aggregator]="rask-log-aggregator/Dockerfile.rask-log-aggregator"
  [rask-log-forwarder]="rask-log-forwarder/app/Dockerfile.rask-log-forwarder"
)

# ----- 関数 -----
usage() {
  cat <<EOF
Usage: [KIND_CLUSTER_NAME=cluster-name] $0 [all|svc1,svc2]
  all          : 全サービスをビルド
  svc1,svc2    : カンマ区切りで特定サービスのみビルド
  
Examples:
  ./build-images.sh all                    # 全サービスビルド
  ./build-images.sh auth-token-manager     # auth-token-managerのみ
  ./build-images.sh alt-backend,alt-frontend
  
Environment:
  KIND_CLUSTER_NAME : kindクラスター名 (デフォルト: alt-prod)
EOF
  exit 1
}

check_deps() {
  for cmd in docker kind git date sha256sum; do
    if ! command -v "$cmd" &>/dev/null; then
      echo -e "${RED}✗ $cmd が必要です${NC}" >&2
      exit 1
    fi
  done
  
  # kindクラスター存在確認
  if ! kind get clusters | grep -q "^${KIND_CLUSTER_NAME}$"; then
    echo -e "${YELLOW}⚠ kindクラスター '${KIND_CLUSTER_NAME}' が見つかりません${NC}"
    echo -e "${BLUE}利用可能なクラスター:${NC}"
    kind get clusters | sed 's/^/  /'
    exit 1
  fi
  
  echo -e "${GREEN}✓ 依存コマンド & kindクラスター OK${NC}"
}

generate_sha256_tag() {
  local svc="$1"
  local context="$2"
  
  # ディレクトリの内容からSHA256ハッシュを生成
  local content_hash=$(find "$context" -type f \( -name "*.ts" -o -name "*.go" -o -name "*.py" -o -name "*.rs" -o -name "Dockerfile*" -o -name "*.json" -o -name "*.yaml" \) \
    -exec sha256sum {} \; | sort | sha256sum | cut -d' ' -f1 | head -c 16)
  
  printf 'sha256-%s' "$content_hash"
}

build_and_load() {
  local svc="$1"
  local df_path="${SERVICE_CONFIGS[$svc]-}"

  # 定義チェック
  if [[ -z "$df_path" ]]; then
    echo -e "${YELLOW}⚠ 未定義サービス: $svc${NC}"
    return
  fi

  # ディレクトリ存在チェック
  local dir="$(dirname "$df_path")"
  if [[ ! -d "$dir" ]]; then
    echo -e "${YELLOW}⚠ path 不存在: $dir${NC}"
    return
  fi

  # SHA256タグ生成
  local sha_tag="$(generate_sha256_tag "$svc" "$dir")"
  local sha_image="${svc}:${sha_tag}"
  local latest_image="${svc}:latest"

  # ビルド
  echo -e "${BLUE}▶ Building $svc → $sha_image${NC}"
  pushd "$dir" >/dev/null
  docker build --pull -f "$(basename "$df_path")" -t "$sha_image" .
  docker tag "$sha_image" "$latest_image"
  popd >/dev/null

  # kindクラスターにロード
  echo -e "${CYAN}↪ Loading to kind cluster: $KIND_CLUSTER_NAME${NC}"
  kind load docker-image "$sha_image" --name "$KIND_CLUSTER_NAME"
  kind load docker-image "$latest_image" --name "$KIND_CLUSTER_NAME"

  echo -e "${GREEN}✓ 完了: $svc${NC}"
  echo -e "${GREEN}  📦 Image: $sha_image${NC}"
  echo -e "${GREEN}  📦 Latest: $latest_image${NC}"
  echo -e "${GREEN}  🔄 Loaded to kind cluster: $KIND_CLUSTER_NAME${NC}\n"
}

main() {
  [[ $# -eq 0 ]] && usage

  check_deps

  local target="$1"
  
  echo -e "${BLUE}🚀 Starting build process${NC}"
  echo -e "${BLUE}Kind cluster: ${KIND_CLUSTER_NAME}${NC}"
  echo -e "${BLUE}Git SHA: ${GIT_SHA}${NC}\n"
  
  if [[ "$target" == all ]]; then
    echo -e "${BLUE}Building all services...${NC}"
    for svc in "${!SERVICE_CONFIGS[@]}"; do build_and_load "$svc"; done
  else
    IFS=',' read -ra list <<<"$target"
    for svc in "${list[@]}"; do build_and_load "${svc// /}"; done
  fi

  echo -e "${GREEN}🎉 All services completed!${NC}"
  echo -e "${GREEN}Images loaded to kind cluster: ${KIND_CLUSTER_NAME}${NC}"
}

main "$@"
