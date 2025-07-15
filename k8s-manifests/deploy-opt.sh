#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

# ───────────────────────────────────────────────────────────
# deploy-opt.sh — apply → image tag patch → (optional) rollout restart → status watch
# Pattern‑B 対応：単一リポジトリ IMAGE_PREFIX:<service>-<TAG_BASE>
#   TAG_BASE は build-images.sh が出力時に環境変数で渡す想定
#
# 使い方例
#   TAG_BASE=20250715220100-nogit IMAGE_PREFIX=example/project \
#       ./deploy-opt.sh production -r
#   （TAG_BASE を渡さなければ従来どおり apply だけ実施）
#
# 引数
#   <env>               development | staging | production (必須)
#   -d / --dry-run      server-side dry‑run (適用せず YAML 出力)
#   -r / --restart      apply 後に rollout restart
#   -n / --namespace    デフォルト alt-<env> を上書き
#   -h / --help         ヘルプ
# 環境変数
#   IMAGE_PREFIX        プライベートレポ (例 example/project) ※必須
#   TAG_BASE            <timestamp>-<sha> 形式 (省略時 set image スキップ)
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
command -v kubectl &>/dev/null || { echo -e "${RED}kubectl missing${NC}"; exit 1; }

# サービスリスト（Deployment 名 = コンテナ名前提）
SERVICES=(
  alt-backend alt-frontend pre-processor news-creator search-indexer tag-generator \
  migrate clickhouse db nginx nginx-external rask-log-aggregator meilisearch
)

apply_overlay(){
  echo -e "${BLUE}▶ Applying overlay ($ENV)${NC}"
  if $DRY; then
    kubectl apply -k "k8s/overlays/$ENV" --dry-run=server -o yaml | less -R
  else
    kubectl apply -k "k8s/overlays/$ENV"
    echo -e "${GREEN}✓ apply complete${NC}"
  fi
}

patch_images(){
  $DRY && return 0
  [[ -z "$TAG_BASE" ]] && { echo -e "${YELLOW}TAG_BASE 未指定 → image patch skip${NC}"; return; }

  echo -e "${CYAN}▶ set image to ${IMAGE_PREFIX}:<svc>-${TAG_BASE}${NC}"
  for svc in "${SERVICES[@]}"; do
    local img="${IMAGE_PREFIX}:${svc}-${TAG_BASE}"
    # Deployment が存在する場合のみ更新
    if kubectl -n "$TARGET_NS" get deployment "$svc" &>/dev/null; then
      echo -e "${CYAN}  ↪ $svc → $img${NC}"
      kubectl -n "$TARGET_NS" set image deployment/"$svc" "$svc"="$img" --record=true || {
        echo -e "${YELLOW}⚠ set image failed on $svc (ignored)${NC}";
      }
    fi
  done
}

rollout_restart_all(){
  $DRY && return 0
  $DO_RESTART || return 0
  echo -e "${CYAN}▶ rollout restart in $TARGET_NS${NC}"
  for kind in deployment statefulset daemonset; do
    mapfile -t res < <(kubectl -n "$TARGET_NS" get "$kind" -o name 2>/dev/null || true)
    for r in "${res[@]}"; do
      [[ -z "$r" ]] && continue
      echo -e "${CYAN}  ↻ restarting $r${NC}"
      kubectl -n "$TARGET_NS" rollout restart "$r" || echo -e "${YELLOW}⚠ restart failed on $r (ignored)${NC}"
    done
  done
}

wait_rollout(){
  $DRY && { echo -e "${YELLOW}dry-run: skip rollout status${NC}"; return; }
  echo -e "${CYAN}▶ waiting rollout status${NC}"
  for kind in deployment statefulset daemonset; do
    for r in $(kubectl -n "$TARGET_NS" get "$kind" -o name); do
      echo -e "${CYAN}  ↪ $r${NC}"
      kubectl -n "$TARGET_NS" rollout status "$r" --timeout=180s || {
        echo -e "${RED}✗ rollout failed: $r${NC}"; exit 1; }
    done
  done
  echo -e "${GREEN}✓ all rollouts complete${NC}"
}

main(){
  local start=$(date +%s)
  apply_overlay
  patch_images
  rollout_restart_all
  wait_rollout
  echo -e "${GREEN}Done in $(( $(date +%s) - start ))s${NC}"
}

main "$@"
