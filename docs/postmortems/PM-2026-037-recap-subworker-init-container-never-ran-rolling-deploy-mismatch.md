---
title: "PM-2026-037 recap-subworker 初回 remediation の init container が rolling deploy で一度も起動されず 3days Recap が当日再発"
date: 2026-04-22
tags:
  - alt
  - postmortem
  - recap-subworker
  - docker-compose
  - deploy
  - near-miss
---

# ポストモーテム: recap-subworker 初回 remediation の init container が rolling deploy で一度も起動されず 3days Recap が当日再発

## メタデータ

| 項目 | 内容 |
|------|------|
| インシデントID | PM-2026-037 |
| 発生日時 | 2026-04-22 17:29 JST (初回 remediation デプロイ完走時点で、init container が起動しないまま recap-subworker が up) |
| 検知日時 | 2026-04-22 18:21:46 JST (手動キックした 3days Recap が `classification returned 0 results for 954 articles (service may be unavailable)` で失敗) |
| 復旧日時 | 2026-04-22 夜 (direct bind mount に切り替えた再デプロイで 3days Recap が completed を返したタイミング) |
| 影響時間 | 約 1 時間 (17:29 の初回デプロイ完走から、direct bind 修正の再デプロイ完走まで)。ただし [[PM-2026-036]] で既に 9 日間の outage が継続していた状態の延長であり、ユーザー可視状態の差分増分は実質なし |
| 重大度 | SEV-4 (near-miss) — [[PM-2026-036]] の修正が silent に ineffective だったことを同日中に検知し切替、新規のユーザー影響は無し |
| 作成者 | recap / platform チーム |
| レビュアー | — |
| ステータス | Draft |

## サマリー

2026-04-22 午後に [[PM-2026-036]] の remediation ([[000825]] 初期版: named volume + busybox one-shot init container) をデプロイ。デプロイ自体は全サービス healthy で完走したが、同日 18:21 JST の手動キックで 3days Recap が **[[PM-2026-036]] と完全に同じ文言** (`classification returned 0 results for 954 articles (service may be unavailable)`) で再度失敗。調査の結果、**init container が一度も起動されていない** ことが判明 (`docker ps -a` に痕跡なし、名前付き volume は作成されたが空のまま)。

真因は compose パターンと Alt の deploy model のミスマッチ。Alt は「1 サービスの失敗が全体を止めない」設計のため、サービス単位で targeted に compose 操作する rolling model を採る。この経路では対象サービス単独で `docker compose up <service>` 相当が `--no-deps` 付きで実行され、`depends_on` で参照される init container は起動されない ([docker/compose#9591](https://github.com/docker/compose/issues/9591) の `--no-deps` + `service_completed_successfully` 系の普遍的な挙動)。ADR-000825 初期版の検討時にこの整合性を verify していなかった。

最終 remediation として compose を direct directory-scoped bind mount に切り替え: `${RECAP_SUBWORKER_DATA_HOST_PATH:-/var/lib/alt-recap-subworker-data}:/app/data:ro`。Docker Compose v2.24+ は directory-scoped bind の host source 不在時に container create を refuse する ([docker/compose#12735](https://github.com/docker/compose/issues/12735) で 2025-04 の v2.35 時点も継続確認)。つまり **compose engine 自体が fail-closed を担保する** ため、独立した init container と名前付き volume 経由の coordination を介さずに、元の設計意図 (「artefact 不在時は起動させない」) を達成できる。

## 影響

- **影響を受けたサービス**: recap-subworker の classification 経路のみ。[[PM-2026-036]] 本体と同じスコープ。
- **影響を受けたユーザー**: 単一ホスト開発環境の操作ユーザ 1 名、手動キック 1 件。
- **機能への影響**: 3days Recap ジョブ 1 件の結果喪失 (`result_count=0, elapsed_seconds` ~25 分)。
- **データ損失**: なし。failed job の partial write はない。
- **PM-2026-036 との関係**: [[PM-2026-036]] は 2026-04-14 からの 9 日間 outage を扱う。本 PM はその修正適用が silent に失敗したことを扱う。**ユーザー可視状態の「壊れている」継続期間の純増は分 / 時間オーダー**に過ぎず、fresh な業務影響は無し。ただし「直したつもりの修正が deploy 後も直っていなかった」事実は再発防止の重要サンプル。
- **潜在影響 (避けられたもの)**: 手動キックで再発を検知せず自動バッチ待ち (次回 02:00 JST) で発覚した場合、さらに 8 時間の "silent ineffective" 期間が発生していた。検知の早さは user-triggered smoke 運用のおかげ。
- **SLO/SLA違反**: 個別 SLO 未設定。

## タイムライン

全時刻 JST。

| 時刻 | イベント |
|---|---|
| 2026-04-22 昼〜午後 | [[PM-2026-036]] の remediation を設計・実装。commits `96a2edc81` (named volume + busybox init container) + `6931f7c8a` (host-persistent path への env override 追加) を origin/main に push |
| 2026-04-22 17:29 | release-deploy 経路でデプロイ dispatched。約 23 分後に完走、全サービス healthy。**この時点で init container は起動されていないが、外形は正常に見える** |
| 2026-04-22 18:14 | recap-subworker container re-create 完了 (`docker volume inspect` 上の `CreatedAt` 時刻)。名前付き volume `alt_recap_subworker_artifacts` は作成されているが populate されていない状態 |
| 2026-04-22 18:21:46 | **検知**: 手動キックした 3days Recap が `classification returned 0 results for 954 articles (service may be unavailable)` で失敗。ユーザーがチャットで報告 |
| 2026-04-22 18:30 頃 | **対応開始**: `docker ps -a --filter name=recap-subworker-artifacts-init` で存在しないことを確認、`docker exec alt-recap-subworker-1 ls -la /app/data/` で空を確認、`docker inspect alt-recap-subworker-1` で名前付き volume のマウントを確認 |
| 2026-04-22 19:00 頃 | **原因切り分け**: deploy playbook の per-service rolling model が `--no-deps` 相当で compose を叩いていること、従って `depends_on: service_completed_successfully` で gate された init container が起動されないことを特定 |
| 2026-04-22 19:30 頃 | web-researcher skill を起動。docker/compose issue tracker と community.docker source で `dependencies: false` → `--no-deps` のマッピングを確認、v2.24+ の directory bind missing-source refuse 挙動を確認 |
| 2026-04-22 20:00 頃 | **緩和策適用**: 直接 bind mount に切り替える新 compose 版を設計・実装 (commit `473b98251`)。 init container + 名前付き volume を撤廃、`${RECAP_SUBWORKER_DATA_HOST_PATH:-/var/lib/alt-recap-subworker-data}:/app/data:ro` の directory bind に統一 |
| 2026-04-22 夜 | prod host に `/var/lib/alt-recap-subworker-data/` を配置 (artefact tarball を展開、uid/gid 999:999、mode `u=rwX,go-rwx`)。12 ファイル揃って配置完了を `ls -la` で確認 |
| 2026-04-22 夜 | **復旧確認**: push → release-deploy 完走 → 手動 3days Recap が `status=completed` を返し、9 日間の outage が解消 |

## 検知

- **検知方法**: ユーザー手動キックによる 3days Recap → 結果 `classification returned 0 results` の symptom でチャット報告。
- **TTD (Time to Detect)**: 初回デプロイ完走 (17:29) から検知 (18:21) まで約 **52 分**。再発検知自体は高速だが、「デプロイ完走 = 修正適用完了」の暗黙前提が外れる経験ではあった。
- **検知の評価**: **外形監視・CI・deploy の success ステータスのいずれもこの silent failure を捕捉できなかった**。理由:
  1. `/health` smoke は uvicorn 応答のみ見るので classifier 状態を反映しない ([[PM-2026-036]] で既出の穴、未解消)
  2. `docker compose ps` は `Up (healthy)` を表示。名前付き volume が空かどうかは外形から分からない
  3. deploy job の成功判定は per-service healthy + compose ps の整合までで、**「init container が実際に走ったか」は verify していない**
  4. staging e2e の `recap-subworker` suite は現状 hurl suite 不在で short-circuit されており、`/v1/classify-runs` の実データ smoke は走らない ([[PM-2026-036]] AI #6 として tracked、未着手)

## 根本原因分析

### 直接原因

ADR-000825 初期版の compose パターン:

```yaml
recap-subworker:
  depends_on:
    recap-subworker-artifacts-init:
      condition: service_completed_successfully
  volumes:
    - recap_subworker_artifacts:/app/data:ro

recap-subworker-artifacts-init:
  restart: "no"
  volumes:
    - <host path>:/src:ro
    - recap_subworker_artifacts:/dst
  command: <test -s then cp -r>
```

は「compose 全体を一度に up する」前提 (e.g. `docker compose up -d`) では init が正しく走る。しかし Alt の deploy は rolling model (対象サービスごとに `community.docker.docker_compose_v2` を呼び、`dependencies: false` を固定) で、これは内部的に `docker compose up <service> --no-deps` を発行する。**`--no-deps` は `depends_on` で参照されたサービスを起動も create もしない** (community.docker collection の source で確認、docker/compose 本体の挙動)。

結果、init container は 1 度も作られないまま recap-subworker が upgrade され、名前付き volume は populate されず、`/app/data` は空ディレクトリのまま。classifier の runtime guard は `FileNotFoundError` を発行して worker pool init が 300s で timeout、recap-worker 側は `classification returned 0 results for N articles` として fail-fast。

### Five Whys

1. **なぜ 3days Recap が "classification returned 0 results" で失敗したか？**
   → classifier worker pool が 300s 以内に初期化できず、5 チャンク全てで空配列を返したから。

2. **なぜ classifier worker pool が初期化できなかったか？**
   → `/app/data` 配下の joblib モデルが load できなかったから。具体的には `is_file()` guard が False を返し `FileNotFoundError`。

3. **なぜ `/app/data` のモデルが読めなかったか？**
   → 名前付き volume `alt_recap_subworker_artifacts` が作成のみで populate されておらず、マウントした `/app/data` は空のままだったから。

4. **なぜ名前付き volume が populate されなかったか？**
   → populate する役割だった init container (`recap-subworker-artifacts-init`) が 1 度も作成・起動されていなかったから。`docker ps -a` に痕跡なし。

5. **なぜ init container が起動されなかったか？**
   → Alt の deploy model は per-service rolling で各サービスを `--no-deps` 相当で up する。`depends_on: service_completed_successfully` で gate された init container はこの経路では起動対象外となる。ADR-000825 初期版はこの deploy model との整合性を verify せずに「`docker compose up` の全体一括起動」だけ想定していた。

6. **なぜ deploy model との整合性を verify しないまま ADR が通ったか？** (補足)
   → ADR レビュー項目に「本パターンが Alt の deploy model (rolling / per-service / `--no-deps` 前提) と互換か」という明示的セルフチェック項目が不在。過去の ADR も単独サービス起動前提で書かれたものが多く、deploy model 整合の文化が組織に根付いていなかった。

### 根本原因

**「compose パターンと deploy model の整合性を ADR レビューで verify する仕組みが組織にまだ無かった」**。

- 技術的に init container は compose 公式推奨の健全なパターン ([Docker Docs: Control startup order](https://docs.docker.com/compose/how-tos/startup-order/))。
- しかし Alt は fault-tolerant rolling deploy を採用しているため、`--no-deps` と非互換な compose pattern は silent に skip される。
- ADR-000825 初期版は単独 compose の視点で正しい設計だったが、deploy model と組み合わせたときの挙動を事前に想像できていなかった。

### 寄与要因

- **同文のエラーメッセージ ([[PM-2026-033]] / [[PM-2026-035]] / [[PM-2026-036]] に続き 4 連続)**: 原因層が全く違うのに user-visible 症状が同一。今回も初動は「PM-036 の修正がまだ反映されていないのでは?」と誤誘導された可能性があった。実際は別の層の問題。[[PM-2026-036]] AI #8 (error 文言区分化) が未着手のまま同様の混乱が再発。
- **deploy success = 修正適用成功、という暗黙の前提**: 「release-deploy が ✓ で完了した」ことが「意図した compose パターンが実働している」を意味しない可能性を考慮していなかった。compose 一部だけが適用される失敗モードの想定が欠如。
- **staging e2e で real artefact 経路が動いていない**: staging は recap-subworker を完全に stub しているため、real `/v1/classify-runs` → classifier → model load の経路が CI で 1 度も走らない ([[PM-2026-036]] AI #6 未着手)。prod 初回適用まで populate 失敗に気づけなかった。
- **deploy 経路の透明性低下**: サービス単位 rolling を内部で抽象化しているため、どのサービスがどのコマンドで実際に up されたかが (compose 上位ユーザ視点で) 可視化されない。init container が起動されない失敗モードが外形ログから判別しにくい。

## 対応の評価

### うまくいったこと

- **同文 error への耐性が [[PM-2026-036]] よりも高かった**。PM-035 → PM-036 で「同文 ≠ 同根本原因」を痛感した直後の経験だったため、今回は即 `docker ps -a` / `docker inspect` / `docker exec ls` の観測事実から入れた。PM-036 時の「3 PM 連続同文」の学びが生きた。
- **web-researcher skill による挙動確定**. `dependencies: false` が内部的にどう展開されるかは docker/compose の github issue tracker と community.docker source に埋まっていたが、公式 doc だけでは短時間では確定できなかった。skill を使って一次情報の issue / source に当たった判断で 15 分以内に確信度の高い結論に到達。
- **設計の差し替えが選択肢間で明確に比較できた**。web 調査レポートで (A) deploy-tooling 側に init expansion を追加、(B) direct bind に切替、(C) image に init logic 吸収、の 3 案を trade-off 付きで出し、Alt の設計原則 (deploy-tooling 変更を最小化する、compose engine の挙動に責務を寄せる) に沿って (B) を即決できた。
- **Fail-closed が多層で残っている**。init container 層が失敗しても、Settings validator (起動時) + classifier.py `is_file()` guard (runtime) の 2 層が覆っていた。どちらも `FileNotFoundError` で fail を表面化させる (silent ではない) 設計。今回の silent failure は init container 層のみのもので、本当に silent だったら PM-036 の 9 日ラテントの再現になっていた。
- **PM / ADR / runbook を同セッションで update**。design change に追随して ADR-000825 addendum、PM-2026-036 addendum、runbook 2 本を同セッションで書き切り、ドキュメント債務を翌日に持ち越さなかった。
- **情報衛生のチェック**。公開 OSS 境界を意識して、deploy-tooling 側の内部 (playbook ファイル名、workflow 行番号、services_filter 実装) を Alt の public docs から全削除。[[feedback_no_host_names_in_public]] の精神を compose 抽象レベルで維持。

### うまくいかなかったこと

- **ADR レビューで deploy model との整合性を確認する仕組みが無かった**。結果 1 回 prod に出してから気づく構造。ADR テンプレートまたは ADR レビュー checklist に明示項目を追加する必要 (AI #1)。
- **deploy success の信号の解像度が低すぎた**。「全サービス healthy」で deploy 成功を宣言しているが、「意図した compose pattern が実際に活きているか」の verify が自動化されていない。通常サービス healthcheck では捕捉できない「init container が走ったかどうか」を検知する仕組みが要る (AI #2)。
- **[[PM-2026-036]] の 3 PM 継承 AI (#6 classify-runs real data smoke、#11 recap_failed_tasks Prometheus、#12 error 文言区分) はまだ未着手**。仮にこれらが導入されていれば、prod 初回適用直後に smoke で拾えた可能性が高い。継承の滞留を打破する運用ルールが必要 (AI #3)。
- **同セッション内での再発という心理的動揺**。PM-036 の remediation が直ったつもりで出た数時間後に、同 symptom で再発 — という体験は疲労感を増す。全く同じ user-visible symptom に対して「別の根本原因」を示せる error 文言の解像度を上げる必要 ([[PM-2026-036]] AI #8 に更なる優先度)。

### 運が良かったこと

- **user の手動 smoke 習慣**. 02:00 JST 自動バッチを待たずに user が即座に手動キックしてくれたため、52 分で再発検知。自動バッチ頼みだと翌朝まで気づけず。
- **単一ホスト開発環境**. マルチテナント prod なら 52 分間の "ineffective remediation" でも影響は限定的だが、現時点で小規模開発環境であることが救いになっている。
- **Docker Compose v2.24+ の fail-closed 挙動**. direct bind 採用判断は、compose engine 側が missing-source を refuse する挙動に依存している。この挙動が無ければ direct bind でも silent empty-directory になる可能性があった ([[PM-2026-036]] の file-scoped bind と同じ footgun)。2024 年の compose 破壊的変更のおかげで採用できた設計。
- **local dev に 2026-01-31 時点の artefact snapshot が残っていた**. host path への配置 artefact source として使えた。再訓練が必要だった場合は復旧がさらに数時間遅れていた。

## アクションアイテム

| # | カテゴリ | アクション | 担当 | 期限 | ステータス |
|---|---|---|---|---|---|
| 1 | プロセス | ADR テンプレート (`docs/ADR/template.md`) に「本 ADR が採用する compose / deploy / CI パターンは Alt の per-service rolling deploy model と整合するか」のセルフチェック項目を追加。既存 ADR の rolling 互換性 audit も follow-up で実施 | platform | 2026-05-15 | TODO |
| 2 | 検知 | deploy job の post-check で、compose file に宣言された **全 service に対して `docker compose ps --all --services` または同等の state 照合**を実施し、「宣言されているのに存在しない」サービス (今回の init container のようなケース) を警告する仕組みを導入検討 | platform | 2026-05-31 | TODO |
| 3 | プロセス | [[PM-2026-033]] → [[PM-2026-035]] → [[PM-2026-036]] → 本 PM と 4 PM 連続で継承されている AI (classify-runs real smoke / `recap_failed_tasks` Prometheus / error 文言区分) の滞留を打破するため、**次の AI 継承が 1 件でも発生したら全開発停止して着手する** ルールを運用に明示。[[PM-2026-036]] の該当 AI を無期限 TODO から優先度 P0 に昇格 | recap / observability | 2026-05-15 | TODO |
| 4 | 予防 | `.env.example` に `RECAP_SUBWORKER_DATA_HOST_PATH` の説明行を追加。dev workstation で compose up したいとき `../recap-subworker/data` にフォールバックする設定例を記載。未設定時の default `/var/lib/alt-recap-subworker-data` が存在しないと compose up が fail する事実も同時に記載 | recap | 2026-05-01 | TODO |
| 5 | 予防 | [[runner-setup]] §2.6 (recap-subworker artefact 事前配置) の存在を [[runbook]] index / README に明示リンク。prod host が runner 兼務である Alt の単一マシン構成を前提に、`/var/lib/alt-recap-subworker-data/` 確認を weekly ops check 項目に組み込む | ops | 2026-05-15 | TODO |
| 6 | プロセス | web-researcher skill を「挙動に不確実性のある deploy / ansible / compose 変更の ADR 執筆前」のルーチンに組み込む。本 PM で 15 分で挙動確定できた実績を ADR writing 手順に記録する | docs / platform | 2026-05-15 | TODO |
| 7 | 予防 | compose pattern レビューで「`depends_on: service_completed_successfully` を使った init container」が提案された場合、本 PM を参照する注記を `docs/runbooks/` にまとめ、rolling deploy 非互換な代替案 (今回の direct bind や baked image) を必ず比較するテンプレートを用意 | platform | 2026-05-31 | TODO |

## 教訓

### 技術面

- **`docker compose up <service> --no-deps` は `depends_on: service_completed_successfully` を起動しない**。compose 単体起動と rolling deploy (targeted per-service) での挙動差は設計時に見落としやすい。init container に gate される compose pattern を採用するなら、deploy tooling がその init を明示的に up するかどうかを事前に確認する必要がある。
- **Docker Compose v2.24+ の directory-scoped bind missing-source refuse は高度な fail-closed 装置**。init container + 名前付き volume で実現していた「artefact 不在時に起動させない」は、実は compose engine 単体で提供されている。シンプルさ重視なら init container を挟まず直接 bind する方が薄く robust。
- **多層 fail-closed は各層独立で設計する**。本 PM では init container 層が抜けたが、Settings validator と classifier is_file guard は残っていた。**どれか 1 層が silent に skip されても他層が catch できる** 設計だったことで、ineffective remediation が短時間で露見した。各層の責務を区別し「この層が抜けたら何が起こるか」をレビューで問うべき。
- **ADR はパッチ、addendum は診断**. ADR-000825 は 1 セッション内で初期版 → 初期運用失敗 → addendum (direct bind に移行) という経路を辿った。ADR の rapid iteration を許容する体制がある一方で、Related ADRs / Decision history が内部的に複雑化する。addendum が膨らむ前に新 ADR に切り出す判断基準が要る。

### 組織面

- **「deploy 成功 ≠ 修正適用成功」を前提に smoke check を入れる**。release-deploy の ✓ は必要条件に過ぎず十分条件ではない。本 PM は user の手動 smoke 習慣に救われたが、自動化された post-deploy smoke (実データ `/v1/classify-runs` 等) を組み込むのが正しい姿。
- **同文 error への耐性を意識的に育てる**. 今回は [[PM-2026-036]] 直後だったので即「別根本原因かも」と動けたが、半年後に同種が起きたら初動で "PM-036 の再発" と誤判断する可能性が高い。error 文言区分化 ([[PM-2026-036]] AI #8) は開発者人間のパターン認識に依存しない仕組みで、最優先で実装すべき。
- **ADR レビュー項目として "deploy model との整合性"**. 本件の根本原因はまさに「compose pattern が rolling と非互換」だった。ADR テンプレートに自問項目を追加することで、同型ミスは少なくとも 1 段階前で止まるようになる (AI #1)。
- **情報衛生を守りながら設計理由を記録する**. 当初 ADR addendum に deploy-tooling 側 (private repo) の playbook ファイル名・行番号を書きかけたが、public OSS 境界違反を指摘されて compose/engine 抽象レベルに書き直した。設計決定の「なぜ」を記録する際、private な内部を参照せずに意思決定理由を説明できる必要がある。これは Alt の OSS / Private 二元配置特有の文章術。

## 参考資料

### 本 PM の修正

- [[000825]] recap-subworker の joblib artefact 欠落を Pydantic validator と named-volume-with-init-container で 2 層 fail-closed にする — 初期版 (撤回) + addendum (direct bind 移行、本 PM の final remediation)
- Alt commit `473b98251` — `fix(compose): drop init container and direct-bind recap-subworker data from host path`
- Alt commit `ece1f73f3` — `docs(adr,postmortem,runbooks): record direct-bind migration and runner-side host path bootstrap`

### 関連 PM / ADR / runbook

- [[PM-2026-036]] recap-subworker joblib artefacts bind-mount が空ディレクトリ化し 3days Recap が 8 日間 silent に失敗 — 本 PM の親。直接 3days Recap outage 本体を扱う
- [[PM-2026-035]] learning_machine artifacts 欠落で 3days Recap 948 件分 classification 失敗 — PM-036 / 本 PM の先行、同文 symptom 4 連の 1 件目
- [[PM-2026-033]] mTLS server-side gap — 同文 symptom 4 連の 0 件目 (TLS 層の別件)
- [[000727]] mTLS Phase 2 — Settings validator パターンの起源
- [[runner-setup]] §2.6 — recap-subworker prod artefact host path の runner bootstrap 手順

### 外部参照

- Docker Compose v2.24+ directory bind missing-source refuse: [docker/compose#11345](https://github.com/docker/compose/issues/11345), [#12735](https://github.com/docker/compose/issues/12735)
- `docker compose up --no-deps` + `service_completed_successfully` edge case: [docker/compose#9591](https://github.com/docker/compose/issues/9591)
- Compose file reference on bind mounts: [docs.docker.com/reference/compose-file/volumes](https://docs.docker.com/reference/compose-file/volumes/)
- community.docker collection source (docker_compose_v2 module): [ansible-collections/community.docker](https://github.com/ansible-collections/community.docker) 配下の該当 module

---

> **Blameless Postmortem の原則:** このドキュメントは個人の過失を追及するためではなく、
> システムの脆弱性とプロセスの改善機会を特定するために作成されています。
> 「誰が悪いか」ではなく「システムのどこが改善できるか」に焦点を当ててください。
>
> 特に本 PM では、ADR-000825 初期版を「init container を使う選択をした設計者の見落とし」ではなく、
> **「ADR レビュー時に compose パターンと deploy model の整合性を verify する仕組みが組織に存在しなかった」** 穴として扱っています。
> 同じ穴は AI #1 (ADR テンプレート更新) + AI #2 (deploy post-check 強化) + AI #6 (不確実性のある変更の ADR 前 web 調査ルーチン化) で塞ぐべきです。
> init container + `service_completed_successfully` は Docker 公式推奨の正当なパターンであり、非難の対象は設計選択ではなく「deploy model との組み合わせを verify せずに merge が通る組織の仕組み」です。
