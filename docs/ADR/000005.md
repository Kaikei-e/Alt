# ゼロダウンタイムデプロイとサービスメッシュの導入

## ステータス

採択（Accepted）

## コンテキスト

2025年7月下旬、Altプロジェクトはセキュリティ対策とKubernetes基盤を確立し、本番環境でのデプロイ準備が整いつつあった。しかし、実際の運用を想定すると、以下の重要な課題が残されていた：

1. **デプロイメントリスク**: アプリケーション更新時に一時的なサービス停止が発生し、ユーザー体験を損なう
2. **ロールバックの困難性**: デプロイ後に問題が発覚した場合、迅速にロールバックできる仕組みがない
3. **サービス間通信のセキュリティ**: マイクロサービス間の通信が平文で行われており、内部ネットワークへの侵入時にリスクが高い
4. **証明書管理の複雑性**: 各サービスでSSL/TLS証明書を個別に管理するのは運用負荷が高い
5. **外部アクセス制御**: RSSフィードの取得など、外部へのHTTPリクエストが制御されておらず、不正なドメインへのアクセスリスクがある
6. **開発ワークフローの複雑化**: ローカル開発とKubernetes環境の差異により、開発者の生産性が低下

## 決定

高可用性とセキュリティを確保しつつ、開発体験を向上させるため、以下の戦略的なインフラストラクチャ改善を実施した：

### 1. Blue-Greenデプロイメント戦略

**概念:**
- **Blue環境**: 現在稼働中の本番環境
- **Green環境**: 新バージョンをデプロイする環境
- **切り替え**: ヘルスチェック成功後、トラフィックをBlueからGreenへ瞬時に切り替え

**実装:**
```go
type BlueGreenDeploymentManager struct {
    CurrentEnvironment string // "blue" or "green"
    HealthChecker      HealthCheckService
    LoadBalancer       LoadBalancerService
}

func (m *BlueGreenDeploymentManager) Deploy(version string) error {
    // 1. Green環境に新バージョンをデプロイ
    // 2. ヘルスチェック実行
    // 3. トラフィック切り替え
    // 4. Blue環境を待機状態に（ロールバック用）
}
```

**主要機能:**
- **ヘルスチェック統合**: Green環境が正常稼働していることを確認してから切り替え
- **ロールバック機能**: 問題発生時、即座にBlue環境へ切り戻し
- **段階的ロールアウト**: 一部のトラフィック（10%）をGreenに流し、モニタリング後に全体切り替え

**利点:**
- ゼロダウンタイムデプロイ
- 高速なロールバック（数秒）
- 本番環境での実テスト可能

### 2. Linkerd mTLSによる自動サービス暗号化

**課題:**
- マイクロサービス間の通信が平文
- 各サービスで証明書を個別管理するのは運用負荷が高い

**解決策: Linkerd Service Mesh**
- **mTLS（mutual TLS）**: サービス間通信を自動的に暗号化
- **証明書の自動管理**: Linkerdが証明書の発行、更新、ローテーションを自動化
- **ゼロトラストネットワーク**: サービス間通信を常に検証

**アーキテクチャ:**
```
Service A → Linkerd Proxy → (encrypted) → Linkerd Proxy → Service B
```

**実装:**
- Linkerdをデータプレーンとしてインストール
- 各サービスにLinkerd Proxyを自動インジェクション
- mTLSをデフォルトで有効化

**効果:**
- **セキュリティ向上**: サービス間通信の盗聴・改ざんを防止
- **運用負荷削減**: 証明書管理の自動化により、手動SSL/TLS設定が不要
- **可観測性**: Linkerdのダッシュボードでサービスメッシュの状態を可視化

### 3. Skaffoldによるローカル/本番統一ワークフロー

**課題:**
- ローカル開発（Docker Compose）と本番（Kubernetes）の環境差異
- コンテナイメージのビルド、プッシュ、デプロイの手動実行が煩雑

**解決策: Skaffold**
- **自動ビルド**: コード変更を検知し、コンテナイメージを自動ビルド
- **自動デプロイ**: Kubernetesへ自動デプロイ
- **プロファイル**: dev、staging、prod環境ごとに設定を切り替え

**skaffold.yaml例:**
```yaml
apiVersion: skaffold/v4beta1
kind: Config
build:
  artifacts:
    - image: alt-backend
      context: ./alt-backend
    - image: alt-frontend
      context: ./alt-frontend
deploy:
  helm:
    releases:
      - name: alt-backend
        chartPath: ./helm/alt-backend
profiles:
  - name: dev
    activation:
      - command: dev
  - name: prod
    activation:
      - command: run
```

**開発ワークフロー:**
1. `skaffold dev`: ローカルKubernetes（K3s、Minikube）で自動リロード
2. `skaffold run -p staging`: ステージング環境へデプロイ
3. `skaffold run -p prod`: 本番環境へデプロイ

**効果:**
- **開発速度向上**: コード変更から動作確認まで数秒
- **環境差異の解消**: ローカルと本番で同じマニフェストを使用
- **デプロイメント自動化**: CI/CDパイプラインと統合

### 4. Envoy Forward Proxyによる外部アクセス制御

**課題:**
- RSSフィード取得時、任意のドメインへアクセス可能（SSRF攻撃のリスク）
- 外部へのリクエストがロギングされていない

**解決策: Envoy Forward Proxy**
- **ドメインホワイトリスト**: 許可されたドメインのみアクセス可能
- **リクエストロギング**: 全ての外部リクエストを記録
- **レート制限**: 外部APIへのリクエスト数を制限

**アーキテクチャ:**
```
alt-backend/pre-processor
    ↓ (HTTP Proxy経由)
Envoy Forward Proxy
    ↓ (許可されたドメインのみ)
外部RSS Feed
```

**設定例:**
```yaml
static_resources:
  listeners:
    - name: forward_proxy
      address:
        socket_address:
          address: 0.0.0.0
          port_value: 8080
      filter_chains:
        - filters:
            - name: envoy.http_connection_manager
              config:
                access_log:
                  - name: envoy.access_loggers.file
                    config:
                      path: /var/log/envoy/access.log
```

**効果:**
- **SSRF攻撃の防止**: ホワイトリスト外のドメインへアクセス不可
- **可視性向上**: 外部アクセスのロギングと監視
- **コンプライアンス**: データ流出防止（許可されたドメインのみ通信）

### 5. Sidecar Proxyアーキテクチャ

**進化:**
- Envoy Forward Proxyをサービスごとに独立したSidecarとして配置
- **CONNECT tunneling**: HTTPSプロキシとしても動作
- **Auto-learning domain management**: アクセスパターンから自動的にホワイトリストを学習

**実装:**
```yaml
apiVersion: v1
kind: Pod
metadata:
  name: pre-processor
spec:
  containers:
    - name: pre-processor
      image: pre-processor:latest
    - name: envoy-sidecar
      image: envoyproxy/envoy:latest
      volumeMounts:
        - name: envoy-config
          mountPath: /etc/envoy
```

**効果:**
- **統一されたプロキシ戦略**: 全サービスで共通のプロキシ設定
- **サービスごとの独立性**: 各サービスが独自のプロキシポリシーを持てる
- **可観測性**: プロキシレベルでのメトリクス収集

## 結果・影響

### 利点

1. **ゼロダウンタイムデプロイの実現**
   - Blue-Greenデプロイメントにより、ユーザーへの影響ゼロ
   - デプロイ中もサービスが継続稼働
   - ロールバック時間が数秒に短縮

2. **セキュリティの大幅強化**
   - Linkerd mTLSでサービス間通信を自動暗号化
   - Envoy Proxyで外部アクセスを制御
   - ゼロトラストネットワークの実現

3. **運用負荷の削減**
   - Linkerdによる証明書管理の自動化
   - Skaffoldによるデプロイメント自動化
   - 環境差異の解消

4. **開発体験の向上**
   - Skaffold devでローカル開発が高速化
   - Kubernetes環境での開発が容易
   - CI/CDパイプラインとのシームレスな統合

### 注意点・トレードオフ

1. **システム複雑性の増加**
   - Blue-Green環境の二重管理
   - Linkerd Service Meshの学習曲線
   - Envoy Proxyの設定管理

2. **リソースオーバーヘッド**
   - Blue-Green環境で2倍のリソースが必要（切り替え時）
   - Linkerd Proxyの各Podへのインジェクション（CPU/メモリ増加）
   - Envoy Sidecarのリソース消費

3. **デバッグの複雑化**
   - サービスメッシュ導入により、ネットワークスタックが複雑化
   - Linkerdのプロキシを介した通信のトラブルシューティング

4. **コスト増加**
   - Blue-Green環境の維持コスト
   - Linkerd、Skaffold、Envoyの学習と運用コスト

## 参考コミット

- `ece227e0` - Implement blue-green deployment manager
- `b8f0e1b8` - Integrate health check with deployment strategy
- `e78f30cc` - Add deployment enhancements and rollback functionality
- `9c950258` - Introduce Linkerd mTLS for service mesh
- `b66a3fa2` - Configure Linkerd mTLS across all services
- `25db9d7b` - Integrate Linkerd with Kubernetes manifests
- `e456676d` - Add skaffold configuration for dev/staging/prod profiles
- `850d1fa6` - Update skaffold with automatic rebuild
- `5d4f081a` - Enhance skaffold with Helm integration
- `9b57a90f` - Introduce Envoy proxy for forward proxying
- `fa8d9bc6` - Configure Envoy proxy with domain whitelisting
- `b5c5721d` - Enhance Envoy proxy with logging and monitoring
- `30b91762` - Implement sidecar proxy service architecture
- `a2a4c28c` - Add CONNECT tunneling support to Envoy sidecar
- `82a356eb` - Implement auto-learning domain management in sidecar
