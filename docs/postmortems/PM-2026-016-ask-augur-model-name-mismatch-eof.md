# PM-2026-016: Ask Augur チャットストリームのモデル名不一致による即時 EOF 障害

## メタデータ

| 項目 | 内容 |
|------|------|
| インシデントID | PM-2026-016 |
| 発生日時 | 2026-04-03 14:37 (JST) |
| 復旧日時 | 2026-04-03 15:44 (JST) |
| 影響時間 | 約 1 時間 7 分 |
| 重大度 | SEV-3（主要機能の完全停止、ただし他機能への波及なし） |
| ステータス | Approved |

## サマリー

[[000613]]（Gemma3 → Gemma4 移行）のコミット後、rag-orchestrator コンテナがリビルドされなかったため、旧モデル名 `gemma3-4b-12k` を news-creator に送信し続けた。news-creator-backend（Ollama）に存在しないモデルへのリクエストとなり、接続後即座に EOF が返却された。Ask Augur のチャット機能が 2 回連続で完全失敗した。バッチ要約・検索・埋め込みなど他の機能には影響なし。

## 影響

- **影響を受けたサービス:** rag-orchestrator → news-creator（chat proxy パス）
- **影響を受けたリクエスト:** Ask Augur チャットリクエスト 2 件（2026-04-03 05:37-05:38 UTC）
- **機能への影響:** Ask Augur（RAG ベース対話型回答）の完全停止
- **データ損失:** なし
- **他機能への影響:** なし（バッチ要約、ベクトル検索、リランキング、記事取得はすべて正常稼働）

## タイムライン

| 時刻 (JST) | イベント |
|-------------|---------|
| 2026-04-03 13:29 | **トリガー**: [[000613]] コミット。Gemma3 → Gemma4 移行。モデル名が `gemma3-4b-12k` → `gemma4-e4b-12k` に変更 |
| 2026-04-03 時刻不明 | news-creator / news-creator-backend が再起動（Gemma4 モデルをロード）。rag-orchestrator は再起動**されず** |
| 2026-04-03 14:37 | **発生**: Ask Augur へのチャットリクエスト。rag-orchestrator が `gemma3-4b-12k` を送信 → news-creator proxy → news-creator-backend で「モデル不在」→ 即座に EOF |
| 2026-04-03 14:37 | rag-orchestrator ログ: `ollama_chat_stream_read_error`, `unexpected EOF`, `chunks_received: 0` |
| 2026-04-03 14:38 | 2 回目のリクエストも同様に失敗（EOF, 0 chunks） |
| 2026-04-03 15:40 | **検知**: ログ分析により根本原因を特定。rag-orchestrator の `model` フィールドが `gemma3-4b-12k`（旧名）であることを確認 |
| 2026-04-03 15:44 | **復旧**: rag-orchestrator コンテナをリビルド・再起動。`AUGUR_KNOWLEDGE_MODEL=gemma4-e4b-12k` を確認 |

## 検知

- **検知方法:** ユーザー報告（「Augur が 2 連続で失敗」）→ ログ分析
- **検知までの時間 (TTD):** 約 62 分
- **検知の評価:** Ask Augur の成功/失敗を監視するアラートが存在せず、ユーザー報告に依存した。PM-2026-008 の D-1 アクションアイテム（TTFT メトリクスとアラート）が未実装のまま再発

## 根本原因分析

### 直接原因

rag-orchestrator コンテナが Gemma4 移行コミット後にリビルドされず、旧モデル名 `gemma3-4b-12k` を news-creator に送信し続けた。news-creator-backend（Ollama）に該当モデルが存在しないため、即座にエラーレスポンスが返却された。

### Five Whys

1. **なぜ Ask Augur が EOF で失敗したか？**
   → news-creator-backend（Ollama）が `gemma3-4b-12k` モデルを見つけられず、エラーレスポンスを返却。news-creator proxy がこれを RuntimeError として処理し、StreamingResponse が即座に閉じられた

2. **なぜ存在しないモデル名が送信されたか？**
   → rag-orchestrator のコンテナが Gemma4 移行後にリビルドされておらず、compose の環境変数 `AUGUR_KNOWLEDGE_MODEL=gemma4-e4b-12k` が反映されていなかった

3. **なぜリビルドされなかったか？**
   → Gemma4 移行コミットで news-creator / news-creator-backend はリビルドされたが、rag-orchestrator のリビルドが漏れた。rag-orchestrator は Go サービスのため、環境変数の変更だけでなくバイナリの再コンパイルも必要

4. **なぜリビルド漏れに気づかなかったか？**
   → モデル移行時の「影響を受ける全コンテナのリビルドチェックリスト」が存在しなかった。rag-orchestrator は AI サービスではなくオーケストレーターのため、モデル変更の影響範囲として認識されにくかった

5. **なぜ影響範囲の認識が不十分だったか？**
   → rag-orchestrator が LLM モデル名を環境変数で受け取り、downstream に転送するという依存関係がドキュメント化されていなかった

### 根本原因

モデル移行時に影響を受ける全コンテナを特定・リビルドする運用プロセスが確立されていなかった。加えて、`raw=true` + 手動テンプレート構築という脆弱なプロキシアーキテクチャが、モデル変更時の障害リスクを増大させていた。

### 寄与要因

- **news-creator chat proxy の `raw=true` アプローチ**: `/api/generate` に直接送信するため、Ollama のモデル存在チェックのエラーメッセージが不明瞭（HTTP 200 → 即 EOF として伝播）
- **Ask Augur のモニタリング不在**: PM-2026-008 の D-1（TTFT アラート）が未実装のまま
- **rag-orchestrator の長時間起動**: 28 時間稼働中で、その間にモデル移行が行われた

## 対応の評価

### うまくいったこと

- rag-orchestrator の構造化ログ（`ollama_chat_stream_read_error`, `model` フィールド）により、モデル名不一致を迅速に特定できた
- 根本原因の特定後、rag-orchestrator のリビルドで即座に復旧できた
- 調査の過程で `raw=true` の構造的問題を特定し、`/api/chat` + `think=false` への根本的移行を実施できた

### うまくいかなかったこと

- Gemma4 移行時に rag-orchestrator のリビルドが漏れた
- 検知がユーザー報告に依存し、TTD が 62 分に達した
- PM-2026-008 のアクションアイテム D-1（TTFT メトリクス・アラート）が未実装のまま、同種の検知遅延が再発した

### 運が良かったこと

- 障害が Ask Augur のみに限定され、バッチ要約・検索・埋め込み等の他機能に波及しなかった
- rag-orchestrator → news-creator のリクエストが即座に EOF で失敗したため、タイムアウト待ちによるリソース消費が発生しなかった

## アクションアイテム

| # | カテゴリ | アクション | 担当 | 期限 | ステータス |
|---|----------|-----------|------|------|-----------|
| 1 | 予防 | news-creator chat proxy を `/api/chat` + `think=false` に移行（[[000614]]） | 開発担当者 | 2026-04-03 | **完了** |
| 2 | 予防 | rag-orchestrator コンテナをリビルドし `gemma4-e4b-12k` を反映 | 開発担当者 | 2026-04-03 | **完了** |
| 3 | 予防 | handler 非ストリーミングパスの `think` パラメータ転送欠落を修正 | 開発担当者 | 2026-04-03 | **完了** |
| 4 | 検知 | Ask Augur の成功率・TTFT メトリクスとアラートを追加（PM-2026-008 D-1 の再掲） | 開発担当者 | 2026-04-14 | TODO |
| 5 | プロセス | モデル移行時の影響コンテナリビルドチェックリストを作成 | 開発担当者 | 2026-04-14 | TODO |

## 教訓

### 技術的教訓

1. **`raw=true` は短期回避策であり恒久策ではない**: Ollama のテンプレートエンジンをバイパスすると、モデル固有のテンプレート形式の知識がアプリケーションコードに漏洩する。`/api/chat` + `think=false` で Ollama にテンプレート処理を委任するのが正しいアプローチ
2. **Go サービスの環境変数変更はリビルド必須**: compose の環境変数を変更しても、実行中のコンテナには反映されない。コンテナの再作成（`up -d`）またはリビルド（`up --build -d`）が必要
3. **即座の EOF は「モデル不在」のシグナル**: Ollama が接続直後に EOF を返す場合、リクエストされたモデルが存在しない可能性が高い

### プロセス的教訓

1. **モデル移行は AI サービスだけでなくオーケストレーターも影響を受ける**: モデル名を環境変数で参照する全コンテナを洗い出す必要がある
2. **未実装のアクションアイテムは負債になる**: PM-2026-008 で計画された TTFT アラートが未実装のまま、同種の検知遅延が再発した

## 参考資料

- [[000613]] Gemma3 → Gemma4:E4B Q4_K_M 移行 ADR
- [[000614]] chat proxy を `/api/chat` + `think=false` に移行
- PM-2026-008: Ollama パラメータ不一致によるモデルリロード遅延
- [Ollama Thinking docs](https://docs.ollama.com/capabilities/thinking)
- [GitHub #14793: generate API ignores think=false](https://github.com/ollama/ollama/issues/14793)

---

> **Blameless Postmortem の原則:** このドキュメントは個人の過失を追及するためではなく、
> システムの脆弱性とプロセスの改善機会を特定するために作成されています。
> 「誰が悪いか」ではなく「システムのどこが改善できるか」に焦点を当ててください。
