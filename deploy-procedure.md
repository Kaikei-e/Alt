# Helm-based Deployment Procedure for Alt Services

## Prerequisites
- Helm 3.x installed
- kubectl configured with cluster access
- Charts directory exists with Helm charts

## ビルドステップ
```bash
export IMAGE_PREFIX=user-name/project-name
export TAG_BASE=$(date +%Y%m%d%H%M%S)-$(git rev-parse --short=HEAD 2>/dev/null || echo 'nogit')
./build-images.sh all
```

## Helm方式デプロイステップ

### 基本デプロイ (Helm Chart使用)
```bash
# move to k8s-manifests
cd ./k8s-manifests

# Deploy with Helm charts
IMAGE_PREFIX=$IMAGE_PREFIX TAG_BASE=$TAG_BASE ./deploy-opt.sh production -r
```

### 個別Chart デプロイ
```bash
# 個別サービスのデプロイ例
helm upgrade --install alt-backend ../charts/alt-backend \
  -n alt-apps \
  -f ../charts/alt-backend/values-production.yaml \
  --set image.tag="$TAG_BASE" \
  --set image.repository="$IMAGE_PREFIX/alt-backend"

# GPU対応サービス（news-creator）
helm upgrade --install news-creator ../charts/news-creator \
  -n alt-apps \
  -f ../charts/news-creator/values-production.yaml \
  --set image.tag="$TAG_BASE" \
  --set image.repository="$IMAGE_PREFIX/news-creator"
```

### 環境別デプロイ
```bash
# Development環境
IMAGE_PREFIX=$IMAGE_PREFIX TAG_BASE=$TAG_BASE ./deploy-opt.sh development

# Staging環境
IMAGE_PREFIX=$IMAGE_PREFIX TAG_BASE=$TAG_BASE ./deploy-opt.sh staging

# Production環境（フルデプロイ）
IMAGE_PREFIX=$IMAGE_PREFIX TAG_BASE=$TAG_BASE ./deploy-opt.sh production -r
```

### デプロイ検証
```bash
# 全チャートの状態確認
helm list --all-namespaces

# 特定サービスの状態確認
kubectl get pods -n alt-apps
kubectl get pods -n alt-database
kubectl get pods -n alt-auth

# ロールアウト状況確認
kubectl rollout status deployment/alt-backend -n alt-apps
```

### ロールバック手順
```bash
# Helm履歴確認
helm history alt-backend -n alt-apps

# 前バージョンにロールバック
helm rollback alt-backend 1 -n alt-apps

# 全サービスロールバック（新スクリプト使用予定）
# ./rollback-helm.sh production --revision=1
```

## Chart依存関係とデプロイ順序

1. **Infrastructure Charts** (自動順序制御)
   - common-config, common-ssl, common-secrets
   - postgres, auth-postgres, kratos-postgres, clickhouse
   - meilisearch, nginx, nginx-external

2. **Application Charts**
   - alt-backend, auth-service, pre-processor
   - search-indexer, tag-generator, news-creator
   - rask-log-aggregator, alt-frontend

3. **Operational Charts**
   - migrate, backup, monitoring

## トラブルシューティング
```bash
# Chart linting
helm lint ../charts/alt-backend

# Template確認（dry-run）
IMAGE_PREFIX=$IMAGE_PREFIX TAG_BASE=$TAG_BASE ./deploy-opt.sh production -d

# Values確認
helm get values alt-backend -n alt-apps
```