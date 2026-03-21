# Embedder 追加調査メモ

_Date: 2026-03-21_

## 目的

Ask Augur の 2026-03-21 タイムアウト事象に関連して、`rag-orchestrator` から見た embedder 経路を追加調査し、現状の失敗モード、既存の劣化許容、未対策の穴、修正優先度を整理する。

このメモは次の2点を切り分ける。

- `news-creator` 側の GPU 競合が直接引き起こす 5 分タイムアウト
- `embedder` 側の遅延・障害が retrieval / indexing に与える二次影響

結論として、今回の主因は `news-creator` 側だが、embedder についても「速く失敗する」「non-fatal な箇所は確実に劣化継続する」「観測しやすい」の3点を補強する価値がある。

## 対象コード

- `rag-orchestrator/internal/adapter/rag_augur/ollama_embedder.go`
- `rag-orchestrator/internal/usecase/retrieval/expand_queries.go`
- `rag-orchestrator/internal/usecase/retrieval/embed_and_search.go`
- `rag-orchestrator/internal/usecase/retrieve_context_usecase.go`
- `rag-orchestrator/internal/adapter/rag_http/handler.go`
- `rag-orchestrator/internal/worker/worker.go`
- `rag-orchestrator/internal/di/container.go`
- `rag-orchestrator/internal/infra/config/config.go`
- `compose/rag.yaml`
- `docs/ADR/000275.md`
- `docs/ADR/000036.md`
- `docs/services/rag-orchestrator.md`

## 要約

### 結論

1. Ask Augur の 5 分タイムアウトの直接原因は embedder ではなく `news-creator-backend` 側の LLM 競合である可能性が高い。
2. 一方で `rag-orchestrator` の embedder には、観測性とテストの薄さ、timeout 設計の粗さが残っている。
3. retrieval にはすでに部分的な劣化許容が入っている。
4. indexing 側は ADR-275 により backoff と timeout が入っており、retrieval より先に最低限の防御が入っている。
5. 今回の実装対象としては、embedder を大改修するより、timeout とログ区別、劣化許容の契約テスト追加が妥当。

### 優先度

- 高: retrieval の original embedding は必須依存なので、失敗時のエラーを短時間で返すこと
- 高: expanded embedding failure が non-fatal であることをテストで固定すること
- 中: embedder の timeout / bad status / decode failure / context deadline をログで区別すること
- 中: `EMBEDDER_TIMEOUT=30` が現在の Ask Augur 体感要件に妥当か再確認すること
- 低: circuit breaker や active health check の導入

## 現在のアーキテクチャ

### retrieval 経路

`rag-orchestrator` の retrieval は大きく2段階で embedder を使う。

1. Stage 1 で original query を embed する
2. Stage 2 で expanded queries と tags を embed する

実装上の挙動は明確に分かれている。

- `expand_queries.go`
  - original query embedding 失敗は `failed to encode original query` で即時エラー
- `embed_and_search.go`
  - expanded embedding 失敗は `expanded_embedding_failed` を出して継続

つまり、embedder は retrieval 全体に対して「必須部分」と「劣化可能部分」の両方を持っている。

### indexing 経路

indexing は `IndexArticleUsecase` から embedder を使う。こちらは Ask Augur の online retrieval ではなく、バックグラウンドの索引更新に関わる。

`worker.go` では次の防御がすでに入っている。

- 1 ジョブあたり 60 秒 timeout
- 失敗時の exponential backoff
- 最大 5 分 backoff

これは `docs/ADR/000275.md` で設計化済み。

## 現行設定

`compose/rag.yaml` と `config.go` から見える関連設定は次の通り。

- `EMBEDDER_EXTERNAL=${EMBEDDER_EXTERNAL}`
- `EMBEDDER_TIMEOUT=30`
- デフォルト embedder URL: `http://embedder-external:11436`
- モデル: `embeddinggemma`

一方、サービス文書 `docs/services/rag-orchestrator.md` でも同じ値が案内されている。

重要なのは、embedder は `http.Client.Timeout` ベースの単純な全体 timeout であり、接続・ヘッダ・本文読み取りを個別に分けていないこと。

## 実装詳細

### 1. adapter: `ollama_embedder.go`

現状の embedder adapter はかなり薄い。

- `NewOllamaEmbedder(...)` で `http.Client{Timeout: timeout}` を生成
- `Encode(ctx, texts)` で `/api/embed` に POST
- 200 以外は `ollama returned status: <code>`
- transport error は `failed to call ollama`
- decode error は `failed to decode response`

ログは以下の3種類しかない。

- `ollama_embed_started`
- `ollama_embed_failed`
- `ollama_embed_bad_status`
- `ollama_embed_completed`

不足している点:

- `context deadline exceeded` と `client timeout` の区別が弱い
- response body の一部や upstream のエラーメッセージを出していない
- retry / fallback / classification がない
- adapter 専用テストファイルが存在しない

`rag-orchestrator/internal/adapter/rag_augur` 配下には
`ollama_generator_test.go` と `reranker_client_test.go` はあるが、
`ollama_embedder_test.go` は無い。

### 2. retrieval Stage 1: `expand_queries.go`

Stage 1 の original query embedding は必須依存で、ここが失敗すると retrieval は終了する。

現行コード上の意味:

- これは妥当
- ただし timeout に達するまで待つので、短すぎれば精度悪化、長すぎれば Ask Augur の初期応答が鈍る

今回の観点では、「必須依存だから無限に待ってよい」ではなく、「必須依存でも短めの SLA 内で諦める」が必要。

### 3. retrieval Stage 2: `embed_and_search.go`

expanded query embedding は既に non-fatal である。

この設計の意味:

- original query の vector search は継続できる
- BM25 も並列で継続できる
- expanded embedding だけ失敗しても最低限の context retrieval は成立し得る

これは Ask Augur の劣化耐性としては正しい方向。

ただし現状の問題は、これが「コード上そうなっている」だけで、契約テストが薄い点にある。

## 既存の防御

### 1. indexing 側は ADR-275 でかなり守られている

ADR-275 が対処したのは次の問題。

- embedder down 時の無制限ポーリング
- caller 依存 context による retry loop 加速
- timeout なし処理

その結果:

- worker は失敗時 backoff
- `UpsertIndex` は 90 秒 server-side timeout
- job 処理は 60 秒 timeout

このため、embedder 側障害による「CPU 浪費」「無限リトライ」は retrieval よりも indexing 側で先に抑えられている。

### 2. retrieval 側にも部分的な degrade がある

- query expansion は race + fallback
- expanded embedding は non-fatal
- BM25 は non-fatal
- rerank も別 timeout を持つ

つまり、retrieval 全体が脆弱なのではなく、「original query embedding」と「augur generation」が太いボトルネックで残っている。

## Ask Augur 遅延との関係

### 直接原因ではない理由

今回の既知ログでは、Ask Augur は `ollama_chat_stream_*` 側で 5 分後に `context deadline exceeded` になっている。これは answer generation 側の待ちであり、embedder の典型的な 30 秒 timeout と整合しない。

また、`news-creator` 側では次が確認できる。

- `DistributingGateway.generate()` は常に local
- 階層要約 map/reduce が local GPU を占有し得る
- `OLLAMA_NUM_PARALLEL=2`

この構図は 5 分ハングの説明として強い。

### それでも embedder を直す価値がある理由

1. retrieval の先頭で original embedding が詰まると、Ask Augur の TTFT に直接効く
2. expanded embedding が遅い場合、今は non-fatal でも観測性が弱く、再発切り分けが難しい
3. embedder adapter にテストが無いため、timeout 周りの変更が壊れやすい
4. indexing 側と retrieval 側で failure handling の成熟度に差がある

## 現状の問題点

### 問題1: timeout の表現力が低い

`http.Client.Timeout` に寄せているため、次が混ざりやすい。

- 接続失敗
- TLS/ヘッダ待ち
- 本文読み取り遅延
- context cancellation

少なくともログ上は区別したい。

### 問題2: adapter テストが無い

embedder adapter 単体の正常系・異常系が固定されていない。

必要な最低限のケース:

- 200 + 正常 JSON
- 非 200 status
- invalid JSON
- transport timeout
- context cancellation

### 問題3: retrieval 側の degrade 契約テストが不足

コード上は次の意図がある。

- original embedding 失敗: hard fail
- expanded embedding 失敗: degrade and continue

この境界は今回の修正で最も壊したくない箇所なので、明示テストが必要。

### 問題4: 30 秒 timeout が現状 UX に対して長い可能性

ADR-36 の想定では embedding は約 155ms。もちろん現在の実運用ではネットワークや load により変動するが、それでも 30 秒は retrieval の体感待機としてはかなり長い。

Ask Augur の用途上、original embedding で 30 秒待った後に generation 側も待つと、全体の失敗時間が長くなる。

ここは単純に「短くすべき」とは断言しないが、少なくとも見直し対象。

## 推奨対応

### 短期

1. `ollama_embedder.go` に adapter テストを追加
2. transport error を分類してログ出しを改善
3. `retrieve_context_usecase_test.go` に original/expanded の failure 境界テストを追加
4. Ask Augur の体感要件を前提に `EMBEDDER_TIMEOUT` を再評価する

### 中期

1. embedder の metrics を追加する
2. timeout を接続系と応答系で分離する
3. retrieval stage ごとに elapsed を structured log へ出す

### 今回の主タスクには入れない方がよい

1. circuit breaker 導入
2. active health check
3. embedder multi-endpoint failover
4. embedding cache 導入

これらは有効ではあるが、Ask Augur の今回のタイムアウト解消に対しては大きすぎる。

## 推奨テストケース

### Go adapter

- `TestOllamaEmbedder_Encode_Success`
- `TestOllamaEmbedder_Encode_BadStatus`
- `TestOllamaEmbedder_Encode_InvalidJSON`
- `TestOllamaEmbedder_Encode_Timeout`
- `TestOllamaEmbedder_Encode_ContextCanceled`

### retrieval

- original embedding が失敗したら `RetrieveContextUsecase.Execute` がエラーを返す
- expanded embedding が失敗しても original results が返れば retrieval は継続する
- BM25 と expanded embedding の双方が落ちても original vector search が生きていれば応答可能

## 実装方針の提案

今回の Ask Augur 対策としては、次の順が妥当。

1. `news-creator` 側の local GPU 競合を解消する
2. 同時に `rag-orchestrator` の embedder adapter と retrieval degrade 契約をテストで固定する
3. `EMBEDDER_TIMEOUT` を現行の 30 秒から見直すなら、ログ計測を入れたうえで決める

この順なら、主因の修正を遅らせずに embedder も手当てできる。

## 調査からの判断

### 今回の主因

- `news-creator` の local LLM 競合

### 今回の副次改善対象

- `rag-orchestrator` embedder adapter の観測性
- retrieval の degrade 契約テスト
- embedder timeout の妥当性確認

### 今回は見送ってよいもの

- embedder の大規模アーキ変更
- circuit breaker
- multi-remote failover

## 参照

- `rag-orchestrator/internal/adapter/rag_augur/ollama_embedder.go`
- `rag-orchestrator/internal/usecase/retrieval/expand_queries.go`
- `rag-orchestrator/internal/usecase/retrieval/embed_and_search.go`
- `rag-orchestrator/internal/usecase/retrieve_context_usecase.go`
- `rag-orchestrator/internal/adapter/rag_http/handler.go`
- `rag-orchestrator/internal/worker/worker.go`
- `rag-orchestrator/internal/infra/config/config.go`
- `compose/rag.yaml`
- `docs/ADR/000275.md`
- `docs/ADR/000036.md`
- `docs/services/rag-orchestrator.md`
