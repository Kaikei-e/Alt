#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

# ───────────────────────────────────────────────────────────
# build-images.sh — Pattern‑B (単一リポジトリ) 用ビルド & プッシュスクリプト
# • 1 つのリポジトリ (IMAGE_PREFIX) に各サービスを「<service>-<timestamp>-<sha>」形式のタグで格納
# • <service>-latest タグも追加
# • SKIP_PUSH=true でリモート push をスキップ
# • ビルドしたイメージは containerd(k8s.io) にインポート
#
# 使い方例:
#   IMAGE_PREFIX=myuser/project-alt ./build-images.sh all
#   IMAGE_PREFIX=myuser/project-alt SKIP_PUSH=true ./build-images.sh alt-backend,alt-frontend
#
# 必要な環境変数:
#   IMAGE_PREFIX : <namespace>/<repo> 形式 (必須)
#   SKIP_PUSH    : true で push をスキップ (省略時 push 実行)
# ───────────────────────────────────────────────────────────

# ----- カラー -----
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BLUE='\033[0;34m'; CYAN='\033[0;36m'; NC='\033[0m'

# ----- グローバル変数 -----
TIMESTAMP="$(date +%Y%m%d%H%M%S)"
GIT_SHA="$(git rev-parse --short=HEAD 2>/dev/null || echo 'nogit')"
SKIP_PUSH="${SKIP_PUSH:-false}"
IMAGE_PREFIX="${IMAGE_PREFIX:-}"

# ----- サービス → Dockerfile パス -----
declare -A SERVICE_CONFIGS=(
  [alt-backend]="alt-backend/Dockerfile.backend"
  [alt-frontend]="alt-frontend/Dockerfile.frontend"
  [auth-service]="auth-service/Dockerfile"
  [pre-processor]="pre-processor/Dockerfile"
  [news-creator]="news-creator/Dockerfile.creator"
  [search-indexer]="search-indexer/Dockerfile.search-indexer"
  [tag-generator]="tag-generator/Dockerfile.tag-generator"
  [migrate]="migrate/Dockerfile.migrate"
  [rask-log-aggregator]="rask-log-aggregator/Dockerfile.rask-log-aggregator"
  [rask-log-forwarder]="rask-log-forwarder/app/Dockerfile.rask-log-forwarder"
)

# ----- 関数 -----
usage() {
  cat <<EOF
Usage: IMAGE_PREFIX=<namespace/repo> [SKIP_PUSH=true] $0 [all|svc1,svc2]
  IMAGE_PREFIX : 必須。例 myuser/project-alt など
  SKIP_PUSH    : true で push をスキップ (省略時 push 実施)
  all          : 全サービスをビルド
  svc1,svc2    : カンマ区切りで特定サービスのみビルド
EOF
  exit 1
}

check_deps() {
  for cmd in docker ctr git date; do
    if ! command -v "$cmd" &>/dev/null; then
      echo -e "${RED}✗ $cmd が必要です${NC}" >&2
      exit 1
    fi
  done
  echo -e "${GREEN}✓ 依存コマンド OK${NC}"
}

tag_for() {
  local svc="$1"
  printf '%s-%s-%s' "$svc" "$TIMESTAMP" "$GIT_SHA"
}

build_and_push() {
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

  # 画像名 & タグ組み立て
  local tag="$(tag_for "$svc")"
  local full_image="${IMAGE_PREFIX}:${tag}"
  local latest_image="${IMAGE_PREFIX}:${svc}-latest"

  # ビルド
  echo -e "${BLUE}▶ Building $svc → $full_image${NC}"
  pushd "$dir" >/dev/null
  docker build --pull -f "$(basename "$df_path")" -t "$full_image" .
  docker tag "$full_image" "$latest_image"
  popd >/dev/null

  # push
  if [[ "$SKIP_PUSH" != true ]]; then
    echo -e "${CYAN}↪ Pushing $full_image${NC}"
    docker push "$full_image" && docker push "$latest_image" || {
      echo -e "${RED}✗ push 失敗: $full_image${NC}" >&2
      exit 1
    }
    echo -e "${GREEN}✓ push 成功${NC}"
  else
    echo -e "${YELLOW}⚠ push スキップ (SKIP_PUSH=true)${NC}"
  fi

  # containerd import
  echo -e "${CYAN}↪ Import to containerd${NC}"
  docker save "$full_image" | sudo ctr -n k8s.io images import -
  echo -e "${GREEN}✓ 完了: $svc${NC}\n"
}

main() {
  # IMAGE_PREFIX 必須
  [[ -z "$IMAGE_PREFIX" ]] && { echo -e "${RED}IMAGE_PREFIX 必須${NC}"; usage; }
  [[ $# -eq 0 ]] && usage

  check_deps

  local target="$1"
  if [[ "$target" == all ]]; then
    for svc in "${!SERVICE_CONFIGS[@]}"; do build_and_push "$svc"; done
  else
    IFS=',' read -ra list <<<"$target"
    for svc in "${list[@]}"; do build_and_push "${svc// /}"; done
  fi

  echo -e "${GREEN}All services completed (${TIMESTAMP})${NC}"
}

main "$@"
