# ポストモーテム: news-creator の `/health` 契約変更で pre-processor の health gate が開かず、自動要約が 11 日間停止した障害

## メタデータ

| 項目 | 内容 |
|------|------|
| インシデントID | PM-2026-056 |
| 発生日時 | 2026-08-18 20:43 (JST)（バッチ経路による最後の要約生成。恒久化したのは 2026-08-20 13:13 JST の news-creator 新イメージ起動時） |
| 検知日時 | 2026-08-29 17:00 頃 (JST)（プロダクトオーナーが Knowledge Home の SUMMARIZING チップの滞留を目視で発見） |
| 復旧日時 | 未了（修正はワーキングツリーに実装済み・未コミット / 未デプロイ。pre-processor の再ビルドが必要） |
| 影響時間 | 未検知期間 約 10 日 20 時間（08-18 20:43 → 08-29 17:00）。自動要約の停止は 2026-08-29 時点で継続中 |
| 重大度 | SEV-2（主要機能である自動要約が全ユーザーに対して完全停止。データ損失なし・再生成可能） |
| 作成者 | pre-processor / プラットフォーム担当 |
| レビュアー | — |
| ステータス | Draft |

## サマリー

2026-08-29 17:00 頃 JST、Knowledge Home に取り込まれた新着記事が「SUMMARIZING」チップ（`knowledge_home_items.summary_state='pending'`）のまま何時間経っても ready に遷移しない、とプロダクトオーナーが目視で発見した。調査の結果、**pre-processor の自動要約ジョブ 3 本（`summarization` / `quality-check` / `queue-worker`）が、そもそもジョブとして登録されていなかった**ことが判明した。遅かったのではなく、存在していなかった。

原因は news-creator の `/health` のレスポンス契約の変更である。commit `9fdff3fa5`（2026-08-19 18:55 JST authored、コンテナ起動は 2026-08-20 13:13 JST）が `/health` を上流 I/O を伴わない安価な liveness に変え、Ollama とモデルの到達性を `/health/deep` に移した。一方 pre-processor の `CheckNewsCreatorHealth` は `/health` のボディに `models[]` が非空であることを健全性の条件としていたため、news-creator が完全に健全であるにもかかわらず health check は永久に失敗し続けた。`startHealthGatedJob` は `WaitForHealthy` が nil を返した後にしかジョブを登録しないため、health gate が開かないまま 3 本のジョブが一度も登録されなかった。

影響は 2026-08-19 09:02 JST 以降に取り込まれた 312 記事のうち **249 記事（79.8%）が未要約**、`knowledge_home_items` の pending 875 件（うち 872 件が 10 分以上滞留）。ユーザーが記事画面から明示的に叩くオンデマンドのストリーミング要約だけは別経路（alt-backend StreamSummarize → pre-processor `HandleStreamSummarize`）のため生き残っており、これが日次 2〜15 件の `SummaryVersionCreated` を発生させ続けたことで、パイプライン生存を見る唯一のゲージが健全に見え、11 日間アラートが一度も鳴らなかった。修正は 2026-08-29 18:00 頃 JST に TDD で実装済みだが、コンパイル言語のためイメージ再ビルドが必要で、本ドキュメント作成時点で本番復旧は未了である。

## 影響

- **影響を受けたサービス:** pre-processor（`summarization` / `quality-check` / `queue-worker` ジョブ）、alt-frontend-sv（Knowledge Home の要約抜粋 / SUMMARIZING チップ）、knowledge-sovereign（`SummaryVersionCreated` の生成停止）
- **影響を受けたユーザー数/割合:** 全ユーザー（単一ユーザーのセルフホスト構成）。自動要約を利用する全経路が対象
- **機能への影響:** 主要機能の完全停止
  - **自動要約:** 2026-08-19 09:02 JST 以降に取り込んだ **312 記事のうち 249 記事（79.8%）が未要約**。日次カバレッジは 08-18 の 100%（30/30）から 15〜33% に低下し、残ったカバレッジはすべてオンデマンド経由
  - **Knowledge Home:** `knowledge_home_items`（projection v7）の pending 合計 **875 件**、うち 249 件が 08-19 以降の生成分、**872 件が 10 分以上滞留**。検知直前の 3 時間では 16:48〜16:50 JST に取り込んだ 6 記事のうち 5 件が 25 分後も pending
  - **today_digest:** `unsummarized_articles` カウンタが実態を反映して増加し続ける状態
  - **quality-check ジョブ:** 同じ health gate に阻まれ未登録（要約品質の再チェックが 11 日間ゼロ）
- **影響を受けなかった機能:** RAG（outbox 経路が独立）、タグ付け（`TagSetVersionCreated` は 36 件中 34 件でカバレッジ維持）、検索、recap、オンデマンド要約（10 秒未満で成功）
- **データ損失:** なし。`articles` と `knowledge_events` は無傷で、要約は再生成可能。イベントログ上の欠落は「作られなかった `SummaryVersionCreated`」だけであり、破壊された記録はない
- **SLO/SLA違反:** 判定不能。要約カバレッジに関する SLI / アラートが存在しない（それ自体が本障害の主要な教訓）
- **切り分けで健全と確認された領域:** alt-db（`outbox_events` は pending 0 / failed 0、ship lag 3 秒、`pg_stat_activity` にブロックなし、pgbouncer にプール待ちなし）、knowledge-sovereign（projector checkpoint が `1406364` = `max(event_seq)` に一致、lag 0、dedupe drop 0、空 `summary_text` イベント 0）、news-creator / Ollama / GPU（`/health/deep` は `{"status":"pass"}`、VRAM 4800/8188 MiB、GPU 使用率 0%）、検知 6 時間前の alt-backend 系デプロイ（RestartCount=0、因果関係なし）

## タイムライン

すべて JST（括弧内は UTC）。

| 時刻 (JST) | イベント |
|-------------|---------|
| 08-18 20:43:39 | `summary_versions.model='pre-processor'` の最終行。**バッチ経路による最後の要約**（last known good） |
| 08-19 09:02 | 取り込みのギャップ明けに記事の取り込みが再開。以降、バッチ経路の要約は 1 件も生成されない（この時点での機構は後述の寄与要因 3） |
| 08-19 18:55 (09:55Z) | **トリガー（契約変更）** — commit `9fdff3fa5` が authored。`/health` から `models[]` が消え、モデル到達性は `/health/deep` に移動。イメージは 18:59 JST にビルド |
| 08-20 01:27〜02:19 | Ollama 0.32.14 への bump と Gemma4 のモデル参照 / 整合性チェック関連コミット群 |
| 08-20 13:13 (04:13Z) | **発生（恒久化）** — news-creator が新イメージで起動。以降 pre-processor の health check は永久に失敗する状態が確定 |
| 08-24 00:47 (08-23 15:47Z) | pre-processor がイメージ `sha-4c55d7e` で再作成される。health gate は開かず、3 ジョブは登録されないまま |
| 08-26 06:47:18 (08-25 21:47Z) | redis stream `alt:events:articles` の最終エントリ。以降 mq-hub は無音（寄与要因 1） |
| 08-29 10:56〜11:07 | alt-backend / harvester / data-hub / frontend / notifier の定例デプロイ（本障害とは無関係） |
| **08-29 17:00 頃** | **検知** — プロダクトオーナーが Knowledge Home で SUMMARIZING チップが 15〜20 分滞留しているのを目視で発見。アラートは 1 通も発火していない |
| 08-29 17:00 頃 | **対応開始** — read-only の並列調査 4 系統（コンテナログ / alt-db / knowledge-sovereign-db / コードトレース）を起動 |
| 08-29 17:10 (08:10Z) | ClickHouse `otel_error_logs` に pre-processor の `still waiting for news creator health, retrying` が直近 3 時間で 108 件（36 件/時 × 3、ジョブ 3 本 × 5 分周期）記録されているのを確認。仮説形成 |
| 08-29 17:25 頃 | **原因特定** — news-creator の `/health` ボディに `models` が存在しないことを確認。pre-processor の起動ログで 3 ジョブすべてが health 待ちのままであることを確認。同時に `/health/deep` が `{"status":"pass","checks":[{"name":"ollama","status":"pass","critical":true,"latency_ms":4}]}` を返し、news-creator 自体は完全に健全と判明 |
| 08-29 17:45〜18:00 | 4 系統の調査が同一結論に収束。alt-db / knowledge-sovereign / news-creator を証拠付きで容疑から除外。同時に「primary path」とされていた mq-hub 経路が実際には死んでいることを発見（寄与要因 1） |
| 08-29 18:00〜18:10 | **修正実装** — TDD（RED 8 ケース → GREEN）で `CheckNewsCreatorHealth` を `/health/deep` に切り替え。ローカルの CI parity（`go test -race` / `go vet` / `gofmt` / `golangci-lint` / `gosec` / `go mod tidy`）すべて green |
| — | **緩和策適用** — 未了。pre-processor のイメージ再ビルドとデプロイが必要 |
| — | **復旧確認** — 未了。gate が 10 秒以内に開くこと、`EnqueueUnsummarizedBatch` が 5 分周期で回り始めること、249 記事のバックログが解消することの確認をもってクローズする |

## 検知

- **検知方法:** プロダクトオーナーの目視（Knowledge Home の SUMMARIZING チップが ready に変わらないことに気付いて通報）
- **検知までの時間 (TTD):** 約 10 日 20 時間（バッチ要約の停止 08-18 20:43 から起算）。契約変更が恒久化した 08-20 13:13 から起算しても約 9 日 4 時間
- **検知の評価:** 機能していない。以下の 4 点がそれぞれ独立に検知を殺していた。

  1. **生存判定のゲージが producer を区別しない。** knowledge-sovereign の exporter は `knowledge_event_last_occurrence_age_seconds{event_type="SummaryVersionCreated"}` を出しているが、これは「要約が誰かによって作られてから何秒経ったか」しか見ない。生き残ったオンデマンド経路が日次 2〜15 件のイベントを出し続けたため、このゲージは 14 日間の最大でも 28.3 時間、通常は 6 時間未満で推移し、**歴史的に全体の 99% を占めていたバッチ producer（`summary_versions.model='pre-processor'`）が 11 日間死んでいる事実を完全に覆い隠した**。
  2. **そのゲージはアラートの主語ですらない。** `knowledge-loop-rules.yml` での唯一の用途は recap 停止アラートの分母（`SummaryVersionCreated < 21600` を「パイプラインは生きている」条件として join する）であり、要約そのものを主語にしたアラートは存在しない。
  3. **Knowledge Home 側にカバレッジのアラートがない。** `knowledge-home-slo-alerts.yml` には 9 本のアラートがあるが、可用性バーンレート・projection lag・空応答率・why 欠損率などが対象で、**要約カバレッジや `pending` の滞留時間を見るものは 1 本もない**。ユーザーが実際に見ていた症状（チップが変わらない）を表す指標が監視側に存在しなかった。
  4. **失敗が ERROR ログとして正常に出続けていた。** health gate のリトライループは `still waiting for news creator health, retrying` を 5 分ごと、ジョブごとに出し、10 秒ごとの `news creator service is up but no models are loaded` は 6 時間で 6480 行に達していた。ERROR は 24 時間の ClickHouse 保持期間の全域で 36 件/時の一定レートで出続けていたが、**「ジョブが一度も登録されていない」という状態を表す別種のシグナルはなく**、無限にリトライするログは定常ノイズと区別できなかった。

## 根本原因分析

### 直接原因

pre-processor の `CheckNewsCreatorHealth`（`pre-processor/app/service/health_checker.go`）が `GET /health` のレスポンスボディに `models[]` が非空であることを健全性の条件としていたが、news-creator の `/health` は commit `9fdff3fa5` 以降 `{"status":"healthy","service":"news-creator"}` のみを返すようになり、`models` フィールド自体が消滅した。結果として health check は常に「モデルが 0 個」と判定し、`startHealthGatedJob` の gate が永久に閉じたままとなり、`summarization` / `quality-check` / `queue-worker` の 3 ジョブが一度も job group に登録されなかった。

### Five Whys

1. **なぜ Knowledge Home の記事が SUMMARIZING のままだったのか？**
   → `knowledge_home_items.summary_state='pending'` を ready に変える `SummaryVersionCreated` イベントが発生していなかったため。要約が作られていない。

2. **なぜ要約が作られなかったのか？**
   → 自動要約の 3 ジョブ（5 分周期の enqueue sweep `EnqueueUnsummarizedBatch`、10 秒周期の dequeue worker、quality-check）が pre-processor に**ジョブとして登録されていなかった**ため。処理が遅かったのではなく、実行主体が存在しなかった。

3. **なぜジョブが登録されなかったのか？**
   → `startHealthGatedJob` は `WaitForHealthy(ctx)` が nil を返した後にのみ `register()` を呼ぶ設計であり、health check が一度も成功しなかったため。gate の外側にジョブは生まれない。

4. **なぜ health check が成功しなかったのか？**
   → checker が news-creator の `/health` に `models[]` を要求していたが、provider 側がその契約を変更し、`/health` を上流 I/O なしの liveness に、モデル到達性を `/health/deep` に分離したため。**news-creator は全期間を通じて健全であり、健全性を伝える口が変わっただけだった。**

5. **なぜ provider の契約変更が consumer を壊したまま 11 日間気付かれなかったのか？**
   → `/health` の consumer が誰であるかを機械的に検証する仕組みが存在しないため。news-creator には consumer 契約テスト（Pact）がなく、`/health` の形状に依存しているサービスを列挙する手順も PR チェックリストもない。変更は provider のリポジトリ内では完結して見え、CI もすべて green だった。

6. **なぜ壊れたことがランタイムでも分からなかったのか？**
   → 失敗が「登録されないジョブ」という**不在**として現れ、ログ上は無限リトライという定常状態に見えたため。加えて監視側は「要約イベントが最近発生したか」しか見ておらず、生き残った別 producer（オンデマンド要約）がその条件を満たし続けた。**壊れた producer と生きている producer が同じメトリクスに混ざっていた。**

### 根本原因

**health エンドポイントのレスポンス形状が、テストもドキュメントもされないままサービス間契約として機能していた。** provider（news-creator）は `/health` を liveness に純化し、依存の到達性を `/health/deep` に分離するという妥当な改善を行ったが、その形状に依存する consumer（pre-processor）が存在することを検証する手段がリポジトリのどこにもなかった。さらに consumer 側は、この契約の充足を「機能そのものの起動条件」に据えていたため、契約の破れが**性能劣化ではなく機能の不在**として現れ、無限リトライのログに埋もれた。

### 寄与要因

#### 1. 「primary path」とされていたイベント駆動経路が、そもそも動いていなかった

`pre-processor/app/handler/job_handler.go` は 5 分周期の sweep に `Fallback safety net; primary path is event-driven via ArticleCreated events` というコメントを付けている。しかし実際には:

- mq-hub に `ArticleCreated` を publish する唯一の実装は data-hub の `DataHubService.CreateArticle`（`alt-backend/app/dataplane/connect/datahubapi/handler.go`）だが、**これは現行の取り込み経路上にない**。実際の取り込みは alt-backend の `fetch_article_usecase` → `alt_db/save_article_driver.go` が `articles` と `outbox_events`（`ARTICLE_UPSERT`）に書き、harvester の outbox worker が RAG upsert と knowledge-sovereign への `ArticleCreated` を行う経路で、mq-hub を一切通らない。
- redis stream `alt:events:articles` の最終エントリは 2026-08-26 06:47:18 JST。保持されている 10000 件はすべて `ArticleUpdated` で、pre-processor の `consumer/event_handler.go` はこれを Debug レベルで黙って捨てる（`default:` 節の unknown 扱い）。
- `PublishSummarizeRequested` は port / gateway / driver / mock に実装が揃っているが、**production の呼び出し元が 0 件**の dead wiring である。

つまり「fallback」と書かれた 5 分周期の sweep が実際には**唯一の自動 enqueue 経路**であり、health gate の失敗はそれをそのまま「自動要約の 100% 停止」に変換した。冗長性があるという記述と実態が乖離していたことが、被害を部分劣化ではなく全停止にした。

#### 2. 監視の死角（詳細は「検知」セクション）

producer 別に分解されないゲージ、要約カバレッジのアラート不在、`pending` 滞留時間の未監視、そして「ジョブが登録されていない」ことを表すシグナルの不在。CLAUDE.md のルール 8 が禁じる silent fallback と同型の形——**「意図的に無効」と「配線が壊れている」が外から区別できない状態**——が、DI ではなくリトライループの中に現れていた。

#### 3. 契約変更より前に始まっていた原因不明の初期崩壊（確度: 中）

記事→要約のカバレッジは 08-18 に 100%（30/30）だったのが 08-19 には 19%（21 記事 / 4 要約）に落ちており、これは新しい news-creator コンテナが起動した 08-20 13:13 JST よりも前である。バッチ経路の最後の要約も 08-18 20:43 JST で、契約変更の効力発生より前に止まっている。

仮説として、`9fdff3fa5` 以前の `/health` は `list_models()` が失敗すると `models: []` を返す実装だった（例外を握って `error: "ollama_unavailable"` を足しつつ `status: healthy` を返す）ため、08-19〜08-20 に集中した Ollama 0.32.14 への bump と Gemma4 のモデル参照 / 整合性チェックの変更が、**同じ「models が空」というシグネチャを別の機構で発生させていた**可能性がある。だとすれば、一時的な障害として自然回復し得たものが、契約変更によって恒久化したことになる。当時のログは ClickHouse の保持期間（24 時間）を超えており、確証は得られていない。

## 対応の評価

### うまくいったこと

- **イミュータブルなイベントログが「どこで止まったか」の証明を自明にした。** `summary_versions.model` 別の件数と `knowledge_events` の type 別カウントを引くだけで、「タグは付いている / 要約だけが無い」「オンデマンドだけが生きている」が数分で確定した。状態フラグではなくイベントで持っていたおかげで、遡及的な原因究明が可能だった。
- **4 系統の並列 read-only 調査が約 1 時間で単一の原因に収束した。** ログ / alt-db / knowledge-sovereign-db / コードトレースが独立に進み、互いの結論を突き合わせる形で alt-db・projector・news-creator を証拠付きで消去できた。本番への変更はゼロ。
- **projector と checkpoint の設計が健全だったため、リプロジェクションが不要と即断できた。** checkpoint が `max(event_seq)` に一致し、pending の各アイテムが `ArticleCreated` と `TagSetVersionCreated` を持ちながら `SummaryVersionCreated` だけを欠くことが確認できたため、「投影の壊れ」ではなく「上流の不在」と切り分けられた。
- **修正が TDD で入った。** 先に 8 ケースの RED（リクエストパスの記録、`pass` / `warn` / `fail` / フィールド欠損 / 未知の status / 404）を書いてから実装しており、同じ契約破れが再発すればテストが先に落ちる。ローカルで CI parity 全段（`go test -race` / `go vet` / `gofmt` / `golangci-lint` / `gosec` / `go mod tidy`）を通してある。
- **`/health` と `/health/deep` の分離という provider 側の設計自体は正しかった。** compose の liveness probe が毎回 Ollama を叩かないようにする改善であり、この障害は設計の誤りではなく移行の取りこぼしである。修正も設計を戻すのではなく consumer を新しい契約に合わせる方向で入った。

### うまくいかなかったこと

- **検知が完全に目視依存だった。** アラートは 1 通も発火せず、11 日分の ERROR ログが誰にも読まれずに流れていた。ユーザーがたまたま Knowledge Home のチップに注意を向けなければ、さらに長期の潜伏が続いていた。
- **11 日分の一次証拠が失われた。** ClickHouse の保持期間が 24 時間のため、初期崩壊（寄与要因 3）の機構が特定できず、「確度: 中」の仮説で止めざるを得なかった。
- **コード内のコメントが実態と食い違っていた。** 「primary path is event-driven」という記述を信じると、sweep の停止は冗長経路の 1 本喪失に見える。実際にはそれが唯一の経路だった。コメントが調査の初期仮説を一度誤らせた。
- **復旧がコンパイル・再ビルドを要する。** 原因特定から修正実装までは約 1 時間だったが、Go サービスのため設定変更やコンテナ再起動では戻せず、本ドキュメント作成時点で本番はまだ止まっている。
- **`quality-check` ジョブの 11 日間の停止に、検知の瞬間まで誰も気付いていなかった。** 同じ gate に阻まれていたにもかかわらず、その機能の欠落を示す症状は一切観測されなかった。この機能には「動いているか」を判定する手段が実質的に存在しない。

### 運が良かったこと

- **オンデマンド要約が別経路で生き残っていたため、サービスは劣化状態で使い続けられた。** ユーザーは読みたい記事を明示的に要約させることができ、体験は「不便」に留まり「使えない」には至らなかった。
- **同じ幸運が検知を殺した。** 生き残った 1 種類の producer がパイプライン生存ゲージを健全に保ち続けた。**ユーザーにとっては幸運、検知にとっては不運**という、同じ事実の両面である。設計として意図されたフェイルオーバーではなく、たまたま経路が違っただけだった。
- **データが失われなかった。** 記事本体もイベントログも無傷で、要約は再生成可能である。もし要約の生成が記事の取り込みと同一トランザクションに結合していたら、11 日分の取り込み自体が失われていた可能性がある。
- **バックログの解消が構造的に可能だった。** `ListUnsummarizedArticles` は `created_at DESC, id DESC` のキーセットページングで時間窓のフィルタを持たないため、gate が開けば新しい順に自然に消化される。時間窓付きのクエリだったら、11 日分は永久に取り残されていた。

## アクションアイテム

### 予防（Prevent）

| # | アクション | 担当 | 期限 |
|---|-----------|------|------|
| P1 | news-creator の `/health` / `/health/deep` のレスポンス形状に対する契約テストを入れる。理想は pre-processor を consumer とする Pact CDC、最低でも provider 側で pre-processor が依存するフィールド（`status` の語彙 `pass` / `warn` / `fail` と HTTP status の対応）を固定する provider テスト | news-creator 担当 / pre-processor 担当 | 2026-09-05 |
| P2 | health gate 付きジョブの登録失敗を可視化する。N 分（例: 10 分）経っても登録に至らないジョブについて、定常リトライログとは別種の `*_job_unregistered` シグナルを出すか、起動失敗として fail-fast する。CLAUDE.md ルール 8 の「配線されていない依存を無言でスキップしない」を、DI だけでなくリトライループにも適用する | pre-processor 担当 | 2026-09-05 |
| P3 | mq-hub の「primary path」を実態に合わせる。現行の取り込み経路（`save_article` / outbox）に `ArticleCreated` の producer を配線して宣言どおりイベント駆動にするか、それが不要なら `job_handler.go` の "Fallback safety net; primary path is event-driven" というコメントと dead wiring（`PublishSummarizeRequested` の未使用実装一式）を削除する。**どちらでもよいが、コメントと実態の乖離を残さない** | プラットフォーム担当 | 2026-09-12 |
| P4 | pre-processor の consumer で `ArticleUpdated` を明示的に扱う。現状は `default:` 節で Debug ログのみの黙殺であり、10000 件が無処理で流れていた事実が誰にも見えなかった。処理するか、明示的に「無視する」と分岐に書いて件数をメトリクス化する | pre-processor 担当 | 2026-09-12 |

### 検知（Detect）

| # | アクション | 担当 | 期限 |
|---|-----------|------|------|
| D1 | **要約カバレッジのアラートを追加する。** 24 時間の `SummaryVersionCreated / ArticleCreated` 比が 0.5 を下回ったら発報する。あわせて `knowledge_event_last_occurrence_age_seconds` を producer（`summary_versions.model`）別に分解し、バッチ producer の死をオンデマンド producer が覆い隠せないようにする。本インシデントの最優先アクション | オブザーバビリティ担当 | 2026-09-05 |
| D2 | `knowledge_home_items` の `pending` 滞留時間をアラート化する（例: 直近 24 時間に生成されたアイテムの `pending` が 1 時間を超えたら発報）。ユーザーが実際に見ていた症状——チップが変わらない——を、そのまま監視の主語にする | オブザーバビリティ担当 | 2026-09-05 |
| D3 | redis stream の consumer group が 24 時間以上 `inactive` である一方で記事の取り込みは継続している、という状態を検知するアラートを追加する。寄与要因 1 の「経路が死んでいるのに誰も気付かない」を直接捉える指標 | オブザーバビリティ担当 | 2026-09-12 |
| D4 | pre-processor に「設定されたジョブ数 / 実際に登録されたジョブ数」のメトリクスを出す。P2 のシグナルを外部から観測可能にする対 | pre-processor 担当 | 2026-09-12 |

### 緩和（Mitigate）

| # | アクション | 担当 | 期限 |
|---|-----------|------|------|
| M1 | `/health/deep` への切り替え修正を含む pre-processor のイメージを再ビルドしてデプロイする。gate は 10 秒以内に開く見込み | プラットフォーム担当 | 2026-08-30 |
| M2 | デプロイ後にバックログの消化を観測する。sweep は 5 分周期・バッチ 10 件・新しい順・時間窓なしのため、249 件は約 120 件/時で 2 時間程度で解消し、その後より古い未要約記事へ進む見込み（pending ジョブがある間は sweep が自己延期し、強制間隔は 30 分）。想定どおりに減らない場合は 1 回限りの再 enqueue を検討する | pre-processor 担当 | 2026-08-31 |
| M3 | 2026-08-09 から pending のまま残っている `knowledge_reproject_runs` の v7→v8 dry_run レコードを整理する（本インシデントとは独立、優先度低） | knowledge-sovereign 担当 | 2026-09-19 |

### プロセス（Process）

| # | アクション | 担当 | 期限 |
|---|-----------|------|------|
| R1 | health エンドポイントのレスポンス契約を変更する PR では、`grep -rn "/health" --include=*.go` で consumer を洗い出し、PR 本文に影響先を列挙することをルール化する。news-creator の PR チェックリストに追加する | プラットフォーム担当 | 2026-09-05 |
| R2 | trail-projector が出している `trail.branch_anchor_unresolved` のログ洪水（約 21,000 件/時）を別チケットとして切り出す。本インシデントの調査中、本物のエラーがこのノイズに埋もれて可読性を著しく下げた | オブザーバビリティ担当 | 2026-09-12 |

## 教訓

### 技術的な教訓

- **health エンドポイントのレスポンスボディは契約である。** ステータスコードだけでなくボディの形状に依存する consumer がいる以上、`/health` は API の一部であり、テストのない API 変更と同じリスクを持つ。しかも health は「機能の起動条件」に使われがちなため、その破れは性能劣化ではなく**機能の不在**として現れ、最も気付きにくい形の障害になる。
- **gate の外側にジョブは生まれない。** `WaitForHealthy` が nil を返すまで `register()` を呼ばない設計は、起動を非同期化する目的では正しいが、「待ち続ける」と「二度と動かない」の区別を外部に一切出さない。**無限リトライは、時間の経過とともに恒久的な障害と等価になる。** リトライには「これ以上は異常」を宣言する時間軸が要る。
- **同じメトリクスに複数の producer が混ざると、片方の死をもう片方が隠す。** `SummaryVersionCreated` は全体の 99% を占めるバッチと 1% のオンデマンドが同じ event_type を出しており、1% が生きているだけでゲージは健全に見えた。**「最後にいつ起きたか」型の生存監視は、producer 別に分解されない限り生存を証明しない。**
- **カバレッジは率で見なければ落ちたことが分からない。** 「要約が作られた件数」は 0 にならなかった。「取り込んだ記事のうち要約された割合」なら 100% → 19% の崖が一目で見えた。分子だけを見る監視は、分母が増えているときに沈黙する。
- **「fallback」と書かれた経路が唯一の経路になっていることがある。** コメントは実行されないので腐る。冗長性の主張はテストか監視で裏を取れる形にしないと、障害時の被害見積もりを直接誤らせる。

### 組織的な教訓

- **provider 側で完結して見える変更ほど、consumer の棚卸しが要る。** 今回の `/health` 分離は news-creator のリポジトリ内では完全に正しく、CI もすべて green だった。壊れたのは「誰も所有していない境界」であり、CLAUDE.md ルール 7 が producer の配線変更に CDC RED を要求しているのと同じ論理が、health エンドポイントにも当てはまる。
- **ユーザーを救った冗長性が、検知を殺すことがある。** オンデマンド要約の生存は体験の劣化を「不便」に留めたが、同時に監視の目を塞いだ。**部分的に生き残るシステムでは、「まだ動いている部分」が「止まった部分」の証拠を消していないかを疑う必要がある。**
- **監視の主語をユーザーの症状に合わせる。** ユーザーは「チップが SUMMARIZING のまま」を見ていたのに、監視は可用性バーンレートと projection lag を見ていた。9 本のアラートがあってもユーザーが見ている状態を主語にしたものが 1 本もなければ、その 9 本は今回の障害に対して無力である。
- **ログの保持期間は事後分析の解像度の上限を決める。** 24 時間の保持では、10 日前に始まった障害の初期機構は原理的に再構成できない。潜伏しうる障害クラス（不在型・累積型）を持つシステムでは、少なくとも週単位の粗い集計を別途残しておく価値がある。

## 参考資料

- 契約変更コミット: `9fdff3fa5`（`feat(pki): enroll in-process certificates in Python parent workloads` — `/health` の liveness 化と `/health/deep` の追加を含む）
- 修正: `pre-processor/app/service/health_checker.go`（`/health/deep` に切り替え、`status` ∈ {`pass`, `warn`} で健全と判定）+ `health_checker_test.go` / `health_checker_factory_test.go` / `envoy_integration_test.go`。作成時点で未コミット
- 関連コード: `pre-processor/app/handler/job_handler.go`（`startHealthGatedJob` / 3 ジョブの登録）、`pre-processor/app/consumer/event_handler.go`（`ArticleUpdated` の黙殺）、`news-creator/app/news_creator/handler/health_handler.py` / `infra/health_deep.py`（provider 側の新契約）、`alt-backend/app/shared/driver/alt_db/list_unsummarized_articles_driver.go`（バックログ消化順）
- 監視: `observability/prometheus/rules/knowledge-loop-rules.yml`（`SummaryVersionCreated` を分母としてのみ使用）、`observability/prometheus/rules/knowledge-home-slo-alerts.yml`（9 本、カバレッジ系なし）、`knowledge-sovereign/app/usecase/projection_health/exporter.go`
- [[000928]] — 配線されていない依存の silent fallback を禁じる決定（本障害はリトライループに現れた同型の問題）
- [[PM-2026-045]] — silent fallback を根本原因とする先行インシデント
- [[000954]] — alt-backend / alt-harvester / alt-data-hub の 3 バイナリ分割（data-hub が mq-hub publisher を持つ経緯）
- `docs/runbooks/knowledge-home-empty-spike.md` — Knowledge Home が空になる系の調査手順
- `.claude/rules/di-wiring.md` — 配線状態を起動ログと panic で顕在化させる規約

---

> **Blameless Postmortem の原則:** このドキュメントは個人の過失を追及するためではなく、
> システムの脆弱性とプロセスの改善機会を特定するために作成されています。
> 「誰が悪いか」ではなく「システムのどこが改善できるか」に焦点を当ててください。
