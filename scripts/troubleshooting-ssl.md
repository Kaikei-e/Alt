# SSL/TLS Troubleshooting Guide

## 概要
Altプラットフォーム全体のSSL/TLS実装におけるトラブルシューティングガイドです。

## 自動診断

### 包括的セキュリティ検証
```bash
./scripts/test-security-comprehensive.sh
```

### データベースSSL接続テスト
```bash
./scripts/test-database-ssl.sh
```

## 一般的な問題と解決方法

### 1. SSL証明書エラー

#### 症状
- `x509: certificate signed by unknown authority`
- `dial tcp: i/o timeout`

#### 診断
```bash
# 証明書の存在確認
ls -la /Alt/k8s-manifests/scripts/ssl-certs/

# 証明書の有効性確認
openssl x509 -in /Alt/k8s-manifests/scripts/ssl-certs/server.crt -text -noout
```

#### 解決方法
```bash
# 証明書を再生成
cd /Alt/k8s-manifests/scripts
./generate-ssl-certs.sh
```

### 2. sslmode設定エラー

#### 症状
- `sslmode value "disable" unknown`
- データベース接続がSSLなしで確立される

#### 診断
```bash
# 残存するsslmode=disableの検索
find /Alt -name "*.go" -exec grep -l "sslmode=disable" {} \;
```

#### 解決方法
1. 該当ファイルで`sslmode=disable`を`sslmode=prefer`に変更
2. テストケースの更新

### 3. Kubernetes SSL ConfigMap未設定

#### 症状
- Pod起動時の環境変数エラー
- データベース接続時のSSL設定なし

#### 診断
```bash
# ConfigMapの存在確認
kubectl get configmap -n alt-database postgres-ssl-config
kubectl get configmap -n alt-jobs migration-ssl-config
```

#### 解決方法
```bash
# ConfigMapを再適用
kubectl apply -f k8s-manifests/k8s/base/core/database/postgres/ssl-configmap.yaml
kubectl apply -f k8s-manifests/k8s/base/jobs/migrations/ssl-configmap.yaml
```

### 4. Docker Compose SSL設定エラー

#### 症状
- ローカル開発環境でSSL接続失敗

#### 診断
```bash
# compose.yamlのSSL設定確認
grep -n "DB_SSL_MODE" compose.yaml
```

#### 解決方法
環境変数を`DB_SSL_MODE=prefer`に設定

## 設定ファイル確認

### 必須SSL設定

#### PostgreSQL設定 (k8s/base/core/database/postgres/ssl-configmap.yaml)
```yaml
ssl = on
ssl_ca_file = '/var/lib/postgresql/ssl/ca.crt'
ssl_cert_file = '/var/lib/postgresql/ssl/server.crt'
ssl_key_file = '/var/lib/postgresql/ssl/server.key'
```

#### アプリケーション設定
```yaml
DB_SSL_MODE: "prefer"  # 最低限: prefer, 推奨: require
DB_SSL_ROOT_CERT: "/app/ssl/ca.crt"
```

## デバッグコマンド

### SSL接続の確認
```bash
# PostgreSQLへの直接SSL接続テスト
PGPASSWORD=password psql \
  -h localhost -p 5432 -U username -d database \
  -c "SELECT ssl_is_used(), version();"
```

### アプリケーションログ確認
```bash
# Kubernetesログ
kubectl logs -n alt-backend deployment/alt-backend | grep -i ssl

# Docker Composeログ
docker-compose logs alt-backend | grep -i ssl
```

## 緊急時対応

### SSL設定の一時的無効化（開発用のみ）
```bash
# 緊急時のみ: sslmode=disableに一時的に変更
# 注意: 本番環境では絶対に使用しない
export DB_SSL_MODE=disable
```

### 設定ロールバック
```bash
# 前の設定に戻す
git checkout HEAD~1 -- k8s-manifests/k8s/base/core/database/postgres/
kubectl apply -f k8s-manifests/k8s/base/core/database/postgres/
```

## 検証手順

### SSL実装の完全性確認
1. `./scripts/test-security-comprehensive.sh`実行
2. 全チェック項目が✅であることを確認
3. 失敗項目がある場合は対応するセクションを参照

### パフォーマンス影響確認
```bash
# SSL有効化前後のベンチマーク
go test -bench=. ./...
```

## セキュリティベストプラクティス

### 推奨設定
- `sslmode=require` (本番環境)
- `sslmode=prefer` (開発環境)
- 証明書の定期更新 (90日毎)
- SSL設定の定期監査

### 避けるべき設定
- `sslmode=disable` (絶対禁止)
- `ssl=false` (セキュリティリスク)
- 自己署名証明書の本番利用

## 連絡先・エスカレーション

問題が解決しない場合:
1. 自動診断スクリプトの結果を保存
2. 関連ログを収集
3. 設定変更履歴を確認
4. セキュリティチームまたはDevOpsチームに連絡