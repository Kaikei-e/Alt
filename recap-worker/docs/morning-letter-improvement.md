# Morning Letter フィード過多問題の改善メモ

## 背景調査

### 7days Recap パイプラインの流れ
- `PipelineOrchestrator` は fetch → preprocess → dedup → genre → select → dispatch → persist の各ステージを順番に実行し、各ステージで並列度やリトライ、スコアリングを細かく制御している。

```
165:199:recap-worker/recap-worker/src/pipeline.rs
        let fetched = self.stages.fetch.fetch(job).await?;
        let preprocessed = self.stages.preprocess.preprocess(job, fetched).await?;
        let deduplicated = self.stages.dedup.deduplicate(job, preprocessed).await?;
        let genre_bundle = self.stages.genre.assign(job, deduplicated).await?;
        let selected = self.stages.select.select(job, genre_bundle).await?;
        let evidence_bundle = EvidenceBundle::from_genre_bundle(...);
        let dispatched = self.stages.dispatch.dispatch(job, evidence_bundle).await?;
        let persisted = self.stages.persist.persist(job, dispatched).await?;
```

- `SummarySelectStage` はジャンル毎の最大記事数を 20 件に制限し、confidence スコアでソートした上で過剰な記事を切り捨てている。ジャンル起点で均等にサンプリングすることで読みやすさを担保している。

```
28:83:recap-worker/recap-worker/src/pipeline/select.rs
pub(crate) struct SummarySelectStage {
    pub(crate) fn new() -> Self {
        Self { max_articles_per_genre: 20 }
    }
    fn trim_assignments(&self, bundle: GenreBundle) -> Vec<GenreAssignment> {
        let mut per_genre_count = HashMap::new();
        let mut ranked = bundle.assignments;
        ranked.sort_by(|a, b| Self::confidence(a).partial_cmp(&Self::confidence(b)).unwrap_or(Ordering::Equal).reverse());
        ...
        if *count >= self.max_articles_per_genre { continue; }
        *count += 1;
        selected.push(assignment);
```

- Genre stage では coarse → refine（Tag Label Graph, rollout%）の二段構成で重み付けし、タグやクラスタ情報を DAO やメトリクスに反映している。これにより「話題ごとの代表記事だけを残す」設計ができている。

### Morning Pipeline の現状
- `MorningPipeline` は fetch（1 日分）→ preprocess → dedup までしか持たず、ジャンル判定やランキング、selection が無い。Dedup 結果の各記事につきランダムな `group_id` を振って `morning_article_groups` に永続化しているだけ。

```
68:101:recap-worker/recap-worker/src/pipeline/morning.rs
        let fetched = self.fetch.fetch(job).await?;
        let preprocessed = self.preprocess.preprocess(job, fetched).await?;
        let deduplicated = self.dedup.deduplicate(job, preprocessed).await?;
        for article in deduplicated.articles {
            let group_id = Uuid::new_v4();
            groups.push((group_id, article_id, true));
            for dup_id_str in article.duplicates {
                groups.push((group_id, dup_id, false));
            }
        }
        self.recap_dao.save_morning_article_groups(&groups).await?;
```

- alt-backend は `/v1/morning/updates` から全グループを受け取り、ユーザが購読している feedID でフィルタするだけなので、サブスクリプションが多いユーザほど大量の更新が出続ける。
- Atlas には `morning_daily_summaries` / `morning_daily_evidence` テーブルが既に定義されているが、まだ書き込みも読み出しも行っていない。

```
1:30:recap-migration-atlas/migrations/20251120000100_morning_letter.sql
CREATE TABLE morning_daily_summaries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    target_date DATE NOT NULL,
    locale TEXT NOT NULL,
    headline TEXT NOT NULL,
    summary JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE morning_daily_evidence (...);
CREATE TABLE morning_article_groups (...);
```

## 課題整理
- Dedup だけなので「話題」単位の圧縮が行われず、似た系統の記事でも feed ごとに別々の行として残る。
- ユーザ毎の優先度は alt-backend で生 feed によるフィルタしか無く、ランキングや上限が存在しない。
- `morning_daily_*` のテーブルを活用していないため、日次サマリーや Evidence をキャッシュできず、frontend が常に大量の生データを受け取ってしまう。

## 改善方針サマリ
1. **7days パイプライン相当の Genre/Select ステージを Morning 用に導入**して、ジャンル別に代表記事を抽出する。
2. **ユーザ嗜好ベースのスコアリング + ハード上限**を導入して、各 feed/genre で最大件数を決める。
3. **`morning_daily_summaries` と Evidence テーブルを活用**し、日次まとめを永続化 → alt-backend/alt-frontend には整形済み DTO を返す。

## 詳細実装手順

### 1. Morning Pipeline をステージ化してトピック圧縮を導入
1. `MorningPipeline` に `GenreStage` / `SelectStage` を組み込めるよう、`PipelineBuilder` から共有部を切り出す。具体的には `PipelineBuilder::build_for_morning(window_days: u32, select_limit: usize)` のような補助関数を追加し、`AltBackendFetchStage`, `TextPreprocessStage`, `HashDedupStage`, `TwoStageGenreStage`, `SummarySelectStage` を構成する。
2. Morning 用 `SummarySelectStage` では `max_articles_per_genre` を Config から注入できるよう setter を追加（例: `SummarySelectStage::with_cap(cap: usize)`）。早朝通知向けにデフォルト 5〜8 件/genre 程度に下げる。
3. `execute_update` で `selected.assignments` をベースに `group_id` を `assignment.primary_genre + ハッシュ` から生成し、同一ジャンル内の duplicates を `GenreAssignment.article.duplicates` から引き継ぐ。これにより 7days と同等の話題圧縮が働く。
4. Genre Stage の refine/config 呼び出しを流用するため、`GraphOverrideSettings::load_with_fallback` を morning でも起動時に呼ぶ。Refine を使わない設定も `config.morning_genre_refine_enabled` で切り替えられるよう Config を拡張する。

### 2. ユーザ優先度を加味したランキング
1. `recap-worker/src/pipeline/morning.rs` に新たな `score::MorningScore` モジュールを追加し、以下の要素でスコアを決定する:
   - `GenreAssignment.genre_scores`（話題の強さ）
   - `article.tags` 内の購読 feed ID（`TagSignal::Feed` など）でユーザが興味を示した頻度
   - 前日のクリック/既読メトリクス（`recap_worker_config` または alt-backend から取得した prefer feed IDs）※初期は weight=1, 無ければ 0
2. Alt-backend の `MorningUsecase` にリクエストヘッダ `X-Alt-Feed-Weight`（存在する場合）を渡し、Recap Worker 側で `JobContext` に `feed_weights` を埋め込めるよう `JobContext` struct を拡張する。
3. スコアを用いて `selected.assignments` を再ソートし、`per_feed_cap`（例: feed 毎 2 件）を適用する処理を `MorningPipeline::cap_per_feed(assignments)` として実装。feedID は DAO の `recap_job_articles` から JOIN or Alt-backend から fetch した Article Map を活用する。

### 3. 永続化と API の刷新
1. `RecapDao` に以下のメソッドを追加:
   - `upsert_morning_daily_summary(date, locale, headline, summary_json)` → `morning_daily_summaries`
   - `replace_morning_daily_evidence(summary_id, &[MorningEvidenceRow])`
   - `fetch_latest_morning_summary(locale)` と `fetch_morning_evidence(summary_id)`
2. `MorningPipeline::execute_update` の末尾で `morning_daily_*` へ書き込み、既存の `morning_article_groups` は後方互換のために暫く併用（feature flag `MORNING_GROUPS_LEGACY_ENABLED`）。
3. API:
   - `GET /v1/morning/updates` → 新 DTO `{ target_date, headline, sections[] }` を返す。旧挙動は `?legacy=true` で温存。
   - 新 DTO では genre ごとに `primary_article` + `duplicates` + `score` を返し frontend での表示順を固定。

### 4. alt-backend / frontend 側の対応
1. Gateway を更新し、新 DTO を受け取れるデシリアライザを実装。`MorningUsecase` では feedID フィルタ後に空になった場合のみ legacy モードで再フェッチ（段階移行用）。
2. Frontend（`useMorningUpdates`, `MorningUpdateList`）を新レスポンスに合わせて改修。ジャンルタブ表示 or スコア順のセクション見出しを追加し、最大件数を UI レベルでも 5〜8 件に制限。

### 5. 設定・テスト・リリース手順
1. Config: `.env.template` に `MORNING_WINDOW_DAYS`, `MORNING_MAX_PER_GENRE`, `MORNING_MAX_PER_FEED`, `MORNING_GENRE_REFINE_ENABLED` を追加し、`Config` 構造体へ getter を実装。
2. テスト:
   - `recap-worker` のユニット: `MorningPipeline` 用に `#[tokio::test]` を追加し、genre/select/score/cap が適用されることを検証。
   - DAO テストで `morning_daily_*` の CRUD を確認（`sqlx::test`）。
   - alt-backend は `morning_usecase_test.go` にスコア順/上限の期待値を追加し、gateway の JSON スキーマをモックで検証。
   - E2E: `pnpm -C alt-frontend test` + `cargo test -p recap-worker`。
3. Rollout: `legacy` フラグで両実装を併用し、Grafana でエンドポイントのレスポンス件数をモニタリング。件数が目標（例: 10 件以下/ユーザ）に収束したらフラグを外す。

---

この手順により 7days Recap が持つ話題圧縮/ランキング基盤を Morning Letter に移植でき、ユーザ毎に見やすい件数へ抑制できる。
