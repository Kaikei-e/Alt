# ポストモーテム: Acolyte レポートが search-indexer 認証化により空セクションになった問題

## メタデータ

| 項目 | 内容 |
|------|------|
| インシデントID | PM-2026-025 |
| 発生日時 | 2026-04-14 推定 (ADR-000722 マージ直後から) |
| 検知日時 | 2026-04-15 16:54 JST (本人指摘と突合) |
| 復旧日時 | 2026-04-15 17:15 JST |
| 影響時間 | 約 24 時間以上（インシデント潜伏期間 / 新規 report 生成時のみ顕在化） |
| 重大度 | SEV-3（Acolyte レポート本文が空になる機能劣化。他機能は正常） |
| 作成者 | インシデント対応担当（本セッション） |
| レビュアー | — |
| ステータス | Draft |

## サマリー

2026-04-14 に [[000722]] で search-indexer の REST `/v1/search` に `X-Service-Token` 強制が導入された。ところが acolyte-orchestrator の `SearchIndexerGateway` は認証ヘッダを送らない実装のままで、Gatherer 段階の全 variant search が **401 Unauthorized** になり、Acolyte レポート生成は「事実ゼロ」で黙って継続。UI にはレポートは表示されるが本文が空、という degradation が約 24 時間継続した。ユーザーからの「Acolyte が壊れた」報告を契機に、ログから `"Gatherer: variant search failed" error="401 Unauthorized"` と `"No claims for section, producing empty body"` を特定し、gateway に service token 付与を追加して復旧。

## 影響

- **影響を受けたサービス:** acolyte-orchestrator (Python / Starlette / LangGraph)
- **影響を受けたユーザー数/割合:** 少数（Acolyte レポートを閲覧するユーザー。実数は 1 名から確認、全員波及の可能性）
- **機能への影響:** 部分的劣化 — UI / Connect-RPC API は全て HTTP 200、`GetRunStatus` / `ListReportVersions` / `GetReport` も成功。ただし **生成される report の `analysis` 等のセクションが常に空** になっていた
- **データ損失:** なし（ただし**空セクションを含む report version が DB に永続化**された — append-first 設計のため消去不能、新 version 生成でしか上書きできない）
- **SLO/SLA違反:** 本機能には明示 SLO 未定義

### 特に問題のある点

- 症状が「500 エラー」「タイムアウト」といった目立つ失敗ではなく、**「値が埋まる場所に空文字が入る」静かな失敗**だった
- acolyte-orchestrator 自身のヘルスチェックは通り続け、監視は一切発報しなかった
- 空レポートも valid な `version_no` を持って保存されたため、過去の出力と区別が付きづらい

## タイムライン

| 時刻 (JST) | イベント |
|-------------|---------|
| 2026-04-14 深夜(推定) | [[000722]] が merge され search-indexer REST `/v1/search` に `X-Service-Token` 必須化のデプロイが反映 |
| 2026-04-14 以降 | Acolyte の全 report 生成で Gatherer 段階の variant search が `401 Unauthorized` を受ける。LangGraph は warning ログのみで空 fact plan を継続、レポートは空セクションで生成完了 |
| 2026-04-15 16:54 | ユーザーが Acolyte レポート閲覧中に「壊れた」と気づき報告（UI 表示は正常だが本文が空） |
| 2026-04-15 17:00 | **対応開始** — インシデント対応担当がスタック全体の健全性とログを横断確認。nginx / BFF / acolyte-orchestrator / alt-backend 全て 200 応答で「表層上は健全」と判定 |
| 2026-04-15 17:05 | **原因特定** — acolyte-orchestrator のアプリログから `"Gatherer: variant search failed" ... "401 Unauthorized" ... 'http://search-indexer:9300/v1/search'` を発見。直後に `"No claims for section, producing empty body"` の連続警告を突合し、Gatherer → search-indexer の認証欠落を根本原因として確定 |
| 2026-04-15 17:10 | **緩和策適用** — `acolyte/gateway/search_indexer_gw.py` の `__init__` で `settings.resolve_service_secret()` を取得し `X-Service-Token` ヘッダを全 `search_articles` 呼び出しに付与するパッチを適用。acolyte-orchestrator コンテナを `docker compose up --build -d --force-recreate` で再ビルド |
| 2026-04-15 17:13 | **復旧確認** — acolyte-orchestrator 内から `httpx.get http://search-indexer:9300/v1/search` を token あり / なしで実行。token 付きで 200、token なしで 401 を確認 |
| 2026-04-15 17:15 | acolyte-orchestrator / tls-sidecar 両方 healthy。次回以降の新規レポート生成から fact plan が埋まる状態 |

## 検知

- **検知方法:** ユーザー報告（「Acolyte が壊れた」）
- **検知までの時間 (TTD):** ≒ 24 時間以上（搭載時刻の記録が残っていないため推定）
- **検知の評価:**
  - Acolyte のヘルスチェックは「プロセスが生きているか」しか見ておらず、「レポート出力の品質」は検知できていない
  - nginx / BFF / acolyte-orchestrator / alt-backend のアクセスログはすべて 200 を返し続けた。HTTP 層で失敗を可視化する仕組みがなかった
  - acolyte-orchestrator のアプリ内 warning ログ (`"Gatherer: variant search failed"`) は出ていたが、ログを定点観測する運用も alert 化もなかった
  - 「UI は動く・裏で品質が落ちる」タイプのサイレント失敗で、エンドユーザーが気づくまで 24h 放置

## 根本原因分析

### 直接原因

`acolyte-orchestrator/acolyte/gateway/search_indexer_gw.py:43-46` の `search_articles` は `httpx.AsyncClient.get(...)` を **認証ヘッダ無し**で呼んでいた。search-indexer は [[000722]] 以降 `X-Service-Token` を必須とするため 401 を返したが、呼び出し側は `resp.raise_for_status()` を囲む例外ハンドラで warning ログを出すだけで、Gatherer は **空の ArticleHit リストで継続**する設計だった。

### Five Whys

1. **なぜ Acolyte のレポートが空セクションになったのか？**
   Gatherer 段階の variant search が毎回 0 ヒットを返していたため、Curator → Writer に渡る fact plan が空で、Writer が "No claims for section, producing empty body" に fallback した。

2. **なぜ variant search が 0 ヒットだったのか？**
   search-indexer が全 variant query に対して 401 Unauthorized を返し、`SearchIndexerGateway.search_articles` が例外→ warning 化→空リスト返却していた。

3. **なぜ search-indexer が 401 を返したのか？**
   [[000722]] で REST `/v1/search` に `ServiceAuthMiddleware.RequireServiceAuth` が必須化され、`X-Service-Token` ヘッダを付けていない呼び出しは 401 で拒否されるようになったが、acolyte-orchestrator の `SearchIndexerGateway` は token を送らない実装のままだった。

4. **なぜ acolyte-orchestrator は token を送らない実装のままだったのか？**
   `settings.resolve_service_secret()` は既に存在していて、Connect-RPC 経路の `AcolyteConnectService` 等では利用されていた。しかし `SearchIndexerGateway` は作成時期が [[000722]] 以前で、search-indexer が **未認証**で動いていた前提のコードだった。search-indexer 側の認証境界拡張は別トラックで進行し、**クライアント側の追従が別 PR で行われなかった**。

5. **なぜ search-indexer 側の変更にクライアントの追従漏れが起きたのか？**
   - contract-level の統合テスト（acolyte → search-indexer の実通信）が CI に組まれていなかった
   - search-indexer のマイグレーション ADR ([[000722]]) が「呼び出し側の更新」を明示チェックリスト化していなかった
   - acolyte-orchestrator / search-indexer の所有が別コンテキストで進行し、クロスサービスの契約変更アナウンスが点在
   - 401 になっても acolyte-orchestrator のコードが warning で握りつぶす設計になっており、**失敗が顕在化しない**ため回帰が検知されないままだった

### 根本原因

**サービス間 API の認証境界を片側だけ変更したときに、クライアント側の更新漏れを catch する仕組みが存在しなかった。** さらに、認証失敗 (401) を下流の呼び出し元が warning で握りつぶして処理継続する設計が、サイレント degradation を発生させた。

### 寄与要因

- acolyte-orchestrator のアプリ warning ログ量が多く (`Gatherer: variant search failed` は信号だが、周囲の INFO/WARNING で埋もれていた)
- `"No claims for section, producing empty body"` が「正常応答」のように処理され、空レポートを永続化する一貫したフォールバック経路が組まれていた（失敗のシグナルになり得ず）
- 本番で「空セクションのレポート」と「意図的に短いレポート」を区別する自動検証がなく、事実ゼロの生成が異常として扱われていなかった
- 全レイヤの HTTP アクセスログが `200 OK` だけを計上し、アプリケーション境界より内側の 401 はプラットフォーム可視化対象外だった

## 対応の評価

### うまくいったこと

- ユーザーの「Acolyte が壊れた」という 1 行の報告に対して、5 層（nginx / BFF / acolyte / alt-backend / search-indexer）のログを短時間で横断確認でき、**15 分以内に根本原因を特定**できた
- `SearchIndexerGateway.__init__` で token を一度解決してインスタンスに保持する設計にしたため、**コード変更行数が 8 行程度**で済み、機能回帰リスクが小さかった
- `settings.resolve_service_secret()` が既に存在していたため、新しい secret 配布や env 追加が不要だった
- acolyte-orchestrator の volume やインジェクションに変更を加えずに済んだ（`/run/secrets/service_secret` が既にマウント済）

### うまくいかなかったこと

- **インシデント検知がユーザー報告に依存した**。24h 以上、空レポートが生成され続けた
- [[000722]] がマージされた時点で、**クライアント側（acolyte, 他の search-indexer 呼び出し元）の更新漏れを検証するテスト**がなかった
- acolyte-orchestrator の `warning` レベルログがアラートに昇格していなかった
- 過去 24h に生成された report version は空のまま DB に残っており、**データ修復にはユーザー操作による再生成が必要**

### 運が良かったこと

- Acolyte の利用頻度が低めで、影響が目視可能な数ユーザーに留まった
- search-indexer 側は 401 で防御成功したため、**機密情報が漏洩する方向の失敗ではなかった**
- `GetFeedTags` / `RandomSubscription` 追加 (ADR-000729) の別作業中に徹底ログ分析を行う動機があり、今回と直交する視点で触発的に発見できた
- acolyte-orchestrator の `warning` ログの出力フォーマットが構造化 (JSON) されており、`grep` で即座に突き止められた

## アクションアイテム

| # | カテゴリ | アクション | 担当 | 期限 | ステータス |
|---|----------|-----------|------|------|-----------|
| 1 | 予防 | `acolyte-orchestrator/acolyte/gateway/search_indexer_gw.py` に `X-Service-Token` を付与する修正をデプロイ済み。テスト (`tests/contract/`) に search-indexer との CDC 契約テストを追加し、401 を検出可能にする | acolyte-orchestrator owner | 2026-04-22 | **DONE** ([[000736]] Phase D4: consumer test で `X-Service-Token` を `.with_header(...)` で pin、search-indexer provider 検証の除外解除) |
| 2 | 予防 | search-indexer のクライアント一覧（alt-backend、pre-processor、acolyte-orchestrator、tag-generator、recap-worker 他）を棚卸しし、**全クライアントが `X-Service-Token` を送っているか**を cross-check するスクリプトを CI に追加 | search-indexer owner | 2026-04-29 | **DONE** ([[000735]] で alt-backend / rag-orchestrator の consumer pact を追加、[[000736]] Phase E2 で `proto-contract.yaml` が `search-indexer` Provider 検証を PR blocking gate として実行) |
| 3 | 予防 | 以後、サービス間 API の認証境界を変更する ADR のテンプレートに「クライアント側の更新 PR リスト」チェックボックスを必須化 | platform/architecture owner | 2026-04-22 | TODO |
| 4 | 検知 | acolyte-orchestrator の `"Gatherer: variant search failed"` および `"No claims for section, producing empty body"` を Prometheus counter として expose し、閾値超で alert 発報 | acolyte-orchestrator owner | 2026-05-06 | TODO |
| 5 | 検知 | search-indexer `/v1/search` の `401 Unauthorized` をサービス別に集計する dashboard を Grafana に追加（正規クライアント以外からの 401 を異常として可視化） | observability owner | 2026-05-06 | TODO |
| 6 | 緩和 | Gatherer が連続して empty ヒットを返した場合、レポート生成を **"degraded" ステータス**で保存し、UI 側で視覚的に警告を出す。少なくとも「空レポートと正常レポートが区別つかない」状態を解消 | acolyte-orchestrator owner | 2026-05-13 | TODO |
| 7 | 緩和 | 本インシデントで生成された空レポートを識別し、ユーザーに再生成を促すバナーを UI に追加（または admin 側でバルク再生成） | acolyte-orchestrator owner + frontend owner | 2026-04-29 | TODO |
| 8 | プロセス | ポストモーテムから得られた知見を基に、サービス間契約変更の **ペア PR** ポリシー（契約側と呼び出し側を同一 PR または同一 Merge Train）を docs/runbooks に追記 | platform owner | 2026-04-29 | TODO |

### カテゴリの説明

- **予防:** 同種のインシデントが再発しないようにするための対策
- **検知:** より早く検知するための監視・アラートの改善
- **緩和:** 発生時の影響を最小化するための対策
- **プロセス:** インシデント対応プロセス自体の改善

## 教訓

### 技術的な教訓

- **サービス間契約変更は呼び出し側と同時にデプロイする**。片側だけ更新すると、クライアント側で沈黙失敗するパターンが発生する
- **認証失敗を warning で握りつぶして処理を継続する設計は危険**。401 は本来 fail-fast にすべきシグナル。degraded mode の実装には「degraded である」ことがユーザーに見える仕掛けが必須
- **外部呼び出しは認証ヘッダを「インスタンス生成時に一度解決」するパターンが安全**。今回のように各呼び出しで `token=self._headers` にしておけば、settings から解決済み・未解決の切替も容易
- HTTP 層の 200 OK はサービス健全性の指標として不十分。**アプリ境界（usecase / gateway）内の warning/error を可観測性に昇格**する設計が必要

### 組織的・プロセス的な教訓

- セキュリティハードニング ADR（例: [[000722]]）は往々にして「新しい防御層を立てる」ことに焦点が当たり、「既存呼び出し側の追従」が後回しになる。ADR template の「完了条件」に呼び出し側一覧と各更新 PR を明記する慣行が望ましい
- 「UI は動く・裏で品質が落ちる」タイプのサイレント失敗は**ユーザー報告まで 24h オーダー**で気づかれない。アプリ内部の warning を**必ずメトリクス化**し、SRE が定点観測できる足場を作ること
- アラート量を抑えたい誘惑でアプリ warning を握りつぶすと、今回のような事象が起きる。**warning は減らすのではなく、警告として機能する水準に調整**する
- 今回、別作業（ADR-000729 の Connect-RPC 移行 follow-up）の副産物として発見できたが、発見経路に構造的な再現性はない。意図せざる副産物に頼るのではなく、**定期的な health audit（週次 / スプリントごと）を runbook 化**する

## 参考資料

- 関連 ADR: [[000722]] search-indexer に ADR-000717 の認証境界を拡張し Critical/High/Medium/Low 全 10 件の脆弱性を閉じる
- 関連 ADR: [[000717]] alt-backend の Critical 認証欠陥を修正し /v1/internal/* と /v1/dashboard/* を閉じる
- 関連 ADR: [[000729]] user-facing REST を Connect-RPC へ移行し BFF outbound を全 mTLS 化する（本インシデントの発見トリガー）
- 修正コミット（本対応）: `acolyte-orchestrator/acolyte/gateway/search_indexer_gw.py` の `SearchIndexerGateway.__init__` と `search_articles` にサービストークン付与
- ログエビデンス: acolyte-orchestrator container logs, `"Gatherer: variant search failed" error="Client error '401 Unauthorized' for url 'http://search-indexer:9300/v1/search?q=..."`, `"No facts for section, using empty claim plan"`, `"No claims for section, producing empty body"`

---

> **Blameless Postmortem の原則:** このドキュメントは個人の過失を追及するためではなく、
> システムの脆弱性とプロセスの改善機会を特定するために作成されています。
> 「誰が悪いか」ではなく「システムのどこが改善できるか」に焦点を当ててください。
