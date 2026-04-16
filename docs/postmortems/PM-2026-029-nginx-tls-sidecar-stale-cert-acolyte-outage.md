# ポストモーテム: nginx TLS sidecar の cert メモリ固定による Acolyte 停止

## メタデータ

| 項目 | 内容 |
|------|------|
| インシデントID | PM-2026-029 |
| 発生日時 | 2026-04-16 頃（`alt-acolyte-orchestrator-tls-sidecar-1` がメモリ内に保持していた旧 cert の NotAfter 到達時点） |
| 検知日時 | 2026-04-16（ユーザー報告: "Acolyte が死んでいる"） |
| 復旧日時 | 2026-04-16（一次: `docker compose restart <sidecar>`。恒久: pki-agent proxy mode cutover 完了時点） |
| 影響時間 | 一次復旧まで約 10 分、恒久策 cutover まで数時間 |
| 重大度 | SEV-3（Acolyte レポート生成経路のみ停止。Knowledge Home / feeds / recap は[[PM-2026-028]]の復旧で既に健全） |
| 作成者 | pki / platform チーム |
| レビュアー | — |
| ステータス | Draft |

## サマリー

[[PM-2026-028]] の恒久策として pki-agent を導入した直後、**Acolyte の report 生成が失敗する**とのユーザー報告。調査の結果、`pki-agent-acolyte-orchestrator` は 1 時間前に新 cert を正常に発行済みで `/certs/svc-cert.pem` の NotAfter は新しかったが、その前段の **nginx TLS sidecar (`alt-acolyte-orchestrator-tls-sidecar-1`) が 32 時間稼働し続けており、起動時にロードした期限切れ直前の旧 cert をメモリに掴んだまま**だった。nginx は SIGHUP/reload なしには cert を再ロードしないため、pki-agent の atomic rename による差し替えがまったく効いていなかった。これは[[pki-agent-security-audit-2026-04-16]] で F-007 Medium として既に記録済みの既知リスクが顕在化したもの。一次復旧は該当 sidecar を `docker compose restart` で再起動しメモリ上の cert を更新することで約 1 分。恒久策として、**nginx TLS sidecar 自体を Go で置き換え**、pki-agent に optional な TLS reverse proxy mode を追加した（[[ADR-000748]]）。Go の `tls.Config.GetCertificate` は handshake ごとに mtime を見て再ロードするため、cert 差し替えは次の接続で即座に反映される — nginx の「起動時ロード固定」問題は Go では構造的に発生しない。

## 影響

- **影響を受けたサービス:** acolyte-orchestrator（Acolyte report 生成パイプライン）
- **二次被害（軽微）:** 同パターンの tag-generator も将来的に同じ経路で fail する構造的欠陥を抱えていた（今回は期限切れ到達前に発見・修正）
- **影響を受けたユーザー:** 発生時間帯に Acolyte report を要求した全ユーザー
- **機能への影響:**
  - BFF → acolyte-orchestrator:9443 の mTLS が `certificate has expired` で全失敗
  - `GenerateReport` / `GetReport` 系 Connect-RPC が全滅
  - Acolyte 経由の summary / tagging / lens 整形がすべて停止
- **データ損失:** なし（失敗リクエストはユーザー側でリトライ可能、checkpoint resume あり）
- **SLO/SLA違反:** Acolyte 個別 SLO は未設定。Knowledge Home SLO への波及はなし（既に復旧済みの経路を通らない）

## タイムライン

| 時刻 (JST, 概算) | イベント |
|-------------|---------|
| 2026-04-14 頃 | nginx TLS sidecar (acolyte-orchestrator) が起動。その時点の cert ファイルをメモリにロード |
| 2026-04-15 23:28 | alt-backend の cert が期限切れ到達、[[PM-2026-028]] 発生 |
| 2026-04-16 00:26 | [[PM-2026-028]] 一次復旧完了。この時 acolyte-orchestrator-tls-sidecar は触られず、古い cert を保持したまま継続稼働 |
| 2026-04-16 01:45 頃 | pki-agent Phase 2 cutover。全 subject の cert volume は pki-agent 管理下となり、適切に rotation 開始。しかし **nginx sidecar はメモリ上の cert を更新しない** |
| 2026-04-16（検知） | **発生** — sidecar が memory hold していた cert の NotAfter 到達、mTLS が破綻 |
| 2026-04-16 | **検知** — ユーザーが "Acolyte が死んでいる" と報告 |
| 2026-04-16 | **対応開始** — `docker ps` で `acolyte-orchestrator-tls-sidecar` が Up 32h を確認。`docker exec <sidecar> openssl x509 -in /certs/svc-cert.pem -noout -enddate` で disk 上は新 cert（pki-agent 発行、1h 前）、しかし nginx は旧 cert で handshake している矛盾を特定 |
| 2026-04-16 | **一次復旧** — `docker compose restart alt-acolyte-orchestrator-tls-sidecar-1` で nginx プロセス再起動、新 cert をロードし直して mTLS 回復 |
| 2026-04-16 | **恒久策設計** — nginx 相当機能を Go の pki-agent proxy mode に統合する方針確定。`inotifywait + nginx -s reload` によるパッチは「軟弱な shell グルーを廃する」という[[ADR-000747]]の方針に反するため却下 |
| 2026-04-16 | **恒久策実装** — pki-agent に `PROXY_LISTEN` / `PROXY_UPSTREAM` / `PROXY_VERIFY_CLIENT` / `PROXY_ALLOWED_PEERS` を追加。`internal/adapter/handler/proxy.go` 新設で `httputil.ReverseProxy` + `certReloader` 構成 |
| 2026-04-16 | **恒久策 cutover** — `acolyte-orchestrator-tls-sidecar` / `tag-generator-tls-sidecar` を compose から削除、該当 pki-agent に `network_mode: "service:<upstream>"` + proxy env を設定して起動。両 pki-agent `healthy`、ログに `TLS reverse proxy listening` 確認 |

## 検知

- **検知方法:** ユーザーの目視（Acolyte の report 生成が失敗することを発見して通報）
- **検知までの時間 (TTD):** 期限切れ発生から数分〜十数分（BFF リトライが効かなくなった瞬間からユーザー体感）
- **検知の評価:** 不十分。[[PM-2026-028]] で整備した `PkiAgentCertExpirySoon` / `PkiAgentCertExpired` は **ディスク上の cert** を対象としており、**nginx メモリ内 cert** の状態は観測できていなかった。sidecar が旧 cert を hold していても pki-agent は「healthy」「新 cert 発行済み」と報告するため、アラートが鳴らない盲点だった

## 根本原因分析

### 直接原因

nginx 1.27-alpine は起動時に `ssl_certificate` 指定のファイルを読み込んでメモリにロードする。その後の TLS handshake はすべてメモリ上の cert を使うため、ディスク上の cert ファイルを atomic rename で差し替えても **SIGHUP / `nginx -s reload` なしには反映されない**。pki-agent は atomic rename で cert を差し替えているが、nginx 側にリロードを促す手段が存在しなかった。

### Five Whys

1. **なぜ mTLS が期限切れで失敗したのか？**
   → nginx プロセスがメモリに保持していた cert の NotAfter が到達した。
2. **なぜ pki-agent が新 cert を発行済みなのに nginx がそれを使わなかったのか？**
   → nginx は起動時にしか cert を読み込まない設計。差し替えられたディスク上のファイルは認識されない。
3. **なぜこの前提で sidecar を組んでいたのか？**
   → 旧 shell cert-renewer 時代は `docker compose restart` を人手で掛ける運用想定だった。pki-agent 移行時に自動 reload 化が未完のまま cutover に至った。[[pki-agent-security-audit-2026-04-16]] F-007 Medium として記録されていたが、[[PM-2026-028]] の緊急復旧では優先度を下げた。
4. **なぜ F-007 を優先度下げたのに監視でも捕まえられなかったのか？**
   → Prometheus の `pki_agent_cert_remaining_seconds` はディスク上の cert を読む設計。nginx のメモリに載っている cert の NotAfter は別物であり、観測経路がそもそも存在しなかった。
5. **なぜ観測経路がなかったのか？**
   → "cert lifecycle は pki-agent が見る、TLS 終端は nginx が見る" という責務分離のはずが、**nginx 側の cert 状態を観測する主体が不在**だった。pki-agent は nginx の内側を知らず、nginx は自分が古い cert を使っていることを知る手段がない。

### 寄与要因

- **nginx のリロード機構が Docker の immutable container 哲学と相性が悪い**: `nginx -s reload` は実行中コンテナの中で副作用を起こす操作。Docker sidecar パターンの「コンテナ = プロセス」観と齟齬。`inotifywait` + reload の組み合わせは shell 起動スクリプトの複雑化を招く
- **[[PM-2026-028]] の成功体験による視野狭窄**: 「cert 側を Go 化すれば OK」という前提に立ったが、実際は **TLS 終端側のリロード能力** まで含めて Go 化する必要があった。pki-agent の責務を「cert 発行と配布」に限定した設計判断が逆に盲点を作った
- **Go の `certReloader` は handshake 単位で mtime を見る** という構造的優位を既に持っていたが、これを全経路で活かす設計になっていなかった。[[alt-backend]] / [[auth-hub]] など Go サービスでは機能していたが、Python サービスの前段だけ nginx というハイブリッドが残っていた
- **24h leaf TTL** が nginx 再起動の必要性を 24h に 1 回以上のペースで要求する構造。コンテナ設計として「起動時 cert ロード」との前提不一致が累積

## 対応の評価

### うまくいったこと

- **症状と原因の切り分けが早かった**: "pki-agent が新 cert を発行済み" と "mTLS が期限切れ" が矛盾する時点で「TLS 終端側のメモリ状態」に着目できた。nginx が起動時 cert ロード固定である事実が既知だったため、数分で特定
- **一次復旧が単純**: `docker compose restart <sidecar>` で 1 分以内に症状解消。恒久策が完成するまでの時間稼ぎが確実にできた
- **恒久策の設計判断**: shell entrypoint に `inotifywait + nginx -s reload` を仕込む誘惑があったが、[[ADR-000747]]の「脆弱な shell グルーを廃する」方針に忠実に、nginx sidecar 自体を Go に置き換える判断を素早く下せた
- **既存資産の再利用**: `auth-hub/tlsutil/tlsutil.go` の `certReloader` が既に handshake ごとの mtime check を実装済みだった。新規開発ではなく移植で片付いた。TDD で `proxy_test.go` を先に書き RED→GREEN→REFACTOR
- **コンテナ構成の副次的な簡素化**: 16 本の nginx sidecar + pki-agent sidecar 体制 → 8 本の pki-agent 単独体制。Python サービスあたり 3 コンテナ → 2 コンテナへ削減

### うまくいかなかったこと

- **F-007 の放置**: security audit で Medium として既に記録されていたのに、2026-04-30 期限としていた結果、24h leaf の回転周期内に顕在化してしまった。Medium でも「実害到達までの時間が lifetime 単位」の項目は High 扱いすべきだった
- **TLS 終端側のメモリ cert 監視が空白**: nginx が旧 cert を掴み続けても気付けない観測ギャップ。pki-agent 移行後もこの経路は改善されないままだった
- **cutover 完全性の検証不足**: [[PM-2026-028]] の Phase 2 で pki-agent を導入した時点で、**全 consumer で cert が実際にホットリロードされるか**のエンドツーエンド検証を省略していた。Go consumer は `certReloader` で動くが、nginx consumer は動かないという非対称を cutover 検証で捕捉できたはず

## アクションアイテム

### 予防（Prevent）

- [x] **[Platform] nginx TLS sidecar の Go 化（pki-agent proxy mode）** — acolyte-orchestrator / tag-generator の nginx sidecar を `pki-agent` の optional proxy mode に統合。`tls.Config.GetCertificate` + `certReloader` により cert 差し替えは handshake 単位で反映される構造。完了: 2026-04-16、[[ADR-000748]]
- [x] **[Platform] F-007 の恒久解消** — [[pki-agent-security-audit-2026-04-16]] F-007 Medium を Closed に更新。完了: 2026-04-16
- [ ] **[Platform] pki-agent 専用 JWK provisioner + CN allowlist** — [[PM-2026-028]] から継続。`bootstrap` provisioner 共有で 1 侵害から全 CN 偽造可能な構造は未解消。**担当: platform、期限: 2026-04-30**（[[PM-2026-028]] から据え置き）

### 検知（Detect）

- [ ] **[Platform] TLS 終端側の live cert 監視** — pki-agent proxy mode への移行で構造的にはメモリ cert 固定問題は消滅したが、念のため **外部から `:9443` へ TLS handshake して peer cert の NotAfter を観測する** blackbox exporter を追加。`pki-agent` 自身のメトリクスとは独立した検証経路。**担当: platform、期限: 2026-05-07**
- [ ] **[Platform] cert ディスク更新 → 実 handshake cert 更新の整合性 e2e 試験** — pki-agent rotation 直後に `openssl s_client -connect <svc>:9443 < /dev/null | openssl x509 -noout -enddate` を回し、ディスクと実提供 cert の NotAfter 差分を Prometheus に出す。**担当: platform、期限: 2026-05-07**
- [ ] **[Platform] Medium 脆弱性の lifetime-aware 再分類** — 「lifetime 単位で実害到達する問題は Medium でも High 扱い」というスコアリング指針を security audit テンプレに明記。**担当: platform、期限: 2026-05-14**

### 緩和（Mitigate）

- [x] **[Platform] Go の handshake-per-mtime-check 構造の全面採用** — TLS 終端が Go で統一されたことで、cert 差し替えは接続ごとに即反映される。nginx 時代の「起動時ロード固定」による盲点が構造的に消滅。完了: 2026-04-16
- [x] **[Platform] コンテナ数削減による運用面積縮小** — 16 sidecar → 8 pki-agent に統合。観測・監視・ヘルスチェック対象が半減し、運用ミスの確率も低下。完了: 2026-04-16

### プロセス（Process）

- [ ] **[Platform] cutover 完了判定チェックリストの標準化** — pki-agent 移行のような "cert lifecycle を入れ替える" 作業では、**全 consumer 種別（Go / Python / Rust / nginx-front）で cert rotation が実 handshake に反映されることの E2E 検証**を cutover 完了条件に含める。**担当: platform、期限: 2026-05-14**
- [ ] **[Platform] `docs/runbooks/pki-agent-recovery.md` への追記** — TLS 終端側（Go proxy / nginx 相当）の cert 状態確認手順、`docker compose restart` による一次復旧手順を追記。**担当: platform、期限: 2026-04-30**

## 教訓

### 技術的な教訓

- **「cert が更新された」と「サービスが新 cert を使っている」は別の事実**: pki-agent が atomic rename を完了した瞬間は "ディスクが更新された" に過ぎない。実際に接続で使われる cert は TLS 終端プロセスのメモリ内状態に依存する。両者のギャップを監視しないと、発行系は healthy なのに終端で旧 cert というズレが見えない
- **reload 機構の有無が言語/ミドルウェア選択の一級要件**: Go の `GetCertificate` callback はこの問題を構造的に解決する。nginx は reload を要求する。短命 cert 運用下では「reload なしに新 cert を拾えるか」がミドルウェア選択の核心
- **Medium 脆弱性でも「実害までの時間が lifetime 単位」なら即時対処**: CVSS や単純な severity だけでは「次の rotation で致命化する」類の項目の緊急度を見誤る。lifetime-aware トリアージが必要
- **一つのインシデントが次のインシデントの種を撒く**: [[PM-2026-028]] で pki-agent を急いで cutover した結果、nginx 側の盲点が 24h 以内に顕在化。緊急対応では「対象範囲の完全性」より「緊急経路の復旧」を優先するため、未カバー領域のフォローアップ期限を厳しく設定する必要がある

### 組織的な教訓

- **security audit の finding をカンバン化する**: F-007 が Medium として記録されただけで具体的な担当者・期限・blocker リンクが曖昧だった。audit findings → GitHub Issue / action item への自動起票プロセスが要る
- **段階的 cutover の完了判定を厳格化**: 「主要 consumer で動けば OK」ではなく、「全 consumer 種別で実 handshake レベルの検証」を完了条件に含めるべき。Python 前段の nginx 特殊ケースが抜け落ちた
- **連続 incident のナラティブ連結**: 本件は[[PM-2026-028]]の直接の続編であり、両方セットで読まれるべき。単独では「pki-agent 凄い」だけで終わるが、セットで読むと「緊急対応の副作用が次の障害を生んだ」構造が見える。連番ポストモーテム間の相互参照を明示

## 関連リソース

- [[PM-2026-028]]: east-west mTLS 証明書期限切れによる Knowledge Home 停止（本インシデントの前提となる直接の前日談）
- [[ADR-000748]]: nginx TLS sidecar を Go の pki-agent proxy モードに統合して cert rotation を hot-reload 化する
- [[ADR-000747]]: mTLS cert ライフサイクルを compose 埋め込み shell から専用 Go サイドカー pki-agent に移行する
- [[ADR-000741]]: Python/Rust 含む受信側 mTLS 統一と `PEER_IDENTITY_TRUSTED` ゲート
- [[pki-agent-security-audit-2026-04-16]]: F-007 Medium として本件を事前記録（Closed）
- `docs/runbooks/pki-agent-recovery.md`: 期限切れ緊急対応ランブック
- `pki-agent/internal/adapter/handler/proxy.go`: Go TLS reverse proxy 実装
- `pki-agent/internal/adapter/handler/proxy_test.go`: TDD tests
- `compose/pki.yaml`: pki-agent-acolyte-orchestrator / pki-agent-tag-generator の `network_mode` + proxy env 設定
