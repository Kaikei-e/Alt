# セキュリティ強化とKubernetes基盤の導入

## ステータス

採択（Accepted）

## コンテキスト

2025年7月上旬、Altプロジェクトはテスト基盤と可観測性を確立し、開発速度が加速していた。しかし、本番環境へのデプロイを見据えた際に、以下の重要な課題が明らかになった：

1. **セキュリティ脆弱性**: CSRF攻撃、DoS攻撃、XSS、インジェクション攻撃に対する防御がない
2. **スケーラビリティ**: Docker Composeは開発環境には適しているが、本番のオートスケーリングやロードバランシングには不向き
3. **ユーザー体験の分岐**: モバイルとデスクトップで異なるUI/UX要件があるが、コードが混在している
4. **入力検証の欠如**: ユーザー入力やフィード URLの検証が不十分で、セキュリティリスクが高い

特に、公開APIとして運用するためにはセキュリティ対策が必須であり、また、トラフィック増加に備えたKubernetesベースのインフラ構築が急務となっていた。

## 決定

セキュリティとスケーラビリティを両立させるため、以下の多層的な対策とインフラストラクチャを導入した：

### 1. CSRF保護の実装

**脅威:**
- クロスサイトリクエストフォージェリ（CSRF）により、ユーザーの意図しない操作が実行されるリスク

**対策:**
- **CSRFトークン**: 状態変更操作（POST、PUT、DELETE）にトークン検証を義務化
- **トークン生成**: 暗号学的に安全な乱数生成器（crypto/rand）
- **有効期限**: 1時間のトークン有効期限設定
- **セッション連携**: ユーザーセッションごとにトークンを発行・検証

**実装:**
```go
type CSRFProtection struct {
    SecretKey []byte
    Expiration time.Duration // 1 hour
}

func (c *CSRFProtection) GenerateToken(userID string) (string, error)
func (c *CSRFProtection) ValidateToken(token string, userID string) error
```

**テストカバレッジ:**
- トークン生成と検証のユニットテスト
- トークン有効期限のテスト
- 無効なトークンの拒否テスト

### 2. DoS対策とレート制限

**脅威:**
- サービス拒否攻撃により、正規ユーザーがサービスを利用できなくなるリスク

**対策:**
- **IPベースのレート制限**: 同一IPからのリクエスト数を制限
- **サーキットブレーカーパターン**: 過負荷時に一時的にリクエストを拒否
- **設定可能なバースト保護**: 瞬間的な高トラフィックを許容しつつ、持続的な攻撃を防御

**実装:**
```go
type RateLimiter struct {
    Limit      int           // 秒あたりのリクエスト数
    Burst      int           // 瞬間的な許容数
    Expiration time.Duration // IPアドレスのキャッシュ有効期限
}
```

**統合テスト:**
- レート制限超過時のHTTP 429応答
- 正常範囲内のリクエスト処理
- サーキットブレーカーの動作確認

### 3. デスクトップアプリケーション層の分離

**課題:**
- モバイルとデスクトップで異なるUI要件（仮想化スクロール、マルチカラムレイアウト）
- コードの可読性と保守性の低下

**解決策:**
- **デスクトップ専用コンポーネント**: `desktop/feeds/` ディレクトリ配下に分離
- **仮想化スクロール**: 大量のフィード表示でもパフォーマンスを維持
- **既読ステータス永続化**: ローカルストレージとAPI連携
- **お気に入り機能**: デスクトップユーザー向け拡張機能

**アーキテクチャ:**
```
src/
├── components/
│   ├── mobile/        # モバイル専用
│   └── desktop/       # デスクトップ専用
├── hooks/
│   ├── useMobileFeeds
│   └── useDesktopFeeds
```

### 4. 入力サニタイゼーションとバリデーション

**脅威:**
- XSS攻撃: 悪意のあるスクリプトの埋め込み
- SQLインジェクション: データベースへの不正アクセス
- パストラバーサル: ファイルシステムへの不正アクセス

**対策:**
**フロントエンド:**
- **sanitize-html**: HTMLコンテンツの浄化
- **許可リストベース**: 許可されたHTMLタグと属性のみ受け入れ

**バックエンド（Go）:**
- **Goのescapeパッケージ**: HTMLエスケープ
- **パラメータ化クエリ**: SQLインジェクション対策

**Python（Tag-generator、Pre-processor）:**
- **Pydantic**: 入力バリデーションとデータモデル定義
- **型安全性**: ランタイムでの型チェック

**実装例:**
```python
from pydantic import BaseModel, HttpUrl

class FeedInput(BaseModel):
    url: HttpUrl  # URLの形式を自動検証
    title: str
    description: Optional[str]
```

### 5. Helmチャートによる本番デプロイメント

**課題:**
- Docker Composeは本番環境のオーケストレーションに不向き
- スケーリング、ロードバランシング、ヘルスチェックの自動化が必要

**解決策:**
- **Kubernetes + Helm**: 宣言的なインフラ管理
- **Kustomize**: 環境別設定（dev、staging、prod）
- **K3s**: 軽量Kubernetesディストリビューション（開発環境）

**Helm Chart構成:**
```
helm/
├── alt-backend/
│   ├── Chart.yaml
│   ├── values.yaml
│   └── templates/
├── alt-frontend/
├── search-indexer/
├── tag-generator/
└── pre-processor/
```

**主要機能:**
- **オートスケーリング**: HPA（Horizontal Pod Autoscaler）によるCPU/メモリベースのスケーリング
- **ローリングアップデート**: ゼロダウンタイムデプロイ
- **ヘルスチェック**: liveness/readiness probes
- **シークレット管理**: Kubernetesシークレットによる機密情報管理

### 6. PostgreSQL SSL/TLS with Cert-Manager

**セキュリティ要件:**
- データベース接続の暗号化
- 証明書の自動更新

**実装:**
- **cert-manager**: Kubernetes上での証明書ライフサイクル管理
- **Let's Encryptまたは自己署名証明書**: SSL/TLS証明書の自動発行
- **PostgreSQLのSSLモード**: require/verify-ca/verify-full

## 結果・影響

### 利点

1. **セキュリティ態勢の大幅強化**
   - CSRF、DoS、XSS、インジェクション攻撃に対する多層防御
   - 本番環境での安全なAPI公開が可能
   - コンプライアンス要件への適合（データ暗号化、入力検証）

2. **本番環境への道筋が明確化**
   - Kubernetesベースのインフラで、スケーラビリティとレジリエンスを確保
   - Helmチャートにより、デプロイメントの再現性と可搬性が向上
   - 環境別設定（Kustomize）で、dev/staging/prodの一貫した管理

3. **ユーザー体験の向上**
   - デスクトップとモバイルの最適化により、各プラットフォームで最高の体験を提供
   - 仮想化スクロールでパフォーマンス改善
   - お気に入り機能などの拡張機能でエンゲージメント向上

4. **開発速度とコード品質の改善**
   - Pydanticによる型安全性で、ランタイムエラーを削減
   - 包括的なセキュリティテストで、脆弱性を早期発見
   - コンポーネント分離で、並行開発が容易化

### 注意点・トレードオフ

1. **システム複雑性の増加**
   - Kubernetesの学習曲線が急峻
   - Helmチャートのメンテナンスコスト
   - cert-managerなど、追加コンポーネントの運用負荷

2. **リソースオーバーヘッド**
   - Kubernetesはコントロールプレーンでリソースを消費
   - レート制限、CSRF検証などのミドルウェアによるレイテンシ増加（微小）

3. **開発ワークフローの変化**
   - ローカル開発がDocker Compose、本番がKubernetesと二重管理
   - Helmチャートの変更時は複数環境でテストが必要

4. **コスト増加**
   - Kubernetesクラスターの運用コスト（クラウドの場合）
   - cert-managerとLet's Encryptは無料だが、証明書管理の運用工数

## 参考コミット

- `51cb1ece` - Implement CSRF token verification system
- `9700b502` - Add CSRF test coverage with expiration tests
- `f3277bef` - Implement DoS protection rate limiting with circuit breaker
- `799efc0d` - Add DoS integration tests
- `eef1575a` - Create desktop feeds layout with multi-column design
- `a04209fc` - Implement desktop feeds functionality with virtualization
- `0565e9ed` - Add virtualized scrolling for performance optimization
- `1ae5da4f` - Integrate sanitize-html for content sanitization
- `2393495a` - Implement Pydantic input sanitization in Python services
- `dbea7157` - Add Kubernetes manifests for all services
- `21825257` - Implement Helm deployment strategy
- `6eec4e7a` - Create Helm charts for microservices
- `bb1f1b4a` - Implement PostgreSQL SSL with cert-manager
- `98c75ad9` - Configure database SSL connections
- `11975042` - Add cert-manager for certificate lifecycle management
