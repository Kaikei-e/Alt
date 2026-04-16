# ポストモーテム: pki-agent sidecar の netns 幽霊化による AcolyteService/ListReports 502

## メタデータ

| 項目 | 内容 |
|------|------|
| インシデントID | PM-2026-030 |
| 発生日時 | 2026-04-17 00:07 JST（ラテント発火開始。`acolyte-orchestrator` コンテナが Pact ゲート付き deploy で recreate され、`pki-agent-acolyte-orchestrator` が旧 netns に取り残された時刻） |
| 検知日時 | 2026-04-17 06:38 JST（ユーザーが DevTools で `api/v2/alt.acolyte.v1.AcolyteService/ListReports:1 Failed to load resource: 502` を発見） |
| 復旧日時 | 2026-04-17 06:49 JST（一次: `docker compose up -d --no-deps --force-recreate pki-agent-acolyte-orchestrator`）／2026-04-17 06:55 JST（恒久: 3 層防御 commit `1c7679801` デプロイ完了） |
| 影響時間 | ユーザー体感 11 分（06:38 → 06:49）／ラテント期間 6 時間 31 分（00:07 → 06:38） |
| 重大度 | SEV-3（Acolyte Reports ページの single-feature 停止。Knowledge Home / Feeds / Recap / Summarize 経路は影響なし。単一ホスト開発環境の単一ユーザー） |
| 作成者 | pki / platform チーム |
| レビュアー | — |
| ステータス | Draft |

## サマリー

[[ADR-000754]]（Desktop FeedDetail editorial rail）の Pact ゲート付き deploy で `acolyte-orchestrator` コンテナが `--force-recreate` された際、`pki-agent-acolyte-orchestrator` sidecar が `network_mode: service:acolyte-orchestrator` で古いコンテナ ID (`debe114cb69b...`) を指したまま取り残され、新しい netns (IP `172.18.0.35`) からは **:9443 で誰も listen していない**状態になった。[[ADR-000715]]以来 pki-agent の TLS reverse proxy ( = [[PM-2026-029]] の nginx sidecar 置換) は起動以降 `tick ok` ログしか出さない sidecar として稼働していたが、今回は listener 自体が幽霊化したにもかかわらず **sidecar は healthy を報告し続けた**。6 時間 31 分後にユーザーが Acolyte Reports を開こうとして 502 を踏み、初めて可視化された。一次復旧は sidecar の `--force-recreate` で約 1 分。恒久策として 3 層防御を [[ADR-000757]] で実装: (1) pki-agent の server goroutine 終了を fatal に (`exit(4)` + `restart: unless-stopped` respawn)、(2) `pki-agent healthcheck` が `PROXY_LISTEN` 指定時は TCP dial で listener もプローブ、(3) `scripts/deploy.sh` が親サービス recreate 時に `pki-agent-<svc>` を cascading recreate する。同パターンは [[PM-2026-029]] と並び、「cert/proxy sidecar が生存しているが機能していない」silent failure の第二形態として記録する。

## 影響

- **影響を受けたサービス:** acolyte-orchestrator 経由の全 RPC（`AcolyteService.ListReports` / `GetReport` / `CreateReport` / `StartReportRun` / `StreamRunProgress` 等）
- **影響を受けた画面:** `/acolyte/reports` およびその配下。その他 (Knowledge Home, Feeds, Recap, Augur chat, tag-trail) は経路が異なるため影響なし
- **影響を受けたユーザー数/割合:** 単一ホスト開発環境の操作ユーザー 1 名。実質的な外部顧客影響はゼロ
- **機能への影響:** Acolyte レポート一覧が完全に取得不能（BFF → `https://acolyte-orchestrator:9443` が `connection refused` で全 502）
- **データ損失:** なし（read 系 RPC のみ失敗。Write 系の `CreateReport` 等もエラー返却で副作用なし、checkpoint resume あり）
- **SLO/SLA違反:** Acolyte 個別 SLO は未設定。Knowledge Home SLO 経路には波及しない
- **潜在影響:** ラテント期間 6h31m の間、Pact gate は全サービス healthy と判定していた。本番多人数運用では silent failure として累積する危険があった

## タイムライン

| 時刻 (JST) | イベント |
|-------------|---------|
| 2026-04-16 23:52–23:56 | `49333835e` / `c8242ee68` など一連の deploy-script fix が main にマージ、新 `--skip-verify` ワークフローで Pact gate を通せるように |
| 2026-04-17 00:04:15 | commit `593888896 feat(alt-frontend-sv): add desktop editorial rail to FeedDetail view` |
| 2026-04-17 00:04–00:07 | FE-only deploy (`scripts/deploy.sh --skip-verify production`) が実行。layered rolling recreate の中で `acolyte-orchestrator` も `--no-deps --force-recreate`。**新 ID `20b4306fd0d0` が割り当てられ、172.18.0.35 が新 netns に再紐づけ**。この時点で `pki-agent-acolyte-orchestrator` は deploy.sh の対象外だったため recreate されず、古い container ID `debe114...` を指したまま user-space だけ稼働を継続 |
| 2026-04-17 00:07 | **ラテント発火開始** — `pki-agent-acolyte-orchestrator` の TLS reverse proxy ( :9443 ) が外部 IP 空間から unreachable。プロセス自身の loopback と /proc/net/tcp6 は旧 netns を保持するため listener が alive に見える |
| 2026-04-17 01:17–06:44 | 追加 FE 修正 (`c38974161` / `8531c354e` / `091011597` / `7ee076cc5` / `182cc13a8` / `63eff5caf`) と、2 回目の deploy。`acolyte-orchestrator` は 2 度目の recreate はされず pki-agent 幽霊状態が維持 |
| 2026-04-17 06:38 | **検知** — ユーザーが Acolyte Reports ページに遷移、DevTools console に `api/v2/alt.acolyte.v1.AcolyteService/ListReports: Failed to load resource: 502` が出ているのに気づきチャットで報告 |
| 2026-04-17 06:39 | **対応開始** — BFF ログ (`alt-alt-butterfly-facade-1`) に `Post "https://acolyte-orchestrator:9443/.../ListReports": dial tcp 172.18.0.35:9443: connect: connection refused` を発見 |
| 2026-04-17 06:40–06:44 | **切り分け** — acolyte-orchestrator は `Up 7 hours (healthy)` かつ FastAPI :8090 は応答。pki-agent 側で `docker exec alt-pki-agent-acolyte-orchestrator-1 awk "\$4==\"0A\" {print \$2}" /proc/net/tcp6` → `[::]:9443` LISTEN は存在。しかし `docker exec acolyte-orchestrator python3 ... socket.connect_ex(('127.0.0.1',9443))` → `111` (ECONNREFUSED)。矛盾の原因を `docker inspect alt-pki-agent-acolyte-orchestrator-1 --format '{{.HostConfig.NetworkMode}}'` で確認 → `container:debe114cb69b...`、しかし現在の acolyte-orchestrator は ID `20b4306fd0d0`。**ネットワーク namespace 幽霊化** を特定 |
| 2026-04-17 06:47 | **一次緩和の試行** — `docker compose restart pki-agent-acolyte-orchestrator` は `joining network namespace of container: No such container: debe114...` で失敗。restart では netns 参照の更新ができないと判明 |
| 2026-04-17 06:49 | **一次復旧** — `docker compose up -d --no-deps --force-recreate pki-agent-acolyte-orchestrator`。新 sidecar が新 netns の 172.18.0.35 に join、`TLS reverse proxy listening addr=:9443` ログ出現、`connect_ex=0` で外部疎通確認。BFF → acolyte:9443 が `502 → 401` (認証未添付のため正常な拒否) に回復 |
| 2026-04-17 06:50 | **根本原因の設計セッション** — Web best-practice 調査: Go errgroup による fatal propagation、Kubernetes liveness-probe の port-level probe、Docker `network_mode: service:X` 公式 semantics を参照。3 層防御方針 (fail-fast + listener probe + deploy cascading) を確定 |
| 2026-04-17 06:51–06:54 | **恒久策実装** — `pki-agent/cmd/pki-agent/healthcheck.go` 新設（`runHealthcheck` + unit test 5 件）、`main.go` を `chan error` ベースの fail-fast に refactor、`scripts/_deploy_lib.sh:deploy_single_service` が `pki-agent-<svc>` を cascading で `--force-recreate` + `wait_until_healthy` するように変更。`go test ./... -race` 緑、ADR-000757 執筆 |
| 2026-04-17 06:55 | commit `1c7679801 fix(pki-agent): make sidecar fail-fast on silent listener death and cascade recreate with parent` |
| 2026-04-17 06:55–06:56 | `scripts/deploy.sh --skip-verify production` で pki-agent 新バイナリを全関連 sidecar に適用。`pki-agent-acolyte-orchestrator` / `pki-agent-tag-generator`（`PROXY_LISTEN=:9443` を持つ唯一の 2 件）で healthcheck に listener プローブが効くことを `PROXY_LISTEN=:59999 /usr/local/bin/pki-agent healthcheck` → `exit=1`、通常時 `exit=0` で確認 |

## 検知

- **検知方法:** ユーザー報告（DevTools Network タブでの 502 目視）
- **検知までの時間 (TTD):** 6 時間 31 分（ラテント期間）
- **検知の評価:** **遅すぎた**。問題は deploy 完了直後 (00:07 JST) に発火していたが、pki-agent の `/healthz` が cert 鮮度しか見ていなかったため Docker の healthcheck は green を出し続け、アラート発火もなかった。もし本番多人数運用であれば 502 率として先に指標に出たはずだが、単一ホスト開発環境では指標ダッシュボードを常時見ていない。本質的には **TTD の改善策が本インシデントのアクションアイテムの中心**（ADR-000757 Decision 2 の listener プローブが該当）。

## 根本原因分析

### 直接原因

`pki-agent-acolyte-orchestrator` コンテナが、既に `--force-recreate` で置き換わった acolyte-orchestrator 旧コンテナの netns に join した状態のまま稼働し続けた。外部 IP (172.18.0.35) が新 netns に再紐づけされたため、BFF が `https://acolyte-orchestrator:9443` に dial しても旧 netns の listener には到達できず、新 netns に listener は存在しないので ECONNREFUSED。

### Five Whys

1. **なぜ BFF から acolyte:9443 が connection refused だったか？**
   → 172.18.0.35:9443 に listen しているプロセスが（新 netns から見て）存在しなかったから。
2. **なぜ pki-agent sidecar は listener を提供していなかったか？**
   → pki-agent プロセス自身は動いていて /proc/net/tcp6 に `[::]:9443` LISTEN が見えているが、それは既に消滅した旧 netns の中だけ。新 netns からは見えない。
3. **なぜ pki-agent は旧 netns に取り残されたのか？**
   → `scripts/deploy.sh --skip-verify production` の rolling recreate が `acolyte-orchestrator` を `--force-recreate` した際、その sidecar である `pki-agent-acolyte-orchestrator` は `DEFAULT_LAYERS` にも deploy 対象リストにも無く、deploy.sh が一切触らなかった。`network_mode: service:X` は compose の仕様上、X の recreate で自動追随しない。
4. **なぜ deploy.sh は sidecar を cascading しなかったのか？**
   → [[ADR-000747]] pki-agent 導入時および [[ADR-000752]] deploy スクリプト策定時、sidecar と親サービスの netns 結合が recreate 時に壊れることが想定されていなかった。[[PM-2026-029]] で nginx sidecar → pki-agent proxy への移行を決めた際に「cert 自体は hot-reload できるから再起動不要」という認識で終わってしまい、**sidecar プロセスそのものが recreate される必要があるケース (= 親の netns 再作成) は別問題**という視点が抜けていた。
5. **なぜその視点の抜けを検知できなかったのか？**
   → pki-agent の healthcheck が cert 鮮度だけを見ていて、sidecar が担う本来の機能 (TLS reverse proxy としての受付能力) を一切プローブしていなかった。`listening しているか = healthy` というユーザー目線の定義と、`cert が新しい = healthy` という現実装の定義の間に silent ギャップがあり、それが静かに積み重なっていた。

### 根本原因

**compose の `network_mode: service:X` 構成において、X が recreate されるときに sidecar を一緒に recreate する運用契約が deploy パイプラインに存在しなかった** こと、および **sidecar の healthcheck が担っている機能の liveness を反映していなかった** ことの複合。前者が発火源、後者が検知を遅らせた増幅器。

### 寄与要因

- **[[PM-2026-029]] の過信**: "pki-agent を Go で書き直したので cert hot-reload が解決した" という認識で、nginx sidecar クラスの全障害が消えたように感じていた。実際には「cert が古い」とは別軸の「netns が古い」類似問題が残っていた。
- **デプロイ回数の急増**: 2026-04-16 23:09 以降に `49333835e` / `1f37d06f5` / `6b16b6a2f` / `fe48a364d` / `2e1a5e518` など deploy script 修正が立て続けに 5 回、その後 FE 変更で更に 2 回 deploy が走った。sidecar 幽霊化が累積しやすい条件。
- **Pact gate の設計範囲**: Pact `can-i-deploy` はサービス間契約の互換性だけを見ており、インフラ的な netns 整合性は対象外。gate 通過 = デプロイ健全、という油断につながっていた。
- **単一ホスト運用**: 本番マルチホスト + 監視基盤があれば、502 率が指標として先に浮上したはず。単一ホスト開発環境の不可視性が TTD 悪化に寄与。

## 対応の評価

### うまくいったこと

- **切り分けのスピード** — 検知後 10 分以内に netns 幽霊化という非自明な root cause にたどり着けた。`docker inspect --format '{{.HostConfig.NetworkMode}}'` で stale container ID を拾う手順は記憶に刻まれた。
- **一次復旧の最小侵襲性** — `--force-recreate` 1 コマンドで復旧。compose down / up を避け、他サービスは無停止。
- **恒久策の設計品質** — Web best-practice (Go errgroup, K8s liveness probe port-level, Docker network_mode semantics) を事前調査してから、3 層防御として層ごとに責務を明確化した上で実装できた。単層で強引に塞ぐ誘惑を避けられた。
- **TDD** — `runHealthcheck` を RED→GREEN→REFACTOR で書いたため、listener プローブの条件分岐 (`PROXY_LISTEN=""` 時のスキップ、bare `:PORT` の loopback 解決) に自信が持てた。
- **ADR で代替案を記録** — 却下した "inotify で親 PID 監視"、"pki-agent を別 netns に出して upstream を 0.0.0.0 バインド" などの選択肢と却下理由を ADR-000757 に残し、将来の振り返り時に同じ議論を繰り返さなくて済む形にした。

### うまくいかなかったこと

- **検知の遅さ** — 6 時間 31 分のラテント期間。sidecar 自身から「listener が機能していない」シグナルを発する仕組みが無かった。
- **最初の緩和試行の迂回** — `docker compose restart` が netns 参照を更新できないことに気づかず 1 回空振りした (06:47 → 06:49)。runbook に "sidecar が netns orphan の場合は restart ではなく --force-recreate" と明記すべきだった。
- **[[PM-2026-029]] のアクションアイテム漏れ** — "pki-agent proxy mode の liveness を cert 鮮度以外でも測る" という follow-up が同 postmortem のアクションアイテムに入っていなかった。今回の発生はそのカバレッジ不足が顕在化しただけで、実質的には既知リスクだった。
- **Pact broker に silent failure の痕跡を残せていない** — deploy gate が pass したあと sidecar が使えなくなっても broker は気づきようがない。それ自体はツールのスコープ外だが、"deploy 後の function-level smoke" が global_smoke の 4 エンドポイント (`/health` 系) に限られていて、acolyte/tag-generator といった sidecar 越しのサービスを叩いていなかった。

### 運が良かったこと

- **検知経路がブラウザ DevTools** — ユーザーが たまたま DevTools を開いていたのが救い。開いていなければさらに長期化していた。本番ではアラートで拾う必要がある。
- **単一ユーザー開発環境** — 本番多人数運用で同じことが起きていれば、Acolyte を使う全ユーザーが 6h31m ブロックされていた。PR/お試し運用のうちに顕在化したのは結果的に好都合。
- **書き込み系 RPC が偶然叩かれなかった** — `CreateReport` / `StartReportRun` など副作用のある RPC がラテント期間中に呼ばれなかった。呼ばれていたらユーザーは「保存したのに取れない」混乱を感じた可能性。
- **`--force-recreate` ですぐ直るパターンだった** — 親 netns はまだ同じ 172.18.0.35 に割り当てられたまま IP 遷移していなかったため、sidecar の cascading recreate だけで完結した。IP が別に移動していたら `docker network inspect` からやり直す必要があった。

## アクションアイテム

| # | カテゴリ | アクション | 担当 | 期限 | ステータス |
|---|----------|-----------|------|------|-----------|
| 1 | 予防 | pki-agent の server goroutine 終了を fatal 化 (`chan error` → `os.Exit(4)`) | pki チーム | 2026-04-17 | **Done** ([[ADR-000757]] Decision 1, commit `1c7679801`) |
| 2 | 検知 | `pki-agent healthcheck` に TCP dial ベースの listener プローブを追加 (`PROXY_LISTEN` gated) | pki チーム | 2026-04-17 | **Done** ([[ADR-000757]] Decision 2, `cmd/pki-agent/healthcheck.go`) |
| 3 | 予防 | `scripts/_deploy_lib.sh:deploy_single_service` が親 recreate 時に `pki-agent-<svc>` を cascading で `--force-recreate` + `wait_until_healthy` | platform チーム | 2026-04-17 | **Done** ([[ADR-000757]] Decision 3) |
| 4 | プロセス | runbook (`docs/runbooks/pki-agent-ops.md` 新設 or 既存更新) に "sidecar が netns orphan のとき restart では戻らない、`up -d --no-deps --force-recreate <sidecar>` を使う" と明記 | pki チーム | 2026-04-24 | TODO |
| 5 | 検知 | `global_smoke` に acolyte と tag-generator の liveness probe を追加（例: `curl -fsSk https://localhost:<extern>/alt.acolyte.v1.AcolyteService/HealthCheck` または BFF 経由の HealthCheck RPC） | platform チーム | 2026-04-30 | TODO |
| 6 | 検知 | pki-agent の Prometheus metrics に `pki_agent_proxy_listener_up{subject}` を追加し、0 になったら ALERT 化 | pki チーム | 2026-05-07 | TODO |
| 7 | 予防 | 手動 `docker compose up --force-recreate <svc>` 経路でも sidecar を cascade する仕組みを検討（Compose v2.30+ の `depends_on.restart: true` の評価含む） | platform チーム | 2026-05-15 | TODO |
| 8 | プロセス | [[PM-2026-029]] のアクションアイテムに「proxy liveness を cert 鮮度以外でも測る」を遡及追加（過去 PM のアップデート） | pki チーム | 2026-04-24 | TODO |

## 教訓

### 技術面

- **Sidecar が提供する機能の liveness はその機能そのものをプローブせよ**。cert 鮮度のような「関連指標」だけを見ていると、関連が切れた瞬間に silent failure になる。[[PM-2026-029]] (nginx のメモリ保持) も今回 (pki-agent の netns orphan) も、本質的には同じ失敗パターンの別形態だった。
- **`network_mode: service:X` は便利だが、X の recreate と sidecar の recreate を結び付ける責任は compose では自動化されない**。deploy pipeline 側で明示的に cascading するか、compose v2.30+ の `depends_on.restart: true` を導入するか、あるいはそもそも独立した netns に移行するか、いずれかの明示的な設計判断が必要。
- **Go の `http.Server.ListenAndServe*` の戻り値を捨てると silent death が起きる**。これは errgroup パターンの教科書的事例。新しく Go の長期稼働 daemon を書くときは `golang.org/x/sync/errgroup` か同等の `chan error` + `select` を最初から仕込む。
- **Docker healthcheck は fire-and-forget ではない**。"healthy" の意味をユーザー視点と実装視点で揃える責任は開発者側にある。CLAUDE.md にも "service-specific rules" として healthcheck の定義を書く運用を広げる。

### 組織面

- **Postmortem のアクションアイテムに "follow-up のフォロー" を入れる運用**を徹底する。[[PM-2026-029]] で proxy liveness 改善を入れておけば今回は発生しなかった。postmortem review の最後に「このアクションアイテムで防ぎきれない派生シナリオは？」を必ず問う。
- **"sidecar を Go で書き直した = 安全" と信じすぎない**。書き直しは nginx 固有の問題 (memory hold) は消したが、その代わりに Go 固有の silent goroutine death と docker-compose 仕様の netns semantics という新しい問題を引き入れた。移行は常にトレードオフで、旧問題の完全廃絶ではない。
- **Deploy script の変更後は必ず "最初に deploy されるユーザー" になる覚悟を持つ**。今回は deploy.sh の `--skip-verify` 周辺を立て続けに 5 回修正した直後で、sidecar cascade 欠落が露見。変更自体の検証と並行して「この変更で別経路が壊れていないか」を意識的に確認する枠を作る。

## 参考資料

- [[ADR-000757]] — 3 層防御 (fail-fast / listener probe / cascading recreate) を規定する決定記録
- [[ADR-000747]] — mTLS cert ライフサイクルを shell から pki-agent に移行した経緯
- [[ADR-000752]] — Pact ゲート付き manual deploy
- [[ADR-000754]] — 今回の障害が露見した直接のトリガとなった Desktop FeedDetail rail
- [[PM-2026-029]] — nginx TLS sidecar 版 silent failure の先行事例（同パターンの別形態）
- [[PM-2026-028]] — mTLS 全体停止の先行事例（pki-agent 導入の背景）
- [[pki-agent-security-audit-2026-04-16]] — 既知の F-007 Medium (memory-hold cert) からの派生視点
- Web best-practices 調査:
  - [golang.org/x/sync/errgroup](https://pkg.go.dev/golang.org/x/sync/errgroup) — goroutine エラー伝播の公式推奨
  - [GKE readiness vs liveness probes](https://cloud.google.com/blog/products/containers-kubernetes/kubernetes-best-practices-setting-up-health-checks-with-readiness-and-liveness-probes) — 機能的ポートをプローブすべき根拠
  - [Docker compose network_mode spec](https://docs.docker.com/reference/compose-file/services/#network_mode) — 共用 netns の recreate semantics
- Commits:
  - `593888896` — `acolyte-orchestrator` 再作成を誘発した FE deploy
  - `1c7679801` — 3 層防御の適用 commit

---

> **Blameless Postmortem の原則:** このドキュメントは個人の過失を追及するためではなく、
> システムの脆弱性とプロセスの改善機会を特定するために作成されています。
> 「誰が悪いか」ではなく「システムのどこが改善できるか」に焦点を当ててください。
