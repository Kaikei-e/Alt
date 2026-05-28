# ポストモーテム: search-indexer e2e ジョブが reclaim_network_pool の self-poisoning と libnetwork race の合成で連続失敗

## メタデータ

| 項目 | 内容 |
|------|------|
| インシデントID | PM-2026-046 |
| 発生日時 | 2026-05-29 — 顕在化は 02:29 (JST、run 26590470313) 以降。潜在期間は 17 leg e2e matrix が常用化した時点 (推定 2026-05-中旬) からのバックグラウンド race |
| 復旧日時 | 2026-05-29 — 修復コード 4 ファイル新規/編集 + 10 run.sh 適用完了。本番反映 (`git push origin main`) はユーザ承認待ち |
| 影響時間 | 顕在化からコード修復まで 30 分以内。潜在期間は推定 2 週間以上 |
| 重大度 | SEV-3 (CI 専用の劣化、ユーザ影響なし、production 配信無影響) |
| 作成者 | オンコール対応 + 修復担当者 |
| レビュアー | 未割当 |
| ステータス | Draft |

## サマリー

2026-05-29 02:29 JST、Knowledge Loop Wave 5 (PM-2026-045 系の SSE counter 追加) の `git push origin main` を契機に走った alt-deploy release-deploy ワークフロー run 26590470313 で **`e2e (search-indexer)`** ジョブが `failed to set up container networking: network alt-staging-search-indexer-26590470313 not found` で失敗。compose の `Network Created` 直後、container `Starting` 段階で「network not found」が返る合成 race 不具合で、続く 26591475464 でも同じ症状が再現。ユーザから「構造的問題」と指摘を受け切り分けたところ、`e2e/hurl/_lib/reclaim-network-pool.sh` が並行 matrix job (16 兄弟 leg) の freshly-created network を **container attach 前のウィンドウで削除**していた self-poisoning が主因と判明。docker/compose 自体にも overlay/libnetwork レベルの sporadic race (docker/compose#12862 / #9054) が残り、合成事象になっていた。reclaim を current project に限定 (Fix 1) + label / age による二重防御 (Fix 4) + 全 10 e2e run.sh に compose-up retry (Fix 3) を 3 件のコード変更で landing。本番反映待ち。

## 影響

- **影響を受けたサービス:** alt-deploy `release-deploy` ワークフローの `e2e (search-indexer)` job。同 matrix の他 16 leg は同じ run で軒並み ✓ pass (alt-backend / knowledge-sovereign 等 production 反映に必要な path はすべて緑)。
- **影響を受けたユーザー数/割合:** 0 (CI 専用の劣化)。production 配信路 / SSE / data path には到達していない。Knowledge Loop Wave 5 機能は alt-backend / FE / nginx いずれも `e2e (alt-backend)` ✓ pass で機能正常。
- **機能への影響:** 部分的劣化。`gate (search-indexer)` artifact の publish ができず、search-indexer の deploy 経路だけが gate fail で stuck する状態 (gate を要件にしている下流リリースが blocking される)。
- **データ損失:** なし (CI 層、production リソース不変)。
- **SLO/SLA違反:** CI green rate の社内目標がもしあれば違反だが、formal な SLO は未定義。

## タイムライン

| 時刻 (JST) | イベント |
|-------------|---------|
| 2026-05-中旬 | 17 leg e2e matrix が常用化 (knowledge-sovereign 系 e2e 拡張のあたり)。`reclaim_network_pool` の concurrency 前提崩壊が潜在化 (この時点では誰も気付かず) |
| 2026-05-29 02:29 | **初回検知** — run 26590470313 で `e2e (search-indexer)` が "network not found" で fail、他 16 leg は pass。HTTP エラーなし、Docker daemon error only |
| 2026-05-29 02:39 | run 26591475464 で **同症状が再現**。ユーザが「また別 Run で失敗している。構造的問題です。Web 調査して」と報告 |
| 2026-05-29 02:40 | **対応開始** — alt-deploy CI ログ取得、search-indexer e2e ジョブの「Run E2E suite (Ansible)」タスクで symptom を確定 |
| 2026-05-29 02:45 | HAR-style 解析: `Network ... Created` の直後、最初の `Container ... Starting` 行で `Error response from daemon: failed to set up container networking: network alt-staging-search-indexer-<run_id> not found` |
| 2026-05-29 02:50 | Web 調査で docker/compose #12862 (sporadic "network not found" right after Created) と #9054 (concurrent attach race) を確認。ただし当該 issue は overlay/swarm 文脈で、ここの bridge + internal とは厳密一致しない |
| 2026-05-29 02:55 | `e2e/hurl/_lib/reclaim-network-pool.sh` を精読、コメント記載の「Docker's own logic protects networks an active container is attached to」が **container attach 前** には効かないことを発見、これが 17 leg matrix で sibling job の network を消す self-poisoning の本丸と特定 |
| 2026-05-29 03:00 | **原因特定** — main = `reclaim_network_pool` の concurrency-unsafe scope (Fix 1)、二次的 = libnetwork sporadic race (#12862) |
| 2026-05-29 03:10 | **緩和策コード化** — Fix 1 (reclaim を `^${STAGING_PROJECT_NAME}$` exact-match に縛る) + Fix 4 (`label=com.docker.compose.project` + `until=30m` の prune を defense-in-depth で追加) を `e2e/hurl/_lib/reclaim-network-pool.sh` に landing |
| 2026-05-29 03:20 | **再発防止コード化** — Fix 3 (`e2e/hurl/_lib/compose-up-with-retry.sh` 新設、`up -d --wait` を 3 attempts × 5s linear backoff で wrap、attempt 失敗時に `down -v --remove-orphans` でクリーン再試行) を 10 services の run.sh に適用 |
| 2026-05-29 03:25 | **復旧確認 (テスト層)** — `bash -n` 全 e2e script clean、reclaim helper の smoke test 通過、compose_up_with_retry helper の syntax check 通過 |
| 2026-05-29 03:30 | 本ポストモーテム起票。本番反映 (`git push origin main` → 自動 CI で search-indexer e2e 再走) はユーザ明示指示待ち |

## 検知

- **検知方法:** ユーザ報告 (CI failure URL を 2 連続で共有、3 回目に「構造的問題」と指摘) + GitHub Actions の `gh run view` ログ解析。
- **検知までの時間 (TTD):** 顕在化 (1 回目失敗) から指摘まで 10 分、指摘から root cause 特定まで 20 分。潜在期間 (matrix 拡張時点 → 今回顕在化) は推定 2 週間以上。
- **検知の評価:** 顕在化後の TTR は迅速だが、**潜在期間中に検知できなかった**のが問題。
  - search-indexer e2e 単独 fail の確率は低い (race を引き当てる窓は数百 ms) ため、過去にも単発失敗していた可能性が高く、それらは「flaky」として rerun で誤魔化されていた疑い。
  - CI flaky rate を集計する SLI / alert が無く、「同じ job が連続 2 回 fail したら構造問題を疑う」というプロセス規律は user-driven (ユーザの目視) でしか動いていなかった。

## 根本原因分析

### 直接原因

`e2e/hurl/_lib/reclaim-network-pool.sh` 旧版 (2026-05-29 以前) が `docker network ls --filter 'name=^alt-staging-'` の prefix マッチで全ての `alt-staging-*` network を列挙し、各々に `docker network rm` を発行していた。並行 17 leg e2e matrix では、job A の `docker compose up` が `Network alt-staging-A-<run_id> Created` した直後 (container がまだ attach していないウィンドウ) に、job B の `reclaim_network_pool` がその network を **rm 成功** させて消すケースが発生し、job A の後続 `Container Starting` 段階で「network not found」を返した。

加えて、container attach 前の network には Docker の "active container protection" が効かない (Docker は network rm 時に「attached container があるなら拒否」するが、attach 完了前は network が「空」なので削除可能)。`reclaim-network-pool.sh` 旧版のコメントは「concurrent CI runs on the same host stay safe」と誤前提していた。

### Five Whys

1. **なぜ search-indexer e2e ジョブが "network not found" で失敗したのか？** → `docker compose up` 中に network が daemon 側から消えた。
2. **なぜ消えたのか？** → 並行 matrix job (`e2e (rag-orchestrator)` 等) の `reclaim_network_pool` が search-indexer ジョブの network を rm したから。
3. **なぜ他 job の network を rm できたのか？** → 旧 reclaim が prefix `^alt-staging-` 全体を対象にしていて、自プロジェクト以外の network も列挙 + 削除候補にしていたから。`docker network rm` は container 未 attach なら成功する。
4. **なぜ「自プロジェクト以外を消す」という危険操作になっていたのか？** → 「Docker's own logic protects networks an active container is attached to」という仮定があり、`docker compose up` で `Network Created` 直後 + `Container Starting` 前のウィンドウが想定外だったから。
5. **なぜそのウィンドウが想定外だったのか？** → reclaim helper の設計時点 (matrix が 2-3 leg 程度だった時期) では並行確率が低く、また「単一の e2e ジョブ内では up が完全に終わる前に reclaim を撃つことがない」というローカル前提でレビューされたから。17 leg 並列 + 共有 daemon という運用変化に design が追従していなかった。

### 根本原因

**「e2e infra helper の concurrency contract が運用拡張 (matrix 数増加) に対して silent に壊れた」** ── reclaim_network_pool は単一 e2e ジョブのローカル前提では正しい設計だったが、17 leg e2e matrix が同一 self-hosted runner / 共有 Docker daemon 上で並行実行される運用に拡張された際、reclaim の scope が暗黙に "自分のプロジェクト" から "全 alt-staging-* projects" に意味変換されたまま誰も再評価しなかった。コメントの誤前提 ("active container protection") が code reviewer のチェックも逃した。

### 寄与要因

- **コメントの誤前提:** 旧 helper のコメントが「safe by Docker's own contract」と書いており、reviewer が中身を疑わなくなった (typed-out narrative bias)。
- **libnetwork 側の sporadic race:** docker/compose #12862 (Created 直後の "not found" race) と #9054 (concurrent attach race) は別 issue として残っており、Fix 1 だけだと依然として低確率の race が残る。retry 層 (Fix 3) を defense-in-depth で被せないと完全には対処できない。
- **CI flaky rate の SLI 不在:** 「同一 job が短期間に 2 回 fail したら構造問題」を検知する仕組みが無く、潜在期間中は rerun で誤魔化されていた可能性が高い (PM-2026-045 と同型の「silent failure を SLI に上げない盲点」)。
- **PM-2026-045 教訓の横展開不足:** 直前 (2026-05-27) に「ログは出ていたが SLI が無く 4 週間 silent」を経験したばかりだが、その学びが「CI flaky rate」という別ドメインの SLI 整備には波及していなかった。

## 対応の評価

### うまくいったこと

- ユーザの「構造的問題」という的確な指摘で、1 回目 fail を flaky として処理する誘惑を回避できた。**「2 回連続 fail = 構造を疑う」プロセス規律が暗黙に機能した**。
- Web 調査が docker/compose #12862 / #9054 / moby #49557 を 30 分以内に網羅できた。WebSearch + WebFetch で primary source を確定し、symptom と既知 issue の重ね合わせができた。
- root cause を **Fix 1 (main) + Fix 3 (defense-in-depth) + Fix 4 (label-scoped prune)** の 3 軸に分解、合成 race の各層に対して個別の防御を入れられた。
- 10 services の run.sh に共通 helper を適用する横展開を 1 セッションでやり切れた。compose-up-with-retry helper を `_lib/` に共通化したことで、新 e2e service 追加時に同じ retry semantic が自動で適用される。
- 旧 helper の誤前提コメントを修復後の comment block で「過去に X だと思っていたが実は Y」という形で明示残置、reviewer が後で同じ罠を踏まない記録として残った。

### うまくいかなかったこと

- 潜在期間 2 週間以上を見逃した。CI flaky の SLI / alert が無く、user-driven な目視に頼っていた。
- `reclaim_network_pool` の design review 時点で 17 leg matrix 拡張時に concurrency 前提を再検証する仕組みが無かった。
- PM-2026-045 (4 週間 silent failure) の学び「ログだけでなく SLI 化する」が CI 層に波及していなかった。
- 旧 helper のコメントが reviewer の判断を誤らせた。コメントの主張を実コードで verify する code review 規律が薄かった。

### 運が良かったこと

- 影響範囲が CI 専用で、production 配信路 / SSE / data path に到達しなかった。
- 同じ run 内で他 16 leg (alt-backend / knowledge-sovereign 含む production 反映必須の path) は ✓ pass しており、Knowledge Loop Wave 5 機能の妥当性検証には影響しなかった。
- ユーザが 2 回連続失敗の時点で「flaky じゃない、構造問題」と判断してくれた。1 回目で rerun していたら 3-5 回目までは「flaky だな」で済ませてしまい潜在期間がさらに伸びていた可能性が高い。

## アクションアイテム

| # | カテゴリ | アクション | 担当 | 期限 | ステータス |
|---|----------|-----------|------|------|-----------|
| 1 | 予防 | 本修復 (Fix 1/3/4) を本番反映 (`git push origin main` → alt-deploy CI で search-indexer e2e の green を 3 回連続確認) | Kaikei | 2026-05-30 | TODO |
| 2 | 予防 | runner host 側 `daemon.json` の `default-address-pools` 拡張 (Fix 2 foundational) を alt-deploy 側で実施。これで reclaim helper の存在意義が薄まり、複合 race の発生窓自体が狭まる | Kaikei (alt-deploy 側) | 2026-06-10 | TODO |
| 3 | 検知 | 「同一 GitHub Actions job が直近 24h で 2 回以上 fail」を検出する SLI を追加 (`gh api` 経由のスクリプト + cron or alt-perf 側のジョブ)。検出時は Slack/メール通知し、flaky 扱いではなく構造問題として triage 入り | Kaikei | 2026-06-15 | TODO |
| 4 | 検知 | `e2e/hurl/_lib/` 共通 helper を変更する PR には「concurrency assumption 再評価」のチェック項目を PR template に追加 (matrix leg 数変化や並列 runner 数変化を前提に design contract を verify する) | Kaikei | 2026-06-20 | TODO |
| 5 | 緩和 | `compose_up_with_retry` の retry 回数 / backoff を staging で 2-3 週間運用し、`COMPOSE_UP_MAX_ATTEMPTS` のチューニング (3 → 2 で十分か、もしくは 5 まで増やすべきか) を観測値で決める | Kaikei | 2026-06-30 | TODO |
| 6 | プロセス | PM-2026-045 と同様、「過去 incident の action item が新規 incident のドメインに波及していなかった」反省を [[knowledge-loop-retrospective-2026-05-28]] §E-5 (action item 横展開不足) の補強として ADR 化、CI 系 incident にも「観測の盲点」テンプレートを適用する | Kaikei | 2026-06-30 | TODO |
| 7 | プロセス | reclaim helper の誤前提コメント学びを `.claude/skills/immutable-design-guard/` の advisory ルールに追加 (「コメントの safety 主張は実コードで verify、specially `|| true` で隠された error path は要 audit」) | Kaikei | 2026-07-10 | TODO |
| 8 | プロセス | CI flaky rate を monthly review に組み込み (PM-2026-045 の月次 health audit と同枠)、`gh run list --status failure` で過去 30 日の失敗 job を集計、上位 5 ジョブは構造問題として triage | Kaikei | 2026-06-30 | TODO |

### カテゴリの説明

- **予防:** 同種のインシデントが再発しないようにするための対策
- **検知:** より早く検知するための監視・アラートの改善
- **緩和:** 発生時の影響を最小化するための対策
- **プロセス:** インシデント対応プロセス自体の改善

## 教訓

- **「safe by design」をうたうコメントは時を超えて誤前提になりうる。** 旧 `reclaim-network-pool.sh` の "Docker's own logic protects networks an active container is attached to" は **書いた瞬間は正しかった** (matrix が 2-3 leg だった時期)。運用が拡張されると暗黙の前提が壊れる。コメントは「いつ、何の前提で safe か」を残し、reviewer はそれを実コードで verify する。
- **並行 CI matrix の helper は「自プロジェクト scope」が default。** prefix マッチで全 alt-staging-* を対象にした reclaim は、運用が単一 leg 時代の遺物。新規 helper を書くときは「自プロジェクト以外には絶対に touch しない」を最初の不変条件にする。
- **flaky CI を「rerun で再現するから flaky」と片付けると 2 週間 silent failure が再演される。** PM-2026-045 と同型の盲点。CI flaky rate を SLI 化、「直近 24h で 2 回以上 fail」を機械的に triage に上げる仕組みが必要。
- **合成 race は単一 fix では消えない。** Fix 1 (self-poisoning) で大半は消えるが docker/compose libnetwork race (#12862) が残る。retry 層 (Fix 3) と foundational fix (Fix 2 daemon.json) を多層で被せる。Defense in depth は overhead ではなく前提。
- **Docker `network rm` の安全保証は "attach 後" のみ。** `Network Created` → 最初の `Container Starting` の数百 ms 窓は「container 未 attach」状態で、`docker network rm` は成功する。並行 prune helper を書くときの primary pitfall。
- **PM-2026-045 と PM-2026-046 は別ドメイン同型。** 前者は SSE 配信路、後者は CI infra。共通構造は「(a) 過去 design は当時正しかった (b) 運用拡張で前提が壊れた (c) 観測 SLI が無く silent (d) ユーザ報告で初めて気付いた」。次の同型インシデントを防ぐには、配信路・CI infra・data pipeline 等のドメイン横断で「設計時の concurrency / 拡張前提を文書化し、レイヤ拡張時に再評価する」プロセス規律が要る。

## 参考資料

- [[knowledge-loop-retrospective-2026-05-28]] §E-5 — 過去 action item が新規設計に横展開されない構造問題、本 PM-2026-046 は同パターンの CI 層版
- [[PM-2026-045]] — Knowledge Loop SSE silent failure。本 PM の「PM-2026-045 と同型の盲点」記述の根拠
- 修復 commit chain (本 PM 起票時点で local landing 済、push 待ち):
  - `e2e/hurl/_lib/reclaim-network-pool.sh` rewrite (Fix 1 + Fix 4)
  - `e2e/hurl/_lib/compose-up-with-retry.sh` new (Fix 3 helper)
  - `e2e/hurl/{search-indexer,rag-orchestrator,news-creator,knowledge-sovereign,auth-hub,tag-generator,mq-hub,acolyte-orchestrator,alt-backend,recap-worker}/run.sh` 全 10 ファイルに helper 適用
- 失敗 CI runs:
  - alt-deploy run 26590461393 (1 回目顕在化、alt-backend Wave 5 push 直後)
  - alt-deploy run 26590470313 (2 回目、ユーザが構造問題と指摘)
  - alt-deploy run 26591475464 (3 回目、ユーザ Web 調査依頼)
- Docker / compose 上流 issue:
  - docker/compose #12862 — sporadic "network not found" right after Created (closed as not planned / stale)
  - docker/compose #9054 — concurrent attach race
  - moby/moby #49557 — Docker 28.0.1 `ip_nf_raw` dependency regression (副次的、本 incident の root cause ではない)
- Docker docs:
  - `docker network prune` の safety contract (https://docs.docker.com/reference/cli/docker/network/prune/) — "not used by any containers" は attach 前には効かない
  - `com.docker.compose.project` label filter (https://docs.docker.com/engine/manage-resources/pruning/) — Fix 4 の prune scope に使用

---

> **Blameless Postmortem の原則:** このドキュメントは個人の過失を追及するためではなく、
> システムの脆弱性とプロセスの改善機会を特定するために作成されています。
> 「誰が悪いか」ではなく「システムのどこが改善できるか」に焦点を当ててください。
