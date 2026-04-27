# ポストモーテム: /admin/knowledge-home の Start Reproject が JSONB NOT NULL constraint で永続 502 だった潜伏バグ

## メタデータ

| 項目 | 内容 |
|------|------|
| インシデントID | PM-2026-040 |
| 発生日時 | 不明 — `knowledge_reproject_runs` schema 確立時から潜伏 (推定: knowledge-sovereign full-separation の実装完了時、本セッションより数週間〜数か月前) |
| 検知日時 | 2026-04-27 22:00 (JST) 前後 — 本番 push 後の operator 操作で初めて顕在化 |
| 復旧日時 | 2026-04-27 22:17 (JST) — fix commit `8d1c36f6f` ローカル landed (production 反映は別途 `git push origin main` → dispatch-deploy 経路を要する) |
| 影響時間 | 顕在化からローカル fix まで約 17 分。潜伏期間は数週間〜数か月の可能性 |
| 重大度 | SEV-4 (ヒヤリハット — admin operator の運用機能 1 つが利用不能だっただけで、エンドユーザ機能・データ・event log への副作用なし) |
| 作成者 | alt-backend / knowledge-sovereign 担当 |
| レビュアー | (Pending) |
| ステータス | Draft |

## サマリー

本番 `https://curionoah.com/admin/knowledge-home` の Knowledge Home reproject panel で `Start Reproject` ボタンを押すと `POST /api/admin/knowledge-home` が一律 502 (Bad Gateway) を返す状態が、`knowledge-sovereign` full-separation 後の `knowledge_reproject_runs` テーブル landed 以降ずっと潜伏していた。直接原因は `alt-backend` の `knowledge_reproject_usecase.StartReproject` が `domain.ReprojectRun` を構築する際に `CheckpointPayload` / `StatsJSON` / `DiffSummaryJSON` の 3 つの `json.RawMessage` フィールドを zero value (nil) のまま下流に流し、PostgreSQL の `NOT NULL DEFAULT '{}'` 列に NULL が送られ INSERT が constraint 違反で reject されていたこと。本ヒヤリハットは admin 運用機能のみに閉じておりデータ損失・エンドユーザ影響はない。usecase で empty JSON を seed するとともに、driver 側にも `emptyJSONIfNil` ガードを追加して同一クラスのバグの再発を構造的に塞いだ。

## 影響

- **影響を受けたサービス:** `alt-frontend-sv` の `/admin/knowledge-home` ページの Knowledge Home reproject panel (Start Reproject)。`alt-backend` の `KnowledgeHomeAdminService.StartReproject` RPC。`knowledge-sovereign` の `CreateReprojectRun` RPC。
- **影響を受けたユーザー数/割合:** admin role を持つ operator のみ。サービス全体の admin 権限保有者は限定的で、エンドユーザ (`/feeds` / `/loop` / `/augur` 等) には一切影響なし。
- **機能への影響:** Knowledge Home の shadow/swap reproject (compare → swap → rollback フロー) の起動が **完全停止**。compare / swap / rollback などの後続フローは Start Reproject によって生成される `knowledge_reproject_runs` row 前提なので連鎖的に使用不能。一方、本セッションで新設した Knowledge Loop reproject panel (`/admin/knowledge-home/reproject-loop`) は別経路 (knowledge-sovereign metrics port を直叩き) で正常稼働。
- **データ損失:** なし。`knowledge_reproject_runs` への INSERT が rollback されただけで、`knowledge_events` / `knowledge_loop_entries` / `knowledge_home_items` 等の本体 projection / event log は無傷。
- **SLO/SLA違反:** なし。admin 運用機能にユーザ影響 SLO は設定されていない。

## タイムライン

全時刻は JST。インシデント原因のコード混入時刻は不明 (履歴探索を要する); ここでは検知 → 復旧の本セッション内タイムラインを記録する。

| 時刻 (JST) | イベント |
|-------------|---------|
| 不明 | knowledge-sovereign full-separation で `knowledge_reproject_runs` 用 INSERT path が landed。`StartReproject` usecase が 3 つの JSONB 列を zero-value `json.RawMessage` のまま流す実装で commit。**潜伏バグ混入。** |
| 2026-04-27 20:21 〜 21:40 | 本日の Knowledge Loop work が landed (5689900a5 〜 0dfc7985d)。WhyMappingVersion 6 → 7 bump、Knowledge Loop reproject panel (新 admin UI) 追加、Loop の各種拡張。**いずれも `knowledge_reproject_runs` には触っていない。** |
| 2026-04-27 21:50 | ADR 6 本 (000858–000863) を含む `f7d61fc97` が origin/main に push され、dispatch-deploy 経路で production 反映。 |
| 2026-04-27 22:00 前後 | **検知** — operator が WhyMappingVersion v7 cutover を実行する目的で `/admin/knowledge-home` を開く。新 Loop reproject panel (TYPE "REPROJECT" 確認 + terracotta) と既存の Knowledge Home reproject panel (Mode / From Version / To Version) の両方が同居しており、operator は前者ではなく後者の `Start Reproject` を `fromVersion=v3, toVersion=v4, mode=dry_run` で押下。ブラウザ DevTools Network に `POST https://curionoah.com/api/admin/knowledge-home 502 (Bad Gateway)` を観測。 |
| 22:00 前後 | **対応開始** — 観測した 502 + Request body (`{action:"start_reproject", mode:"dry_run", fromVersion:"v3", toVersion:"v4"}`) を Claude session に共有し原因調査を依頼。 |
| 22:00 〜 22:15 | 経路を逆引き: BFF (`/api/admin/knowledge-home` POST handler) → alt-backend `KnowledgeHomeAdminService.StartReproject` → `knowledge_reproject_usecase.StartReproject` → `sovereign_client.CreateReprojectRun` → knowledge-sovereign `rpc_infra.CreateReprojectRun` → `sovereign_db.Repository.CreateReprojectRun` の INSERT。 |
| 22:15 前後 | **原因特定** — INSERT 文と migration `00001_initial_schema.sql` の schema を突合。`checkpoint_payload` / `stats_json` / `diff_summary_json` が `JSONB NOT NULL DEFAULT '{}'`、driver の `r.pool.Exec(...)` が 3 つの値を **明示的に** 渡している → PostgreSQL の DEFAULT は INSERT で値が省略された場合のみ発動するため nil が NULL として送られ NOT NULL 違反。 |
| 22:15 〜 22:17 | **緩和策適用 (恒久 fix)** — alt-backend usecase 側で 3 fields を `json.RawMessage("{}")` で seed。knowledge-sovereign driver 側にも `emptyJSONIfNil` helper を追加し INSERT / UPDATE 両 path で nil → `{}` を normalise。回帰テスト追加。`go test ./...` 全 green、`go vet` clean、`gofmt` clean。 |
| 2026-04-27 22:17 | **ローカル landed** — commit `8d1c36f6f` 作成。**production 反映は別途 push が必要**。 |

## 検知

- **検知方法:** ユーザー (admin operator) によるブラウザ DevTools Console / Network での 502 観測。
- **検知までの時間 (TTD):** バグ混入から **数週間〜数か月** (実時間)。本日の operator 操作からは数秒で検知された。
- **検知の評価:** 不十分。`POST /api/admin/knowledge-home start_reproject` を end-to-end で DB INSERT まで含めて検証する自動テスト (Hurl / integration) が存在せず、unit test が `mockCreateReprojectRunPort` で port を mock していたため DB constraint まで到達しなかった。本番でも admin の使用頻度が低く、何らかの観測ダッシュボードに 5xx がスパイクするほどには発生していなかった (= 単発の 502 がメトリクスでは飲み込まれていた)。

## 根本原因分析

### 直接原因

`alt-backend/app/usecase/knowledge_reproject_usecase/usecase.go` の `StartReproject` が `domain.ReprojectRun{...}` リテラルを構築する際、`CheckpointPayload` / `StatsJSON` / `DiffSummaryJSON` (型: `json.RawMessage`) を初期化せずに使用していた。Go の zero value は nil。これが proto byte field を経由して knowledge-sovereign の `r.pool.Exec(...)` に渡り、pgx driver は nil `json.RawMessage` を NULL として送信。PostgreSQL の `JSONB NOT NULL DEFAULT '{}'` 制約は INSERT で **値が省略された場合のみ** DEFAULT を発動するため、nil 値の明示送信に対して DEFAULT は適用されず `null value in column "checkpoint_payload" of relation "knowledge_reproject_runs" violates not-null constraint` エラーで INSERT が reject。これが Connect-RPC `CodeInternal` として全層 (knowledge-sovereign → alt-backend → BFF) を伝播し、最終的に BFF の汎用 catch が HTTP 502 にマップしてブラウザに到達した。

### Five Whys

1. **なぜ Start Reproject が 502 になったか？** → BFF が上流 RPC エラーを catch して 502 にマップしたため。
2. **なぜ上流 RPC がエラーになったか？** → knowledge-sovereign の `CreateReprojectRun` が PostgreSQL の NOT NULL constraint 違反を返したため。
3. **なぜ NOT NULL constraint に違反したか？** → JSONB 列に NULL が送られたため。`NOT NULL DEFAULT '{}'` は値の明示送信時には DEFAULT を発動しない。
4. **なぜ NULL が送られたか？** → Go の `json.RawMessage` を `domain.ReprojectRun` リテラル構築で初期化せず、nil 値が proto / driver を経由して PostgreSQL に届いたため。
5. **なぜ初期化されていなかったか？** → DB schema (NOT NULL DEFAULT '{}') と Go 構造体の zero value (nil) と proto の bytes (`[]byte(nil)`) と pgx の NULL semantics の 4 層が暗黙の契約で繋がっており、どの層も自分の責務として「他層が NULL を送らないように normalise する」役割を持っていなかった。proto bytes フィールドに `omitempty` の概念がなく `json.RawMessage` 型は型レベルでは nil を許容するため、コンパイラ / linter / unit test (port を mock) が誰一人として silent contract 違反を検出できなかった。

### 根本原因

「DB schema が NOT NULL DEFAULT を宣言したから初期化責務は DB 側にある」という見落としと、その見落としを **コンパイラ / linter / unit test / proto / pgx のいずれの層もカバーしない silent contract** が複合して成立していた。具体的には:

- DB schema 設計者は `DEFAULT '{}'` で「呼び出し側は省略してよい」と思った
- Go 実装者は zero value (nil) を「省略相当」と認識した
- しかし pgx (および PostgreSQL ワイヤープロトコル) は **値を明示的に NULL として送信** し、PostgreSQL は値が指定されたとみなして DEFAULT を発動しない
- 4 層 (DB schema / Go struct / proto bytes / pgx) すべてが個別には正しく動作しているが、4 層またがる契約は誰の責務として明示されていない

### 寄与要因

- **新旧 reproject panel の同居による operator 混乱**: 同セッション内で新設した Knowledge Loop reproject panel (TRUNCATE-and-rerun, WhyMappingVersion v7 用) と既存の Knowledge Home reproject panel (shadow/swap, projection_version v3/v4 用) が同じ `/admin/knowledge-home` の同じタブに同居していた。両者は概念が異なる別系統だが、UI 上の視覚的分離が弱く、operator は WhyMappingVersion v7 cutover の意図で誤って Knowledge Home reproject 側を押下した。これが潜伏バグを発火させるトリガになった。
- **使用頻度の低さ**: Knowledge Home shadow/swap reproject は projection_version の major bump 時のみ実行される運用機能で、日常的なヘルスチェック対象ではなかった。本日の v7 bump (Knowledge Loop 側の概念) を契機に operator が「reproject ボタンを探した」結果、初めて触られた。
- **integration test の不在**: `/api/admin/knowledge-home` の `start_reproject` action を DB INSERT までカバーする Hurl / integration test がなく、unit test 層で mock port にとどまっていた。
- **メトリクス上の silent failure**: 本番で散発的に発生していたとしても 502 のスパイクとして可視化されておらず、Grafana / Prometheus 上で異常として認識されていなかった。

## 対応の評価

### うまくいったこと

- ブラウザ DevTools Network の Request body 共有により、operator が押した action がピンポイントで `start_reproject` (mode=dry_run / fromVersion=v3 / toVersion=v4) と即座に特定できた。
- BFF → alt-backend → knowledge-sovereign → DB の 4 層を逆引きする調査が、コードベース内の grep + Read で完結した。production ログへの直接アクセスは不要だった (schema と INSERT 文の突合だけで semantic gap を特定できた)。
- 二層 fix (usecase で seed + driver で normalise) を採用したことで、将来 別経路の caller (migration backfill / 直接 DB / 新 RPC client) が同じ罠を踏む可能性を構造的に塞いだ。
- 回帰テスト `seeds_empty-object_JSON_into_checkpoint_payload_/_stats_json_/_diff_summary_json` を追加し、JSONEq 比較で空 JSON object が確実に seed されることを検証。同種バグの再発を CI で検出可能にした。
- 本日 landed させた Knowledge Loop reproject panel (b68553e7d / 0dfc7985d) は別経路で動作していたため、本来の v7 cutover (= Loop 側 reproject) は影響を受けず、operator が正しい panel を押せば即座に v7 cutover を完了できる状態が保たれていた。

### うまくいかなかったこと

- 潜伏期間が数週間〜数か月に及んだ可能性がある。production の admin 機能 1 つが恒常的に壊れていたが、誰も気づかなかった。
- 5xx スパイクのアラート / ダッシュボードが admin 系エンドポイントを十分にカバーしておらず、silent failure を構造的に検出できなかった。
- 同じ `/admin/knowledge-home` ページに 2 つの異なる概念の reproject panel を配置する UX デザイン判断が、operator の混同を誘発した。本セッションの新 panel 追加時にこの混同リスクを防ぐ設計レビュー (タブ分離 / 明示的な視覚分離) が十分でなかった。
- `NOT NULL DEFAULT '{}'` JSONB 列の Go 側送信パターンに対する project-wide guideline / lint rule が存在せず、同一クラスのバグが他のテーブル / 他の usecase に潜伏している可能性を未調査のまま残した。

### 運が良かったこと

- バグが admin 経路にとどまっており、エンドユーザ向け機能 (`/feeds` / `/loop` / `/augur` / `/recap` 等) の hot path には一切影響しなかった。
- データ整合性の毀損なし: `knowledge_reproject_runs` への INSERT が完全に rollback されていただけで、event log や本体 projection は無傷。reproject_run row が中途半端な状態で残ることもなかった (transaction 全体が constraint 違反で abort されるため)。
- operator が本セッションで新 Knowledge Loop reproject panel に気づいてそちら経由で v7 cutover を実行していたら、本ヒヤリハットは表面化しないまま潜伏が続いていた。混同したからこそ初めて検出された。
- 検知から fix まで 17 分。本セッション内で原因特定 → 二層 fix → 回帰テスト追加 → Phase 5 ローカル CI parity (lint / format / type / test) 完了まで一貫して実行できた。

## アクションアイテム

| # | カテゴリ | アクション | 担当 | 期限 | ステータス |
|---|----------|-----------|------|------|-----------|
| 1 | 予防 | `8d1c36f6f` を `git push origin main` し、dispatch-deploy 経路で production 反映 | alt-backend / knowledge-sovereign 担当 | 2026-04-28 | TODO |
| 2 | 予防 | `alt-backend` / `knowledge-sovereign` 全体で JSONB `NOT NULL DEFAULT` 列を grep し、Go 構造体 literal 経由で zero value のまま流しているパターンを横断 audit。同型バグが他テーブル (例: `knowledge_projection_audits.details_json`、各種 `*_payload` JSONB 列) に潜伏していないか確認 | alt-backend / knowledge-sovereign 担当 | 2026-05-04 | TODO |
| 3 | 予防 | PostgreSQL `NOT NULL DEFAULT` JSONB 列の Go 側送信に対する project-wide guideline を `docs/best_practices/go.md` に追記 (推奨パターン: domain layer で `json.RawMessage("{}")` を seed、driver layer で `emptyJSONIfNil` 等の helper で defense in depth) | alt-backend 担当 | 2026-05-04 | TODO |
| 4 | 検知 | `e2e/hurl/alt-backend/` に `/api/admin/knowledge-home start_reproject` を DB INSERT まで含めて検証する Hurl scenario を追加。既存 invariant 防波堤テスト (ADR-000859) と同じ層に積む | alt-backend / e2e 担当 | 2026-05-11 | TODO |
| 5 | 検知 | `/admin/*` 経路の 5xx を Grafana ダッシュボードに集約し、admin 系エンドポイントの error rate がベースラインを超えた場合に alert する rule を `observability/` 配下に追加 | observability 担当 | 2026-05-18 | TODO |
| 6 | プロセス | `/admin/knowledge-home` ページの UX を改善: Knowledge Home reproject (shadow/swap, `projection_version` v3/v4) と Knowledge Loop reproject (TRUNCATE-and-rerun, `WhyMappingVersion` v7) を視覚的に明確に分離 (タブ分離 or 大きな見出し + 概念差の説明)。両 panel の冒頭注記に「もう一方は別系統」の cross-reference を入れる | alt-frontend-sv 担当 | 2026-05-04 | TODO |
| 7 | プロセス | `WhyMappingVersion` bump 後の reproject 手順を `docs/runbooks/knowledge-loop-reproject.md` の冒頭に明記。「**Knowledge Home の Start Reproject ボタンではなく、`/admin/knowledge-home` の最下部の Knowledge Loop reproject panel から実行する**」を太字で先頭に置く | alt-backend / docs 担当 | 2026-04-30 | TODO |
| 8 | プロセス | 新 admin UI を追加する際は「既存 panel との混同リスク」を設計レビュー項目に追加。本ポストモーテムを参照する形で `docs/best_practices/` (なければ新設) に admin UI 追加チェックリストを作成 | alt-frontend-sv 担当 | 2026-05-11 | TODO |

## 教訓

### 技術的な教訓

- **PostgreSQL `NOT NULL DEFAULT` の semantics は直感に反する**: DEFAULT は「INSERT で値が省略された場合」のみ発動し、「値が NULL として送られた場合」には発動しない。Go の zero value (nil `json.RawMessage` / `[]byte(nil)`) を経由すると pgx は値ありとして NULL を送るため、DB 側の DEFAULT は無効化される。同種の罠は他言語 / 他 ORM-less driver でも普遍的に存在する。
- **Silent contract は層を跨ぐ**: DB schema / Go struct / proto bytes / pgx wire protocol の 4 層がそれぞれ単独では正しく動作していたが、4 層またがる契約 ("空 JSON は省略でよい") を誰の責務として明示するかが決まっていなかった。型システム / コンパイラ / linter / unit test (port mock) のいずれもこの semantic gap を検出できない。**型レベルで表現できない契約は、driver layer の defense-in-depth helper で吸収する**のが現実解。
- **Mock を多用する unit test は DB constraint バグを検出できない**: `mockCreateReprojectRunPort` は port を呼んだことだけを確認し、port が DB にどう書くかは見ない。**DB-touching な経路は integration test (Hurl / DB-backed go test) で end-to-end 検証** することがやはり必要。Test pyramid の各層には別の責務がある。

### 組織・プロセス的な教訓

- **使用頻度の低い admin 機能は腐る**: 日常的に触られない機能は、コードが landed した瞬間から degradation が始まる。CI / production の両方で「実際に動かす」テストが必要。本ケースでは整合した integration test が無かった + admin 経路 5xx のアラートが無かった、の二重欠落で潜伏が長期化した。
- **新 UI を追加する際の "concept proximity" リスク**: 同じ場所に異なる概念の操作を並べると、operator は label よりも視覚的な近さで操作を選びがち。本セッションで Knowledge Loop reproject panel を Knowledge Home reproject の真下に配置したことで、概念の異なる 2 つの reproject 操作の混同を生んだ。**新 panel 追加時は「既存 panel との混同で誤作動を起こすか」を必ず検討する** プロセスが要る。
- **混同が偶然 fix のトリガになった**: operator が "正しい" panel を押していたら本ヒヤリハットは検出されないまま潜伏が続いていた。**運が良かったケースを運用に依存させない**ためにも、Action Item #4 / #5 の検知層整備が重要。

## 参考資料

- 修正 commit: `8d1c36f6f` (`fix(reproject): seed empty JSON into knowledge_reproject_runs JSONB columns`)
- 関連 ADR: [[000863]] Knowledge Loop の full reproject を /admin/knowledge-home から実行できる admin endpoint と panel を追加し WhyMappingVersion を運用上 visible にする (本セッションで landed した、混同のもう一方の panel を導入した ADR)
- 関連 ADR: [[000861]] Surface Planner v2 signal 拡張 + WhyMappingVersion 6 → 7 bump (混同のトリガになった v7 cutover の根拠)
- 関連 ADR: [[000859]] canonical-contract 不変条件の防波堤テスト (Action Item #4 で同層に integration test を追加する先)
- 関連 schema: `knowledge-sovereign/migrations/00001_initial_schema.sql` の `CREATE TABLE knowledge_reproject_runs`
- 関連 runbook: [[knowledge-loop-reproject]]
- 関連ポストモーテム: [[PM-2026-039]] (本日の Knowledge Loop 関連の別ヒヤリハット。同じ `/admin/` `/loop/` 周辺の UX 課題が現れている)

---

> **Blameless Postmortem の原則:** このドキュメントは個人の過失を追及するためではなく、
> システムの脆弱性とプロセスの改善機会を特定するために作成されています。
> 「誰が悪いか」ではなく「システムのどこが改善できるか」に焦点を当ててください。
