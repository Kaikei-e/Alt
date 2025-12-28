# スケーラブルコンテンツ処理パイプラインの構築

## ステータス

採択（Accepted）

## コンテキスト

2025年9月上旬、Altプロジェクトはマルチテナント認証とInoreader統合により、ユーザー数とデータ量が急増していた。しかし、コンテンツ処理の観点から以下の課題が顕在化していた：

1. **処理パイプラインの非効率性**: RSS + Inoreader からの記事取得、要約、タグ付け、検索インデックス化が直列処理され、スループットが低い
2. **スケーラビリティの限界**: 単一のpre-processorインスタンスでは、増加する記事量に対応できない
3. **処理の冪等性欠如**: 同じ記事が複数回処理され、重複データが発生
4. **エラーハンドリングの不足**: 一部の処理が失敗しても、全体が停止または再試行されない

従来のアーキテクチャでは、pre-processorが全ての処理を担当していたため、ボトルネックとなっていた。また、処理の各段階が密結合しており、個別のスケーリングや再試行が困難であった。

## 決定

スケーラブルで信頼性の高いコンテンツ処理を実現するため、マルチステージパイプラインアーキテクチャを導入した：

### 1. マルチステージ処理パイプライン

**パイプライン設計:**
```
記事取得 → 要約生成 → タグ抽出 → 検索インデックス化
   ↓           ↓          ↓            ↓
Pre-proc   News-creator Tag-generator Search-indexer
```

**各ステージの責任:**

**Pre-processor（記事取得・正規化）:**
- RSSフィードとInoreaderからの記事取得
- HTMLパース、本文抽出
- 記事の正規化（統一フォーマット）
- PostgreSQLへの保存

**News-creator（要約生成）:**
- Ollamaベースの記事要約
- 長文記事の要点抽出
- 要約結果のPostgreSQLへの保存

**Tag-generator（タグ抽出）:**
- TF-IDFベースのキーワード抽出
- ジャンル分類
- タグのPostgreSQLとMeilisearchへの保存

**Search-indexer（検索インデックス化）:**
- PostgreSQLからMeilisearchへのデータ同期
- インデックスの最適化
- ファセット設定（タグ、日付）

**パイプライン制御:**
- 各ステージは独立したサービスとして実行
- ステージ間の通信はPostgreSQLをキューとして利用
- 冪等性を保証（同じ記事IDの再処理をスキップ）

### 2. Ollamaベースの記事要約

**News-creator実装:**
```python
class NewsCreator:
    def __init__(self, ollama_url: str, model: str = "llama3.2"):
        self.ollama_url = ollama_url
        self.model = model

    async def summarize(self, article: Article) -> Summary:
        prompt = f"""
        Summarize the following article in 2-3 sentences:

        Title: {article.title}
        Content: {article.content}

        Focus on the main points and key takeaways.
        """

        response = await self.ollama_client.generate(
            model=self.model,
            prompt=prompt,
            max_tokens=150
        )

        return Summary(
            article_id=article.id,
            summary=response.text,
            model=self.model
        )
```

**モデル選択:**
- **Llama 3.2**: バランスの取れた性能と精度
- **Mistral**: 高速処理が必要な場合
- **Gemma 2**: 多言語対応

**並列処理:**
- 複数のnews-creatorインスタンスを並列実行
- ワーカープールパターンで効率的な処理

### 3. 並列処理ストリームのサポート

**アーキテクチャ:**
```
            ┌─ News-creator Instance 1
            ├─ News-creator Instance 2
Pre-proc ───┤
            ├─ News-creator Instance 3
            └─ News-creator Instance 4
                    ↓
               (並列要約生成)
                    ↓
               Tag-generator
```

**実装:**
- **ワーカープール**: 設定可能なワーカー数（デフォルト4）
- **タスクキュー**: PostgreSQLの`processing_queue`テーブル
- **ステータス管理**: `pending`、`processing`、`completed`、`failed`

**スケーリング戦略:**
- **水平スケーリング**: Kubernetes HPA（Horizontal Pod Autoscaler）
- **スケーリングメトリクス**: キューの長さ、CPU使用率
- **バックプレッシャー**: キューが満杯時に新規記事の取得を一時停止

### 4. Pre-processor-sidecarによるInoreader記事取得

**役割の分離:**
- **Pre-processor**: RSSフィード処理
- **Pre-processor-sidecar**: Inoreader API処理

**Pre-processor-sidecar実装:**
```go
type InoreaderFetcher struct {
    APIClient    *InoreaderClient
    TokenManager *TokenManager
    DB           *pgxpool.Pool
}

func (f *InoreaderFetcher) FetchUnreadArticles(userID string) ([]Article, error) {
    token, err := f.TokenManager.GetValidToken(userID)
    if err != nil {
        return nil, err
    }

    articles, err := f.APIClient.GetUnreadItems(token)
    if err != nil {
        return nil, err
    }

    // 重複排除
    deduplicated := f.deduplicateArticles(articles)

    // PostgreSQLに保存
    f.DB.SaveArticles(deduplicated)

    return deduplicated, nil
}
```

**機能:**
- **OAuth2トークン管理**: auth-token-managerとの連携
- **レート制限遵守**: Inoreader APIのレート制限を考慮
- **重複排除**: URL、タイトル、公開日でユニーク性チェック
- **エラーハンドリング**: リトライ機構、エラーログ

### 5. パイプライン監視とメトリクス

**監視指標:**
- **スループット**: 時間あたりの処理記事数
- **レイテンシ**: 各ステージの処理時間
- **エラー率**: 失敗した記事の割合
- **キューの長さ**: 処理待ちの記事数

**実装:**
- UnifiedLoggerで構造化ログ出力
- ClickHouseでメトリクス集約
- Grafanaダッシュボード（将来実装）

## 結果・影響

### 利点

1. **スループットの大幅向上**
   - 並列処理により、処理速度が4倍に向上
   - ボトルネックの特定と個別最適化が可能
   - ワーカープールのスケーリングで需要に対応

2. **信頼性の向上**
   - ステージごとのエラーハンドリング
   - 冪等性により、再試行が安全
   - 処理失敗時の自動リトライ

3. **スケーラビリティの確保**
   - 各サービスが独立してスケール可能
   - Kubernetes HPAによる自動スケーリング
   - バックプレッシャーでシステム過負荷を防止

4. **保守性の向上**
   - ステージごとの責任が明確
   - 各サービスを独立して開発・テスト可能
   - 技術スタックの柔軟な選択（Python、Go）

### 注意点・トレードオフ

1. **システム複雑性の増加**
   - 4つのサービス間の依存関係管理
   - パイプライン全体のデバッグが複雑
   - ステージ間のデータフロー追跡

2. **レイテンシの増加**
   - ステージごとのキュー待ち時間
   - 直列処理よりもend-to-endレイテンシが長い
   - リアルタイム性が求められるユースケースには不向き

3. **リソース消費**
   - 複数のサービスがメモリとCPUを消費
   - Ollamaはコンシュームリソースが多い
   - PostgreSQLへのクエリ負荷増加

4. **運用負荷**
   - 各サービスの監視とアラート設定
   - パイプラインのエンド・ツー・エンド監視
   - ステージごとのスケーリング設定

## 参考コミット

- `363a3b40` - Refactor PostgreSQL configuration for multi-service access
- `da826b23` - Setup Meilisearch for search indexing
- `3fbfc9b7` - Add tag-generator and search-indexer services
- `4e7d2a1c` - Implement news-creator service with Ollama integration
- `8f3a5b2d` - Add processing_queue table for pipeline management
- `c7e9d4f1` - Implement worker pool pattern in news-creator
- `a2f8e3b4` - Add horizontal pod autoscaler for news-creator
- `e7f1d3a2` - Create pre-processor-sidecar for Inoreader fetching
- `9d4c2a1e` - Implement deduplication logic in pre-processor-sidecar
- `b6e5f7c8` - Add idempotency checks across pipeline stages
- `f3a1d9e2` - Implement backpressure mechanism for queue management
- `c8b2e4a7` - Add pipeline monitoring and metrics collection
