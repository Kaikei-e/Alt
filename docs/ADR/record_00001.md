# Docker Composeによるマイクロサービス基盤とクリーンアーキテクチャの採用

## ステータス

採択（Accepted）

## コンテキスト

2025年5月末、Altプロジェクトが開始された時点で、以下の要件と課題が存在していた：

1. **モジュール性の必要性**: RSS読み取り機能を核としつつ、将来的なAI機能拡張やマイクロサービス化を見据えた設計が必要
2. **開発環境の統一**: フロントエンド、バックエンド、データベースなど複数のコンポーネントを効率的に管理できる開発環境
3. **保守性**: テスト駆動開発（TDD）が可能で、コードの保守性と拡張性が高いアーキテクチャパターン
4. **迅速な立ち上げ**: プロトタイプから本番環境まで一貫したワークフローでスムーズに移行できる基盤

従来のモノリシックなアプローチでは、将来的なスケーリングや機能分離が困難になることが予想された。また、開発者間での環境差異を最小限に抑え、「動作する」環境を素早く構築する必要があった。

## 決定

プロジェクトの基盤として、以下の技術スタックとアーキテクチャパターンを採用した：

### 1. Docker Composeによるオーケストレーション

**採用技術:**
- Docker Compose（マルチコンテナ管理）
- Nginx（リバースプロキシ、静的ファイル配信）
- Next.js 14（フロントエンド）
- Go 1.23（バックエンドAPI）
- PostgreSQL 16（データベース）

**主要な構成:**
```yaml
services:
  alt-frontend:   # Next.js アプリケーション
  alt-backend:    # Go API サーバー
  db:             # PostgreSQL データベース
  nginx:          # リバースプロキシ
  migration:      # データベースマイグレーション
```

**理由:**
- Kubernetesよりも軽量で、ローカル開発に適している
- サービス間の依存関係（depends_on、health checks）を明示的に管理可能
- 環境変数の一元管理（.env.template）
- 本番環境への移行パス（後のKubernetes化を見据えた設計）

### 2. Go バックエンドでのClean Architecture実装

**アーキテクチャレイヤー:**
```
REST Handler (プレゼンテーション層)
    ↓
Usecase (ビジネスロジック層)
    ↓
Port (インターフェース定義)
    ↓
Gateway (外部システム接続)
    ↓
Driver (データベース、外部API)
```

**主要な実装パターン:**
- **依存性の逆転**: UsecaseはPortインターフェースに依存し、具体的なGateway実装には依存しない
- **テスタビリティ**: 各レイヤーが独立しており、モック化が容易
- **明確な責任分離**: ビジネスロジック（Usecase）とインフラストラクチャ（Gateway/Driver）の分離

**例: FeedFetchingコンポーネント**
- `FetchSingleFeedPort` (インターフェース)
- `FetchSingleFeedUsecase` (ビジネスロジック)
- `FetchSingleFeedGateway` (実装)
- `FetchFeedDriver` (データベースアクセス)

### 3. GoMockによるテスト駆動開発の導入

**採用ツール:**
- GoMock: インターフェースからモック自動生成
- Makefile: `make generate-mocks` でポート定義からモック生成

**メリット:**
- インターフェース駆動開発を促進
- 各レイヤーを独立してテスト可能
- テストファーストなワークフローを確立

### 4. データベースマイグレーション管理

**実装:**
- 専用migrationサービスをDocker Compose内で定義
- SQLベースのマイグレーションスクリプト（`001_initial_feeds.sql`など）
- アプリケーション起動前にマイグレーションを自動実行

## 結果・影響

### 利点

1. **開発環境の一貫性**
   - `make up` 一つでフロントエンド、バックエンド、データベース、プロキシが全て起動
   - チーム全員が同一の環境で開発可能
   - 新規参加者のオンボーディング時間を大幅短縮

2. **保守性の向上**
   - Clean Architectureにより、ビジネスロジックとインフラ層が明確に分離
   - テストカバレッジの向上（各レイヤーを独立してテスト）
   - 技術スタック変更時の影響範囲を局所化

3. **スケーラビリティへの布石**
   - サービス単位での独立性が高く、後のマイクロサービス化が容易
   - Docker Composeの設定をKubernetesマニフェストに変換しやすい

4. **迅速なイテレーション**
   - ホットリロード対応（Next.js、Go Air）
   - データベーススキーマ変更もマイグレーションで管理
   - フィードバックループの高速化

### 注意点・トレードオフ

1. **初期セットアップコスト**
   - Clean Architectureのレイヤー構造により、初期の実装コストが増加
   - 小規模な機能でも複数ファイルの作成が必要

2. **Docker Composeの制約**
   - 本番環境でのオーケストレーションには限界（後にKubernetes導入が必要）
   - ローカルリソース消費が大きい

3. **学習曲線**
   - Clean Architectureパターンの理解が必要
   - GoMockの使い方、インターフェース設計スキルの習得

4. **オーバーエンジニアリングのリスク**
   - シンプルなCRUD操作でも複数レイヤーを経由
   - 初期段階では過剰に見える設計だが、長期的には価値を発揮

## 参考コミット

- `2761c548` - Initial commit（プロジェクト開始）
- `d2db64ac` - Add Docker Compose setup with Nginx, Next.js frontend, and Go backend
- `516f3cd0` - Add Go module and initial application setup with a simple HTTP server
- `0907c013` - Initialize Next.js application with essential configuration files
- `dbd64ccd` - Add generate-mocks target to Makefile for automated GoMock mock generation
- `38b03953` - Add FetchSingleFeed use case and gateway implementation
- `1ca875a5` - Enhance Docker Compose configuration by adding health checks for services
- `42163899` - Add initial database migration scripts for feeds and feed_links tables
- `3ad78ef8` - Refactor database connection to use context and implement GetSingleFeed method in AltDBRepository
