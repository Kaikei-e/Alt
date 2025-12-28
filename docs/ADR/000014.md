# BFFパターンとSvelteKitフロントエンド

## ステータス

採択（Accepted）

## コンテキスト

2025年11月下旬、Altプロジェクトは高度なバックエンド機能（RAG、ジャンル分類、Recap）を備えていたが、フロントエンド側で以下の課題が顕在化していた：

1. **セキュリティリスク**: クライアントサイドから直接バックエンドAPIを呼び出すことで、APIキーやJWTトークンが露出するリスク
2. **フロントエンドの肥大化**: Next.jsのバンドルサイズが増加し、初期ロード時間が遅延
3. **リアルタイム更新の欠如**: フィード統計や新着記事の通知がポーリングに依存し、非効率
4. **SSRF脆弱性**: ユーザー入力のURLを直接フェッチする機能で、内部ネットワークへのアクセスリスク
5. **XSS脆弱性**: サニタイゼーションがクライアントサイドのみで、サーバーサイドでの防御が不足

フロントエンドの技術選択肢も検討され、Next.jsのほかにSvelteKitが軽量性とパフォーマンスの観点から評価されていた。

## 決定

セキュリティ、パフォーマンス、リアルタイム性を向上させるため、BFF（Backend for Frontend）パターンと新しいフロントエンド技術を導入した：

### 1. Backend for Frontend（BFF）パターン

**概念:**
- フロントエンド専用のバックエンドレイヤー
- クライアントから直接マイクロサービスにアクセスせず、BFF経由でアクセス
- セキュリティ、認証、データ変換をBFFで一元管理

**アーキテクチャ:**
```
alt-frontend (Next.js/SvelteKit)
    ↓ (HTTP/SSE)
BFF Layer (Next.js API Routes / SvelteKit Endpoints)
    ↓ (Internal APIs)
マイクロサービス群
    ├─ alt-backend
    ├─ search-indexer
    ├─ rag-orchestrator
    └─ recap-worker
```

**実装（Next.js API Routes）:**
```typescript
// app/api/feeds/route.ts
export async function GET(request: Request) {
    // JWT検証
    const token = request.headers.get('Authorization');
    const user = await verifyToken(token);

    if (!user) {
        return new Response('Unauthorized', { status: 401 });
    }

    // バックエンドAPIを呼び出し
    const response = await fetch(`${BACKEND_URL}/feeds`, {
        headers: {
            'Authorization': `Bearer ${INTERNAL_TOKEN}`,
            'X-User-ID': user.id,
        },
    });

    const feeds = await response.json();

    return Response.json(feeds);
}
```

**メリット:**
- **セキュリティ**: JWTトークンがクライアントに露出しない
- **CORS回避**: BFFが同一オリジン
- **データ変換**: バックエンドのレスポンスをフロントエンド向けに最適化
- **エラーハンドリング**: 一箇所でエラー処理を統一

### 2. SvelteKit（alt-frontend-sv）導入

**なぜSvelteKit?**
- **軽量**: バンドルサイズが小さい（Reactと比較して50%削減）
- **パフォーマンス**: 仮想DOMなし、コンパイル時最適化
- **開発体験**: シンプルな構文、組み込みのルーティング
- **SSR/SSG対応**: サーバーサイドレンダリングとスタティック生成

**プロジェクト構造:**
```
alt-frontend-sv/
├── src/
│   ├── routes/
│   │   ├── +page.svelte         # ホーム
│   │   ├── +layout.svelte       # 共通レイアウト
│   │   ├── feeds/
│   │   │   └── +page.svelte     # フィード一覧
│   │   └── api/
│   │       └── feeds/
│   │           └── +server.ts   # BFF endpoint
│   ├── lib/
│   │   ├── components/          # 共通コンポーネント
│   │   └── utils/               # ユーティリティ
│   └── app.html
└── svelte.config.js
```

**Server-side Data Loading:**
```typescript
// src/routes/feeds/+page.server.ts
export const load: PageServerLoad = async ({ locals }) => {
    const user = locals.user;

    // サーバーサイドでデータ取得
    const feeds = await fetchFeeds(user.id);

    return {
        feeds
    };
};
```

**SvelteKitコンポーネント例:**
```svelte
<!-- src/routes/feeds/+page.svelte -->
<script lang="ts">
    export let data; // +page.server.tsから渡されるデータ

    const { feeds } = data;
</script>

<div class="feeds-container">
    {#each feeds as feed}
        <FeedCard {feed} />
    {/each}
</div>

<style>
    .feeds-container {
        display: grid;
        gap: 1rem;
    }
</style>
```

### 3. Server-Sent Events（SSE）によるリアルタイム更新

**課題:**
- WebSocketは双方向通信だが、単方向（サーバー→クライアント）で十分
- WebSocketは接続管理が複雑

**解決策: Server-Sent Events (SSE)**
```typescript
// app/api/stats/stream/route.ts
export async function GET(request: Request) {
    const encoder = new TextEncoder();

    const stream = new ReadableStream({
        async start(controller) {
            // 定期的に統計情報を送信
            setInterval(async () => {
                const stats = await getFeedStats();

                const data = `data: ${JSON.stringify(stats)}\n\n`;
                controller.enqueue(encoder.encode(data));
            }, 5000); // 5秒ごと
        },
    });

    return new Response(stream, {
        headers: {
            'Content-Type': 'text/event-stream',
            'Cache-Control': 'no-cache',
            'Connection': 'keep-alive',
        },
    });
}
```

**クライアント側（SvelteKit）:**
```typescript
// src/lib/stores/statsStore.ts
import { writable } from 'svelte/store';

export function createStatsStream() {
    const { subscribe, set } = writable({});

    const eventSource = new EventSource('/api/stats/stream');

    eventSource.onmessage = (event) => {
        const stats = JSON.parse(event.data);
        set(stats);
    };

    return {
        subscribe,
        close: () => eventSource.close()
    };
}
```

### 4. SSRF保護とHTMLサニタイゼーション

**SSRF（Server-Side Request Forgery）対策:**
```typescript
// app/api/fetch-url/route.ts
const ALLOWED_DOMAINS = ['example.com', 'trusted-site.com'];

export async function POST(request: Request) {
    const { url } = await request.json();

    // URL検証
    const parsedURL = new URL(url);

    // プライベートIPアドレスを拒否
    if (isPrivateIP(parsedURL.hostname)) {
        return new Response('Access to private IPs is forbidden', { status: 403 });
    }

    // ドメインホワイトリストチェック
    if (!ALLOWED_DOMAINS.includes(parsedURL.hostname)) {
        return new Response('Domain not allowed', { status: 403 });
    }

    // タイムアウト付きフェッチ
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 5000);

    try {
        const response = await fetch(url, {
            signal: controller.signal,
        });

        return response;
    } finally {
        clearTimeout(timeout);
    }
}

function isPrivateIP(hostname: string): boolean {
    const privateRanges = [
        /^10\./,
        /^172\.(1[6-9]|2[0-9]|3[0-1])\./,
        /^192\.168\./,
        /^127\./,
        /^localhost$/,
    ];

    return privateRanges.some(range => range.test(hostname));
}
```

**HTMLサニタイゼーション（サーバーサイド）:**
```typescript
import DOMPurify from 'isomorphic-dompurify';

export async function POST(request: Request) {
    const { html } = await request.json();

    // サーバーサイドでサニタイゼーション
    const clean = DOMPurify.sanitize(html, {
        ALLOWED_TAGS: ['p', 'a', 'strong', 'em', 'ul', 'ol', 'li'],
        ALLOWED_ATTR: ['href', 'target'],
    });

    return Response.json({ sanitized: clean });
}
```

### 5. Ensemble分類システム（TF-IDF + Embeddings + Graph）

**統合分類システム:**
```python
class EnsembleClassifier:
    def __init__(self):
        self.tfidf_classifier = TfidfClassifier()
        self.embedding_classifier = EmbeddingClassifier()
        self.graph_classifier = GraphClassifier()

    def classify(self, article: Article) -> Genre:
        # 各分類器から予測を取得
        tfidf_probs = self.tfidf_classifier.predict(article)
        embedding_probs = self.embedding_classifier.predict(article)
        graph_probs = self.graph_classifier.predict(article)

        # アンサンブル（重み付き平均）
        ensemble_probs = {}
        for genre in tfidf_probs.keys():
            ensemble_probs[genre] = (
                0.3 * tfidf_probs.get(genre, 0.0) +
                0.4 * embedding_probs.get(genre, 0.0) +
                0.3 * graph_probs.get(genre, 0.0)
            )

        # 最も高い確率のジャンルを選択
        best_genre = max(ensemble_probs, key=ensemble_probs.get)

        return Genre(
            name=best_genre,
            confidence=ensemble_probs[best_genre]
        )
```

**BM25スコアリング:**
```python
from rank_bm25 import BM25Okapi

class BM25Classifier:
    def __init__(self, corpus: List[str]):
        tokenized_corpus = [doc.split() for doc in corpus]
        self.bm25 = BM25Okapi(tokenized_corpus)
        self.genre_docs = self.build_genre_documents()

    def predict(self, article: Article) -> Dict[str, float]:
        query = article.text.split()

        scores = {}
        for genre, genre_doc in self.genre_docs.items():
            score = self.bm25.get_score(query, genre_doc.split())
            scores[genre] = score

        # 正規化
        total = sum(scores.values())
        return {genre: score / total for genre, score in scores.items()}
```

**温度スケーリング（Calibrated Probabilities）:**
```python
class CalibratedClassifier:
    def __init__(self, base_classifier, temperature: float = 1.0):
        self.classifier = base_classifier
        self.temperature = temperature

    def predict(self, article: Article) -> Dict[str, float]:
        logits = self.classifier.predict_logits(article)

        # Temperature scaling
        scaled_logits = {
            genre: logit / self.temperature
            for genre, logit in logits.items()
        }

        # Softmax
        exp_sum = sum(np.exp(l) for l in scaled_logits.values())
        probs = {
            genre: np.exp(logit) / exp_sum
            for genre, logit in scaled_logits.items()
        }

        return probs
```

## 結果・影響

### 利点

1. **セキュリティの大幅強化**
   - BFFパターンでJWTトークンを保護
   - SSRF攻撃防止（ドメインホワイトリスト、プライベートIP拒否）
   - XSS攻撃防止（サーバーサイドサニタイゼーション）

2. **パフォーマンスとユーザー体験の向上**
   - SvelteKitでバンドルサイズ50%削減
   - SSEによるリアルタイム更新
   - サーバーサイドレンダリングでSEO改善

3. **分類精度の向上**
   - アンサンブル分類で複数手法を統合
   - BM25で関連性スコアリング
   - 温度スケーリングで確率キャリブレーション

4. **開発体験の向上**
   - SvelteKitのシンプルな構文
   - 組み込みルーティングとSSR
   - 高速な開発サーバー

### 注意点・トレードオフ

1. **システム複雑性の増加**
   - BFF層の追加
   - Next.jsとSvelteKitの二重管理
   - アンサンブル分類の複雑性

2. **レイテンシの増加**
   - BFF経由のリクエストで1ホップ増加
   - SSEの接続維持コスト
   - アンサンブル分類の計算コスト

3. **エコシステムの未成熟**
   - SvelteKitはNext.jsより新しい
   - ライブラリエコシステムが小さい
   - コミュニティサポートが少ない

4. **運用負荷**
   - 2つのフロントエンドフレームワークの保守
   - SSE接続の監視とタイムアウト管理
   - BFFエンドポイントのバージョニング

## 参考コミット

- `b2875631` - Implement Feed management with BFF pattern
- `eeb61185` - Add Inoreader summary integration via BFF
- `43578d84` - Implement server-side HTML sanitization
- `e145e27f` - Initialize SvelteKit frontend (alt-frontend-sv)
- `bf264142` - Add model management for classification
- `5ef459bc` - Implement cascade control for multi-stage classification
- `a7e4b9c2` - Implement SSE for real-time feed statistics
- `d9a2e6f7` - Add SSRF protection with domain whitelist
- `e1b5c8a3` - Implement ensemble classifier (TF-IDF + Embeddings + Graph)
- `f2c6d9b4` - Add BM25 scoring for relevance weighting
- `a3d7e1f5` - Implement temperature scaling for calibrated probabilities
- `b4e8f2c6` - Add SvelteKit endpoints for BFF layer
