# 包括的テストフレームワークと可観測性基盤の確立

## ステータス

採択（Accepted）

## コンテキスト

2025年6月下旬、Altプロジェクトはマイクロサービス基盤とAI処理パイプラインを導入し、機能的には大きく進化していた。しかし、システムの複雑性が増すにつれて、以下の課題が顕在化していた：

1. **品質保証の欠如**: 自動テストがなく、リグレッションのリスクが高い
2. **デバッグの困難性**: 複数のサービスにまたがるログを追跡するのが困難
3. **デザイン統一性の不足**: フロントエンドのUIがアドホックに実装され、一貫性がない
4. **運用可視性の欠如**: システム全体のヘルスやパフォーマンスを把握する手段がない

特に、手動テストのみに依存していたため、バグの早期発見が困難であり、デプロイ後に問題が発覚するケースが増えていた。また、サービスが増えるにつれて、ログの散在が深刻化し、問題の原因特定に時間がかかっていた。

## 決定

システムの信頼性と保守性を確保するため、以下の包括的な品質基盤と可観測性スタックを導入した：

### 1. 包括的テストフレームワークの導入

#### フロントエンド（Next.js）
**Vitest**による単体/コンポーネントテスト
- **役割**: React コンポーネント、ユーティリティ関数の単体テスト
- **特徴**: 高速な実行、ESMネイティブサポート、Jestの代替
- **カバレッジ**: コンポーネントのレンダリング、イベントハンドリング、状態管理

**Playwright**によるE2Eテスト
- **役割**: ユーザーシナリオの自動テスト
- **特徴**: クロスブラウザ対応、スクリーンショット比較、ネットワークモック
- **カバレッジ**: フィード一覧表示、無限スクロール、記事の既読/未読管理

**GitHub Actions統合**
- プルリクエスト時に自動テスト実行
- テスト失敗時にマージをブロック

#### バックエンド（Go）
**標準のtestingパッケージ**
- **go test ./...** でサービス全体をテスト
- **-race フラグ**による競合検出
- **-cover フラグ**によるカバレッジ測定

**GoMockの活用**
- インターフェースのモック自動生成
- 各レイヤーを独立してテスト

**GitHub Actions統合**
- コミットごとに自動テスト実行
- Gosecによる静的解析

#### Python（Tag-generator、Pre-processor）
**pytest**
- **役割**: Python サービスの単体テスト
- **特徴**: フィクスチャ、パラメータ化テスト
- **カバレッジ**: タグ抽出ロジック、要約生成

### 2. Rask Log Aggregation Stack（Rust + ClickHouse）

**アーキテクチャ:**
```
各サービス（JSON logs）
    ↓
Rask Log Forwarder (Rust)
    ↓
Rask Log Aggregator (Rust)
    ↓
ClickHouse 25.6（カラム型データベース）
```

**Rask Log Forwarder**
- **役割**: Dockerコンテナのログをリアルタイムで収集
- **技術**: Rust（高性能、低レイテンシ）
- **機能**: ログの構造化、メタデータ付与（サービス名、タイムスタンプ）

**Rask Log Aggregator**
- **役割**: 複数のForwarderからログを集約
- **技術**: Rust（並行処理、高スループット）
- **機能**: ログの正規化、バッチ処理、ClickHouseへの効率的な書き込み

**ClickHouse**
- **役割**: 大量ログの高速検索・分析
- **特徴**: カラム型ストレージ、圧縮率が高い、SQLクエリ対応
- **ユースケース**: エラー率の時系列分析、サービス別のレイテンシ追跡

**採用理由:**
- **パフォーマンス**: Rust製でオーバーヘッドが最小限、ClickHouseは数十億レコードでも高速クエリ
- **コスト**: ELKスタックと比較してリソース消費が少ない
- **スケーラビリティ**: 水平スケールが容易

### 3. Vaporwaveデザインシステムとテーマ管理

**Alt Vaporwave Design System**
- **コンセプト**: グラスモーフィズムとレトロフューチャーの融合
- **特徴**: 半透明背景、ネオングロー、グラデーション
- **CSS変数**: テーマカラーを一元管理

**Theme Management System**
- **ライト/ダークモード**: ユーザー設定に基づいた自動切り替え
- **Theme Toggle**: ワンクリックで切り替え可能
- **一貫性**: デスクトップとモバイルで統一されたデザイン

**実装:**
- Chakra UIをベースにカスタムテーマを構築
- CSS変数でテーマカラーを定義
- LocalStorageでユーザー設定を永続化

### 4. UnifiedLoggerによる構造化ログ標準化

**課題:**
- サービスごとにログフォーマットが異なる
- ログレベルの不統一
- コンテキスト情報の欠如

**解決策:**
```go
type UnifiedLogger struct {
    ServiceName string
    Level       string
    Timestamp   time.Time
    Message     string
    Context     map[string]interface{}
}
```

**標準化ルール:**
- **JSON出力**: 構造化されたログ（機械可読）
- **コンテキスト情報**: リクエストID、ユーザーID、トレースID
- **ログレベル**: DEBUG、INFO、WARN、ERROR、FATALの5段階
- **Rask互換性**: Rask Log Forwarderが自動的に解析可能な形式

## 結果・影響

### 利点

1. **品質の大幅向上**
   - 自動テストによりリグレッションを早期検出
   - E2Eテストでユーザー体験を保証
   - CI/CDパイプラインで品質ゲートを自動化

2. **デバッグ効率の劇的改善**
   - ClickHouseで数秒でエラーログを検索
   - サービス間の依存関係をログから追跡
   - 構造化ログにより、機械的な分析が可能

3. **一貫したユーザー体験**
   - デザインシステムによりUI/UXの統一性向上
   - テーマ切り替えでアクセシビリティ向上
   - ブランドイメージの確立

4. **運用の可視性向上**
   - リアルタイムでシステムヘルスを監視
   - エラー率、レイテンシ、スループットの時系列分析
   - 問題の兆候を早期に発見

### 注意点・トレードオフ

1. **初期投資コスト**
   - テストコードの記述に時間がかかる
   - Rask Log Aggregation Stackのセットアップと学習コスト
   - デザインシステムの構築に初期工数が必要

2. **リソース消費**
   - ClickHouseはメモリとディスク容量を消費
   - E2Eテストの実行時間が長い（CI/CD時間の増加）
   - Rask Forwarder/AggregatorのCPU/メモリオーバーヘッド

3. **メンテナンス負荷**
   - テストコードの保守（API変更時にテストも更新）
   - ログスキーマの変更時にRask Aggregatorの設定更新
   - デザインシステムの進化に伴うコンポーネント更新

4. **学習曲線**
   - Vitest、Playwright、pytestの使い方習得
   - ClickHouse SQLの学習
   - デザインシステムのガイドライン理解

## 参考コミット

- `f10bebc9` - Add Playwright testing configuration and initial tests for mobile feeds
- `f1747c9f` - Integrate Vitest for component testing and add initial tests for FeedCard component
- `83581e11` - Add GitHub Actions workflow for Go backend testing
- `f7bee259` - Introduce Rask Log Forwarder service
- `96e83082` - Add Rask log aggregator with ClickHouse integration
- `cfe5c1af` - Add ClickHouse migration scripts
- `fd17ce04` - Implement Alt Vaporwave Design System
- `b71a5ada` - Add Theme Management System with light/dark mode
- `d41e16fe` - Add Theme Toggle component
- `cb02a503` - Implement UnifiedLogger for standardized logging
- `6c4fd9e6` - Refactor logging structure across services
- `ec98be02` - Add logging documentation and best practices
