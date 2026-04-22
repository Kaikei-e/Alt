---
title: "PM-2026-038 recap-worker の rust-bert キャッシュが空 bind のまま起動し subgenre-splitter が silent に keyword-only fallback、3days Recap のジャンル分類が 2 バケットに崩壊"
date: 2026-04-22
tags:
  - alt
  - postmortem
  - recap-worker
  - rust-bert
  - silent-failure
  - classification
  - quality-degradation
---

# ポストモーテム: recap-worker の rust-bert キャッシュが空 bind のまま起動し subgenre-splitter が silent に keyword-only fallback、3days Recap のジャンル分類が 2 バケットに崩壊

## メタデータ

| 項目 | 内容 |
|------|------|
| インシデントID | PM-2026-038 |
| 発生日時 | 2026-04-20 頃 (recap-worker の Dockerfile リファクタリング一連がデプロイされ、rust-bert キャッシュが image 焼き込み → host bind に切り替わった時点) |
| 検知日時 | 2026-04-22 (ユーザーが「3days Recap のジャンル生成精度とレポート品質がものすごく悪い」とチャットで報告) |
| 復旧日時 | 執筆時点 (2026-04-22) — コード修正 landed、`populate-cache` 実行とデプロイ待ち |
| 影響時間 | 約 2 日間の品質劣化 (自動バッチ約 2 回 + 手動キック複数) |
| 重大度 | SEV-3 (3days Recap の user-visible 品質劣化、単一ホスト開発環境) |
| 作成者 | recap / platform チーム |
| レビュアー | — |
| ステータス | Draft |

## サマリー

2026-04-22 午前、ユーザーから「3days Recap のジャンル生成精度と Recap レポート品質がものすごく悪い」との報告を受け調査を開始。[[PM-2026-036]] / [[PM-2026-037]] の recap-subworker 側 joblib artefact 問題は本日午前中に [[000825]] + commit `473b98251` で解消済みで、**recap-subworker 自体は healthy かつ分類 artefacts を正しくロード**していた。問題は上流の別サービス `recap-worker` (Rust) 側に潜んでいた。

真因は docker bind-mount の footgun + silent fallback の合わせ技。直近 3 日 (2026-04-19 〜 2026-04-22) で recap-worker の Dockerfile をスリム化し rust-bert キャッシュを **image 焼き込みから `/opt/rustbert-cache` への host bind に分離** した (commit `3bc849f99` / `d7153ad36` / `f2fff67b5`)。同時に `warmup` subcommand が導入されたが、**prod host で `/opt/rustbert-cache` を populate するタイミングが host provisioning runbook に明記されず**、本 Alt リポジトリの `docs/runbooks/3days-recap-artefact-recovery.md` §"ブロッカー 1" は `mkdir -p + chown 999:999` で終わっていた。結果、prod host には **空の `/opt/rustbert-cache` が `:ro` で bind された状態**が成立した。

起動時 `rust_bert::SentenceEmbeddingsBuilder::remote(AllMiniLmL12V2)` は tokenizer / model weights を cache に書こうとするが ro fs で拒否され、`Read-only file system (os error 30)` を送出。`pipeline/orchestrator.rs:155-171` の従来実装はこの `Err` を `tracing::error!` に 1 行だけ出して `None` に丸め、下流の `subcluster_large_genres` が "embedding 不在の小ジャンルは元のまま" 経路に遷移した。**コンテナは `Up (healthy)` を維持、`/health` は 200 を返し続け、外形監視・Prometheus・smoke のどれも異常を検出しなかった**。

user-visible 症状:
- 直近 3 ジョブの `recap_subworker_runs`: 30 ジャンル分類のうち `consumer_tech` (10 clusters) / `politics_government` (7 clusters) の 2 バケットのみ dispatch 成功、残り 25-28 ジャンルは `genres_no_evidence` で空セクション化。
- recap-worker ログ: `"total_genres":30,"genres_stored":2,"genres_failed":0,"genres_skipped":0,"genres_no_evidence":28` (2026-04-22 11:02)。
- `recap_final_sections` 過去 7 日で 0 行 (Knowledge Home の 3days Recap 画面は stale)。

本件は [[PM-2026-036]] と **同型の "host bind empty footgun + silent fallback" が別サービス (recap-worker) で顕在化**した 5 本目の PM。PM-035 → PM-036 → PM-037 が recap-subworker (Python) 側の joblib 経路を仕留めた直後、同日のうちに recap-worker (Rust) 側の rust-bert 経路でも同型が判明した。同 family の ADR-000825 パターン (Settings validator で起動時 fail-closed) を Rust crate 側にも対称適用するのが恒久対策。

## 影響

- **影響を受けたサービス:** `recap-worker` の embedding 初期化経路。下流 `pipeline/select.rs::subcluster_large_genres` の subgenre 分割が全ジャンルで skip され、クラスタリング全体がキーワードマッチだけで動く degraded mode に。`/health` は uvicorn/axum liveness のみ見るので外形監視上は「healthy」と表示され続けていた。
- **影響を受けた画面:** Knowledge Home の 3days Recap セクション (ジャンル網羅性が崩壊、30 ジャンル中 2-5 のみ)、レポート品質 (大量記事が 2 バケットに集約されるため summary がノイジー)。
- **影響を受けたユーザー数/割合:** 単一ホスト開発環境の操作ユーザー 1 名。マルチテナント本番であれば Knowledge Home を購読する全ユーザー。
- **機能への影響:**
  - **3days recap:** ジョブ自体は `completed` で返るが `genre_distribution` は 2 ジャンル。残りジャンルは `genres_no_evidence` で空、ユーザーから見るとレポートが hollow に見える。
  - **1day / 7days recap:** 同じ経路を共有。特に 7days は記事数が多いので 2 バケット崩壊の影響がより可視的。
  - **recap_evaluator:** 別件で `http://news-creator:11434/api/tags` が 404 を返し G-Eval summary evaluation が始動時に disabled 化されている。classification 評価 API は `np.nan is an invalid document` で 400 を返していた。これらは本 PM の主因ではないが、**品質監視の自動検知層が無力化されていた寄与要因**。
- **データ損失:** なし。ジョブは `completed` で terminate しており `recap_outputs` / `recap_final_sections` への書き込みは部分的に成立 (ただし大部分が `genres_no_evidence` で skip)。
- **SLO/SLA違反:** 個別 SLO 未設定。
- **潜在影響:**
  - 本番マルチテナントなら 2 日間の Knowledge Home ジャンル網羅性劣化が user-visible UX 劣化に直結。
  - [[PM-2026-036]] の remediation (ADR-000825) が **recap-subworker 側だけ** 仕留めていた穴: backend が joblib / learning_machine のどちらでも対称に fail-closed を入れたが、**上流の recap-worker が持つ全く別の artefact 依存 (rust-bert host cache)** は audit されていなかった。PM-036 教訓 AI #12 (「他バックエンドの silent failure audit」) を recap-worker 側にも展開する契機を本 PM が提供する。

## タイムライン

全時刻 JST。

| 時刻 | イベント |
|---|---|
| 2026-04-19 〜 2026-04-20 | recap-worker の Dockerfile スリム化 + rust-bert キャッシュ外部化の一連コミット (`3bc849f99` / `b07584639` / `c9e97fdee` / `f2fff67b5` / `967ecc9d5` / `d7153ad36`)。image から rust-bert 重みを除外し `/opt/rustbert-cache` 相当の host bind 前提に差し替え。`warmup` subcommand を新規追加。ただし **prod host に populate する手順は明文化されず**、Alt 側 `3days-recap-artefact-recovery.md` §ブロッカー 1 は `mkdir -p + chown 999:999` まで。alt-deploy 側 `recover-3days-recap.sh` には `provision-cache` はあるが `populate-cache` は未実装。 |
| 2026-04-20 以降 | 新 image がデプロイされるたび `/opt/rustbert-cache` が空 `:ro` のまま起動、embedding 初期化が `Read-only file system (os error 30)` で失敗。`tracing::error!` 1 行が出て `Option<Embedder>` が `None` に degrade、pipeline は keyword-only で動作継続。外形監視は healthy を維持。 |
| 2026-04-20 〜 2026-04-22 | [[PM-2026-035]] / [[PM-2026-036]] / [[PM-2026-037]] の recap-subworker 側対応に集中 (joblib artefact 欠落 / bind-mount empty / init container 不整合)。同日 3 PM シリーズの診断中、recap-worker 側は `Up 2 days (healthy)` を維持していたため scope 外と判定。 |
| 2026-04-22 午前 | [[PM-2026-036]] の remediation (ADR-000825 addendum / commit `473b98251`) が landed、recap-subworker 側の 3days Recap outage が解消。**この時点で recap-worker 側 rust-bert cache 空問題はまだ発現中**、しかしジョブは `completed` で返るので PM-035/036/037 時代の `classification returned 0 results ...` エラーとは異なる pattern。 |
| 2026-04-22 午前 | **検知**: ユーザーから「3days Recap のジャンル生成精度と Recap レポート品質がものすごく悪い」との報告。症状がエラー文言ではなく「品質劣化」なので従来 PM と異なる切り口での調査開始。 |
| 2026-04-22 午前 | Plan Context Loader + 2 並列 Explore Agent + docker exec による DB / ログ直接観測を同時投入。30 分以内に以下を確定:<br>(1) `alt-recap-worker-1` 起動直後ログに `Token file not found "/opt/rustbert-cache/huggingface/token"` → `Read-only file system (os error 30)` → `Embedding service failed to initialize. ... Falling back to keyword-only filtering.`<br>(2) `docker exec alt-recap-worker-1 sh -c 'ls /opt/rustbert-cache \| head; mount \| grep rustbert'` で host bind は存在するが空かつ `ext4 (ro,relatime)` (ファイルシステム自体が ro)。<br>(3) `recap_subworker_runs` の直近 10 分: `consumer_tech` / `politics_government` の 2 ジャンルしか dispatch 成功していない。<br>(4) recap-worker ログで `"total_genres":30,"genres_stored":2,"genres_no_evidence":28`。 |
| 2026-04-22 午前 | 真因特定: PM-036 と同型だが別サービスの別 artefact。ユーザー承認のうえ Plan Mode で計画策定、TDD outside-in で修正開始。 |
| 2026-04-22 午前 | TDD RED 先行: `src/pipeline/embedding.rs` の `#[cfg(test)] mod tests` に `require_or_degrade` の 4 ケース + `src/config.rs` の `mod tests` に `from_env_defaults_embedding_required_to_false` / `from_env_parses_embedding_required_true` の 2 ケース。 |
| 2026-04-22 午前 | GREEN 実装: `EmbeddingAvailability` enum + 純関数 `require_or_degrade` を `embedding.rs` に追加、`Config` に `FeatureToggle` ベースの `embedding_required` + env `RECAP_WORKER_EMBEDDING_REQUIRED` (default `false`) 対応、`orchestrator.rs:155-171` を書き換えて fail-closed policy を適用。`compose/recap.yaml` の recap-worker に `RECAP_WORKER_EMBEDDING_REQUIRED=${RECAP_WORKER_EMBEDDING_REQUIRED:-true}` を追加。 |
| 2026-04-22 午前 | Alt 側 `3days-recap-artefact-recovery.md` §"ブロッカー 1" を汎用記述に redact (`feedback_no_host_names_in_public` 準拠)、具体的な image sha / secrets パス / ワンライナーは alt-deploy (Private) の `scripts/recover-3days-recap.sh` に `populate-cache` sub-command として集約。alt-deploy `operations.md` §7.4 に復旧手順を追加。 |
| 2026-04-22 午前 | CI parity local: `cargo test --lib` で embedding / config の新 6 テスト全 GREEN、`cargo clippy --all-targets -- -D warnings` 無警告。 |
| 2026-04-22 午前 | [[000827]] / 本 PM を執筆。populate-cache の実行とデプロイは user 手動作業として handoff ([[feedback_no_auto_run_commands]] / [[feedback_no_auto_push]])。 |

## 検知

- **検知方法:** ユーザー報告 (チャット経由)。「3days Recap のジャンル生成精度と Recap レポート品質がものすごく悪い」。
- **TTD (Time to Detect):** 約 **2 日間 (~48 時間)**。PM-036 より短いが、ユーザーが UI を毎日見ている前提で得られた値であり、自動検知はゼロ。
- **検知の評価:** **外形監視・CI・deploy success ステータスの全てをすり抜けた silent quality degradation**。理由:
  1. `/health` smoke は axum liveness のみ見るので embedding 初期化の成否を反映しない (PM-035/036/037 と同じ穴、recap-worker 版)。
  2. `docker compose ps` は `Up (healthy)` を表示。recap-worker は通常運用を続けており、pipeline も `completed` ステータスで返していた。
  3. エラー文言が「PM-033/035/036/037 の `classification returned 0 results ...`」と異なる新パターン (`Embedding service failed to initialize. ... Falling back to keyword-only filtering.`)。3 PM 連続同文を意識していた運用者視点でも「既知 PM の再発」と誤誘導されにくい反面、**キーワードアラートが用意されていない新種**のため初動遅延。
  4. `recap_failed_tasks` の累積件数はゼロ (ジョブは `completed` で返るため)。PM-036 AI #7 (Prometheus exposer) が実装されていても本件は検知できなかった可能性がある。
  5. recap-evaluator の G-Eval summary evaluation は `http://news-creator:11434/api/tags` 404 で自動無効化されており、**品質評価自体が機能していなかった** (別件、secondary 寄与要因)。
  6. `recap_final_sections` テーブルの空白は DB 側で直接クエリしないと見えない。Grafana / smoke には expose されていない。

### 本来の検知ルート (仮想)

- [[PM-2026-036]] AI #6 (mTLS 経由の `/v1/classify-runs` real data smoke) が recap-worker 側まで広がっていれば、起動後数秒で `genre_distribution` の 2 バケット崩壊を検知できた。
- recap-evaluator G-Eval が動作していれば、品質メトリクスの急激な低下で alerting 可能だったはず。これが secondary issue で止まっていたため検知が user 報告まで遅延。

## 根本原因分析

### 直接原因

`src/pipeline/orchestrator.rs:155-171` の従来実装:

```rust
let embedding_service: Option<Arc<dyn Embedder>> = match EmbeddingService::new() {
    Ok(s) => {
        tracing::info!("Embedding service initialized successfully (AllMiniLmL12V2)");
        Some(Arc::new(s) as _)
    }
    Err(e) => {
        tracing::error!(
            error = %e,
            error_chain = ?e,
            "Embedding service failed to initialize. \
             Large genres cannot be split into subgenres. \
             Falling back to keyword-only filtering."
        );
        None
    }
};
```

init 失敗を `error!` 1 行で吸収し `None` に丸めるため、下流 `subcluster_large_genres` は「`embedding_service.is_none()` のときは元のジャンル割当を変更しない」という明示的な degrade 経路に落ちる (`select.rs:94` 付近)。この経路は `subcluster_large_genres_handles_no_embedding_service` テスト (select.rs:523) でカバーされており **意図的設計** だが、**"意図的に degrade できる" と "起動時の init 失敗を degrade 経路で吸収する" は別議論**。本件では後者として誤用されていた。

host 側の直接事実:

- `/opt/rustbert-cache`: 存在 (runbook §ブロッカー 1 の `mkdir -p + chown` は実施済) だが `total 8` (空ディレクトリ)。
- `mount | grep rustbert`: `/dev/nvme0n1p2 on /opt/rustbert-cache type ext4 (ro,relatime)`。
- compose/recap.yaml:136: `- /opt/rustbert-cache:/opt/rustbert-cache:ro` (container に ro bind)。

`rust_bert::SentenceEmbeddingsBuilder::remote(AllMiniLmL12V2)` は初回起動時に tokenizer.json / rust_model.ot / config.json / vocab.txt を HuggingFace Hub からダウンロードし `$RUSTBERT_CACHE` に書き込もうとする。cache が空かつ ro なので書き込みに失敗、`hf-hub` crate が `IO error Read-only file system (os error 30)` を送出。

### Five Whys

1. **なぜ 3days Recap のジャンル分類が 2 バケットに崩壊したか？**
   → recap-worker の `pipeline/select.rs::subcluster_large_genres` が embedding_service unavailable 経路に落ちて「大ジャンル (consumer_tech / politics_government 等) をサブジャンルに分割する」処理を skip していた。

2. **なぜ embedding_service が unavailable だったか？**
   → `orchestrator.rs:155-171` で `EmbeddingService::new()` が `Err` を返し、その Err を `error!` log に出した上で `None` に degrade していたから (silent fallback)。

3. **なぜ `EmbeddingService::new()` が `Err` を返したか？**
   → `rust_bert::SentenceEmbeddingsBuilder::remote(AllMiniLmL12V2)` が tokenizer ダウンロード先 `/opt/rustbert-cache/huggingface/...` への書き込みで `Read-only file system (os error 30)` を発生させたから。

4. **なぜ書き込みに失敗したか？**
   → `/opt/rustbert-cache` が (a) ホスト側で空ディレクトリ + (b) compose で `:ro` bind という組み合わせで container に見えていたから。加えて host fs 自体が `ro,relatime` mount であった。

5. **なぜ `/opt/rustbert-cache` が空のままだったか？**
   → Alt 側 `docs/runbooks/3days-recap-artefact-recovery.md` §"ブロッカー 1" は `mkdir -p + chown 999:999` までしか記述しておらず、**model cache を populate する手順が明文化されていなかった**。alt-deploy 側 `scripts/recover-3days-recap.sh` には `provision-cache` sub-command は存在したが同様に mkdir+chown 止まりで、`populate-cache` に相当する sub-command は未実装。warmup subcommand 自体は commit `f2fff67b5` で追加されていたが、**「誰が / いつ / どこから」叩くかの運用手順が確立していなかった**。

6. **なぜ populate 手順が確立していなかったか？** (補足)
   → [[PM-2026-036]] / [[000825]] で recap-subworker の joblib artefact 欠落を是正する際、artefact 配布経路は「2026-04-13 以前から動いていた host path を復旧する」の回帰修復だったため、新たに populate が必要な隣接 artefact (recap-worker の rust-bert cache) の audit が scope 外になっていた。recap-worker の Dockerfile スリム化コミット (`3bc49f99` / `d7153ad36` / `f2fff67b5`) が **"キャッシュを image から外出ししたので host 側で populate してね"** という前提を十分明文化せず merge されていた。

### 根本原因

**「host bind 型 artefact の populate 責任の unowned 化」× 「init 失敗の silent fallback」の 2 層失敗**。

- **運用側**: Dockerfile スリム化で rust-bert cache を image 外に出した際、populate 責任が runbook / alt-deploy / CI のどこにあるか明文化されずに merge された。`warmup` subcommand は存在したが誰も叩かなかった。
- **コード側**: init 失敗をアプリ層で degrade として swallow する設計があった。dev/test 用の意図的 degrade 経路 (keyword-only fallback) と prod での init 失敗を区別する policy gate が存在しなかった。

2 層のどちらか片方でも塞がっていれば silent failure は起きなかった。今回は両方とも未塞がりで 2 日間の品質劣化を許した。

### 寄与要因

- **recap-evaluator の G-Eval が並行障害で無効化**されていた。news-creator 側の endpoint drift (`:11434/api/tags` 404) により G-Eval summary evaluation は起動時に `Ollama is not available` で disable 化され、classification evaluation API も `np.nan is an invalid document` で 400 を返していた。どちらも本 PM の主因ではないが、品質異常を自動検知する唯一のメトリクスが同期的に死んでいたことで TTD が延びた。
- **ADR-825 の audit が recap-subworker 限定**。PM-036 で「他バックエンドの silent failure audit」AI (#12) を設定したが、**そこで想定していたのは recap-subworker の別 backend (learning_machine) であり、上流サービスの別 artefact 依存 (recap-worker の rust-bert cache) は audit スコープに含まれていなかった**。同型の footgun が隣の service に潜むという視点が弱かった。
- **error 文言が PM-033/035/036/037 とは異なる新パターン**. 3 PM 連続で `classification returned 0 results ...` を見ていた運用者の目が、`Embedding service failed to initialize` という新キーワードを「別件」として軽視する方向に動きやすかった。PM-036 AI #8 (エラー文言区分) が recap-worker 側まで広がっていればキーワードベースの alerting で即検知できた可能性。
- **`:ro` bind の安心感**. compose で `:ro` を付けていると「container が cache を汚染しない」という設計上の安全感が勝つが、本件では **"そもそも host に populate が必要"** という別問題を覆い隠す形で作用した。`:ro` が防御効果を持つのは populate 済みの前提付き。

## 対応の評価

### うまくいったこと

- **2 日で検知 (vs PM-036 の 8 日)**. PM-035 → 036 → 037 → 本 PM の同日集中対応で「外形 healthy + 分類品質だけ劣化」という silent failure pattern への体感が研ぎ澄まされており、ユーザー報告直後に 30 分以内で真因特定。
- **PM-036 / 037 のパターンを即移植**. Settings validator で起動時 fail-closed、runbook に populate 手順、ADR に対称記述 — 3 点セットを同日のうちに Rust 側にミラー。パターンの再利用性が実証された。
- **Alt public / alt-deploy private の境界尊重**. ユーザーから「runbook で CI 周り固有かつ機微な情報を含んでいるなら、alt-deploy で管理して」と指示が入り、image sha 解決 / secrets path / ワンライナーは alt-deploy (Private) の `populate-cache` sub-command に集約、Alt 側 runbook は抽象論だけに redact。[[feedback_no_host_names_in_public]] の原則を同セッション内で適用完了。
- **TDD outside-in 順守**. `require_or_degrade` 純関数 4 ケース + `embedding_required` config 2 ケースを RED 先行で commit し、GREEN 実装後に CI parity local (`cargo test` / `cargo clippy`) まで一筆で完走。
- **既存 test を破壊しない設計**. `EmbeddingAvailability::Optional` 経路を明示的に残したことで `subcluster_large_genres_handles_no_embedding_service` 等の既存テストはそのまま pass。意味論の追加であって挙動の破壊ではない。

### うまくいかなかったこと

- **PM-036 の AI #12 が recap-worker 側まで広がっていなかった**. "他バックエンドの silent failure audit" を recap-subworker の別 backend に限定解釈してしまい、上流サービスの別 artefact 依存まで手が届いていなかった。audit scope の定義が甘かった。
- **recap-evaluator の並行障害を scope 外と判定**. news-creator :11434 の 404 で G-Eval / classification evaluation が両方死んでいたが、「本 PM の主因ではない」として本 PR では直さず。結果として品質自動検知層が今後も死んだまま。別 PR で対応する (action item)。
- **recap-worker の Dockerfile 変更の影響評価が不十分だった**. `3bc49f99` / `d7153ad36` / `f2fff67b5` は個別に review されたが、一連として「prod host に populate が必要な新 artefact を導入した」という明示的 AI が runbook / alt-deploy に tracks されず merge された。[[000826]] のセルフチェック項目に「artefact の populate 責任は runbook / CI / image のどこにあるか」を追加する follow-up を検討。
- **populate 手順の手動性**. 現状は運用者が `./scripts/recover-3days-recap.sh populate-cache --yes` を明示実行する設計。CI / deploy 経路で自動的に populate する機構を検討する余地がある (ADR-000763 の rolling deploy model と互換である必要、ただし `--no-deps` でも populate 相当が走るような設計が理想)。

### 運が良かったこと

- **ユーザーの手動 smoke 習慣**. UI を毎日見る運用が無ければ、`recap_failed_tasks` にエラーが積まれないタイプの品質劣化は検知困難だった。
- **単一ホスト開発環境**. 本番マルチテナントなら 2 日間の Knowledge Home ジャンル網羅性劣化は複数ユーザーに可視の UX 劣化となっていた。
- **PM-036 / 037 の経験直後**. "外形 healthy + 品質だけ劣化" という pattern への感度が研ぎ澄まされていた。半年後に同種が起きていれば recap-worker が `Up (healthy)` を維持していることで初動判断が遅れた可能性。
- **既存 degrade 経路がテスト済み**. `subcluster_large_genres_handles_no_embedding_service` が既に存在したため、`EmbeddingAvailability::Optional` 経路は "既にテスト済みの挙動" として安心して残せた。Required と Optional の 2 元化は意味論的拡張だけで済んだ。

## アクションアイテム

| # | カテゴリ | アクション | 担当 | 期限 | ステータス |
|---|---|---|---|---|---|
| 1 | 予防 | `recap-worker/recap-worker/src/pipeline/embedding.rs` に `EmbeddingAvailability` enum + 純関数 `require_or_degrade` を追加。`src/config.rs` に env `RECAP_WORKER_EMBEDDING_REQUIRED` (default `false`) を parse、`Config::embedding_required()` getter を公開。`src/pipeline/orchestrator.rs:155-171` を書き換えて policy gate で Err を昇格/degrade 切り替え。`compose/recap.yaml` で本番デフォルト `true` に設定 | recap / platform | 2026-04-22 | **Done** (本 PR、[[ADR-000827]]) |
| 2 | 予防 | `docs/runbooks/3days-recap-artefact-recovery.md` §"ブロッカー 1" を汎用記述に redact、具体パス・secrets・image sha 解決は `alt-deploy/scripts/recover-3days-recap.sh populate-cache` に集約 | docs / platform | 2026-04-22 | **Done** (本 PR) |
| 3 | 予防 | alt-deploy `scripts/recover-3days-recap.sh` に `populate-cache` sub-command を追加。`ALT_DEPLOY_CHECKOUT` / `RECAP_WORKER_IMAGE` / `HUGGING_FACE_TOKEN_PATH_HOST` から現行 image sha と secrets パスを自動解決して `docker run ... warmup` を走らせる。`verify-cache` も populate 済判定を追加 (model file 1 つ以上が 1 MB+) | ops (alt-deploy) | 2026-04-22 | **Done** (本 PR) |
| 4 | 予防 | alt-deploy `docs/runbooks/operations.md` §7.4 に「3days Recap ジョブ復旧」節を追加し `provision-cache` → `populate-cache` → `verify-cache` → compose restart → Alt 側 smoke の典型シナリオを明文化 | docs / ops | 2026-04-22 | **Done** (本 PR) |
| 5 | プロセス | **prod host で `populate-cache --yes` を実行して `/opt/rustbert-cache` を populate する**。完了後 `docker compose -f compose/compose.yaml -p alt restart recap-worker` で recap-worker を再起動し、`docker logs alt-recap-worker-1 \| grep 'Embedding service initialized successfully'` が 1 行出ることを確認。user 明示承認まで待機 | user / ops | 2026-04-22 | TODO (本 PR 反映後のブロッキング AI) |
| 6 | プロセス | **`git push origin main` で `dispatch-deploy.yaml` → Kaikei-e/alt-deploy 経路でデプロイ**し、新 `RECAP_WORKER_EMBEDDING_REQUIRED=true` + runbook + ADR + PM が prod に反映されることを確認 | user | 2026-04-22 | TODO ([[feedback_no_auto_push]] 準拠、user 明示承認) |
| 7 | 検知 | recap-evaluator の `OLLAMA_URL` を現行 news-creator の正しいエンドポイントに合わせて G-Eval summary evaluation を復活させる。並行して classification evaluation API (`/v1/evaluation/genres`) の NaN 入力ガードを追加し `400 np.nan is invalid document` を消す | recap | 2026-05-15 | TODO (本 PM で detected、別 PR で対応) |
| 8 | 予防 | [[PM-2026-036]] AI #12 (他バックエンド / 他サービスの silent failure audit) を recap-worker まで拡張。`news-creator` / `recap-subworker` / `tag-generator` / `pre-processor` の init-time artefact 依存を洗い出し、fail-closed validator の対称適用状況を表形式で整理 | platform | 2026-05-30 | TODO (PM-036 から継承、scope 拡張) |
| 9 | 検知 | [[PM-2026-036]] AI #6 (`/v1/classify-runs` real data smoke) を recap-worker 側まで広げ、`genre_distribution` が ≥ N ジャンル出ることを assert する Hurl suite を追加。起動後数秒で本 PM 類型を検知できる | recap / platform | 2026-05-15 | TODO (PM-036 から継承、scope 拡張) |
| 10 | 検知 | [[PM-2026-036]] AI #7 (`recap_failed_tasks` Prometheus expose) だけでは本件は検知できない。`recap_subworker_runs` の `status='succeeded'` 件数が `len(recap_genres)` の 10% 未満なら発火するアラート (ジャンル網羅性メトリクス) を追加する案を観測チームで議論 | observability | 2026-05-30 | TODO |
| 11 | プロセス | [[000826]] のセルフチェック項目に「本 ADR で導入される artefact の populate 責任は runbook / CI / image のどこにあるか？ 運用者はどの実行タイミングで populate するか？」を追記 | platform | 2026-05-15 | TODO |
| 12 | プロセス | recap-worker の Dockerfile スリム化で `rust-bert` / `libtorch` / その他 host bind 型 artefact を image 外出しした一連の変更を audit し、populate 責任の明文化が抜けていないかを洗い直す | platform | 2026-05-15 | TODO |

## 教訓

### 技術面

- **host bind 型 artefact を導入する PR は populate 責任を同一 PR 内で確立する**. 本件は `3bc49f99` で rust-bert cache を image 外出しした PR と、`f2fff67b5` で warmup subcommand を追加した PR の間で populate 運用が tracks されていなかった。今後は image slim 化の PR に「populate は誰が / いつ / どのコマンドで」を PR 説明に明記することを運用ルール化。
- **init 失敗の silent fallback は prod で禁止**. dev/test の keyword-only fallback は意図的設計として残す価値があるが、prod で init 失敗を degrade として swallow することは [[PM-2026-036]] / [[PM-2026-038]] 両方の silent failure 源泉。env flag で policy を二元化する (本 PR で `RECAP_WORKER_EMBEDDING_REQUIRED`) か、無条件 bail する設計が必要。
- **rust-bert / hf-hub の挙動**: `SentenceEmbeddingsBuilder::remote(...)` は cache が無ければ必ず HTTP 取得を試みる。cache が `:ro` bind だと書き込み失敗で起動不能。pre-populate するか `:rw` に開けるか、image 焼き込みの 3 択。本件は pre-populate を選んだ (`internal: true` compose 原則との両立)。
- **`subcluster_large_genres_handles_no_embedding_service` の意味論が 2 つある**. `embedding_service = None` は (a) dev/test で意図的に unset した (b) prod で init 失敗した、のどちらでも生じる。テスト名は (a) を意図するが挙動は (b) も通してしまう。`EmbeddingAvailability` enum で呼び出し側が意味を明示する設計に変えたことで、テストの想定と prod の経路が区別できるようになった。

### 組織面

- **audit scope は「このサービスの artefact」だけでなく「このパイプラインの上流/下流の artefact」まで広げる**. [[PM-2026-036]] AI #12 を recap-subworker 内の backend 比較に限定したが、本件は上流 recap-worker の完全に別種の artefact (rust-bert cache) で発生した。次回 PM の AI #12 相当は「pipeline 全体の artefact 依存マップを更新」と書き直す。
- **同日連続 PM シリーズの exhaustion に注意**. PM-035 → 036 → 037 → 038 と 1 日で 4 PM 処理は疲労を招く。同日対応する代わりに「翌日まで待って PM-038 を書く」選択肢もあったが、現役で品質劣化が出ているため対応を選択。次回は SEV レベルに応じて「同日対応は 2 PM まで」等の自衛ルールを検討。
- **public / private 境界の運用者指示を即時に反映**. 今回ユーザーから「CI 周り固有かつ機微な情報を含んでいるなら、alt-deploy で管理して」の指示が途中で入り、Alt 側 runbook を redact + alt-deploy 側 script 拡張で対応した。これを PR ごとに意識する仕組みとして、PR template に「この変更は public repo に host 固有パス / secrets パス / image sha 解決を持ち込んでいないか？」のセルフチェック項目を追加する案。
- **"直近の別 PM で対応した類型"** を思い出す力が TTD を劇的に縮めた. PM-036 の 8 日 vs 本 PM の 2 日の差分は主にこれ。blameless の原則を保ちつつ、過去 PM の参照読みを AI / 運用プロセスに組み込むことで checkpoint として機能する。

## 参考資料

### 本 PM の修正

- [[ADR-000827]] recap-worker の rust-bert embedding 初期化を `RECAP_WORKER_EMBEDDING_REQUIRED` で起動時 fail-closed にする
- Alt repo commit (to be landed) — `feat(recap-worker): gate embedding init on RECAP_WORKER_EMBEDDING_REQUIRED` (embedding.rs + config.rs + orchestrator.rs + compose/recap.yaml)
- Alt repo commit (to be landed) — `docs(runbook): redact 3days-recap-artefact-recovery §ブロッカー 1 to abstract host paths`
- alt-deploy commit (to be landed) — `feat(scripts): add populate-cache sub-command for rust-bert warmup`
- alt-deploy commit (to be landed) — `docs(runbook): document §7.4 3days Recap ジョブ復旧 (populate-cache)`

### 関連 PM / ADR / runbook

- [[PM-2026-036]] recap-subworker joblib artefacts bind-mount が空ディレクトリ化し 3days Recap が 8 日間 silent に失敗 — 本 PM の直系 parent。同型の footgun が別サービス別 artefact で顕在化
- [[PM-2026-037]] recap-subworker 初回 remediation の init container が rolling deploy で一度も起動されず 3days Recap が当日再発 — PM-036 の addendum。rolling deploy 整合性の教訓
- [[PM-2026-035]] recap-subworker learning_machine artifacts 欠落で 3days Recap 948 件分 classification 失敗 — PM-036 の直前、片肺 fail-closed の起源
- [[PM-2026-033]] mTLS server-side gap — "classification returned 0 results ..." 3 PM 連続同文の起点
- [[000825]] recap-subworker の joblib artefact 欠落を Pydantic validator と named-volume-with-init-container で 2 層 fail-closed にする — 本 PM の fix の Python 側先例
- [[000826]] compose パターンと rolling deploy model の整合性を ADR レビューの必須セルフチェックに格上げする — 本 PM の artefact populate 議論も同系
- [[000827]] recap-worker の rust-bert embedding 初期化を `RECAP_WORKER_EMBEDDING_REQUIRED` で起動時 fail-closed にする — 本 PM で決定した設計変更
- [[000727]] Phase 1 ハードニングと Phase 2 pilot を経て東西通信 mTLS を永続デフォルトに昇格する — `load_server_tls_config` が "fail-closed; refusing to start" で行っていた validator パターンの起源
- [[runner-setup]] / [[operations]] (alt-deploy) — `populate-cache` / `recover-3days-recap.sh` の運用手順

### 観測証跡

- recap-worker 起動時ログ:
  ```
  Token file not found "/opt/rustbert-cache/huggingface/token"
  Embedding service failed to initialize. Large genres cannot be split into subgenres.
  Falling back to keyword-only filtering.
  error: Endpoint not available error: An IO error occurred
  Caused by: Read-only file system (os error 30)
  ```
- host 観測:
  ```
  $ docker exec alt-recap-worker-1 sh -c 'ls -la /opt/rustbert-cache/ | head; mount | grep rustbert; id'
  total 8
  drwxr-xr-x 2 root root 4096 Apr 22 04:34 .
  drwxr-xr-x 1 root root 4096 Apr 22 04:08 ..
  /dev/nvme0n1p2 on /opt/rustbert-cache type ext4 (ro,relatime)
  uid=999(recap) gid=999(recap) groups=999(recap)
  ```
- 直近 3 ジョブの `recap_subworker_runs` (dispatch stage succeeded):
  ```
  job_id 99943ac6 | consumer_tech 10 clusters, politics_government 7 clusters (それ以外なし)
  job_id 00b602ac | consumer_tech 11, politics_government 4, space_astronomy 9, software_dev 7, diplomacy_security 1
  job_id c6755c70 | consumer_tech 11, politics_government 4, space_astronomy 9, software_dev 7, diplomacy_security 1
  ```
  → 30 ジャンル taxonomy のうち実際にクラスタリングが成立したのは 2-5 ジャンル。残りは `genres_no_evidence` で空。
- recap-worker ログ (dispatch 完了):
  ```
  "completed persisting final sections","total_genres":30,"genres_stored":2-5,
  "genres_failed":0,"genres_skipped":0,"genres_no_evidence":25-28
  ```
- `recap_final_sections` 過去 7 日: 0 行 (Knowledge Home 画面は stale)。

### 外部資料

- rust-bert `SentenceEmbeddingsBuilder::remote` 仕様: [`rust_bert::pipelines::sentence_embeddings::SentenceEmbeddingsBuilder::remote`](https://docs.rs/rust-bert/latest/rust_bert/pipelines/sentence_embeddings/struct.SentenceEmbeddingsBuilder.html#method.remote) — 初回実行時に HF Hub からダウンロードする前提
- `hf-hub` crate: cache miss 時は `$RUSTBERT_CACHE`/`$HF_HOME` に書き込みを試みる

---

> **Blameless Postmortem の原則:** このドキュメントは個人の過失を追及するためではなく、
> システムの脆弱性とプロセスの改善機会を特定するために作成されています。
> 「誰が悪いか」ではなく「システムのどこが改善できるか」に焦点を当ててください。
>
> 特に本 PM では、recap-worker の Dockerfile スリム化と rust-bert cache 外出し
> を行った設計者の判断を批判するのではなく、
> **「host bind 型 artefact を導入する PR が populate 責任を同一 PR 内で確立する
> ルールを組織が持っていなかった」** 穴として扱っています。
> 同じ穴は AI #8 (他サービスの silent failure audit)、AI #11 (ADR セルフチェック
> 追加)、AI #12 (Dockerfile slim 化 PR の audit) で塞ぐべきです。
> init 失敗を silent に swallow する `None` fallback は [[PM-2026-036]] と同型の
> アプリ層穴であり、Settings validator パターンを全サービスに対称展開することで
> 解決を目指します。
