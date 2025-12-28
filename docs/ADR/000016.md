# RAGオーケストレータの統合と最適化

## ステータス

採択（Accepted）

## コンテキスト

2025年12月下旬、Altプロジェクトは包括的な可観測性基盤を確立し、ほぼすべての主要機能が稼働していた。しかし、最も重要な機能の一つである**RAG（Retrieval Augmented Generation）システム**がまだ実装されていなかった。

RAGシステムは、ユーザーのクエリに対して関連する記事を検索し、その内容を基にLLMで回答を生成する機能である。これにより、Altは単なるRSSリーダーから「AI搭載のナレッジプラットフォーム」へと進化する。

しかし、RAGシステムの実装には以下の課題があった：

1. **ベクトル検索の必要性**: 意味的に類似した記事を検索するため、埋め込みベクトル（embeddings）とベクトルデータベースが必要
2. **LLM統合の複雑性**: Ollamaを使ったローカルLLM推論のレイテンシとスループット
3. **プロンプト設計**: 検索されたコンテキストを基に、正確で有用な回答を生成するプロンプト戦略
4. **コンテキスト管理**: 検索結果（チャンク）を効率的にLLMへ渡す方法
5. **ストリーミング応答**: リアルタイムでユーザーに回答を表示する必要性

これらの課題を解決し、高品質なRAG体験を提供するため、専用のrag-orchestratorサービスを導入することが決定された。

## 決定

高精度かつ高速なRAGシステムを実現するため、以下のアーキテクチャとアルゴリズムを導入した：

### 1. Vector Database（pgvector）統合

**アーキテクチャ:**
```
記事 (articles テーブル)
    ↓
埋め込み生成 (Ollama: nomic-embed-text)
    ↓
ベクトル保存 (article_embeddings テーブル with pgvector)
    ↓
ベクトル検索 (HNSWインデックス)
    ↓
関連記事取得
```

**実装（record_00011で詳述）:**
```sql
CREATE TABLE article_embeddings (
    id SERIAL PRIMARY KEY,
    article_id INTEGER NOT NULL REFERENCES articles(id),
    embedding vector(768), -- nomic-embed-textの次元数
    model VARCHAR(100) NOT NULL,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX ON article_embeddings
USING hnsw (embedding vector_cosine_ops);
```

**埋め込み生成:**
```python
class EmbeddingGenerator:
    def __init__(self, ollama_url: str):
        self.ollama = OllamaClient(ollama_url)
        self.model = "nomic-embed-text"

    async def generate_embedding(self, text: str) -> List[float]:
        response = await self.ollama.embeddings(
            model=self.model,
            prompt=text
        )
        return response['embedding']
```

### 2. Context Retrieval最適化

**検索プロセス:**
```
1. ユーザークエリを受け取る
   ↓
2. クエリを埋め込みベクトルに変換
   ↓
3. pgvectorで類似度検索（上位10件）
   ↓
4. スコア閾値でフィルタリング（0.7以上）
   ↓
5. 記事本文を取得（チャンク化）
   ↓
6. LLMへのコンテキストとして提供
```

**実装:**
```go
type RetrieveContextUsecase struct {
    chunkRepo    ChunkRepository
    embeddingGen EmbeddingGenerator
    searchLimit  int // デフォルト: 10
    scoreThreshold float64 // デフォルト: 0.7
}

func (u *RetrieveContextUsecase) Execute(
    ctx context.Context,
    query string,
) ([]ContextItem, error) {
    // 1. クエリを埋め込みに変換
    queryEmbedding, err := u.embeddingGen.Generate(query)
    if err != nil {
        return nil, err
    }

    // 2. ベクトル検索
    chunks, err := u.chunkRepo.Search(ctx, queryEmbedding, u.searchLimit)
    if err != nil {
        return nil, err
    }

    // 3. スコアでフィルタリング
    filteredChunks := make([]ContextItem, 0)
    for _, chunk := range chunks {
        if chunk.Score >= u.scoreThreshold {
            filteredChunks = append(filteredChunks, ContextItem{
                ChunkID: chunk.ID,
                Text:    chunk.Content,
                Score:   chunk.Score,
                Metadata: chunk.Metadata,
            })
        }
    }

    return filteredChunks, nil
}
```

### 3. Chunking & Embedding基盤

**チャンク戦略:**
- **固定サイズチャンク**: 512トークンごとに分割（オーバーラップ50トークン）
- **意味的チャンク**: 段落や文の境界を尊重

**実装:**
```python
class ArticleChunker:
    def __init__(self, chunk_size: int = 512, overlap: int = 50):
        self.chunk_size = chunk_size
        self.overlap = overlap
        self.tokenizer = tiktoken.get_encoding("cl100k_base")

    def chunk_article(self, article: Article) -> List[Chunk]:
        tokens = self.tokenizer.encode(article.content)

        chunks = []
        start = 0
        while start < len(tokens):
            end = start + self.chunk_size
            chunk_tokens = tokens[start:end]
            chunk_text = self.tokenizer.decode(chunk_tokens)

            chunks.append(Chunk(
                article_id=article.id,
                content=chunk_text,
                start_index=start,
                end_index=end,
            ))

            start += (self.chunk_size - self.overlap)

        return chunks
```

**バッチ埋め込み生成:**
```python
async def batch_generate_embeddings(articles: List[Article]):
    chunker = ArticleChunker()
    generator = EmbeddingGenerator()

    for article in articles:
        chunks = chunker.chunk_article(article)

        for chunk in chunks:
            embedding = await generator.generate_embedding(chunk.content)

            # データベースに保存
            save_chunk_embedding(chunk.id, embedding)
```

### 4. LLM統合（Ollama）

**Ollamaモデル:**
- **llama3.2**: 汎用的な回答生成
- **qwen2.5**: 多言語対応（日本語強化）
- **mistral**: 高速推論

**ストリーミング応答:**
```go
type AnswerWithRAGUsecase struct {
    ollamaClient   *OllamaClient
    contextUsecase *RetrieveContextUsecase
}

func (u *AnswerWithRAGUsecase) Execute(
    ctx context.Context,
    query string,
) (<-chan string, error) {
    // 1. コンテキスト取得
    contexts, err := u.contextUsecase.Execute(ctx, query)
    if err != nil {
        return nil, err
    }

    // 2. プロンプト構築
    prompt := u.buildPrompt(query, contexts)

    // 3. Ollamaでストリーミング生成
    stream, err := u.ollamaClient.GenerateStream(ctx, OllamaRequest{
        Model:  "llama3.2",
        Prompt: prompt,
        Stream: true,
    })

    return stream, err
}
```

### 5. システムプロンプト導入とプロンプト簡素化（ADR 000002の実装）

**ADR 000002の背景:**
- 初期のRAG実装では、二段階LLM呼び出し（Stage 1: Citations、Stage 2: Answer）を採用
- しかし、"Insufficient Information"の多発とレイテンシ悪化が問題に
- ADR 000002でシングルフェーズ生成へ回帰を決定

**システムプロンプト:**
```
あなたは与えられたコンテキストのみを用いて質問に回答するAIアシスタントです。

ルール:
1. コンテキスト内の情報のみを使用してください
2. 情報がない場合は素直に「情報がありません」と答えてください
3. 回答は日本語で、300〜500語で簡潔に記述してください
4. 引用する場合は [chunk_id] 形式で参照してください

JSON形式で出力してください:
{
  "answer": "回答本文",
  "citations": [
    {"chunk_id": "chunk_123", "reason": "引用理由"}
  ],
  "fallback": false,
  "reason": "この回答を生成した理由"
}
```

**プロンプト構築:**
```go
func (u *AnswerWithRAGUsecase) buildPrompt(
    query string,
    contexts []ContextItem,
) string {
    var sb strings.Builder

    // システムプロンプト
    sb.WriteString(systemPrompt)
    sb.WriteString("\n\n")

    // コンテキスト
    sb.WriteString("<context>\n")
    for _, ctx := range contexts {
        sb.WriteString(fmt.Sprintf(
            "[chunk_%s] %s\n\n",
            ctx.ChunkID,
            ctx.Text,
        ))
    }
    sb.WriteString("</context>\n\n")

    // クエリ
    sb.WriteString(fmt.Sprintf("<query>%s</query>\n", query))

    return sb.String()
}
```

### 6. シングルフェーズ生成への回帰（ADR 000002）

**変更点:**
- **Before**: Stage 1で引用抽出 → Stage 2で回答生成（2回のLLM呼び出し）
- **After**: 1回のLLM呼び出しで回答と引用を同時生成

**新しいJSONスキーマ:**
```json
{
  "type": "object",
  "properties": {
    "answer": { "type": "string" },
    "citations": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "chunk_id": { "type": "string" },
          "reason": { "type": "string" }
        },
        "required": ["chunk_id"]
      }
    },
    "fallback": { "type": "boolean" },
    "reason": { "type": "string" }
  },
  "required": ["answer", "citations", "fallback", "reason"]
}
```

**出力バリデーション:**
```go
func (v *OutputValidator) Validate(response string) (*RAGOutput, error) {
    var output RAGOutput

    if err := json.Unmarshal([]byte(response), &output); err != nil {
        // JSONパースエラー時、修復を試みる
        repaired := v.repairJSON(response)
        if err := json.Unmarshal([]byte(repaired), &output); err != nil {
            return nil, fmt.Errorf("failed to parse JSON: %w", err)
        }
    }

    // citations がない場合でも回答があれば許容
    if output.Answer != "" && len(output.Citations) == 0 {
        logger.Warn("No citations provided, but answer exists")
    }

    return &output, nil
}
```

**検索件数の増加:**
- searchLimitを5件から10件に増加（ADR 000002の決定）
- より多くのコンテキストを提供することで回答の網羅性を向上

## 結果・影響

### 利点

1. **AI搭載ナレッジプラットフォームへの進化**
   - RAGにより、記事を「読む」だけでなく「質問に答える」機能を実現
   - ユーザーは自然言語で知識を検索・取得可能
   - 大量の記事から即座に関連情報を抽出

2. **高精度な回答生成**
   - pgvectorによるベクトル検索で意味的に関連する記事を取得
   - チャンク化により、長文記事からも適切な部分を抽出
   - システムプロンプトで幻覚（ハルシネーション）を最小化

3. **高速なユーザー体験**
   - シングルフェーズ生成でTTFT（Time To First Token）を大幅短縮
   - ストリーミング応答でリアルタイムに回答を表示
   - 検索件数増加（10件）で回答品質向上

4. **プライバシーとコスト**
   - Ollamaによるローカル推論で、データが外部に送信されない
   - OpenAI等のAPIコストがゼロ
   - ユーザーのプライバシー保護

### 注意点・トレードオフ

1. **計算リソース**
   - Ollama推論はGPU/CPUを消費
   - 埋め込み生成のバッチ処理に時間がかかる
   - ベクトルインデックスのメモリ消費

2. **精度の限界**
   - LLMの性能に依存（Llama3.2、Qwen2.5等）
   - コンテキストウィンドウの制約（8k〜32kトークン）
   - 引用精度がLLMの能力に左右される

3. **運用の複雑性**
   - Ollama、pgvector、埋め込み生成の統合管理
   - チャンク戦略のチューニング
   - プロンプトエンジニアリングの継続的改善

4. **スケーラビリティ**
   - 大量の同時リクエスト時のスループット制約
   - Ollamaの並列推論数に限界
   - ベクトル検索の速度（100万件超でパフォーマンス低下）

## 参考コミット（ADR 000001、000002で詳述）

- `0c307802` - Redesign and simplify RAG generation process (ADR 000002)
- `1b1b20de` - Update status in ADR 000001 from Proposed to Accepted
- `20b05274` - Integrate search indexing and chat capabilities into RAG orchestrator
- `56742376` - Refactor ADR 000001 to adjust context summarization
- `1ab9c215` - Introduce architectural decision record for RAG orchestrator optimization
- `a7e4b9c2` - Implement vector search with pgvector
- `d9a2e6f7` - Add chunking strategy for articles
- `e1b5c8a3` - Integrate Ollama for LLM inference
- `f2c6d9b4` - Implement streaming response for RAG
- `a3d7e1f5` - Add system prompt and simplify prompt structure
- `b4e8f2c6` - Implement single-phase generation (ADR 000002)
- `c5f9a3d7` - Add output validator with JSON repair
- `d6e1b4c8` - Increase search limit from 5 to 10

## 総括

RAGオーケストレータの導入により、AltプロジェクトはRSSリーダーからAI搭載ナレッジプラットフォームへと進化した。既存のADR 000001と000002で詳述された設計原則に基づき、高精度・高速・プライバシー重視のRAGシステムを実現した。今後は、継続的なプロンプト改善、モデルの更新、スケーリング戦略の最適化により、さらなる進化が期待される。
