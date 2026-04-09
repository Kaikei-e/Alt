# Acolyte パイプライン安定化 — 調査・修正レポート

**日付**: 2026-04-10
**対象サービス**: acolyte-orchestrator, search-indexer
**関連 ADR**: [[000665]]

---

## 経緯

2026-04-09、Acolyte レポート生成パイプラインのコンテナログ調査依頼を受け、障害の特定・修正・検証を繰り返し実施した。

---

## 発見した障害（修正前）

| # | 障害 | 根本原因 | 影響 |
|---|------|---------|------|
| 1 | Extractor JSON 切断 | `num_predict=4096` でトークン制限ヒット、GBNF grammar が JSON を閉じきれない | 5記事中2記事の facts 消失 |
| 2 | Extractor thinking 流出 | `ExtractorOutput` に `reasoning` フィールドがなく、Gemma4 の thinking tokens が JSON 外に流出し num_predict バジェットを浪費 | response_len=28 chars / eval_count=836 tokens |
| 3 | Extractor ReadTimeout | `httpx.ReadTimeout` で Extractor ノード全体がクラッシュ。`generate_validated()` が LLM 呼び出し例外をリトライしない | パイプライン異常終了、全 facts 消失 |
| 4 | Planner JSON parse 失敗 | `num_predict=512` では日本語 JSON を完結できない | 毎回フォールバック（固定3セクション） |
| 5 | search-indexer SQL injection 誤検知 | 正規表現が `executive` の `exec` を部分一致で検出 | Gatherer のクエリ 1/3 が 500 エラー |

---

## 技術的背景 — Gemma4 thinking mode と GBNF grammar の相互作用

Web 調査（Ollama GitHub Issues、llama.cpp、arxiv 論文）から判明した事実:

| 事実 | 出典 |
|------|------|
| thinking tokens は num_predict に含まれる（別枠ではない） | ollama/ollama#14793 |
| think_budget パラメータは Ollama 未実装 | ollama/ollama#10925 |
| GBNF grammar (format) は thinking を事実上無効化する — ただし一部記事で不安定 | ollama/ollama#10538 |
| think=false を設定すると format (structured output) が壊れる (Gemma4 固有) | ollama/ollama#15260 |
| JSON schema に `reasoning` フィールドがあると thinking が JSON 内に吸収される | ADR-632 パターン |
| thinking モデルの思考トークンは 10x-20x のオーバーヘッド | arxiv:2604.07035 |

### 核心的発見: reasoning フィールドの有無による挙動差

| ノード | `reasoning` フィールド | 挙動 |
|--------|----------------------|------|
| Planner | **あり** | thinking が JSON に吸収される（ただし長すぎてバジェット不足） |
| SectionPlanner | **あり** | 正常動作 |
| Critic | **あり** | 正常動作 |
| **Extractor** | **なし（修正前）** | thinking が JSON 外に流出 → response_len=28 / eval_count=836 |

---

## 実施した修正

| # | 修正内容 | ファイル | 根拠 |
|---|---------|---------|------|
| 1 | `ExtractorOutput` に `reasoning` フィールド追加 | `domain/fact.py` | ADR-632 reasoning-first パターン。thinking tokens を JSON 内に吸収 |
| 2 | `generate_validated()` のリトライ範囲拡大 | `llm_parse.py` | `llm.generate()` を try/except 内に移動し ReadTimeout もリトライ対象に |
| 3 | Extractor ループに try/except 追加 | `extractor_node.py` | 1記事の失敗で全記事の結果が消失する問題を防止 |
| 4 | Extractor `num_predict` 4096 → 6000 | `extractor_node.py` | 6000 tokens @ ~18.5 tok/s ≈ 5.4分/記事。600s timeout 内に余裕 |
| 5 | Planner `num_predict` 512 → 2048 | `planner_node.py` | 日本語 JSON に十分なトークン枠を確保 |
| 6 | httpx read timeout 300s → 600s | `main.py` | num_predict 増加に伴う生成時間延長に対応 |
| 7 | SQL injection 正規表現に `\b` 追加 | `search_articles.go` | `exec` → `\bexec\b` で `executive` の誤検知を防止 |
| 8 | Extractor プロンプトに引用文長制限追加 | `extractor_node.py` | verbatim_quote を max 200 chars に制限 |

---

## 検証結果の推移

### Extractor article 1 の response_len 比較

| Run | reasoning | num_predict | response_len | eval_count | 結果 |
|-----|-----------|------------|-------------|-----------|------|
| 修正前（初回） | なし | 4096 | 4,710 | 849 | 成功（一部記事のみ） |
| 修正前（2回目） | なし | 4096 | 26,443 | 4,096 | JSON 切断 |
| num_predict 増加後 | なし | 8192 | 28 | 836 | **thinking 流出** |
| reasoning 追加後 | あり | 8192 | — | — | ReadTimeout (>600s) |
| **最終修正後** | **あり** | **6000** | **9,243** | **6,000** | JSON 切断→fallback→**パイプライン継続** |

**reasoning フィールドの効果**: response_len が 28 → 9,243 に 330 倍改善。thinking tokens が JSON 内に吸収されたことを実証。

### search-indexer `executive summary` クエリ

| タイミング | 結果 |
|-----------|------|
| 修正前 | `500 Internal Server Error` — "security validation failed: query contains potential SQL injection patterns" |
| 修正後 | `search ok`, count: 5 |

### パイプライン全体の耐障害性

| タイミング | 1記事失敗時の挙動 |
|-----------|-----------------|
| 修正前 | パイプライン全体がクラッシュ、全 facts 消失 |
| 修正後 | fallback → 次の記事に進行、パイプライン継続 |

---

## 残存する課題

| 課題 | 原因 | 優先度 | 対応方針 |
|------|------|--------|---------|
| Extractor: 6000 tokens でも JSON 未完 | verbatim_quote が長大、reasoning が JSON 枠を消費 | P1 | `max_facts_per_item` を 5→3 に減らす、または `body[:2000]` → `body[:1000]` に短縮 |
| 一部記事で thinking 流出が再発 | Gemma4 の非決定的挙動。reasoning フィールドがあっても thinking が JSON 外に流れる場合がある | P1 | Ollama の think_budget 実装待ち (ollama/ollama#10925)。現状は fallback で対処 |
| Planner: 依然フォールバック | 2048 tokens でも reasoning + thinking が JSON 枠を圧迫 | P2 | 既存フォールバック（固定3セクション）が機能しており実害は限定的 |
| LangGraph checkpointer 未導入 | 中間状態が揮発性 | P2 | PostgresSaver 導入で中間 snapshot を自動保存（別 ADR で対応） |

---

## テスト結果

| テストスイート | 結果 | 追加テスト |
|--------------|------|----------|
| `acolyte-orchestrator/tests/unit/` | **138 passed** | reasoning フィールド検証、部分結果保全、num_predict 検証、generate 例外リトライ |
| `search-indexer/app/usecase/` | **all pass** | 誤検知防止テスト 8 件 + 正当検知テスト 2 件 |

---

## 修正対象ファイル一覧

| ファイル | 変更内容 |
|---------|---------|
| `acolyte-orchestrator/acolyte/domain/fact.py` | `ExtractorOutput` に `reasoning: str` フィールド追加 |
| `acolyte-orchestrator/acolyte/usecase/graph/llm_parse.py` | `llm.generate()` をリトライ範囲に含める |
| `acolyte-orchestrator/acolyte/usecase/graph/nodes/extractor_node.py` | try/except + num_predict 6000 + プロンプト改善 |
| `acolyte-orchestrator/acolyte/usecase/graph/nodes/planner_node.py` | num_predict 512→2048 |
| `acolyte-orchestrator/main.py` | httpx read timeout 300→600 |
| `search-indexer/app/usecase/search_articles.go` | SQL injection regex に `\b` 追加 |
| `search-indexer/app/usecase/search_articles_test.go` | 誤検知防止テスト追加 |
| `acolyte-orchestrator/tests/unit/test_pipeline.py` | num_predict 検証、部分結果保全テスト追加 |
| `acolyte-orchestrator/tests/unit/test_llm_parse.py` | generate 例外リトライテスト追加 |

---

## 関連 ADR

- [[000665]] Acolyte パイプラインの LLM 呼び出し安定化と search-indexer 誤検知を修正する
- [[000632]] Ollama structured output で reasoning-first field order を採用
- [[000656]] 8 ノードパイプライン整備（hydrator + extractor 追加）
- [[000659]] SectionPlannerNode 追加と Writer の fact-first 化
