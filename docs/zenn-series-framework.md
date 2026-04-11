# Alt プロジェクト Zenn.dev 記事シリーズ構成

## シリーズ概要

**シリーズタイトル**: 「AI強化型RSSプラットフォームを個人開発した話」

**対象読者**:
- マイクロサービスアーキテクチャに興味のあるエンジニア
- ローカルLLM/RAGに関心のある開発者
- Go/TypeScript/Rustでの実践的な設計パターンを学びたい人

**記事構成**: 全10本（本ドキュメントは6本分の骨子 = 60%）

---

## 目次

1. [記事1: 基盤構築編](#記事1-基盤構築編)
2. [記事2: RSS処理パイプライン編](#記事2-rss処理パイプライン編)
3. [記事3: AI強化編（前編）](#記事3-ai強化編前編)
4. [記事4: AI強化編（後編）](#記事4-ai強化編後編)
5. [記事5: API現代化編](#記事5-api現代化編)
6. [記事6: フロントエンド刷新編](#記事6-フロントエンド刷新編)
7. [残り4本の候補テーマ](#残り4本40の候補テーマ)

---

## 記事1: 基盤構築編

### タイトル

**「Docker Compose + Clean ArchitectureでAI-RSSプラットフォームを設計する」**

### 想定読了時間

15分（約8,000字）

### 技術スタック

Go, TypeScript, Docker Compose, PostgreSQL, Meilisearch, ClickHouse

### 対応ADR

- ADR-000001〜000005: 初期アーキテクチャ決定
- ADR-000003, 000031: Docker Compose設計
- ADR-000002, 000006: Clean Architecture採用

### 目次

1. はじめに
   - プロジェクトの動機
   - なぜ個人開発でマイクロサービスを選んだか

2. アーキテクチャ全体像
   - サービス一覧と責務
   - 通信パターン（同期/非同期）

3. Docker Compose設計
   - プロファイルによる環境分離
   - ネットワーク設計
   - ヘルスチェック戦略
   - 開発/本番の差分管理

4. Clean Architecture実践
   - レイヤー構成（Handler → Usecase → Port → Gateway → Driver）
   - 依存性逆転の原則
   - Go/TypeScriptでの実装パターン

5. データストア選定
   - PostgreSQL: トランザクショナルデータ
   - Meilisearch: 全文検索
   - ClickHouse: 分析ログ
   - 使い分けの判断基準

6. まとめ
   - 個人開発でのマイクロサービスの是非
   - 次回予告

### 骨子・要点

```
- 個人開発でも「将来の拡張性」より「今の開発体験」を優先すべき場面がある
- Docker Composeのprofileを活用すると、GPU有無などの環境差を吸収できる
- Clean Architectureは「厳密に守る」より「原則を理解して適用」が重要
- データストアは「得意なことに使う」が鉄則
```

### 図解候補

- [ ] サービス構成図（全体アーキテクチャ）
- [ ] レイヤー依存関係図
- [ ] Docker Composeプロファイル図

---

## 記事2: RSS処理パイプライン編

### タイトル

**「RSSフィード取得から全文抽出まで: 非同期処理パイプラインの設計」**

### 想定読了時間

12分（約7,000字）

### 技術スタック

Go, Meilisearch, Redis, PostgreSQL

### 対応ADR

- ADR-000009〜000011: フィード取得戦略
- ADR-000012〜000014: pre-processor設計
- ADR-000015〜000016: 検索インデックス
- ADR-000017〜000018: エラーハンドリング

### 目次

1. はじめに
   - RSSリーダーの「裏側」
   - 処理パイプラインの全体像

2. フィード取得戦略
   - ポーリング間隔の決定
   - レート制限対策（5秒ルール）
   - フィード更新検知（ETag, Last-Modified）

3. pre-processor設計
   - 記事本文抽出（Readability相当）
   - HTMLパース・サニタイズ
   - 重複検出（URLハッシュ、コンテンツハッシュ）

4. 検索インデックス構築
   - Meilisearchへの同期戦略
   - ファセット設計（タグ、ソース、日付）
   - インデックス更新のタイミング

5. エラーハンドリング
   - リトライ戦略（指数バックオフ）
   - デッドレターキュー的な設計
   - 障害フィードの隔離

6. まとめ
   - パイプライン設計のコツ
   - 次回予告

### 骨子・要点

```
- RSSフィードは「壊れている」ことが多い前提で設計する
- 外部APIへのリクエストは必ず間隔を空ける（5秒ルール）
- 全文抽出は完璧を求めず、80%取れればOKの精神
- エラーは「握りつぶさず記録」「リトライは制限付き」
```

### 図解候補

- [ ] 処理パイプラインシーケンス図
- [ ] フィード取得フロー図
- [ ] エラーハンドリング状態遷移図

---

## 記事3: AI強化編（前編）

### タイトル

**「LLMによる記事要約システムの構築: Ollama + iGPU活用」**

### 想定読了時間

15分（約8,000字）

### 技術スタック

Python (FastAPI), Ollama, Vulkan, AMD Ryzen AI (iGPU)

### 対応ADR

- ADR-000019〜000021: ローカルLLM選定
- ADR-000025: iGPU活用（Ryzen AI 9 HX370）
- ADR-000022〜000023: 要約パイプライン
- ADR-000024: 品質管理

### 目次

1. はじめに
   - なぜローカルLLMか（コスト、プライバシー、レイテンシ）
   - 個人開発でのAI活用の現実

2. ローカルLLM選定
   - Ollama採用理由
   - モデル比較（Qwen, Llama, Gemma）
   - VRAMと品質のトレードオフ

3. iGPU活用
   - Ryzen AI 9 HX370のスペック
   - Vulkanアクセラレーション設定
   - Docker環境でのGPUアクセス（動的GID検出）
   - パフォーマンス計測結果

4. 要約パイプライン設計
   - プロンプトエンジニアリング
   - ストリーミング出力の実装
   - 日本語/英語対応

5. 品質管理
   - 要約評価の難しさ
   - フォールバック戦略（タイムアウト、エラー時）
   - 人間によるスポットチェック

6. まとめ
   - iGPUでのLLM運用の可能性
   - 次回予告（RAGへ）

### 骨子・要点

```
- ローカルLLMは「APIコスト0」だが「インフラコスト」がある
- iGPUは意外と使える（7B-8Bモデルなら実用レベル）
- Vulkanアクセラレーションの設定はハマりポイントが多い
- 要約品質は「完璧」を求めず「読む価値があるか」で判断
```

### 図解候補

- [ ] LLMサービス構成図
- [ ] iGPUアクセス設定図
- [ ] 要約パイプラインフロー図

### コード例候補

```python
# news-creator/要約エンドポイント
@router.post("/api/v1/summarize/stream")
async def stream_summarize(request: SummarizeRequest):
    async def generate():
        async for chunk in ollama_client.chat_stream(
            model="qwen2.5:7b",
            messages=[{"role": "user", "content": prompt}]
        ):
            yield chunk
    return StreamingResponse(generate(), media_type="text/plain")
```

---

## 記事4: AI強化編（後編）

### タイトル

**「RAGパイプライン構築: ベクトル検索で12秒を1.4秒に高速化した話」**

### 想定読了時間

18分（約9,000字）

### 技術スタック

Go, PostgreSQL/pgvector, Python, HNSW

### 対応ADR

- ADR-000021, 000028: pgvector導入
- ADR-000025: RAGアーキテクチャ
- ADR-000036: パフォーマンス最適化（88%改善）
- ADR-000037: Temporal Boost、Morning Letter

### 目次

1. はじめに
   - RAG（Retrieval-Augmented Generation）とは
   - なぜ記事検索にRAGが必要か

2. pgvector導入
   - PostgreSQL拡張としてのpgvector
   - HNSWインデックスの仕組み
   - チャンキング戦略（文章分割）

3. RAGアーキテクチャ
   - rag-orchestrator設計
   - Context Retrieval Usecase
   - LLMクライアント抽象化

4. パフォーマンス最適化
   - 問題: 12秒かかっていたContext Retrieval
   - 原因分析: Query Expansion + Vector Search
   - 解決策1: Query Expansion分離（4-6秒→1秒）
   - 解決策2: Two-Stage Vector Search（5-10秒→30ms）
   - 解決策3: 並列検索（goroutine活用）
   - 結果: 12秒 → 1.4秒（88%改善）

5. Temporal Boost
   - 時間認識スコア調整の必要性
   - ブースト係数設計（6時間: 1.3x, 12時間: 1.15x, ...）
   - Morning Letter機能への応用

6. まとめ
   - RAGパフォーマンス最適化のポイント
   - 次回予告

### 骨子・要点

```
- RAGは「検索 + 生成」の組み合わせ、検索が遅いと全体が遅い
- pgvectorのHNSWは高速だが、JOINすると効かなくなる罠
- Two-Stage検索: まず候補を絞り、次にメタデータを取得
- Query Expansionは軽量モデルに分離すると劇的に速くなる
- 並列化は「独立した処理」を見つけることから始まる
```

### 図解候補

- [ ] RAGアーキテクチャ図
- [ ] Two-Stage検索フロー図
- [ ] パフォーマンス改善Before/After図

### コード例候補

```go
// Two-Stage Vector Search
// Stage 1: Pure vector search (HNSW optimized)
candidateIDs, _ := chunkRepo.SearchCandidates(ctx, queryVector, limit*3)

// Stage 2: Metadata enrichment
contexts, _ := chunkRepo.EnrichWithMetadata(ctx, candidateIDs, limit)
```

```go
// Temporal Boost
func applyTemporalBoost(contexts []ContextItem, now time.Time) {
    for i := range contexts {
        hoursSince := now.Sub(contexts[i].PublishedAt).Hours()
        switch {
        case hoursSince <= 6:
            contexts[i].Score *= 1.3
        case hoursSince <= 12:
            contexts[i].Score *= 1.15
        case hoursSince <= 18:
            contexts[i].Score *= 1.05
        }
    }
}
```

---

## 記事5: API現代化編

### タイトル

**「REST APIからConnect-RPCへ: 型安全なgRPC-Web移行の実践」**

### 想定読了時間

15分（約8,000字）

### 技術スタック

Go (Connect-RPC), TypeScript, Protocol Buffers, buf

### 対応ADR

- ADR-000028: Connect-RPC導入（Phase 1）
- ADR-000029: Streaming認証（UserContext）
- ADR-000030: Phase 2-3（Feed List/Search）
- ADR-000032: Phase 4-6（Articles/RSS/Streaming）
- ADR-000034: フロントエンドクライアント移行
- ADR-000035: Streaming完全移行

### 目次

1. はじめに
   - REST APIの課題（型安全性、二重プロキシ）
   - Connect-RPCとは

2. なぜConnect-RPCか
   - gRPC-Web互換
   - Protocol Buffersによる型安全性
   - RESTとの共存可能性

3. 移行戦略
   - Phase別アプローチ
   - 既存REST APIとの併存期間
   - フィーチャーフラグによる切り替え

4. バックエンド実装
   - buf.gen.yamlの設定
   - Connect-RPCハンドラ実装
   - Unary RPC vs Server Streaming

5. Streaming認証
   - 課題: Streaming RPCでの認証情報伝達
   - 解決: UserContextの完全性確保
   - アダプターパターンの導入

6. フロントエンド統合
   - Transport分離（Client/Server）
   - 動的インポートパターン
   - 既存関数シグネチャの維持

7. まとめ
   - 段階的移行のメリット
   - 次回予告

### 骨子・要点

```
- Connect-RPCは「gRPCの良さ」を「Webで使える形」で提供
- 移行は「一気に」ではなく「Phase別」で進める
- Streaming認証は設計段階で考慮しないとハマる
- フロントエンドは「関数シグネチャ維持」でコンポーネント変更を最小化
```

### 図解候補

- [ ] REST vs Connect-RPC通信経路図
- [ ] Phase別移行ロードマップ
- [ ] Transport分離図

### コード例候補

```protobuf
// proto/alt/feeds/v2/feeds.proto
service FeedService {
  rpc GetFeedStats(GetFeedStatsRequest) returns (GetFeedStatsResponse);
  rpc StreamSummarize(StreamSummarizeRequest) returns (stream StreamSummarizeEvent);
}
```

```typescript
// フロントエンド: 動的インポートパターン
export async function updateFeedReadStatusClient(feedUrl: string): Promise<void> {
  const transport = createClientTransport();
  const { markAsRead } = await import("$lib/connect/feeds");
  await markAsRead(transport, feedUrl);
}
```

---

## 記事6: フロントエンド刷新編

### タイトル

**「Svelte 5 Runesで作るモダンなRSSリーダーUI」**

### 想定読了時間

12分（約7,000字）

### 技術スタック

TypeScript, SvelteKit, Svelte 5, bits-ui

### 対応ADR

- ADR-000026: Svelte 5 Desktop UI（CSR-only）
- ADR-000027: Article ID 3層統合
- ADR-000034: Connect-RPCクライアント移行
- ADR-000035: Streamingアダプター
- ADR-000037: Morning Letter UI

### 目次

1. はじめに
   - Svelte 5の新機能（Runes）
   - なぜSvelteを選んだか

2. CSR-only戦略
   - SSR不要の判断基準
   - SvelteKitでのCSR設定
   - パフォーマンスへの影響

3. Svelte 5 Runes実践
   - $state: リアクティブな状態管理
   - $derived: 派生値の計算
   - $effect: 副作用の処理
   - TanStack Query脱却の経緯

4. bits-uiによるコンポーネント設計
   - Headless UIの利点
   - アクセシビリティ対応
   - カスタムスタイリング

5. ストリーミングUI
   - Connect-RPCストリームの処理
   - スロットリング（50msルール）
   - AbortControllerによるキャンセル

6. Morning Letter UI
   - チャットインターフェース設計
   - リアルタイムレスポンス表示
   - Citation表示の実装

7. まとめ
   - Svelte 5の開発体験
   - 次回予告

### 骨子・要点

```
- Svelte 5のRunesは「Reactのhooksに似ているが違う」
- CSR-onlyは認証必須アプリでは合理的な選択
- TanStack Queryは便利だが、Svelte 5なら不要なケースも
- ストリーミングUIは「スロットリング」が鍵
```

### 図解候補

- [ ] Svelte 5 Runes概念図
- [ ] コンポーネント構成図
- [ ] ストリーミングUI状態遷移図

### コード例候補

```svelte
<script lang="ts">
  // Svelte 5 Runes
  let messages = $state<Message[]>([]);
  let isLoading = $state(false);

  // Throttled streaming
  let bufferedContent = "";
  let lastUpdateTime = 0;
  const THROTTLE_MS = 50;

  function handleDelta(text: string) {
    bufferedContent += text;
    const now = Date.now();
    if (now - lastUpdateTime > THROTTLE_MS) {
      messages[currentIndex].content = bufferedContent;
      lastUpdateTime = now;
    }
  }
</script>
```

---

## 残り4本（40%）の候補テーマ

### 記事7: 認証基盤編（候補）

**タイトル案**: 「Ory Kratosで作るセルフホスト認証基盤」

| 項目 | 内容 |
|------|------|
| 技術スタック | Ory Kratos, Go, JWT |
| 主なトピック | セッション管理、JWT発行、auth-hub設計 |
| 対応ADR | 000007, 000010 |

### 記事8: DevOps編（候補）

**タイトル案**: 「altctl: Docker Compose運用を楽にするCLIツールの作り方」

| 項目 | 内容 |
|------|------|
| 技術スタック | Go (Cobra), Docker Compose |
| 主なトピック | スタック抽象化、alt-perfによるE2E計測 |
| 対応ADR | 000031, 000033 |

### 記事9: 観測性編（候補）

**タイトル案**: 「ClickHouse + Rustで作るログ集約基盤」

| 項目 | 内容 |
|------|------|
| 技術スタック | Rust (Axum), ClickHouse |
| 主なトピック | 構造化ログ、rask-log-aggregator/forwarder |
| 対応ADR | 000008, 000020 |

### 記事10: 振り返り編（候補）

**タイトル案**: 「AI-RSSプラットフォーム開発1年の学び」

| 項目 | 内容 |
|------|------|
| 主なトピック | 技術選定の振り返り、失敗談、今後の展望 |
| 形式 | エッセイ形式 |

---

## シリーズ全体の流れ

```
記事1 (基盤構築)
    ↓
記事2 (RSS処理) ←───── 記事7 (認証) [候補]
    ↓
記事3 (AI前編: LLM要約)
    ↓
記事4 (AI後編: RAG)
    ↓
記事5 (API現代化) ────→ 記事6 (フロントエンド)
    ↓
記事8 (DevOps) [候補] → 記事9 (観測性) [候補]
    ↓
記事10 (振り返り) [候補]
```

---

## 執筆ガイドライン

### 文体

- 「です・ます」調で統一
- 専門用語は初出時に簡単な説明を付ける
- コード例は実際に動作するものを掲載

### 構成

- 各記事は独立して読めるようにする
- ただし、シリーズとしての流れも意識
- 前回・次回への言及を適度に入れる

### コード例

- 重要な部分のみ抜粋（全体はGitHubリンク）
- 言語ごとにシンタックスハイライト
- コメントは日本語で

### 図解

- Mermaid記法を活用
- 複雑なアーキテクチャ図は画像として用意

---

## 付録: ADR対応表

| 記事 | 対応ADR |
|------|---------|
| 1. 基盤構築 | 000001-000006, 000031 |
| 2. RSS処理 | 000009-000018 |
| 3. AI前編 | 000019-000025 |
| 4. AI後編 | 000021, 000025, 000028, 000036, 000037 |
| 5. API現代化 | 000028-000032, 000034, 000035 |
| 6. フロントエンド | 000026, 000027, 000034, 000035, 000037 |

---

*作成日: 2026-01-01*
*最終更新: 2026-01-01*
