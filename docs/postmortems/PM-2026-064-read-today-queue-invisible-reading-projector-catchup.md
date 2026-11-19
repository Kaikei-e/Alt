---
title: "ポストモーテム: reading_projector がシーケンス欠番ごとに tick を終了して追いつきに約 30 時間掛かる状態となり、Read Today キューが /today 画面に表示されなかった障害"
date: 2026-11-19
tags:
  - postmortem
  - knowledge-sovereign
  - reading-projector
  - alt-frontend-sv
---

# ポストモーテム: reading_projector がシーケンス欠番ごとに tick を終了して追いつきに約 30 時間掛かる状態となり、Read Today キューが /today 画面に表示されなかった障害

## メタデータ

| 項目 | 内容 |
|------|------|
| インシデントID | PM-2026-064 |
| 発生日時 | 2026-11-19 16:44:33 (JST)（本番で最初の `reading.queued.v1` イベントが追記された時刻。projector の追いつき遅延自体は 12:55 頃の反映完了時から開始） |
| 検知日時 | 2026-11-19 19:02 (JST)（利用者からの報告。アラートによる検知ではない） |
| 復旧日時 | 2026-11-19 20:59:27 (JST)（`reading_projector.catchup_completed` 出力） |
| 影響時間 | 4 時間 15 分（16:44:33〜20:59:27、最初のキュー追加操作から追いつき完了まで） |
| 重大度 | SEV-3（新機能 1 件が数時間利用不能。全操作はイベントログに保存されておりデータ損失なし） |
| 作成者 | プラットフォーム担当 |
| レビュアー | — |
| ステータス | Draft |

## サマリー

2026-11-19 に「今日読む（Read Today）」キュー（PR #369、[[001021]]）を本番へ出したところ、記事をキューに追加しても `/today` 画面に表示されない状態が 4 時間 15 分続いた。`QueueArticle` は `reading.queued.v1` イベントを追記して成功を返していたが、read model へ畳み込む新規の `reading_projector` がシーケンス 0 から約 113 万行の既存 `knowledge_events` を再生し始める際、ロールバック等で生じたシーケンス欠番 1 件につき 2 tick（10 秒、毎分 6 欠番）を要して追いつけずにいたためである。共通の欠番判定 `projection_gap` が 2 回の観測を要求する一方、`reading_projector` の `RunBatch` が欠番のたびに tick を打ち切っていたことが原因であり、修正（PR #375）なしでは残り約 29.7 時間を要する状態だった。実行中の古いトランザクションが無い場合に同一 tick 内で 2 回目の観測を行ってバッチループを継続する修正を反映した結果、欠番通過速度は毎分 228〜236 件へ改善し、起動から 47 分後の 20:59:27 に追いつきを完了して復旧した。

## 影響

- **影響を受けたサービス:** knowledge-sovereign（`reading_projector`、`reading_queue_items`、`reading_sources`）、alt-frontend-sv（`/today` 画面の Read Today キュー表示）
- **影響を受けたユーザー数/割合:** 全利用者（単一利用者の環境）
- **機能への影響:** 部分的劣化（新機能 1 件が利用不能）
  - 16:44:33〜20:59:27 の 4 時間 15 分にわたり、キューへの追加・解除操作はイベントとして保存されたものの read model（`reading_queue_items`）に到達せず、`/today` 画面が空（`Nothing queued.`）のまま表示された
  - 期間中に発生した `reading.queued.v1` 11 件（17:54:55 まで）およびその後の `reading.unqueued.v1` 3 件が復旧まで画面に反映されなかった
  - 修正を反映しなかった場合、19:02 時点で追いつき完了までさらに約 29.7 時間を要する見込みだった
  - `knowledge_home_projector`・`knowledge_trail_projector` や既存の未読一覧・記事閲覧には影響していない
- **データ損失:** なし。`QueueArticle` / `UnqueueArticle` が追記した全イベント（`reading.queued.v1` 11 件、`reading.unqueued.v1` 3 件）は `knowledge_events` に保持されており、追いつき完了後に `reading_queue_items` へ正しく反映された（有効 7 行、解除済み 1 行の計 8 行）
- **SLO/SLA違反:** なし（`reading-projector` の追従遅延に関する SLO は未定義）

## タイムライン

| 時刻 (JST) | イベント |
|-------------|---------|
| 12:31 | PR #369 をマージ（`09aad313a`）。Read Today キュー機能の本番向け変更が main に入る |
| 12:55 頃まで | リリースパイプラインによる本番反映が完了。`reading_projector` が既存約 113 万行の `knowledge_events` に対してシーケンス 0 から処理を開始 |
| 16:44:33 | 本番で最初の `reading.queued.v1` イベントが追記される（17:54:55 までに計 11 件、その後 `reading.unqueued.v1` 3 件）。`QueueArticle` は毎回成功を返したが、`reading_projector` が該当位置に到達していないため `reading_queue_items` は空のまま |
| 19:02 | **検知・対応開始** — 利用者から「Read Today への追加は動くが Today 画面に何も表示されない」と報告（最初のキュー操作から 2 時間 18 分後。アラートなし）。プラットフォーム担当が読み取り専用で調査（コンテナログおよび DB の確認）を開始。エラーは 0 件だが projector が大きく遅れており、欠番 1 件あたり 2 tick（5 秒 × 2 = 10 秒、毎分 6 欠番）しか進んでいないことを確認 |
| 19:02〜19:12 | 調査中の見直し: 当初、シーケンス進捗から残り時間を 35.7 時間と見積もったが、単位の誤りに気付き残存欠番数 × 10 秒で 29.7 時間に訂正。また「すべての読み取りで `xmin` が ceiling と等しい」という表現を、実測の 1,274 回中 1,253 回に訂正。利用者から推論ではなく実測に基づいて進めるよう指摘を受ける |
| 19:12 | **原因特定** — 利用者から修正の依頼。この時点までに根本原因を特定: `projection_gap` の欠番放棄規則が 2 回の観測を要する設計である一方、`reading_projector` が欠番のたびに tick を終了していた |
| 19:12〜19:40（時刻詳細は未計測） | 検証用の使い捨て PostgreSQL に本番 `knowledge_events` を読み取り専用 `COPY` で複製（約 113 万行を 18 秒で転送）し、`reading_projector` を直接動かして修正前後を実測。修正前は 200 tick で 99 欠番を通過、修正後は 697 tick で先頭に到達。実トランザクションでの 3 通りの writer ケース（欠番の下で開いたまま、後から commit、rollback）もすべて正しく動作することを確認 |
| 19:40 | PR #375 を作成（2 コミット: 失敗テスト `5ec1aef7f`、修正 `fa3c128b6`） |
| 20:01 | PR #375 をマージ。リリースパイプラインが 20:01:30〜20:14:03 で実行され成功（同日夕方の先行 2 回のリリース実行は別件 `PM-2026-065` の契約ゲートで停止していたが、本修正の反映はこの時刻範囲を超えて遅延させていない） |
| 20:12:16 | 修正版の `reading_projector` が起動。欠番の通過速度が毎分 6 件から毎分 228〜236 件へ向上 |
| 20:59:27 | **緩和・復旧** — ログ `reading_projector.catchup_completed` を出力し、起動から 47 分で追いつきを完了（原因除去と復旧が同時であり、別の緩和策は無し）。復旧後の実測で `reading_queue_items` 8 行（有効 7・解除 1、期限 `queued_at + 48h`）、`reading_sources` 631 行、3 projector（Home・Trail・Reading）すべての lag 0、`sequence_gap_waiting` 1 件の正常処理、エラー 0 件、CAS 拒否 0 件を確認（`/today` 画面の目視確認は利用者のログインが必要なため対応者では未確認） |

## 検知

- **検知方法:** 利用者からの報告（`QueueArticle` の操作は通るが `/today` に表示されない）
- **検知までの時間 (TTD):** 2 時間 18 分（最初の `reading.queued.v1` 追記 16:44:33 → 報告 19:02）
- **検知の評価:**
  - `observability/` に `reading-projector` の追従遅れ（lag）を監視するアラートやダッシュボードが存在しなかった（`observability/prometheus/rules/knowledge-home-slo-alerts.yml` に `alt_home_projector_lag_seconds` を参照するルールはあるが、`alt-backend/app/utils/otel/metrics_snapshot.go` の `MetricsSnapshot.RecordProjectorLag` に呼び出し元が無く実稼働で記録されておらず、新規 projector 向けの lag 監視も未整備）
  - `reading_projector` はエラーを返さず稼働を続け、`sequence_gap_waiting` を `Info` レベルで出力していただけであり、エラーログやコンテナの異常終了では気付くことができなかった
  - 修正前の `RunBatch` は `hole.Open()` でループを `return nil` で途中終了していたため、`MaxBatchesPerTick`（40 バッチ）を完走したときに呼ばれる `markBehind`（`reading_projector.catchup_started`）すら欠番に当たる tick では到達しにくい構造だった
  - 書き込み経路（`QueueArticle`）はイベント追記の成功を返し、`/today` 画面（`GetReadTodayQueue`）も投影遅れを示すメタ情報を持たず空状態（`Nothing queued.`）をそのまま返していたため、システム側から遅延が表に出なかった

## 根本原因分析

### 直接原因

`reading_projector` が既存約 113 万行の `knowledge_events` をシーケンス 0 から再生する際、ログ内に約 10,700〜10,900 件（推定。正確な実測総数は未計測）存在するロールバック等で焼けたシーケンス欠番を 1 件通過するたびに 2 tick（5 秒 × 2 = 10 秒）を消費し、最新のキューイベントに数時間（修正なしでは残り約 29.7 時間）到達できなかった。

### Five Whys

1. **なぜ `/today` 画面に記事が表示されなかったか？** → `QueueArticle` が追記した `reading.queued.v1` イベントを、`reading_projector` が read model（`reading_queue_items`）へまだ畳み込めていなかったから
2. **なぜ `reading_projector` が追いつけなかったか？** → シーケンス 0 から約 113 万行の `knowledge_events` を再生する途中で、シーケンス欠番 1 件を越えるのに 2 tick（10 秒、毎分 6 欠番）掛かっていたから（ログ内の焼けた欠番は推定約 10,700 件、正確な実測総数は未計測）
3. **なぜ欠番 1 件に 2 tick 掛かったか？** → 共通の欠番判定 `projection_gap.Tracker.MayAbandon` が「同じ欠番を 2 回観測し、2 回目のスナップショットで `HoleOpen == true` かつ `Xmin >= 1 回目の Ceiling` であること」を放棄条件としている一方、`reading_projector` の `RunBatch` が `ReadSequenceGapFrontier` を 1 tick に 1 回しか呼ばず、さらに欠番の手前を畳み込んだ直後（`if hole.Open() { return nil }`）に tick を終了していたから
4. **なぜその実装のまま本番に出たか？** → 既存の `knowledge_home_projector` と `knowledge_trail_projector` が同じ「1 欠番で tick を抜ける」形をしており、`reading_projector` も同じ型を踏襲した。既存 2 本は長くログ先頭付近で動いていたため大量の欠番を連続で越える遅延が表面化しておらず、単体テストも数件の欠番を含む fake で 1 tick の正しさだけを検証し、多数の欠番を越える tick 数を測っていなかったから
5. **なぜリリース前に追いつき時間の異常に気付けなかったか？** → 本番イベントログの複製を用いた新規 projector の追いつきリハーサルがリリース手順に無く、さらに開発ホスト上で `pgtest`（testcontainers）の PostgreSQL を起動できないため実 DB 境界の統合テストも回せていなかったから

### 根本原因

**常時先頭付近で追従している既存 projector の実装前提（「欠番は稀にしか来ないので 1 tick 1 観測・欠番で tick 打ち切りでよい」）を、シーケンス 0 から過去ログ全量を再生する新規 projector にそのまま持ち込み、本番相当のログ規模と欠番数で追いつき所要時間を検証せずに本番投入したこと。**

加えて、`reading_projector` の追従遅れを観測・通知するアラートと、読み出し側（`/today`）で投影の未到達を区別する手段が無かったため、出荷後 2 時間以上利用者の報告まで検知できなかった。

### 寄与要因

- **シーケンス欠番の蓄積:** `knowledge_events.event_seq` は `BIGSERIAL` で INSERT 時に採番されるため、ロールバックされたトランザクションでも連番が消費され欠番が残る（PostgreSQL 上の通常の挙動）。ログが約 113 万行に成長する間に約 1 万件以上の欠番が蓄積していた
- **実測と整合する単体テストの不在:** `reading_projector/projector_test.go` は数件の欠番のみを扱い、`RunBatch` の呼び出し回数（tick 数）の上限を検証していなかった
- **開発ホスト上の統合テスト不調:** `pgtest`（testcontainers）が開発ホスト上で PostgreSQL コンテナを起動できず、実 PostgreSQL のトランザクション ID とスナップショットを使った検証が普段回せない状態だった
- **兄弟 projector への影響潜在:** `knowledge_home_projector` と `knowledge_trail_projector` も同じ構造のため、同様の遅延を潜在的に抱えているが、運用中は先頭に追従していたため問題が隠れていた

## 失敗の型と再発

- 失敗の型: FP-07, FP-01, FP-16, FP-14
- 再発: PM-2026-061, PM-2026-062, PM-2026-060, PM-2026-041
- 持ち越し: なし
- 止められたはずの検査: (1) 多数の焼けた欠番を含むイベント列に対して `RunBatch` の必要 tick 数が有界であることを検証するユニットテスト（PR #375 の `projector_catchup_test.go` で追加）。(2) 本番イベントログの複製に対して新規 projector（または再投影）を実行し、先頭到達までの tick 数・実時間・欠番通過速度を測るリリース前リハーサル。(3) `reading-projector` のチェックポイント遅れを監視し閾値超過で発火するアラート

## 対応の評価

### うまくいったこと

- 19:02 の報告時点で原因を約 10 分で特定でき、コピー環境での再現と検証が迅速に進んだ
- 読み取り専用の調査と本番 `knowledge_events` の使い捨て PostgreSQL への `COPY`（約 113 万行を 18 秒で転送）により、本番環境へ書き込むことなく修正前後の実測（200 tick で 99 欠番 → 697 tick で先頭到達）と 3 通りの writer トランザクション試験を安全に実施できた
- 共通の安全性保証である `projection_gap.Tracker`（実行中 writer が保持中の欠番は絶対にスキップしない）を緩めることなく、`Xmin >= Ceiling` で古いトランザクションが無いと確認できた場合のみ同一 tick 内で 2 回目の frontier 読み取りを行う最小限の変更で安全に解決した
- TDD で失敗テスト（`5ec1aef7f`）を先に置き、実装修正（`fa3c128b6`）を積んで PR #375 としてマージ・反映できた
- イベント追記（append-only）設計により、遅延中の 11 件の追加・3 件の解除が 1 件も失われず、追いつき完了時に read model がそのまま正しい状態へ収束した

### うまくいかなかったこと

- 調査の初期段階で実測ではなく推論に頼り、復旧見込み時間（35.7 時間 → 実測ベースの 29.7 時間）と frontier の挙動（「すべて `xmin == ceiling`」→ 実測 1,274 回中 1,253 回）を 2 度訂正することになった
- 検知がリリース約 6 時間後（初回キュー操作から 2 時間 18 分後）の利用者報告に依存し、アラートで気付けなかった
- 対応者側では利用者ログインが必要なため、復旧後の `/today` 画面そのものの表示確認までは完了できなかった

### 運が良かったこと

- イベントログへの書き込み自体は正常に動いていたため、データ損失が一切起きなかった
- 同日夕方の先行 2 回のリリース実行が別件（`PM-2026-065`）で停止していたにもかかわらず、本修正 PR #375 のリリース実行（20:01:30〜20:14:03）はその影響で遅延せずに完了した

## アクションアイテム

| # | カテゴリ | アクション | 担当 | 期限 | ステータス |
|---|----------|-----------|------|------|-----------|
| 1 | 予防 | 兄弟の `knowledge_home_projector` と `knowledge_trail_projector` にも同一の「実行中 writer 無しの場合の同一 tick 内再観測・ループ継続」を適用するか方針を決定し、適用する場合は `projector_catchup_test.go` 相当の tick 数上限テストと合わせて実装する（現状は 0 からの再投影に約 30 時間掛かる） | knowledge-sovereign 担当 | 2026-12-03 | TODO |
| 2 | 予防 | 開発ホスト上で `pgtest`（testcontainers）の PostgreSQL コンテナが起動失敗する原因を調査・修復し、`go test -tags=integration` で `projection_gap` と各 projector の統合テストを実行可能にする | プラットフォーム担当 | 2026-12-03 | TODO |
| 3 | 検知 | `reading-projector`（および `knowledge-home-projector`・`knowledge-trail-projector`）のチェックポイント遅れ（最新 `event_seq` との差分および遅延時間）のメトリクスを公開し、`observability/` へアラートルールとダッシュボードを追加して通知到達まで確認する | 観測担当 | 2026-12-05 | TODO |
| 4 | 緩和 | `/today` 画面（`GetReadTodayQueue`）等で投影が追いついていない・遅延している場合に、空状態（`Nothing queued.`）と区別して反映待ちや遅延を表示する仕組みを設計・実装する | フロントエンド担当 | 2026-12-12 | TODO |
| 5 | プロセス | 過去ログの先頭から再生する新規 projector や全件再投影を本番へ出す前に、本番イベントログの読み取り専用コピーへ流して追いつき所要時間（tick 数・実時間・欠番通過速度）を事前計測する確認項目をリリース手順・runbook に追加する | 運用担当 | 2026-12-10 | TODO |
| 6 | プロセス | インシデント調査時に復旧見込みや DB 挙動を報告する際は、推論で断定せずコピー環境や実測クエリの結果を示してから回答する手順を対応ガイドラインに明記する | プラットフォーム担当 | 2026-11-28 | TODO |

### カテゴリの説明

- **予防:** 同種のインシデントが再発しないようにするための対策
- **検知:** より早く検知するための監視・アラートの改善
- **緩和:** 発生時の影響を最小化するための対策
- **プロセス:** インシデント対応プロセス自体の改善

## 教訓

- **「ログ先頭で動いている既存コード」の前提は、「シーケンス 0 から追いつく新規コード」では成り立たない。** 1 欠番あたり 10 秒のコストは平常時の追従では見えなくても、1 万件以上の欠番が溜まった過去ログを最初から再生した瞬間に約 30 時間の遅延になる。
- **単体テストの「1 ステップの正しさ」は「本番データ量での完了時間」を保証しない。** fake 上の数件のイベント・数件の欠番だけでなく、大量の欠番に対する tick 数・所要時間の上限をテストや複製ログでのリハーサルで確かめる必要がある。
- **書き込みの成功と read model の「空」が同じに見えると、遅延は無音になる。** CQRS 構成では projector の lag 監視が必須であり、エラーが 0 件でも「追いついていないこと」を観測できる信号が要る。
- **障害調査では推論を事実として語らず、実測で確かめる。** 本件でも、推論に基づく時間見積もりと `xmin` の挙動は実測によって誤りが明らかになった。使い捨て環境への `COPY` のような安全な実測手段を最初から使うことで、判断の精度と速度が上がる。

## 参考資料

- 判断記録: [[001021]]（「今日読む」キューをイベントから導く read model とし、上限 7 本・期限 48 時間・期限切れは未読に戻す）、[[001018]]（読書イベントと `reading_projector`・`reading_sources` の新設）、[[001022]]（`/today` 画面の新設）、[[000944]]（`knowledge-sovereign` のプロセス内 projector 設計）
- 関連ポストモーテム: [[PM-2026-061]]（本番相当データ分布をテストが再現しなかった例）、[[PM-2026-062]]（失敗や遅延を表に出さず画面が空・古い状態を出し続けた例）、[[PM-2026-060]]（不可視状態がエラーにならず無検知で続いた例）、[[PM-2026-041]]（projector の単体テストが本番形を見逃した例）
- 関連 PR / コミット: PR #369（マージ `09aad313a`、Read Today キュー導入）、PR #375（RED テスト `5ec1aef7f`、修正 `fa3c128b6`）
- 関連ファイル:
  - `knowledge-sovereign/app/usecase/reading_projector/projector.go`
  - `knowledge-sovereign/app/usecase/reading_projector/projector_catchup_test.go`
  - `knowledge-sovereign/app/usecase/projection_gap/gap.go`
  - `knowledge-sovereign/app/driver/sovereign_db/read_events.go`（`ReadSequenceGapFrontier`）
  - `knowledge-sovereign/app/usecase/knowledge_home_projector/projector.go`
  - `knowledge-sovereign/app/usecase/knowledge_trail_projector/projector.go`
  - `knowledge-sovereign/migrations/00035_create_reading_sources_and_queue.sql`
  - `alt-frontend-sv/src/routes/(app)/today/+page.svelte`

---

> **Blameless Postmortem の原則:** このドキュメントは個人の過失を追及するためではなく、
> システムの脆弱性とプロセスの改善機会を特定するために作成されています。
> 「誰が悪いか」ではなく「システムのどこが改善できるか」に焦点を当ててください。
