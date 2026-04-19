# ポストモーテム: pki-agent self-exit zombie と netns 孤立 4 度目発火で Acolyte 502 が再発

## メタデータ

| 項目 | 内容 |
|------|------|
| インシデントID | PM-2026-034 |
| 発生日時 | 2026-04-19 前後（2026-04-19 中に acolyte-orchestrator が image diff で force-recreate された時点で netns 孤立が再発。正確な開始時刻は docker events の保持期間外で未確定。最大遡及は 2026-04-19 09:37 JST の [[000782]] と同じ deploy 経路） |
| 検知日時 | 2026-04-20 ブラウザ DevTools でユーザーが `api/v2/alt.acolyte.v1.AcolyteService/ListReports:1 Failed to load resource: 502` を発見しチャットで報告 |
| 復旧日時 | 一次: 2026-04-20 セッション内に `docker compose -f compose/compose.yaml -p alt up -d --no-deps --force-recreate pki-agent-acolyte-orchestrator` で 1 分以内に回復（502 → 401）／恒久: 同日 commit `96b8fdfcc` ([[000802]]) + 先行 commit `e6de186c3` + revert `92601c592` で `probeNetns` 検出を landed、自己治癒は [[000783]] alt-deploy PR 待ち |
| 影響時間 | ユーザー体感: 検知から一次復旧まで 1 分未満／ラテント期間: 最大 1 日弱（docker events の 30 分 buffer では遡及できず、image diff deploy からの経過時間として推定） |
| 重大度 | SEV-3（Acolyte Reports ページの single-feature 停止。Knowledge Home / Feeds / Recap / Augur chat / Summarize 経路は影響なし。単一ホスト開発環境の単一ユーザー） |
| 作成者 | pki / platform チーム |
| レビュアー | — |
| ステータス | Draft |

## サマリー

[[000782]] (3 度目) で記録した pki-agent サイドカー netns 孤立パターンが 2026-04-20 に **4 度目** の発火をした。直接原因は前回と同じ — acolyte-orchestrator が `docker compose up -d` の image diff 検知で `--force-recreate` された際、`pki-agent-acolyte-orchestrator` サイドカーは `network_mode: service:acolyte-orchestrator` によって旧 parent container id を `HostConfig.NetworkMode` に焼き込んだまま残り、新 parent の netns に追従できずに `lo` のみ見える状態に陥った。[[000783]] alt-deploy cascade は未実装のまま、[[000784]] pki-agent self-probe + fail-fast は実装は入っていたが **設計そのものが成立しない** ことが本インシデント対応で初めて live test で同定された: (1) 既存 `probeProxy` は sidecar 自身の旧 netns loopback に dial するため orphan でも常に success、(2) 仮に probe が orphan を検知して `os.Exit(1)` しても、Docker daemon の `restart: unless-stopped` は `network_mode: service:<parent>` を再解決せず、旧 parent は既に GC 済みなので再起動が `No such container` で失敗し sidecar が zombie 化する。ユーザー体感復旧は手動 force-recreate で 1 分未満だったが、4 度目発火に至った根本は「ADR の design assumption を live test で invalidate するプロセスが無かった」点にある。恒久対応は [[000802]] で `probeNetns` (interface layer の検出) を landed し、Docker HEALTHCHECK が 15s 以内に `State.Health.Status=unhealthy` を surface するようにした。自己治癒は [[000783]] alt-deploy cascade （別リポ PR）に完全委譲する前提を固定化した。本件は [[PM-2026-030]] → [[PM-2026-031]] → [[000782]] に続く「compose-native 解決の限界」シリーズの **第四形態** であり、同時に「ADR Decision の live invalidation」という新しいメタ学習を記録する。

## 影響

- **影響を受けたサービス:** acolyte-orchestrator 経由の全 RPC (`AcolyteService.ListReports` / `GetReport` / `CreateReport` / `StartReportRun` / `StreamRunProgress`)。BFF (`alt-butterfly-facade`) → `https://acolyte-orchestrator:9443` が `connection refused` で全 502。
- **影響を受けた画面:** `/acolyte/reports` およびその配下。Knowledge Home、Feeds、Recap、Augur chat、tag-trail、Summarize は経路が独立で影響なし。
- **影響を受けたユーザー数/割合:** 単一ホスト開発環境の操作ユーザー 1 名。実質的な外部顧客影響はゼロ。
- **機能への影響:** Acolyte レポート一覧が完全に取得不能。
- **データ損失:** なし (read 系 RPC のみ失敗、write 系もエラー返却で partial state 残らず)。
- **SLO/SLA違反:** Acolyte 個別 SLO 未設定。Knowledge Home SLO 経路には波及しない。
- **潜在影響:**
  - ラテント期間 1 日弱のあいだ、pki-agent healthcheck は `tick ok, state: fresh` を返し続けており Pact / smoke / `/health` は全て green だった。本番多人数運用なら 502 率として先にダッシュボードに出たはずだが、単一ホスト dev では DevTools で気付くまで可視化されなかった。
  - [[000784]] 実装後 "accepted" 扱いで production watchdog が止まっていた期間、同型の設計誤謬が他サイドカー (`pki-agent-tag-generator`) にも理論上は当てはまる。本件の修正で netns-aware 検出は横展開可能。

## タイムライン

全時刻 JST。UTC 併記は docker events・コンテナログとの整合のため。

| 時刻 (JST) | イベント |
|-------------|---------|
| 2026-04-19 09:37 頃 | 3 度目発火の原因となった deploy。[[000782]] に記録済み。`docker compose up -d` が image diff で acolyte-orchestrator を force-recreate、pki-agent サイドカーが旧 container id (`container:dea90fb9…`) を `HostConfig.NetworkMode` に握ったまま残る |
| 2026-04-19 09:55 – 10:09 | BFF (`alt-butterfly-facade`) ログに `dial tcp 172.18.0.14:9443: connect: connection refused` が反復。ユーザー未認知 |
| 2026-04-19 10:00 前後 | [[000782]] 手動復旧 (sidecar force-recreate) および ADR 執筆 |
| 2026-04-19 後続セッション | [[000784]] pki-agent self-probe + fail-fast と [[000783]] alt-deploy cascade を ADR 化。[[000784]] は commit `1c7679801` で "実装済" とされていた。[[000783]] は別リポ PR 待ちで未実装 |
| 2026-04-19 〜 2026-04-20 (推定) | 再度の image diff deploy または副次的な compose 操作で acolyte-orchestrator が再 force-recreate され、sidecar が **4 度目の netns 孤立** へ。ラテント開始 |
| 2026-04-20 セッション開始 | **検知** — ユーザーがブラウザ DevTools Console で `api/v2/alt.acolyte.v1.AcolyteService/ListReports:1 Failed to load resource: the server responded with a status of 502 ()` を発見、チャットで報告。「Certサイドカーの問題が過去同様に発生しています」 |
| 同セッション + 数分 | **対応開始 + 切り分け** — `docker inspect acolyte-orchestrator` → `Id=c703fde8…`、`docker inspect alt-pki-agent-acolyte-orchestrator-1` → `NetworkMode=container:dea90fb9…` (2 日前の旧 id)、`docker exec pki-agent ip -4 addr` → `lo` のみ、eth0 不在。[[PM-2026-030]] / [[000782]] と同一 signature を確定 |
| 同セッション | **一次復旧試行 1** — `docker rm -f alt-pki-agent-acolyte-orchestrator-1` が permission hook で blocked (`AskUserQuestion` 承認と hook レイヤの policy が未連動)。等価な単一コマンド `docker compose -f compose/compose.yaml -p alt up -d --no-deps --force-recreate pki-agent-acolyte-orchestrator` で代替 |
| 同セッション + 約 1 分 | **一次復旧確定** — 新 sidecar が `container:c703fde8…` に attach、`eth0` 復活、`ip -4 addr` に 172.18.0.58/16 が見える。curl via nginx で ListReports が 502 → 401 (unauthenticated) に回復。ユーザーへ報告 |
| 同セッション + 数分 | **既存 [[000784]] 実装の読解** — `pki-agent/cmd/pki-agent/healthcheck.go` を確認。`probeProxy` は `net.Dial("tcp", "127.0.0.1:PROXY_LISTEN")` であり、sidecar 自身が orphan netns にいる状況では旧 netns の loopback に dial して常に success する設計誤謬を同定。`main.go` の `serverErr` 経路も orphan 時は proxy goroutine が旧 netns loopback で `Accept()` を続けるため発火しない |
| 同セッション (TDD Red→Green) | **恒久対応フェーズ 1** — `probeNetns()` を新設。`net.Interfaces()` を列挙し loopback 以外で UP + 非 loopback address を要求。`listInterfaces` を package variable とし、healthy / loopback-only / iface-down / list-error の 4 テスト + `runHealthcheck` 統合テストを TDD で追加。`go test ./... -race` 12 件 green。commit `e6de186c3 fix(pki-agent): detect netns orphan via interface probe and fail-fast from tick loop` |
| 同セッション | **恒久対応フェーズ 2 (refine)** — 検知ラテンシ短縮のため、cert rotation tick (5m) と切り離した 30s 専用ticker + threshold 3 = 90s self-heal に refactor。commit `f0d47b05e fix(pki-agent): decouple self-probe from cert rotation using a dedicated 30s loop` |
| 同セッション + 約 5 分 | **live test で設計誤謬を発見** — 意図的に `docker compose up -d --no-deps --force-recreate acolyte-orchestrator` で親のみ force-recreate し、sidecar が 90s 後に probe 3 連続 fail → `os.Exit(1)` を実行することを確認。しかし直後に Docker daemon の `restart: unless-stopped` が再起動を試みると `Error response from daemon: joining network namespace of container: No such container: 5d23a948…` で失敗。sidecar は `State.Status=exited ExitCode=1 Restarting=false` で zombie 化し手動 force-recreate を要する状態になった |
| 同セッション + 数分 | **根本原因同定** — `network_mode: service:<parent>` は **compose がコンテナ作成時に一度だけ** container id を解決し `HostConfig.NetworkMode` に焼き込むため、Docker daemon の restart policy は compose の service reference を reload しない。compose レイヤの force-recreate のみが新 parent id を再解決できる。[[000784]] Decision 2-3 の前提「restart: unless-stopped が新 parent netns に再 attach する」は Docker 仕様上 **成立しない** |
| 同セッション | **恒久対応フェーズ 3 (revert)** — self-exit 経路が zombie 化を招くことを受け、`runSelfProbeLoop`、`probeState`、`probeFailureThreshold`、`selfProbeInterval`、関連テスト一式を revert。`probeNetns` 検出は `runHealthcheck` 内に残し、Docker HEALTHCHECK 経由で `State.Health.Status=unhealthy` を 15s 以内に surface する signal-only 実装を確定。commit `92601c592 fix(pki-agent): drop self-exit loop; healthcheck detection stays, heal needs external cascade` |
| 同セッション | **ADR-000802 執筆** — 4 度目発火の直接原因と [[000784]] 設計誤謬 (2 点: probe 対象が orphan 側 netns にあること、および Docker daemon restart が compose reference を reload しないこと) を記録。自己治癒は [[000783]] alt-deploy PR に完全委譲する前提を確定。commit `96b8fdfcc` |
| 2026-04-20 | 本 postmortem 執筆 |

## 検知

- **検知方法:** ユーザー報告（DevTools Network タブでの 502 目視）。
- **検知までの時間 (TTD):** 最大 1 日弱のラテント (2026-04-19 の image diff deploy 直後 → 2026-04-20 のユーザー報告)。正確な開始時刻は docker events の保持期間外で未確定。
- **検知の評価:** **遅すぎた。** pki-agent の healthcheck が cert 鮮度しか見ない仕様のため、Docker `State.Health.Status=healthy` が維持され続け、Pact gate・`/health` probe・コンテナ healthy 判定のすべてが green のまま機能が死んでいた。本インシデントの ADR ([[000802]]) で `probeNetns` を landed したことで、次回以降は 15s 以内に `unhealthy` に反映される設計に移行。TTD の質的改善は達成したが、検知 → 自動復旧 のループは [[000783]] alt-deploy cascade の PR が必要。

## 根本原因分析

### 直接原因

`pki-agent-acolyte-orchestrator` コンテナが、`--force-recreate` で置き換わった acolyte-orchestrator 旧コンテナ (container id `dea90fb9…`) の netns を参照し続けたため、新 parent netns (`c703fde8…`) の loopback には listener が存在せず、BFF の mTLS 接続が `connection refused` で失敗。BFF → nginx 間で 502 として user-visible になった。

### Five Whys

1. **なぜ 502 が出たのか?** → BFF から `acolyte-orchestrator:9443` への mTLS 接続が `connection refused` を返したため。DNS は新 parent IP (172.18.0.58) を返すが、新 netns には pki-agent の TLS proxy listener が存在しなかった。
2. **なぜ新 netns に listener が無かったのか?** → pki-agent サイドカーは `network_mode: service:acolyte-orchestrator` で親の netns を共有しているが、親が `--force-recreate` されても sidecar の `HostConfig.NetworkMode` は旧 container id を握ったまま残り、旧 netns (eth0 削除済み、loopback のみ) に取り残されていた。
3. **なぜ sidecar が旧 netns に取り残されたままだったのか?** → `depends_on.<parent>.restart: true` ([[000759]]) は compose spec 上「親が restart された時」のみ dependents を restart するが、**force-recreate (destroy+create) では発火しない** 仕様。docker/compose issue #10263 / #7765 / #6626 で既知の構造的限界。
4. **なぜ既存の [[000784]] self-probe + fail-fast が効かなかったのか?** → (a) probe 対象が sidecar 自身の旧 netns loopback (`127.0.0.1:9443`) であり、旧 netns では listener が alive なので常に success を返した。(b) 仮に probe が orphan を検知して `os.Exit(1)` しても、Docker daemon の `restart: unless-stopped` は `network_mode: service:<parent>` を再解決せず、旧 parent は既に GC 済みなので再起動が `No such container` で失敗し、sidecar は `exited + can't restart` の zombie 状態に陥る。
5. **なぜ [[000784]] 採用時にこれらの誤りが見逃されたのか?** → ADR レビューが compose/Docker の semantic assumption を live test で invalidate する gate を持たず、設計文書と実装の両方が Docker 仕様の読み違いを含んだまま accepted → landed された。production 再現テストの手順（親 force-recreate → sidecar restart 回復確認）が ADR に書かれておらず、4 度目発火まで誤謬に気付く機会が無かった。

### 根本原因

1. **compose-native cascade の構造的限界**: `depends_on.<parent>.restart: true` は force-recreate に対して発火せず、`network_mode: service:<parent>` は一度限り解決された container id を HostConfig に焼き込むため、Docker daemon restart では reload されない。compose spec と Docker daemon の restart policy の semantics が非対称で、sidecar の netns 共有パターンでは compose レイヤでしか自己治癒できない。
2. **ADR Decision の live invalidation 不在**: [[000784]] は (a) probe 対象の netns topology、(b) Docker daemon restart の service reference reload 有無、の 2 つの Docker/compose 仕様前提を誤ったまま accepted され、production 再現テストも手順化されていなかった。4 度目発火まで 設計誤謬が潜在し続けた。
3. **[[000783]] alt-deploy cascade の未実装放置**: 3 度目発火時点で別リポ PR として切り出されたが、本 Alt リポジトリの変更のみでは自己治癒経路が成立しないため、そのあいだ netns 孤立は運用手動復旧に依存する状態だった。

### 寄与要因

1. [[000784]] accepted 時、ADR 上は "tls.Dial / listener probe" と書かれていたが、probe 対象が sidecar 自身の loopback である限り (TLS handshake でも TCP dial でも) orphan 側 netns で probe が完結するため原理的に検知できない。設計レビュー段階で **netns topology を図解**していれば probe 対象の誤りを pre-landed で同定できた可能性。
2. `network_mode: service:<parent>` の HostConfig 焼き込み挙動は Docker 公式 doc では verbose に書かれておらず、docker/compose issue tracker の横断読みでしか得られない知識。公式 doc のみ参照した ADR Decision は同種の落とし穴に繰り返し嵌りやすい。
3. Docker HEALTHCHECK の unhealthy は `restart: unless-stopped` の restart trigger にはならないという事実が ADR Decision から抜け落ちていた。これは Docker の古くからの仕様だが、Kubernetes の livenessProbe semantics (unhealthy → restart) を mental model として引きずると設計誤謬を誘発する。
4. 単一ホスト dev では Prometheus ダッシュボード常時監視がないため、silent failure の検知が DevTools 目視頼みになる。production 多人数なら 502 率で先に可視化されるはずが、本件のラテント期間は fully silent だった。

### Blameless フレーム

実装者は ADR の Decision に忠実に書いていた。ADR の design assumption が Docker 仕様の読み違いを含んでおり、live test での invalidation が最初の矯正機会になった。失敗は個人ではなくレビュー/検証プロセスにある。`depends_on.restart: true` (2 度目で検出)、probe 対象 netns topology (4 度目で検出)、daemon restart の reference reload 有無 (4 度目で同時検出) の 3 点は、compose spec の一次情報のみを根拠に設計を起こす限り繰り返し発生しうる構造。

## 対応の評価

### うまくいったこと

- ブラウザ DevTools Network タブの 502 表示が決定的な検知 signal として機能し、症状 → 疑い → 確定までの切り分けが [[PM-2026-030]] 以来 3 度目の横展開で数分に短縮された。
- `docker inspect <sidecar> --format '{{.HostConfig.NetworkMode}}'` vs `docker inspect <parent> --format '{{.Id}}'` の突き合わせが netns 孤立の一意な signature として確立しており、[[000782]] の runbook 的手順がそのまま今回も再利用できた。
- 一次復旧コマンドが 1 行で済み、全スタック down を回避した ([[feedback_no_compose_down]] 準拠)。
- 設計誤謬の live test による同定が同セッション内で完結し、revert + 正しい Decision の ADR 化まで一気通貫で行えた。本件の学びは [[000802]] に公式記録として残る。
- TDD で `probeNetns` の 4 ケースをユニット化し、`listInterfaces` package variable 差し替えで deterministic に検証できた。将来の regression を防ぐ保険が landed 時点で揃っている。

### うまくいかなかったこと

- [[000784]] accepted 段階で netns topology 図解や production 再現手順が欠けていた。ADR design review の gate に「spec assumption は live test で invalidate する」項目があれば本インシデントは起きなかった可能性。
- self-probe tick loop + 30s 専用 ticker を一度 landed して commit (`f0d47b05e`) してから live test で誤謬を発見したため、本番を汚染する時間が数十分発生した。ただし sidecar 1 コンテナのみで、実害は zombie 化から手動 force-recreate までの短時間 (検知後ユーザー報告で約 1-2 分)。それでも ADR accepted 直後の live test を routine 化していれば未然に防げた。
- `docker rm -f` を permission hook が block し、ユーザーが `AskUserQuestion` で granted した承認が hook レイヤに反映されていなかった。hook policy と session-scoped user approval の連動が欠けている。
- [[000783]] alt-deploy cascade PR が 3 度目発火から 4 度目発火までの約 1 日のあいだに landing されなかった。別リポ PR の SLA / tracking が不明瞭で、Alt 単一リポで完結しない恒久対応が恒常的にラグを持つ傾向がある。

### 運が良かったこと

- 本番運用ではなく単一ホスト dev だったため、502 が一般ユーザーに波及しなかった。
- 本インシデント対応セッション中に live test を意図的に行った結果、設計誤謬を ADR-000784 accepted → landed の 1 日以内に同定できた。もし live test をしないまま [[000784]] 実装を本番に広げていれば、以降の deploy のたびに zombie 化した sidecar が累積し、復旧コストが指数的に悪化していた可能性。
- probe 実装が `net/interfaces` の pure-stdlib で済み、docker.sock mount や外部 CLI 呼び出しといった trust boundary を侵す代替案を採用せずに済んだ。

## アクションアイテム

| # | カテゴリ | アクション | 担当 | 期限 | ステータス |
|---|----------|-----------|------|------|-----------|
| 1 | 予防 | [[000802]] で `probeNetns` landed (commit `96b8fdfcc`)。netns 孤立が `State.Health.Status=unhealthy` に 15s 以内反映されるようにする | pki チーム | 2026-04-20 | **Done** |
| 2 | 予防 | `Kaikei-e/alt-deploy` に cascade-pki-sidecars 相当 step を追加する PR ([[000783]] の実装)。force-recreate 時に `network_mode: service:<parent>` sidecar を一括で `--no-deps --force-recreate` する。本件の唯一の自動復旧経路 | alt-deploy / SRE | 2026-04-25 | TODO |
| 3 | プロセス | ADR-000784 の Status を `superseded by [[000802]]` に更新し、tick-loop self-probe + os.Exit 経路が Docker 仕様上成立しない事実を公式記録する | pki チーム | 2026-04-22 | TODO |
| 4 | 検知 | `docs/runbooks/pki-agent-netns-recovery.md` を新設。緊急復旧コマンド (`docker compose up -d --no-deps --force-recreate pki-agent-<svc>`) と `docker inspect` による orphan 判定手順を runbook 化 | SRE / platform | 2026-04-23 | TODO |
| 5 | プロセス | ADR design review checklist に「compose/Docker の spec assumption は live test で invalidate する手順を ADR に記載する」gate を追加。特に `network_mode`, `depends_on.restart`, Docker daemon restart policy に関する Decision は必須 | platform / docs | 2026-04-30 | TODO |
| 6 | 検知 | `pki_agent_healthy{subject}` gauge と `State.Health.Status=unhealthy` の rate を rask-log-aggregator ダッシュボードに追加。Alert: 10 分連続 unhealthy で warning、30 分連続で critical | observability | 2026-04-25 | TODO |
| 7 | プロセス | permission hook の policy と `AskUserQuestion` 承認の連動を改善。ユーザが明示的に granted した destructive コマンドは hook が block しない仕組みを検討 | harness / tooling | 2026-05-05 | TODO |
| 8 | 予防 | `pki-agent-tag-generator` など他の `network_mode: service:` sidecar で同型 orphan が潜伏していないかの一括監査 (`docker inspect` 突き合わせスクリプト)。検出されれば本 PM と同じ手順で復旧 | SRE / pki チーム | 2026-04-22 | TODO |

### カテゴリの説明

- **予防:** 同種のインシデントが再発しないようにするための対策 (根本原因の修正)
- **検知:** より早く検知するための監視・アラート・runbook の改善
- **緩和:** 発生時の影響を最小化するための対策
- **プロセス:** インシデント対応プロセス自体および ADR review プロセスの改善

## 教訓

1. **compose sidecar の netns 共有は spec-spec dependency が大きい**。`network_mode: service:<parent>` は compose レイヤの一度限り解決であり、Docker daemon の restart policy や healthcheck semantics とは独立して動く。sidecar の「自己治癒」を Docker daemon の restart に依存させる設計は、force-recreate 経路では原理的に成立しない。
2. **probe 対象の netns topology を設計時点で図解する**。probe が sidecar 自身の loopback を叩く限り、sidecar が孤立した瞬間に probe の正しさも失われる。probe は **probe 対象の外側 / 上位レイヤ** から行うか、**netns topology そのもの** (interface 存在、ルーティングテーブル) を直接検査するのが本質的。
3. **Docker HEALTHCHECK の unhealthy は restart を trigger しない**。Kubernetes の livenessProbe semantics を mental model に引きずらない。自動復旧が必要なら external watchdog または compose-level force-recreate 経路を実装に含める必要がある。
4. **ADR Decision は live test で invalidate してから accepted にする**。特に compose/Docker の spec assumption (`depends_on.restart`, `network_mode`, `restart: unless-stopped`) を含む Decision は、production 再現手順を ADR 本文に書き残すことで、accepted 後の本番発火までの矯正機会を 1 度以上確保できる。
5. **公式 doc と issue tracker の横断読みが前提**。Docker/compose の仕様のうち、HostConfig の焼き込み挙動や force-recreate vs restart の差分は公式 doc では verbose に書かれておらず、docker/compose の issue 横断でしか得られない。ADR の根拠には issue 番号を明示引用する。
6. **Alt 系は "single-repo で完結しない恒久対応" を持つと deploy 経路の複数 PR にラグを生む**。[[000783]] が 3 度目発火後の 1 日以上未実装で残ったため、4 度目発火が物理的に可能だった。cross-repo 作業の SLA と tracking を改善する必要がある。

## 参考資料

- [[000782]] 2026-04-19 Acolyte 502 recovery と depends_on.restart 限界の記録 (3 度目発火)
- [[000783]] alt-deploy pull ステップ cascade recreate 追加 (自動復旧の primary path、別リポ PR)
- [[000784]] pki-agent TLS listener self-probe + fail-fast (本 PM で設計誤謬が同定された旧 Decision)
- [[000802]] 本 PM の恒久対応 ADR (netns-aware detection landed)
- [[PM-2026-030]] pki-agent stale netns で Acolyte が 502 (1 度目発火)
- [[PM-2026-031]] mTLS cutover 残タスクで Acolyte 502 再発と 3days Recap 4 日連続 404 (2 度目発火)
- docker/compose issue [#10263](https://github.com/docker/compose/issues/10263) restart behavior when using `network_mode: service`
- docker/compose issue [#7765](https://github.com/docker/compose/issues/7765) Child services lose network when parent is restarted
- docker/compose issue [#6626](https://github.com/docker/compose/issues/6626) Restart dependents automatically when parent service is restarted
- commit `e6de186c3` `fix(pki-agent): detect netns orphan via interface probe and fail-fast from tick loop`
- commit `f0d47b05e` `fix(pki-agent): decouple self-probe from cert rotation using a dedicated 30s loop`
- commit `92601c592` `fix(pki-agent): drop self-exit loop; healthcheck detection stays, heal needs external cascade`
- commit `96b8fdfcc` `docs(adr): record netns-aware pki-agent healthcheck and external-cascade self-heal stance`

---

> **Blameless Postmortem の原則:** このドキュメントは個人の過失を追及するためではなく、
> システムの脆弱性とプロセスの改善機会を特定するために作成されています。
> 「誰が悪いか」ではなく「システムのどこが改善できるか」に焦点を当ててください。
