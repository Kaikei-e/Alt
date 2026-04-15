# ポストモーテム: east-west mTLS 証明書期限切れによる Knowledge Home 停止

## メタデータ

| 項目 | 内容 |
|------|------|
| インシデントID | PM-2026-028 |
| 発生日時 | 2026-04-15 23:28 (JST)（alt-backend の mTLS サーバー証明書 NotAfter） |
| 検知日時 | 2026-04-16 00:00 頃 (JST)（ユーザー報告） |
| 復旧日時 | 2026-04-16 00:26 (JST)（BFF restart 後、cert expired ログ 0/分） |
| 影響時間 | 約 58 分（期限切れ到達から復旧完了まで） |
| 重大度 | SEV-2（Knowledge Home / feeds / subscriptions / recap の全 BFF 経由 RPC が失敗。ログイン後 UI が実質空） |
| 作成者 | pki / platform チーム |
| レビュアー | — |
| ステータス | Draft |

## サマリー

2026-04-15 23:28:20 JST に alt-backend の mTLS サーバー証明書（step-ca 発行の 24h leaf）が期限切れを迎えた。BFF (`alt-butterfly-facade`) → alt-backend の mTLS 経路がすべて `tls: failed to verify certificate: x509: certificate has expired` で失敗し、Knowledge Home / feeds / subscriptions / recap の 3 日要約など、**BFF 経由の主要 Connect-RPC 全滅**。直接の原因は 8 サービス分の `*-cert-init` / `*-cert-renewer` サイドカーが稼働していなかったこと。renewer 不在のまま 24h 経過で cert が期限切れ到達し、さらに既存 `cert-init` は「`svc-cert.pem` が存在すれば exit 0」の早期 return のため、期限切れ cert が残留する限り再発行されないという二重欠陥だった。復旧は (1) 期限切れ cert を 8 volume から削除、(2) shell cert-init/renewer を再作成、(3) consumer restart、という手順で約 1 分で達成。根本再発防止として、compose 埋め込み shell cert-init/renewer を専用 Go サイドカー `pki-agent` に置き換えた（[[ADR-000747]]）。

## 影響

- **影響を受けたサービス:** alt-backend, alt-butterfly-facade (BFF), その先の knowledge-home / feeds / subscriptions / recap 各 Connect-RPC
- **影響を受けたユーザー:** 発生時間帯に Alt にアクセスした全ユーザー。認証は生きていたためログインは通るが、Knowledge Home ページで feed / lens / recap が空で返る状態
- **機能への影響:** 主要機能の全面停止相当
  - `GetKnowledgeHome`, `ListLenses`, `StreamKnowledgeHomeUpdates`
  - `ListSubscriptions`, `StreamFeedStats`, `GetUnreadFeeds`
  - `GetThreeDayRecap`
  - いずれも BFF → alt-backend mTLS で失敗
- **データ損失:** なし（read-only 経路で書き込みは発生しない）
- **SLO/SLA違反:** Knowledge Home 可用性 SLO 99.5%（[[knowledge-home-slo-alerts]] 参照）に対し、30 日バジェットを約 58 分で消費。SLI-A 一時 breach

## タイムライン

| 時刻 (JST) | イベント |
|-------------|---------|
| 2026-04-14 23:28 頃 | alt-backend 用の最後の cert 発行。NotBefore=2026-04-14 23:28、NotAfter=2026-04-15 23:28（24h leaf） |
| （長期） | 何らかの運用操作で `*-cert-init` / `*-cert-renewer` 8 サイドカーが docker から消失（原因ログ残存せず）。step-ca は healthy のまま、cert-renewer だけが不在の状態が続く |
| 2026-04-15 23:28:20 | **発生** — alt-backend の svc-cert.pem が期限切れ到達 |
| 2026-04-15 23:28〜 | BFF から alt-backend への mTLS リクエストが `x509: certificate has expired or is not yet valid` で全て失敗。BFF ログに秒単位で ERROR が蓄積 |
| 2026-04-16 00:00 頃 | **検知** — ユーザーが「Knowledge Home が死んでいる」として報告 |
| 2026-04-16 00:05 頃 | **対応開始** — BFF / alt-backend / knowledge-sovereign のログを並行調査。BFF ログから TLS 証明書期限切れと確定 |
| 2026-04-16 00:08 頃 | **原因特定** — step-ca は healthy、cert-renewer 8 本ゼロ稼働を `docker ps -a` で確認。compose/core.yaml の cert-init に「cert が存在すれば exit 0」という二重欠陥があることを把握 |
| 2026-04-16 00:20 | Phase 0 緊急復旧開始: 8 volume から期限切れ cert を `docker run --rm -v` 経由で削除 |
| 2026-04-16 00:22 | 8 本の cert-init を `docker compose up -d --force-recreate` で再実行、step-ca から新 cert 発行 |
| 2026-04-16 00:24 | 8 本の cert-renewer daemon を起動 |
| 2026-04-16 00:25 | **緩和策適用** — consumer 8 サービスを `docker compose restart` |
| 2026-04-16 00:26 | **復旧確認** — BFF ログから `certificate has expired` 0/分、`curl /knowledge-home` が 302 → /feeds にリダイレクト |
| 2026-04-16 00:30〜01:45 | Phase 1-4: `pki-agent` Go マイクロサービス実装、Phase 2 cutover、observability 配線、ADR + runbook + security audit ドキュメント作成 |

## 検知

- **検知方法:** ユーザーの目視（Knowledge Home ページが空なのを発見して通報）
- **検知までの時間 (TTD):** 期限切れ発生から約 32 分
- **検知の評価:** 不十分。以下の検知手段があるべきだった:
  - cert 残存時間の Prometheus メトリクス（期限 4h 前に page）
  - `*-cert-renewer` サイドカーの稼働状態監視
  - BFF ログの `certificate has expired` エラー率が 1 件でも出たら即 page
  - Knowledge Home SLO の burn-rate アラート（機能していたが TTD を大幅短縮するほどではない）

## 根本原因分析

### 直接原因

alt-backend の mTLS サーバー証明書（24h 有効）の renewer サイドカー (`alt-backend-cert-renewer`) が稼働していなかったため、期限切れを検知・更新するメカニズムが機能せず、そのまま NotAfter に到達した。BFF の outbound mTLS は server cert を厳格に検証するため、期限切れ時点から全 RPC が失敗するようになった。

### Five Whys

1. **なぜ cert が期限切れになったのか？**
   → renewer が動いておらず、step-ca に renew を依頼する主体がいなかった。
2. **なぜ renewer が動いていなかったのか？**
   → 8 サービス分すべての `*-cert-init` / `*-cert-renewer` サイドカーが docker から消失していた。`docker ps -a` にも残っておらず、ログからも消失タイミングを特定できない。過去の `docker compose rm` や部分的なクリーンアップ操作により除去されたと推測。
3. **なぜ renewer が消えても気付かなかったのか？**
   → renewer は `restart: unless-stopped` で起動する前提だったが、**renewer の稼働状態や cert 残存有効期限を監視する Prometheus メトリクス・アラートが存在しなかった**。shell cert-renewer は stdout にログを吐くだけで、消えても誰も気付かない。
4. **なぜ cert-init を再実行しても期限切れ cert が再発行されなかったのか？**
   → cert-init シェルスクリプトに `if [ -f /certs/svc-cert.pem ]; then exit 0; fi` という早期 return があった。期限切れ cert のファイルが volume に残存していると、何度 init を走らせても「cert あり」と誤判定してスキップしてしまう欠陥。
5. **なぜこのような脆い設計を放置していたのか？**
   → cert ライフサイクルが compose YAML 内にインラインのシェルスクリプトとして埋め込まれており、ユニットテストが書けない「軟弱なグルー」として運用されていた。smallstep CLI の挙動に全面依存し、`$$` エスケープや `set -e` の穴など shell 特有の脆さも温存。TDD ガードが及ばない箇所の典型。

### 寄与要因

- **24h という短い leaf TTL**: 短命 cert はセキュリティ的に推奨だが、renewer が 1 日抜けただけで致命傷になる。監視の厚みが短命 cert の前提
- **renewer の `step ca renew` は期限切れ cert では動作しない**: smallstep の設計上、renewer は有効な現行 cert を client auth として CA に提示するため、期限切れ後には使えない。その場合の re-enrollment 経路が shell 版には無かった
- **certReloader の存在**: 幸い Go consumer 側（[[alt-backend]] / [[auth-hub]] の `tlsutil.go:44-97`）には mtime ベースのホットリロードが実装されていたため、新 cert を書けば consumer 再起動不要で拾えた。これは良い設計資産だが、逆に「cert を書き換える主体が死ぬと全部死ぬ」非対称性を生んでいた
- **障害が「alt-backend の cert」だけに見えた錯覚**: 実際には 8 サービス全ての cert が同時期に期限切れ迫っていた（全 renewer 不在）。BFF が最初に失敗したのは BFF が最初にアクセスする mTLS 相手が alt-backend だったため。視野狭窄に陥らず全 volume 状態を確認したことで被害拡大を回避

## 対応の評価

### うまくいったこと

- **ログからの即時原因特定**: BFF の構造化ログに `x509: certificate has expired` と明示的に出ていたため、TLS 関連の切り分けに余計な時間をかけずに済んだ
- **Phase 0 / Phase 1 の切り分け**: Go マイクロサービス実装（半日以上要する）を待たず、既存 shell 資産で 1 分以内に一時復旧する判断を明確に分けた。Incident Command 的な優先順位付けとして機能
- **certReloader のホットリロードが効いた**: consumer サービスを `docker restart` で再起動したが、本来は不要だった（mtime 検知で自動リロード）。再起動は保険
- **security-auditor / clean-architecture / web-researcher skill の順序適用**: 再発防止の設計フェーズで OWASP 準拠のレビュー、Alt の Clean Architecture 流儀、smallstep 公式ドキュメントベースの決定ができ、恒久策の品質が高まった

### うまくいかなかったこと

- **検知がユーザー頼み**: cert 期限切れは機械的に予測可能なイベントで、本来なら 4h 前に page すべきだった。観測が存在しなかったのは設計上の欠落
- **renewer の消失に気付く経路が無い**: `docker ps` ベースの死活監視は存在するが、cert-renewer のような「ジョブ的・期限定期実行型」のサービスが消えても即時検知できなかった
- **cert-init の「ファイルあれば skip」欠陥を事前レビューできていなかった**: 設計レビュー段階でこの早期 return の罠に気付けていれば、「期限切れでも skip する」という典型的なアンチパターンを防げた
- **SLO アラートからの TTD 短縮効果が限定的**: [[knowledge-home-slo-alerts]] は機能していたが、ユーザー通報と大差なかった。burn rate の観測窓（5m + 1h）が期限切れタイプの瞬発障害に合っていない可能性

## アクションアイテム

### 予防（Prevent）

- [x] **[Platform] `pki-agent` Go マイクロサービス化** — compose 埋め込み shell cert-init/renewer を専用 Go サイドカー 1 本に統合。responsibility は「対象 volume の cert を期限内に保つ」の 1 つだけ。期限切れ時は新 OTT での re-enrollment にフォールバック（`step ca renew` に依存しない）。実装完了: 2026-04-16、[[ADR-000747]]
- [x] **[Platform] cert-init「ファイルあれば skip」欠陥の解消** — `pki-agent` では期限切れを検出したら自動的に再発行するため、残留 cert がブロッカーにならない。完了: 2026-04-16
- [x] **[Platform] atomic write + rename-before-chmod/chown** — TOCTOU で consumer が partial cert を読む経路を断つ。完了: 2026-04-16
- [ ] **[Platform] pki-agent 専用 JWK provisioner + CN allowlist** — 現状 `bootstrap` provisioner を 8 サイドカーで共有しており、1 侵害で任意 CN 偽造可能（security audit F-001）。`pki-agent` provisioner を step-ca に追加し `x509.allowedNames` で 8 CN に限定する。**担当: platform、期限: 2026-04-30**
- [ ] **[Platform] nginx TLS sidecar の SIGHUP 化** — acolyte-orchestrator / tag-generator の nginx TLS sidecar は cert 差し替え時に reload しない限り古い cert を掴む。`inotifywait -e create,moved_to /certs && nginx -s reload` に差し替える（security audit F-007）。**担当: platform、期限: 2026-04-30**

### 検知（Detect）

- [x] **[Platform] `pki_agent_cert_remaining_seconds` Prometheus メトリクス** — subject ごとの残存秒数を gauge で出す。完了: 2026-04-16
- [x] **[Platform] `PkiAgentCertExpirySoon` アラート (残 4h で page)** — 期限切れの 4 時間前に必ず page。完了: 2026-04-16
- [x] **[Platform] `PkiAgentCertExpired` アラート (healthy=0 で page)** — 期限切れ発生即時 page。完了: 2026-04-16
- [x] **[Platform] `PkiAgentRenewalFailing` アラート (15m 継続失敗で page)** — renewer が壊れているが cert がまだ生きている段階で検知。完了: 2026-04-16
- [x] **[Platform] `PkiAgentDown` アラート (プロセス消失で ticket)** — pki-agent コンテナが消えても気付ける。完了: 2026-04-16
- [ ] **[Platform] Grafana ダッシュボード `pki-agent-overview`** — cert 残存・最終 rotation 時刻・renewal 成功率を 1 枚に。**担当: platform、期限: 2026-05-07**

### 緩和（Mitigate）

- [ ] **[Platform] leaf TTL 24h → 48h への延長検討** — 24h は運用ミスに対する余裕が無い。検知アラートが機能する前提なら 48h でも short-lived の恩恵は保てる。**担当: platform、期限: 2026-05-14**（議論必要、decision only）
- [x] **[Platform] certReloader による consumer 再起動不要ロテーション** — Go consumer は新 cert を mtime 差で拾う。pki-agent 側の atomic rename と組み合わせ、rotation 時に consumer ダウンタイムゼロ。既存資産として確認・維持

### プロセス（Process）

- [x] **[Platform] runbook `docs/runbooks/pki-agent-recovery.md` 新設** — 同種の cert 期限切れが発生した際の復旧手順を明文化。全 subject 一括復旧ワンライナーも含む。完了: 2026-04-16
- [x] **[Platform] `secrets/.gitignore` 追加** — `*_jwk.json` などの秘密ファイル誤コミット経路を塞ぐ（security audit F-003）。完了: 2026-04-16
- [ ] **[Platform] pki-agent 定期演習** — 期限切れ手動シミュレーション（volume から cert を消して、観測がどの段階で page したかを測定）を四半期ごと。**担当: platform、期限: 2026-05-30 初回、以降 quarterly**

## 教訓

### 技術的な教訓

- **「ファイルが存在する」は「有効である」を意味しない**: cert-init の `if [ -f ]` 早期 return は、同パターンが他のイニシャライザ（migration 済み判定、config 済み判定）にも潜んでいる可能性がある。存在チェックで skip する設計は「内容が現在のポリシーに適合するか」まで検証すべき
- **Short-lived は必ず観測とセット**: 24h leaf は 1 日の観測欠如で全停止を招く。lifetime を短くするほど renewer と観測の信頼性要件が跳ね上がる。lifetime × 観測欠如耐性の積が運用の余白
- **Shell in YAML は TDD ガードの外**: compose YAML にインラインで埋め込んだシェルは言語の型安全性もテストランナーも CI も届かない。重要な責務はたとえ小さくても独立した Go/Rust サービスに切り出すべき。サービスの肥大化より、テストなしコードの肥大化を避ける

### 組織的な教訓

- **運用操作のログ残存**: 誰かが `docker compose rm` で 8 renewer を消した形跡があるが、ログが残っていない。破壊的操作は audit log に出す仕組みが要る
- **renewer のような「いなくなっても当面動く」系サービスは要注意**: 直ちに症状が出ないため気付かれにくい。同類のバックグラウンドジョブ（backup, log rotation, metric scraper等）に同種の観測欠落がないか棚卸しすべき
- **障害対応と恒久策の両面記録**: 今回は Phase 0 (1 分復旧) と Phase 1-4 (数時間の恒久策) を明確に分けて記録した。ADR + 本ポストモーテムで「なぜこの設計に至ったか」が 6 ヶ月後の自分にも追える

## 関連リソース

- [[ADR-000747]]: mTLS cert ライフサイクルを compose 埋め込み shell から専用 Go サイドカー pki-agent に移行する
- [[ADR-000725]]: step-ca mTLS 基盤の段階導入
- [[ADR-000741]]: Python/Rust 含む受信側 mTLS 統一
- `docs/runbooks/pki-agent-recovery.md`: 期限切れ緊急対応ランブック
- `docs/review/pki-agent-security-audit-2026-04-16.md`: pki-agent 設計セキュリティ監査
- `pki-agent/`: Go マイクロサービス実装
- `observability/prometheus/rules/pki-agent-alerts.yml`: 4 本のアラート定義
