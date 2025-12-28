# マルチテナント認証とOAuth2統合

## ステータス

採択（Accepted）

## コンテキスト

2025年8月中旬、Altプロジェクトはセキュアなネットワーク基盤を確立し、機能的にも成熟していた。しかし、ユーザー管理と外部サービス統合の観点から、以下の重要な課題が残されていた：

1. **認証システムの欠如**: ユーザー登録、ログイン、セッション管理の仕組みがない
2. **マルチソースコンテンツ取得**: RSSフィードのみならず、Inoreaderなど外部サービスからも記事を取得したい
3. **認証情報の管理**: 外部APIのアクセストークンを安全に管理し、自動更新する仕組みが必要
4. **マルチテナント対応**: 複数ユーザーが独立したデータ空間で利用できる必要がある

従来のシステムは単一ユーザー前提で設計されており、ユーザー識別や認証の概念が存在しなかった。また、Inoreaderとの統合には OAuth2 フローの実装が不可欠であり、トークンのリフレッシュ処理などの複雑性が課題となっていた。

## 決定

統一的な認証基盤とマルチテナント対応を実現するため、以下のサービスとアーキテクチャを導入した：

### 1. Ory Kratosによる統一ID管理

**Ory Kratosとは:**
- オープンソースのIdentity and User Management（IDaaS）
- Self-service login、registration、account recovery
- GDPR、HIPAA準拠のセキュリティ設計

**選定理由:**
- **Auth0、Cognitoとの比較**: セルフホスト可能、プライバシー第一、コスト削減
- **Keycloakとの比較**: 軽量、Kubernetes-native、APIファースト
- **カスタム実装との比較**: セキュリティベストプラクティスが組み込み済み

**アーキテクチャ:**
```
alt-frontend
    ↓ (ログイン/登録フロー)
auth-hub (Kratosラッパー)
    ↓
Ory Kratos (Identity Provider)
    ↓
PostgreSQL (kratos-db)
```

**実装機能:**
- **ユーザー登録**: メール/パスワードベース
- **ログイン**: セッション管理（Cookie）
- **アカウント回復**: パスワードリセット
- **セッション管理**: CSRFトークン統合
- **マルチファクタ認証**: TOTP対応（将来実装）

### 2. OAuth2によるInoreader連携

**Inoreaderとは:**
- 強力なRSSリーダーサービス
- APIによるフィード取得、既読管理、タグ付け機能

**OAuth2 フロー:**
```
1. ユーザーがInoreader連携を要求
   ↓
2. auth-hubが認可URLを生成
   ↓
3. ユーザーがInoreaderで認可
   ↓
4. InoreaderがCallback URLにコードを返す
   ↓
5. auth-hubがコードをアクセストークンに交換
   ↓
6. アクセストークンとリフレッシュトークンをDBに保存
```

**セキュリティ考慮:**
- **State parameter**: CSRF攻撃防止
- **PKCE（Proof Key for Code Exchange）**: 認可コード横取り攻撃防止
- **トークン暗号化**: データベース保存時に暗号化

### 3. Auth-token-manager（Deno）による自動トークン更新

**課題:**
- Inoreaderのアクセストークンは有効期限あり（通常1時間）
- リフレッシュトークンを使った自動更新が必要
- トークン更新失敗時の適切なエラーハンドリング

**解決策: Deno ベースのトークン管理サービス**

**なぜDeno？**
- **セキュリティ**: デフォルトでサンドボックス実行、権限の明示的な付与
- **TypeScript ネイティブ**: 型安全性が高い
- **軽量**: V8エンジンで高速起動

**実装:**
```typescript
interface TokenManager {
  refreshToken(userId: string): Promise<OAuthToken>;
  scheduleRefresh(userId: string, expiresIn: number): void;
  revokeToken(userId: string): Promise<void>;
}

class InoreaderTokenManager implements TokenManager {
  async refreshToken(userId: string): Promise<OAuthToken> {
    // 1. DBからリフレッシュトークンを取得
    // 2. InoreaderのトークンエンドポイントにPOST
    // 3. 新しいアクセストークンとリフレッシュトークンを取得
    // 4. DBに保存
  }

  scheduleRefresh(userId: string, expiresIn: number): void {
    // 有効期限の5分前に自動リフレッシュをスケジュール
    setTimeout(() => this.refreshToken(userId), (expiresIn - 300) * 1000);
  }
}
```

**機能:**
- **自動リフレッシュ**: トークン有効期限の5分前に自動更新
- **エラーハンドリング**: リフレッシュ失敗時にユーザーへ通知
- **リトライ機構**: 一時的なネットワークエラー時にリトライ
- **ログ記録**: 全てのトークン操作を監査ログに記録

### 4. Auth-hub、Auth-token-managerサービスの導入

**Auth-hub（Go）:**
- **役割**: Kratosのラッパー、OAuth2フロー管理
- **API:**
  - `POST /auth/register`: ユーザー登録
  - `POST /auth/login`: ログイン
  - `GET /auth/oauth/inoreader`: Inoreader OAuth2開始
  - `GET /auth/oauth/callback`: OAuth2コールバック
  - `POST /auth/logout`: ログアウト

**Auth-token-manager（Deno）:**
- **役割**: トークンライフサイクル管理
- **スケジューラー**: 定期的なトークン更新
- **API:**
  - `POST /tokens/refresh`: 手動トークン更新
  - `POST /tokens/revoke`: トークン取り消し

### 5. Pre-processor-sidecarによるInoreader記事取得

**アーキテクチャ:**
```
alt-backend
    ↓ (ユーザーリクエスト)
pre-processor-sidecar
    ↓ (OAuth2トークン使用)
Inoreader API
    ↓
記事データ
    ↓
PostgreSQL (articles テーブル)
```

**pre-processor-sidecar:**
- **役割**: Inoreader APIから記事を取得し、標準化
- **技術スタック**: Go（並行処理、HTTPクライアント）
- **機能:**
  - Inoreaderのストリーム取得（unread items）
  - 記事の正規化（RSSフォーマットと統一）
  - 重複排除（URL、タイトルベース）
  - 既読/未読状態の同期

## 結果・影響

### 利点

1. **統一された認証基盤**
   - Ory Kratosによるエンタープライズグレードのユーザー管理
   - セキュアなセッション管理（CSRF対策、HTTPOnly Cookie）
   - GDPR準拠のプライバシー保護

2. **マルチソースコンテンツ取得**
   - Inoreader統合により、ユーザーの既存フィードを活用
   - OAuth2で安全な認可フロー
   - トークン自動更新でユーザー体験向上

3. **スケーラビリティとマルチテナント**
   - ユーザーごとに独立したデータ空間
   - 数千〜数万ユーザーに対応可能
   - Kratosのスケーラビリティを活用

4. **セキュリティの強化**
   - トークン暗号化によるデータ保護
   - Denoのサンドボックスによる安全なトークン管理
   - 監査ログによる全操作の追跡

### 注意点・トレードオフ

1. **システム複雑性の増加**
   - 3つの新規サービス（auth-hub、auth-token-manager、pre-processor-sidecar）
   - OAuth2フローの理解とデバッグ
   - Kratosの設定と運用の学習コスト

2. **依存性の増加**
   - Inoreader APIの可用性に依存
   - Kratosのアップデートへの追従
   - Denoランタイムのメンテナンス

3. **運用負荷**
   - トークン更新の監視
   - OAuth2エラーのトラブルシューティング
   - ユーザーからの認証関連サポート

4. **パフォーマンス**
   - 認証チェックによるレイテンシ増加
   - トークンリフレッシュ時のAPI呼び出し
   - Kratos DBへのクエリ負荷

## 参考コミット

- `887f4f3a` - Add OAuth2 configuration for Inoreader integration
- `b40c1f28` - Implement login and registration flows with Kratos
- `e0f5e809` - Introduce auth-token-manager service (Deno)
- `cc4ca0d4` - Initialize auth-service (auth-hub)
- `e5a2a3eb` - Add auth database schema for users and sessions
- `319ff675` - Add Dockerfile for auth-hub
- `a56c472c` - Integrate auth-hub with Kratos
- `3d8f2b1a` - Implement OAuth2 callback handler
- `9a4e5c7b` - Add token refresh scheduler in auth-token-manager
- `e7f1d3a2` - Create pre-processor-sidecar for Inoreader article fetching
- `c2e8a9f4` - Implement article deduplication in pre-processor-sidecar
- `b5d7c1e3` - Add read/unread status sync with Inoreader
