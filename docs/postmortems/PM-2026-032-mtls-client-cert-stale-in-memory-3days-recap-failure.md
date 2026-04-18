# ポストモーテム: recap-worker の mTLS クライアント証明書が in-memory に固定され 3days Recap が CertificateExpired で停止

## メタデータ

| 項目 | 内容 |
|------|------|
| インシデントID | PM-2026-032 |
| 発生日時 | 2026-04-18 16:00 JST 前後（`pki-agent-recap-worker` が `/certs/svc-cert.pem` をローテ、recap-worker は in-memory の旧 cert を提示し続けた。ラテント発火） |
| 検知日時 | 2026-04-18 19:25:56 JST（= 10:25:56 UTC。ユーザが 3days Recap の手動トリガで失敗 log `alt-backend articles request failed` を観測し、チャットで報告） |
| 復旧日時 | 一次: 2026-04-18 19:54 JST（`docker compose -p alt restart recap-worker` で約 1 分）／恒久: [[ADR-000773]] の 5 コミット + 本 PM 完了後に `./scripts/deploy.sh production` を実行（執筆時点で deploy 待ち） |
| 影響時間 | ラテント 3h25m（16:00 → 19:25 JST）、ユーザ体感 29 分（19:25 → 19:54 JST） |
| 重大度 | SEV-3（単一ホスト開発環境の単一ユーザ、3days Recap 機能のみ停止、7days Recap / Feeds / Augur / Knowledge Home 本体は経路が異なるため影響なし） |
| 作成者 | recap / platform / pki チーム |
| レビュアー | — |
| ステータス | Draft |

## サマリー

2026-04-18 16:00 JST（= 07:00 UTC）、`pki-agent-recap-worker` がスケジュールどおり `/certs/svc-cert.pem` と `/certs/svc-key.pem` を原子的に差し替えたが、recap-worker (Rust) の `reqwest::Client` は起動時（Apr 17 ~21:25 JST）に `Identity::from_pem(...)` で cert バイト列を **in-memory に baked** しており、ディスク上の新 cert に追随しなかった。recap-worker はローテから 3 時間経過した 19:25 JST 時点でも**既に失効した旧 cert**（`NotAfter: 2026-04-18 16:00 JST`）をクライアント証明書として提示しており、alt-backend (Go) の `WebPkiClientVerifier` が期限切れと判定して TLS alert 45 (`certificate_expired`) を送り返していた。`recap-worker` 側ログには `received fatal alert: CertificateExpired` が連続し、3days Recap ジョブは fetch ステージで全 abort した。一次復旧はコンテナを単独再起動するだけで済んだが、同型バグ（クライアント側のみ cert hot-reload が未実装）を監査したところ計 6 サービス（Rust 1 / Go 1 / Python 4）に silent に分散していたことが判明。本 PM と [[ADR-000773]] で全 6 サービスを `alt-backend/app/tlsutil/tlsutil.go` の `certReloader` セマンティクスに揃えた。本件は [[PM-2026-031]] に続く mTLS cutover 系残タスクの第二形態（「listener のルート登録漏れ」から「クライアント側 cert の hot-reload 欠落」へ形が変わった silent failure）として記録する。

## 影響

- **影響を受けたサービス:** recap-worker のアウトバウンド TLS (`https://alt-backend:9443`)。3days Recap パイプラインの fetch ステージ全体が abort。preprocess / dedup / genre / dispatch / subworker / news-creator / persist の下流ステージも自動連鎖で abort
- **影響を受けた画面:** Knowledge Home の 3days Recap セクション
- **影響を受けたユーザ数:** 単一ホスト開発環境の 1 名
- **機能への影響:**
  - 3days Recap job が `recap_job_status_history` に `reason=alt-backend articles request failed: ... received fatal alert: CertificateExpired` を記録して失敗
  - 7days Recap、Feeds、Augur、Knowledge Home の 3days 以外は経路が独立しているため**影響なし**
  - alt-backend 側は正しく最新 cert を提示（サーバ側の `GetCertificate` callback 経路は alt-backend/app/tlsutil で実装済み）
- **データ損失:** なし。recap job は abort のみで partial artifact は残らず、次回成功で上書きされる
- **SLO/SLA 違反:** 個別 SLO 未設定。Knowledge Home 全体 SLO への波及は 7days Recap 経路が健在だったため軽微
- **潜在影響（もし長期化していたら）:**
  - 毎日 07:00 UTC のローテ後に 3days Recap が連続失敗する。[[PM-2026-031]] と同じ「4 日連続 404 になって初めて気付く」パターンを繰り返していた可能性
  - 同じバグが休眠していた 5 サービス（rag-orchestrator / acolyte-orchestrator / recap-evaluator / recap-subworker / tag-generator）で `MTLS_ENFORCE` が有効化されていた場合、同日同時に `CertificateExpired` を発火していた可能性が高い

## タイムライン

全時刻は JST。UTC 併記は recap-worker ログ・`pki-agent-recap-worker` ローテ時刻との整合のため。

| 時刻 (JST) | UTC | イベント |
|---|---|---|
| 2026-04-17 ~21:25 | 2026-04-17 ~12:25 | `alt-recap-worker-1` コンテナ起動。`reqwest::Client::builder().identity(Identity::from_pem(...))` が当時の leaf cert（Apr 17 07:00 UTC 発行／Apr 18 07:00 UTC 失効）を in-memory にロード |
| 2026-04-18 16:00 | 2026-04-18 07:00 | **ラテント発火。** `pki-agent-recap-worker` がスケジュールどおり `/certs/svc-cert.pem` / `svc-key.pem` を新しい 24h cert（`NotBefore: Apr 18 06:59:29 UTC` / `NotAfter: Apr 19 07:00:29 UTC`）へ原子的に差し替え。`/certs/rotated.marker` も更新。recap-worker の `reqwest::Client` は依然として旧 cert を握り続ける（reload 機構なし） |
| 2026-04-18 16:00 以降 | 07:00 以降 | 旧 cert が失効。alt-backend 側の `WebPkiClientVerifier` は以降の handshake で recap-worker の古いクライアント cert を期限切れと判定し、TLS alert 45 を送信 |
| 2026-04-18 19:25:56 | 10:25:56 | recap-worker 側 `fetch call failed ... received fatal alert: CertificateExpired` ログが連続。`job_id=3c1003d5-cd69-440b-b100-f921723a3d1c` の 3days recap job (label=3days) が失敗 |
| 2026-04-18 19:25:56 | 10:25:56 | **検知。** ユーザが UI 上で `alt-backend articles request failed ... client error (S` を観測し、チャットで報告 |
| 2026-04-18 ~19:30 | 10:30 | 調査開始。`docker logs alt-recap-worker-1` で `certificate_expired` alert を特定。`docker exec alt-pki-agent-alt-backend-1 ls -la /certs/` と `docker cp alt-pki-agent-alt-backend-1:/certs/svc-cert.pem | openssl x509 -noout -dates` でサーバ cert が有効であることを確認（→ alt-backend 側は健全と切り分け） |
| 2026-04-18 ~19:40 | 10:40 | クライアント側 cert も有効であることを確認し、コンテナ稼働時間との矛盾から **「reqwest が起動時ロードの古い cert を in-memory に抱えたまま」** と仮説を立てる。`recap-worker/src/clients/mtls.rs:42-74` で `Identity::from_pem` 一発構築を確認、仮説確定 |
| 2026-04-18 ~19:50 | 10:50 | 類似バグの監査を並列開始（Go / Rust / Python 全サービス）。結果、追加 5 サービスに silent に分散していることを確認 |
| 2026-04-18 19:54 | 10:54 | **一次復旧 A.** `docker compose -f compose/compose.yaml -p alt restart recap-worker` 実行。新 `reqwest::Client` は起動時にディスク上の最新 cert をロード → `mTLS listener enabled` と `listening` ログ確認。以降の 3days Recap job は成功 |
| 2026-04-18 ~20:00-23:00 | 11:00-14:00 | TDD で RED → GREEN を回しつつ 6 サービスの恒久対策を実装。`ReloadingCertResolver` (Rust rustls), `tlsutil.certReloader` の移植 (Go rag-orchestrator), `SslContextReloader` + `watch_cert_rotation` (Python httpx 3 サービス), `_ReloadingBackendClient` proxy (Python pyqwest tag-generator) を追加。commits f5cf687dd / 65c334816 / 029b8eb17 / 697d3891f |
| 2026-04-18 ~23:30 | ~14:30 | [[ADR-000773]] 執筆、commit 526bb4141 |
| 2026-04-18 執筆時点 | — | 本 PM 執筆、**恒久デプロイ待ち**（`./scripts/deploy.sh production` を Pact gate + c2quay 経由で実行予定） |

## 検知

- **検知方法:** ユーザ報告（UI 上のエラーバナー → チャット）
- **TTD（Time to Detect）:** 3 時間 25 分（ラテント発火 16:00 JST → ユーザ報告 19:25 JST）
- **検知の評価:** **遅い。** 直接原因である client cert の失効は pki-agent のローテ直後から発火しているが、recap job が毎日 17:00 UTC（02:00 JST）程度の間隔でしか走らない (+ 手動トリガ)ため、ローテ直後の最初の fetch が失敗するまで気付けなかった。[[PM-2026-031]] のアクションアイテム #4（smoke に mTLS 経由の REST 疎通テストを追加）と #5（recap job 連続失敗 alert）は期限 2026-04-24 / 2026-04-30 で TODO のままであり、もしこれらが既に運用に乗っていれば、07:00 UTC 直後に smoke が gate で止めていた、または 2 連続失敗で slack 通知が飛んでいたはず。検知の穴は [[PM-2026-031]] と同じ場所に残っていた

## 根本原因分析

### 直接原因

`recap-worker/recap-worker/src/clients/mtls.rs:42-74` の `build_mtls_client(...)` が `reqwest::Client::builder().identity(Identity::from_pem(&cert_pem + &key_pem))` で起動時に cert を 1 回だけロードし、`reqwest::Client` の内部で保持される `Identity` オブジェクトがプロセスの寿命と一致していた。ディスク上の `/certs/svc-cert.pem` が pki-agent によって差し替えられても、`reqwest` 側には変更通知の仕組みがなく、TLS handshake のたびに **起動時に読んだバイト列**を提示し続けていた。

一方、alt-backend (Go) の `app/tlsutil/tlsutil.go:66-104` の `certReloader` は `GetClientCertificate` / `GetCertificate` callback で各 handshake ごとに `os.Stat` → mtime 比較 → 必要なら `tls.LoadX509KeyPair` を呼ぶ構造で、サーバ側は正しく新 cert を提示していた。

結果として「サーバ側 cert は新しい・クライアント側 cert は古い失効済み」という非対称状態が成立し、alt-backend の `WebPkiClientVerifier` が TLS alert 45 (`certificate_expired`) を返す handshake 失敗が継続した。

### Five Whys

1. **なぜ 3days Recap が `certificate_expired` で失敗するのか？**
   → recap-worker のクライアント cert が失効していたから。
2. **なぜクライアント cert が失効していたのか？**
   → `reqwest::Client` が**プロセス起動時**に `/certs/svc-cert.pem` を in-memory にロードしたまま追随しておらず、ディスクには新 cert があるのに古い失効済み cert を提示していたから。
3. **なぜ `reqwest::Client` が追随していなかったのか？**
   → `Identity::from_pem(...)` は 1 回呼び出したら終わりで、reload 機構がないから。Rust / reqwest でホットリロードするには `rustls::ClientConfig::with_client_cert_resolver(Arc<dyn ResolvesClientCert>)` を自前で差し込む必要があるが、その実装が書かれていなかった。
4. **なぜ ResolvesClientCert 実装がなかったのか？**
   → 2026-04-14 前後の mTLS cutover（commits `5d148ce25` / `a6752c19c`、[[000754]] 期）で recap-worker の outbound mTLS を有効化した際、alt-backend 側（Go）と同等の cert hot-reload が必要であるという認識が抜けていた。alt-backend では `alt-backend/app/tlsutil/tlsutil.go` に `certReloader` が既に存在しており、`auth-hub` / `pre-processor` / `search-indexer` / `alt-butterfly-facade` / `pki-agent` の Go 6 サービスは同ファイルをローカルコピーしてこの性質を保っていた。Rust の recap-worker と rag-orchestrator (Go) と Python 4 サービスには **未移植のまま** mTLS を有効化した、あるいは `MTLS_ENFORCE=true` をデフォルトにした。
5. **なぜ Rust / Go / Python の未移植が silent だったのか？**
   → cutover を検証する経路が `/health` smoke と Pact gate の 2 つだけで、どちらも **クライアント cert をローテ跨ぎで提示し続けて壊れる** 事象を検出できない。Pact は consumer-provider の contract schema 互換性しか見ず、smoke は起動時 1 分以内しか走らないので「24h 後に起きる」バグは構造的に射程外。[[PM-2026-031]] で指摘した「smoke に mTLS 経由の REST 疎通を追加」Action Item は TODO のまま（期限 2026-04-24）で、これが実装されていれば「ローテ直後の handshake 失敗」は compose staging の smoke で捕まえられていた。

### 根本原因（共通）

**mTLS cutover 系作業において、「クライアント側の cert hot-reload を実装する」カテゴリのタスクが Rust / Go (rag-orchestrator) / Python 全系に silent に未着手のまま残っていた**。alt-backend 由来の `certReloader` は Go で 6 サービスに伝播していたが、言語を跨いだ移植が必要な箇所（Rust の `ResolvesClientCert` / Python の `ssl.SSLContext.load_cert_chain` の再呼び出し / pyqwest の transport 再構築）は誰も手をつけていなかった。発火源は cutover 時の認識漏れ、**増幅器は検知側の構造的穴**（Pact gate + `/health` smoke でローテ跨ぎが見えない）だった。

### 寄与要因

- **recap-worker コンテナの長時間稼働。** 22 時間連続で稼働しており、起動時ロード以外で cert が更新される契機がなかった。compose の `restart: unless-stopped` 方針により、明示的に `docker restart` しない限り cert 期限を跨ぐ
- **recap job のスケジュール頻度の低さ。** 3days 分のジョブは 1 日 1 回程度。初回失敗から気付くまでの窓が広い（もし毎時間のジョブであれば 07:00 UTC 直後に気付けたが、24 時間跨いで 4 日連続失敗のパターン（[[PM-2026-031]]）もあり得た）
- **cutover 時の「checklist 視点」不足。** 2026-04-14 の mTLS 有効化時に「クライアント側も cert ホットリロードが必要」という一行が各サービスの CLAUDE.md / runbook に書かれていれば、該当箇所が拾えた可能性がある
- **言語ごとの TLS エコシステムの差異。** Go は `GetClientCertificate` / `GetCertificate` callback という標準仕組みが `crypto/tls` に組み込まれているため alt-backend の `certReloader` がそのまま使える。Rust の `reqwest` / Python の `ssl.SSLContext` / pyqwest の `SyncHTTPTransport` はそれぞれ異なる反映ルートがあり、alt-backend の pattern をそのままコピーできない。この差が「1 言語 1 回は自前で書く必要がある」作業を silent に生んでいた
- **[[PM-2026-031]] アクションアイテム #4 / #5 の期限が後続だった。** 期限はそれぞれ 2026-04-24 / 2026-04-30 で、本件は 2026-04-18。先に smoke 拡張が入っていれば TTD は大幅に短かった

## 対応の評価

### うまくいったこと

- **切り分けのスピード。** 検知から 15 分以内で「サーバ側は健全、クライアント側が古い cert を握っている」と切り分けられた。`docker cp` で双方の `/certs/svc-cert.pem` を抜き、`openssl x509 -noout -dates` で有効期限を確認する手順が [[PM-2026-028]] の教訓から身についており、時間配分が迷わなかった
- **一次復旧の最小侵襲性。** `docker compose -p alt restart recap-worker` 1 コマンドで復旧。他サービス無停止、in-flight ジョブもなし
- **並列監査。** 一次復旧の後、`Explore` サブエージェントを Go / Rust / Python 系それぞれに並列で当て、10 分以内に 6 サービスの同型バグ分布マップを作成できた
- **TDD 厳守。** Rust では `rcgen` + `filetime` の dev-dep で tempdir ベースの cert rotation テストを 4 ケース追加。Go / Python も openssl shell-out で同等テストを追加。全 1,669 tests (Rust 322 lib + Python 1,347 + Go 全パッケージ) pass を確認してから ADR 化
- **commit 分割。** 「意味単位で分ける」ユーザ指示に従い 4 コミット + ADR 1 コミットの計 5 コミットに分割。各コミットが独立してレビュー可能な粒度
- **副次発見の可視化。** `MTLS_ENFORCE=true` のサービス以外も含め 6 サービスが同型バグを抱えていることを判明させ、有効化前に根治できた。将来 `MTLS_ENFORCE` を他サービスで ON にする作業の暗黙の地雷が 1 つ減った

### うまくいかなかったこと

- **初期 3 時間 25 分のラテントを検知できなかった。** [[PM-2026-031]] で追加予定だった smoke の mTLS REST 疎通テストが TODO のままだったため、本件と同じ方法で見逃された
- **コンテナ稼働時間と cert 有効期限の相関を事前にモニタリングしていなかった。** `container_start_time` と `cert_not_after` の差分を prometheus metric として露出していれば、ローテ直前に alert で気付けた
- **cutover 時の言語横断 checklist がなかった。** [[000754]] 期の mTLS 有効化で「各言語でクライアント側 cert のホットリロードを実装したか？」を確認する項目が存在しなかった
- **tag-generator の `__getattr__` proxy は static type 解析が弱い。** pyrefly に `cast("BackendInternalServiceClientSync", reloading)` を 1 箇所入れて黙らせたが、本質的には pyqwest が httpx と同じく in-place reload 可能になれば不要になる。暫定策の負債として残る

### 運が良かったこと

- **単一ホスト開発環境。** 本番マルチテナント運用なら 3days Recap を購読する全ユーザが 3h25m 以上の空を経験していた。本件は 1 ユーザのみ影響
- **3days Recap 経路だけ独立していた。** 他の recap パイプライン（7days）は Connect-RPC 経由で別ハンドラが動く設計であり、巻き込まれなかった
- **同時に複数のローテを跨がなかった。** alt-backend 側と recap-worker 側のローテ時刻が近接しているが、サーバ側は GetCertificate callback で追随していたため、クライアント側だけの failure に限定された
- **ユーザが DevTools / UI バナーを見ていた。** 機能を使わない時間帯であれば、次の手動トリガまで気付けなかった可能性が高い
- **[[PM-2026-031]] の直後で注意が高かった。** 前日のインシデントで mTLS cutover 系が同じカテゴリで silent 潜在していた記憶が新しく、本件も「cutover 系残タスクの可能性」という仮説に最初から寄せて調査できた

## アクションアイテム

| # | カテゴリ | アクション | 担当 | 期限 | ステータス |
|---|---|---|---|---|---|
| 1 | 予防 | recap-worker の `reqwest::Client` を `rustls::ClientConfig::with_client_cert_resolver` + `ReloadingCertResolver` 経由に差し替え、`src/tls.rs` も同 resolver を `with_cert_resolver` で利用 | recap チーム | 2026-04-18 | **Done**（commit f5cf687dd、[[ADR-000773]] Decision 1） |
| 2 | 予防 | rag-orchestrator に `internal/infra/tlsutil` パッケージを移植し、`httpclient/mtls.go` / `pool.go` を `GetClientCertificate` callback 経路に差し替え | rag チーム | 2026-04-18 | **Done**（commit 65c334816、[[ADR-000773]] Decision 2） |
| 3 | 予防 | acolyte-orchestrator / recap-evaluator / recap-subworker に `SslContextReloader` を追加、前 2 サービスは `watch_cert_rotation` asyncio task を lifespan に、subworker は per-request `maybe_reload()` | platform | 2026-04-18 | **Done**（commit 029b8eb17、[[ADR-000773]] Decision 3） |
| 4 | 予防 | tag-generator に `_ReloadingBackendClient` proxy を追加、pyqwest `SyncHTTPTransport` を mtime 変化時に再構築 | tag チーム | 2026-04-18 | **Done**（commit 697d3891f、[[ADR-000773]] Decision 4） |
| 5 | 検知 | `scripts/smoke.sh` / c2quay `deploy.smoke.command` に **pki-agent のローテを跨ぐ E2E テスト**を追加する。実装案: docker exec で `touch -d "1 hour" /certs/svc-cert.pem` 相当のシミュレーション → 続く Hurl 経路で 200 を確認。[[PM-2026-031]] Action Item #4 と統合 | platform | 2026-04-24 | TODO |
| 6 | 検知 | Prometheus metric `mtls_client_cert_age_seconds{service}` を各サービス exporter に追加し、`age > 24*3600 - 600` で slack 通知。現状 `pki_agent_proxy_listener_reachable{subject}` は netns orphan 検知用 ([[PM-2026-031]] Action Item #10) だけなので、client 側の cert 鮮度メトリクスは新規 | observability | 2026-05-07 | TODO |
| 7 | 検知 | `recap_job_status_history` の連続失敗 alert は [[PM-2026-031]] Action Item #5 で既に TODO。期限 2026-04-30 を変更せず同 PM で追跡 | recap チーム | 2026-04-30 | TODO（継承） |
| 8 | 予防 | `docs/runbooks/mtls-cutover-checklist.md` を新規作成。新サービスで mTLS を有効化する際の**言語横断 checklist**（特に「クライアント側 cert の hot-reload を実装したか？」の確認項目）を明文化。[[ADR-000754]] / [[ADR-000759]] / [[ADR-000773]] を related として参照 | docs / platform | 2026-05-01 | TODO |
| 9 | プロセス | ADR template に「**ローテ跨ぎ E2E の動作確認**」セクションを追加。mTLS / secrets 系の ADR では Consequences の前に必ず書かせる | docs | 2026-05-15 | TODO |
| 10 | 予防 | CA bundle のホットリロードは本 ADR スコープ外。leaf cert は 24h ローテだが CA は年単位なので優先度低。Intermediate CA の次回交換（2036-04-11 期限）の半年前（2035-10）までに対処する技術的負債として記録 | platform | 2035-10-01 | Deferred |
| 11 | 予防 | Python 4 サービスで同型 `mtls_client.py` が複製されている状態を `alt-mtls-py` 共有パッケージ化する。CLAUDE.md の「5 サービスで切り出し」基準に達しているが、本 PR ではスコープ外とした | platform | 2026-06-01 | TODO（別 ADR） |
| 12 | 予防 | tag-generator の `_ReloadingBackendClient` proxy 経由を pyqwest 本体の hot-reload 対応に置き換え、ダックタイピング + `cast` を解消する。pyqwest upstream の PR 動向に依存 | tag チーム | 2026-07-01 | Deferred |

## 教訓

### 技術面

- **Go の `GetCertificate` / `GetClientCertificate` callback に相当する機構を、各言語の TLS スタックで一度は自前で書く必要がある。** Rust / Python / pyqwest は「cert はハンドシェイクごとに読み直される」という保証が標準ライブラリ外。各サービス初出の mTLS 実装では必ず最初に「ローテをどう拾うか」を決める儀式を設けるべき
- **`ssl.SSLContext.load_cert_chain()` は同一コンテキストに対して複数回呼べる。** これは Python で「cert だけ in-place で差し替え、httpx.AsyncClient はそのまま使い続ける」を成立させる鍵。`SSLContext` のアイデンティティを変えないので gateway 層の型ヒントを一切触らずに済む
- **pyqwest のように「TLS 材料を構築時に bake する」ライブラリに対しては、proxy レイヤでクライアント丸ごと再生成するしかない。** 言語ごとに最適なパターンが違い、「全サービス同じ helper 関数」にはならないことを受け入れる
- **コンテナの長時間稼働は cert 期限との相性が悪い。** `restart: unless-stopped` で 24h+ 連続稼働するサービスほど、cert ローテがプロセス境界を跨ぐ。起動時 baked-in の pattern は 24h ローテ下で必ず壊れる
- **`docker cp` で両端の cert を抜いて `openssl x509` で dates 比較する**のは mTLS incident の切り分け定型手順。`openssl s_client -connect ... -CAfile ...` で実際の handshake を再現できるのも便利

### 組織面

- **cutover 系作業の残タスクは ADR の Cons だけでなく runbook の checklist に落とす。** [[PM-2026-031]] で「ADR の Cons は設計者への警告、Action Item は実行者への指示。混同しない」と教訓化したが、今回はそれに加えて「言語横断 checklist」の存在が必要だった。runbook レベルで永続化する
- **smoke は `/health` と Pact gate だけでなく、本物のリクエスト経路を最低 1 本通す。** [[PM-2026-031]] Action Item #4 が TODO のままだったことが本件の 3h25m のラテントを生んだ。次の smoke 拡張で `MTLS_ENFORCE=true` なサービス間の実リクエストを Hurl で確認する
- **前日の PM の教訓が翌日のインシデントに直接効く。** [[PM-2026-031]] で「mTLS cutover 系残タスク」と明示した直後に、別形態の残タスクが露呈した。PM を書いた直後は特に、その根本原因に引きずられる場所を能動的に探す時間を確保するべき
- **「休眠 5 サービスに拡張修正」を同日中にやり切れた。** 発火 1 サービス修正で満足せず、監査結果に従って全同型バグを同じ PR で根治したのは、今回のインシデント対応の最大の成果。デプロイ可能な単位のバランスを崩さず、commit を意味単位で分割してレビュー可能性を維持できたことも良い pattern として次回以降に継承する
- **対応中に `docker compose up --build` の誘惑が発生しなかった。** [[PM-2026-031]] の教訓（対応者による独断ビルドの戒め）がメモリに定着しており、デプロイは `./scripts/deploy.sh production` 経由に限定することを維持した。メモリ駆動の行動抑制が機能した好事例

## 参考資料

- [[ADR-000773]] 本 PM で根治した実装決定（全 6 サービスの mTLS クライアント cert hot-reload）
- [[PM-2026-031]] mTLS cutover 残タスクで 3days Recap が 4 日連続 404 に。本 PM の直接の前段。Cons で警告していた「cutover 系作業の残タスクの可視性不足」が別形態で再露出
- [[PM-2026-030]] pki-agent sidecar の netns 幽霊化。sidecar / cert 管理の別系統失敗
- [[PM-2026-029]] nginx TLS sidecar の stale cert 問題。同じ「cert 鮮度と機能 liveness の乖離」クラス
- [[PM-2026-028]] mTLS 証明書期限切れによる Knowledge Home 停止。最初の CertificateExpired クラス PM
- [[ADR-000759]] :9443 を Connect-RPC + REST ハイブリッドに + pki-agent を compose-native cascade。本 PM 直前の mTLS 系 ADR、Cons で「mTLS 運用の継続的検証不足」を予告していた
- [[ADR-000758]] Pact ゲート付きデプロイを c2quay に移行。本 PM 恒久対策のデプロイ基盤
- [[ADR-000757]] pki-agent の 3 層防御。pki-agent ローテ運用の基盤
- [[ADR-000754]] Desktop FeedDetail editorial rail。mTLS cutover 期の一連の変更のリファレンス
- `recap-worker/recap-worker/src/clients/mtls.rs` — `ReloadingCertResolver` 実装
- `recap-worker/recap-worker/src/tls.rs` — 同 resolver の server 側配線
- `alt-backend/app/tlsutil/tlsutil.go:48-213` — 本 PM の移植元となった参照実装
- `rag-orchestrator/internal/infra/tlsutil/tlsutil.go` — Go 側の再ポート
- `acolyte-orchestrator/acolyte/infra/mtls_client.py` / `recap-evaluator/src/recap_evaluator/infra/mtls_client.py` / `recap-subworker/recap_subworker/app/infra/mtls_client.py` — Python 側の `SslContextReloader`
- `tag-generator/app/tag_generator/driver/connect_client_factory.py` — pyqwest `_ReloadingBackendClient` proxy
- commit f5cf687dd — fix(recap-worker): hot-reload mTLS identity on cert rotation
- commit 65c334816 — fix(rag-orchestrator): hot-reload mTLS identity via tlsutil certReloader
- commit 029b8eb17 — fix(python-mtls): reload SSLContext leaf cert on rotation
- commit 697d3891f — fix(tag-generator): reload pyqwest backend client on cert rotation
- commit 526bb4141 — docs(adr): record mTLS cert hot-reload fix across 6 services

---

> **Blameless Postmortem の原則:** このドキュメントは個人の過失を追及するためではなく、
> システムの脆弱性とプロセスの改善機会を特定するために作成されています。
> 「誰が悪いか」ではなく「システムのどこが改善できるか」に焦点を当ててください。
> 特に本 PM では、`reqwest::Client` の起動時 baked-in を「書いた人のミス」ではなく
> 「言語横断の mTLS cutover checklist が存在しなかったシステムの穴」として扱っています。
