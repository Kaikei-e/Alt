# AI処理パイプラインと検索システムの導入

## ステータス

採択（Accepted）

## コンテキスト

Docker Compose基盤が確立された2025年6月中旬、AltプロジェクトはRSSフィード収集機能を持つシンプルな読み取りアプリケーションとして稼働していた。しかし、以下のような課題と新たな要求が顕在化していた：

1. **コンテンツの自動処理ニーズ**: 取得した記事を自動的に要約・分析し、ユーザーにとって価値のある情報を抽出する必要性
2. **検索機能の欠如**: 数千件規模のフィード記事から目的の情報を探すための効率的な検索手段がない
3. **コンテンツ分類の必要性**: 記事を自動的にタグ付けし、トピック別に整理する仕組みがない
4. **スケーラビリティ**: フィード数の増加に伴い、記事処理と検索のパフォーマンスが懸念される

当時のシステムはPostgreSQLのLIKE検索に依存しており、全文検索や意味的な検索には対応できていなかった。また、記事の要約や分類は手作業に頼らざるを得ず、AIの力を活用した自動化が求められていた。

## 決定

コンテンツ処理と検索の両面でシステムを強化するため、以下の新規サービスとアーキテクチャを導入した：

### 1. Pre-processorサービス（Python/Ollama）

**役割:**
- RSSフィードから取得した記事の自動要約
- 記事内容の前処理（HTML除去、正規化）
- LLMを活用した記事分析

**技術スタック:**
- **Python 3.12**: 柔軟なテキスト処理と豊富なMLライブラリ
- **Ollama**: ローカルで動作するLLMランタイム（プライバシー保護と低レイテンシ）
- **pgx**: PostgreSQLとの効率的な通信

**アーキテクチャ:**
```
alt-backend (記事取得)
    ↓
pre-processor (要約・分析)
    ↓
PostgreSQL (article_summaries テーブル)
```

**採用理由:**
- **プライバシー第一**: ユーザーの記事データを外部APIに送信せず、ローカルLLMで処理
- **コスト削減**: OpenAI等のAPIコストが不要
- **柔軟性**: Ollamaは複数のモデル（Llama、Mistral等）をサポートし、用途に応じて切り替え可能

### 2. Meilisearch 1.15.2による高速全文検索

**役割:**
- 記事タイトル、本文、タグの全文検索
- タイポ許容検索（typo tolerance）
- ファセット検索（タグ別フィルタリング）

**技術選択:**
- **Meilisearch**: Rust製の高速検索エンジン
- **特徴**: ミリ秒単位の検索レスポンス、シンプルなAPI、タイポ補正

**アーキテクチャ:**
```
PostgreSQL (記事データ)
    ↓
search-indexer (インデックス同期)
    ↓
Meilisearch (検索エンジン)
    ↑
alt-backend (検索クエリ)
```

**採用理由:**
- **ElasticsearchやAlgoliaとの比較**: 軽量で設定が簡単、ローカル環境での運用が容易
- **開発者体験**: RESTful APIで直感的、ドキュメントが充実
- **パフォーマンス**: 数十万件の記事でも高速検索を実現

### 3. Tag-generatorサービス（Python）

**役割:**
- 記事内容からキーワード・トピックを自動抽出
- 事前定義されたタグ体系に基づく分類
- Meilisearchのファセット検索用メタデータ生成

**処理フロー:**
```
1. 記事本文を受け取る
2. TF-IDFまたはキーワード抽出アルゴリズム実行
3. タグをPostgreSQLとMeilisearchに保存
4. 検索時のフィルタリングに活用
```

**採用理由:**
- **スケーラビリティ**: 記事取得とタグ生成を分離することで、並列処理が可能
- **段階的な改善**: 初期はルールベース、後に機械学習モデルへ進化可能
- **検索精度向上**: タグベースのフィルタリングでユーザー体験が向上

### 4. Search-indexerサービス（Go）

**役割:**
- PostgreSQLとMeilisearchの同期管理
- 新規記事の自動インデックス登録
- インデックス更新の冪等性保証

**実装:**
- カーソルベースのページネーションで効率的にデータ取得
- Meilisearch APIへのバッチ登録
- エラーハンドリングとリトライ機構

## 結果・影響

### 利点

1. **AI駆動のコンテンツキュレーション**
   - 記事要約により、ユーザーは短時間で内容を把握可能
   - 長文記事の読む/読まないの判断が迅速化
   - プライバシーを保ちながらLLMの恩恵を享受

2. **高速かつ柔軟な検索体験**
   - ミリ秒単位の検索レスポンスで、ユーザーストレスを軽減
   - タイポ補正により、検索失敗率が大幅に減少
   - タグフィルタリングで、トピック別の絞り込みが容易

3. **スケーラブルなアーキテクチャ**
   - 各サービスが独立しており、個別にスケール可能
   - Pre-processor、Tag-generator、Search-indexerは並列実行可能
   - データベースとインデックスの分離により、読み取り負荷を分散

4. **開発速度の向上**
   - Meilisearchのシンプルなインターフェースで、検索機能の実装が迅速化
   - Tag-generatorのルールベース実装で、初期バージョンを素早くリリース
   - 後から機械学習モデルへの切り替えが容易

### 注意点・トレードオフ

1. **システム複雑性の増加**
   - 3つの新規サービス（Pre-processor、Tag-generator、Search-indexer）追加により、運用負荷が増加
   - サービス間の依存関係管理が必要

2. **データ同期の課題**
   - PostgreSQLとMeilisearchの二重管理により、データ整合性の維持が必要
   - インデックス再構築時のダウンタイム対策

3. **リソース消費**
   - Ollamaは GPU/CPUリソースを消費（特に大規模モデル使用時）
   - Meilisearchはメモリ使用量が高い（インデックスサイズに依存）

4. **初期学習コスト**
   - Meilisearchの設定（フィルタブル属性、ソート属性）の理解が必要
   - Ollamaのモデル選択とプロンプトエンジニアリングのノウハウ蓄積

## 参考コミット

- `3fbfc9b7` - tag-generator and search-indexer services added to Docker Compose
- `333c5e91` - Initialize search-indexer service with Go module
- `27c070c7` - Integrate Meilisearch for full-text search
- `d0e3c51f` - Implement article fetching in pre-processor
- `1c6b78a2` - Add article_summaries table migration
- `851574fb` - Use pgxpool for efficient database connections
- `0dc91c70` - Add article processing logic to tag-generator
- `b701146d` - Implement cursor-based pagination for article fetching
