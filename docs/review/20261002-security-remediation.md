---
title: Alt 全マイクロサービス 包括的セキュリティ是正・実証監査レポート (2026-10-02)
date: 2026-10-02
tags:
  - security
  - remediation
  - audit
  - microservices
  - alt
  - sol6.1
---

# Alt 全マイクロサービス 包括的セキュリティ是正・実証監査レポート (2026-10-02)

- **レビュー・是正期間**: 2026年10月01日 〜 10月02日 (JST)
- **対象リポジトリ**: `Alt` (`Kaikei-e/Alt`)
- **評価主体**: Native agy Worker (Author) & Codex Sol 6.1 Agents (`sol_auth_review`, `sol_infra_review`, `sol_scope_review`)
- **監査対象スコープ**: 全 53 ID (昇華仮説 10件, 確定是正所見 22件, 棄却仮説 20件, 未解決所見 1件 [D-H01])

## 1. エグゼクティブサマリー & 是正方針

本レポートは、全 53 所見に対するコード・設定・テストフィクスチャの現状を、客観的に記録する。以下の過大主張を厳格に排除する：
全セキュリティ問題の解決、本番デプロイ完了、実機フル負荷試験、完全なゴミデータクリーンアップ（実行・全ストア検証未完了）、シークレット即時失効/CA再読込、Bun 1.3.12本番稼働検証。

## 2. 全 53 ID 網羅マスタートレース・是正台帳

| ID | 区分 | 対象コンポーネント | 是正・判定要約 | 主な参照コード / 設定ファイル | 独立検証 (Sol 6.1) / 現状 |
|:---|:---|:---|:---|:---|:---|
| **I-01** | Superseded | cAdvisor | `D-01`へ昇華（特権実行・ソケット露出課題として統合整理） | `compose/observability.yaml` | `D-01`に従属 |
| **I-02** | Superseded | logging-proxy | `D-01`,`D-05`へ昇華（Docker APIプロキシのスコープ過剰・ドリフト整理） | `compose/logging.yaml`, `compose/backup.yaml` | `D-01`, `D-05`に従属 |
| **I-03** | Superseded | Ory Kratos | `A-03`,`H-DISPROVE-05`へ昇華（ポリシー緩怠とネットワーク隔離分離） | `compose/auth.yaml`, `kratos/kratos.yml` | `A-03`, `H-DISPROVE-05`に従属 |
| **I-04** | Superseded | RAG / Acolyte | `B-H02`,`C-H01`へ昇華（所有者認可とJWT検証分離） | `rag-orchestrator/`, `acolyte-orchestrator/` | `B-H02`, `C-H01`に従属 |
| **I-05** | Superseded | 内部平文ポート | `C-01`,`C-02`へ昇華（search-indexerとpre-processorの平文開口分離） | `search-indexer/`, `pre-processor/` | `C-01`, `C-02`に従属 |
| **I-06** | Superseded | Redis インフラ | `B-05`へ昇華（Redis共有・ACL未設定課題） | `compose/mq.yaml`, `compose/ai.yaml` | `B-05`に従属 |
| **I-07** | Superseded | Feed Harvester | `B-H01`へ昇華（DNS Rebinding/SSRF防御論証） | `alt-backend/app/orchestrator/` | `B-H01`に従属 |
| **I-08** | Superseded | Sovereign/Recap | `B-H03`,`C-H04`へ昇華（起動時トークン必須検証・フェイルファスト） | `knowledge-sovereign/`, `recap-worker/` | `B-H03`, `C-H04`に従属 |
| **I-09** | Superseded | 運用・メトリクス | `B-H04`,`D-H02`へ昇華（オペレーターBearer認証とホスト限定） | `alt-backend/`, `compose/observability.yaml` | `B-H04`, `D-H02`に従属 |
| **I-10** | Superseded | Plecto/Dashboard | `A-02`,`C-H05`へ昇華（Plectoマウント分離とSSEログサニタイズ） | `compose/core.yaml`, `recap-dashboard/` | `A-02`, `C-H05`に従属 |
| **A-01** | Actionable | auth-hub / BFF | 内部JWT(30分→5分) & Kratosキャッシュ60s化、明示的失効API | `auth-hub/internal/usecase/invalidate_session.go` | `sol_auth_review`: FINAL ACCEPT / native PASS |
| **A-02** | Actionable | plecto-proxy | `.plecto`マウントを`manifest.toml`, `dev-key.pub`, 公開CA、signed ARTIFACT DIRECTORYに厳格化 | `compose/core.yaml` | `sol_infra_review`: FINAL ACCEPT 504 |
| **A-03** | Actionable | Ory Kratos | kratos.ymlポリシー強化(HIBP等)、エントリーポイントで安全YAML生成 | `compose/auth.yaml`, `kratos/kratos.yml` | `sol_auth_review`: FINAL ACCEPT / native PASS |
| **A-04** | Actionable | alt-frontend-sv | フロント HTTPS(:8443)既定、`NODE_EXTRA_CA_CERTS`。Biome全適用は無 | `compose/core.yaml` | `sol_auth_review`: FINAL ACCEPT 516 / Auth PASS |
| **B-01** | Actionable | alt-data-hub | 192ピア/プロシージャ詳細認可マップ(`require_peer_procedure.go`)移行 | `alt-backend/app/middleware/require_peer_procedure.go` | `sol_auth_review`: FINAL ACCEPT / native PASS |
| **B-02** | Actionable | sovereign | Lens所有権分離、リモートJWT常に検証・ローカルフォールバック無 | `knowledge-sovereign/app/handler/event_auth_interceptor.go`, `knowledge-sovereign/app/domain/authcontext/jwt.go` | `sol_scope_review`: FINAL ACCEPT 511 / pure Domain PASS |
| **B-03** | Actionable | news-creator | `PEER_IDENTITY_STRICT=true`既定、未認証・空白ピア401遮断 | `news-creator/app/news_creator/infra/peer_identity.py` | `sol_scope_review`: FINAL ACCEPT / native PASS |
| **B-04** | Actionable | knowledge-augur | `compose.augur.yaml`ホストポート11435/11436を`127.0.0.1`に限定 | `compose.augur.yaml` | `sol_infra_review`: FINAL ACCEPT 504 |
| **B-05** | Actionable | redis インフラ | streams/cache/limiterの3ロール分離、UTF-8・LC_ALL=CのACL生成適用 | `docker/redis/entrypoint.sh` | `sol_infra_review`: FINAL ACCEPT / native PASS |
| **B-06** | Actionable | fix_article_title| パブリックDNS IPピン留め、リダイレクト最大10回、最大2MiB制限 | `alt-backend/app/cmd/fix_article_titles/main.go` | `sol_scope_review`: FINAL ACCEPT 515 / native PASS |
| **B-SUPP-01** | Actionable | embedder / rerank | トークン文字数MIN制限(候補数ではない)、パディング検証、入力制限 | `rerank-server/tests/test_rerank_auth.py` | `sol_scope_review`: FINAL ACCEPT / native PASS |
| **C-01** | Actionable | search-indexer | Meili公式nested partial merge検証済(SettingsDiff旧値保持/UpdateWithoutReindex)。一次契約解消、実エンジン実行未検証 | `search-indexer/app/connect/v2/server.go` | `sol_scope_review`: FINAL ACCEPT 528 (一次ソース検証済) / engine RUN UNVERIFIED |
| **C-02** | Actionable | pre-processor | mTLSバイパス分離、URL focus tests PASS、プロキシ完全body reject | `pre-processor/app/middleware/peer_identity_middleware.go` | `sol_scope_review`: FINAL ACCEPT 502 / native URL PASS |
| **C-03** | Actionable | mq-hub | 平文(:9500)5認証インターセプター、ストリーム名正規プレフィックス検証 | `mq-hub/app/connect/v1/mqhub/auth_interceptor.go` | `sol_scope_review`: FINAL ACCEPT / native PASS |
| **C-04** | Actionable | step-ca/pki-agent| プロビジョナー単位X.509テンプレート制約、既存テンプレート修復 | `pki-agent/scripts/bootstrap-pki-provisioner.sh` | `sol_infra_review`: FINAL ACCEPT (real rendered CSR/jq/template native PASS), runtime CA renew/revoke unverified |
| **C-05** | Actionable | recap-evaluator | `/api/v1/evaluations/run`に`EVALUATOR_API_TOKEN_FILE`Bearer認証 | `recap-evaluator/src/recap_evaluator/infra/bearer_auth.py` | `sol_scope_review`: FINAL ACCEPT / native PASS |
| **C-07** | Actionable | tag-generator | `/extract-tags`のno-op撤廃、`@require_auth`と`PEER_IDENTITY_STRICT` | `tag-generator/app/tag_generator/infra/peer_identity.py` | `sol_scope_review`: FINAL ACCEPT / native PASS |
| **D-01** | Actionable | cadvisor/forward | nonroot65534 cap_drop ALL + Docker allowlist containment | `compose/logging.yaml`, `docker/readonly-proxy/` | `sol_infra_review`: FINAL ACCEPT 504 / DAC runtime UNVERIFIED |
| **D-02** | Actionable | rask-aggregator | 13実プロデューサーRASKファイル/マウント、Go/Rust受信送信認証。独立native loader/configとauthor wire区別 | `rask-log-aggregator/app/src/auth.rs`, `compose/` | `sol_auth_review`: ACCEPT 481(Go) / `sol_scope_review`: ACCEPT 501(Rust) / `sol_infra_review`: 504 |
| **D-03** | Actionable | CI/CD (Pact) | raw masterシェル廃止、committed SHA256 validates BOTH artifacts; SHA1 provenance cross check only | `scripts/ci/pact-artifacts.py` | `sol_infra_review`: FINAL ACCEPT / native PASS |
| **D-04** | Actionable | perf (alt-perf) | active SYS_ADMIN/seccomp/root k6削除、署名鍵廃止・短期JWT(k6_api_token)のみ、USER deno。全3ラッパー安全停止中 | `compose/perf.yaml`, `alt-perf/scripts/manifest.ts` | `sol_auth_review`: FINAL ACCEPT (536ソース反映, 30 tests 56 steps cached-only PASS / SQLランタイム未検証・全ラッパー安全停止中) |
| **D-05** | Actionable | backup (restic) | `docker-socket-proxy`/`docker-cli`削除、非特権ダンプ/ホストマウント | `compose/backup.yaml`, `docker/backup/Dockerfile` | `sol_infra_review`: FINAL ACCEPT / restore PITR pending |
| **D-H01** | Unresolved | cAdvisor / proxy | カスタムGo ROプロキシによりアーカイブ取得拒否。cAdvisor是正受容。 | `docker/readonly-proxy/` | cAdvisorソース是正受容 / 実機DACランタイム・シークレット漏洩は未実証・保留 |
| **H-DISPROVE-01** | Rejected | BFF | SameSite=Lax下CSRF懸念: JSON/Protobuf POSTはCORS等で遮断実証済 | `alt-butterfly-facade/` | `sol_auth_review`: 既存防御機構論証済・棄却維持 |
| **H-DISPROVE-02** | Rejected | BFF | パストラバーサル懸念: パス正規化・許可リスト等により遮断実証済 | `alt-butterfly-facade/` | `sol_auth_review`: 既存防御機構論証済・棄却維持 |
| **H-DISPROVE-03** | Rejected | Ory Kratos | 未知ロール注入昇格懸念: スキーマ(`identity.traits.schema.json`)で拒絶 | `compose/auth.yaml` | `sol_auth_review`: 既存防御機構論証済・棄却維持 |
| **H-DISPROVE-04** | Rejected | token-manager | OAuth stateリプレイ懸念: ワンタイム照合で拒絶実証済 | `auth-token-manager/` | `sol_auth_review`: 既存防御機構論証済・棄却維持 |
| **H-DISPROVE-05** | Rejected | Ory Kratos | 管理ポート露出懸念: `kratos-admin`内部ネットワークにのみバインド | `compose/auth.yaml` | `sol_infra_review`: 既存防御機構論証済・棄却維持 |
| **B-H01** | Rejected | Feed Harvester | SSRF/DNS Rebinding懸念: IPピン留め・プライベートIP拒絶実証済 | `alt-backend/app/orchestrator/` | `sol_scope_review`: 既存防御機構論証済・棄却維持 |
| **B-H02** | Rejected | rag-orchestrator | テナント横断漏洩懸念: mTLS終端・Bearer・DB所有者フィルタ実証済 | `rag-orchestrator/` | `sol_scope_review`: 既存防御機構論証済・棄却維持 |
| **B-H03** | Rejected | sovereign | トークン不在時Fail-Open懸念: 起動時検証で`log.Fatal`停止実証済 | `knowledge-sovereign/app/config/` | `sol_scope_review`: 既存防御機構論証済・棄却維持 |
| **B-H04** | Rejected | alt-backend | オペレーターポート露出懸念: Bearer必須・ホストループバック限定バインド | `alt-backend/app/cmd/backend/` | `sol_scope_review`: 既存防御機構論証済・棄却維持 |
| **B-H05** | Rejected | irodori/speaker | 未認証アクセス懸念: speakerは厳格mTLS、irodoriはAPIキー必須実証 | `compose/tts.yaml` | `sol_scope_review`: 既存防御機構論証済・棄却維持 |
| **C-06** | Rejected | oauth-token-init | `/data` 0755懸念: 走査用0755正当、OAuthトークンファイル `/app/secrets/oauth2_token.env` の意図された0640(UID65532)、livemode unverified。他コンテナ破壊防ぐ | `compose/workers.yaml` | `sol_infra_review`: 既存防御機構論証済・棄却維持 |
| **C-H01** | Rejected | acolyte | 平文(:8090)未認証懸念: `UserIdentityInterceptor`によりJWT所有者認可実証 | `acolyte-orchestrator/` | `sol_scope_review`: 既存防御機構論証済・棄却維持 |
| **C-H02** | Rejected | pre-processor | URL不備SSRF懸念: `FetchArticle`は`ErrFetchDisabled`返却、未配線 | `pre-processor/app/` | `sol_scope_review`: 既存防御機構論証済・棄却維持 |
| **C-H03** | Rejected | recap-subworker | `joblib.load` RCE懸念: ユーザー入力フィードと管理MLモデル読込パイプ分離 | `recap-subworker/` | `sol_scope_review`: 既存防御機構論証済・棄却維持 |
| **C-H04** | Rejected | recap-worker | 管理API Fail-Open懸念: トークンファイル不在時は起動時エラー終了実証 | `recap-worker/` | `sol_scope_review`: 既存防御機構論証済・棄却維持 |
| **C-H05** | Rejected | recap-dashboard | SSE漏洩懸念: トークン必須検証、ログ正規表現サニタイズ実証済 | `recap-dashboard/` | `sol_scope_review`: 既存防御機構論証済・棄却維持 |
| **D-H02** | Rejected | Prom/Grafana | 外部露出懸念: `127.0.0.1`限定バインド、Grafanaパスワード必須実証済 | `compose/observability.yaml` | `sol_infra_review`: 既存防御機構論証済・棄却維持 |
| **D-H03** | Rejected | Pact Broker | 公開改ざん懸念: `PACT_BROKER_ALLOW_PUBLIC_READ=false`、Basic認証必須 | `compose/pact.yaml` | `sol_infra_review`: 既存防御機構論証済・棄却維持 |
| **D-H04** | Rejected | altctl CLI | CLIコマンドインジェクション懸念: argv安全伝達、固定sh -c引数分離実証済 | `altctl/` | `sol_infra_review`: 既存防御機構論証済・棄却維持 |
| **D-H05** | Rejected | 歴史的資産 | K8s等誤稼働懸念: Compose include参照ゼロ、非稼働隔離実証済 | リポジトリ構造 | `sol_infra_review`: 既存防御機構論証済・棄却維持 |

## 3. 領域別セキュリティ是正の最新検証ステータス (Latest True FINAL)

- **Go/Rust OTLP (D-02)**: ACCEPT sources Go 481 (sol_auth_review: 13実プロデューサーRASKファイル/マウント送受信認証、独立native loaderとauthor wire区別) & Rust 501 (sol_scope_review: AppEnvGuard20、独立native configとauthor wire区別) & Compose 504 (sol_infra_review: 13実プロデューサーRASKマウント)。
- **Deno / Python**: Deno 410 pure boot/redaction author 24/116 vs native prior 13. Python 500 Relay 4 sessions/file/noRedirect/SDK 7 cap + 8 native loader disabled/five Session probe cases accepted. Relay 515 canonical Parsed URL native 22 cases PASS.
- **Compose 504**: FINAL ACCEPT 10 YAML duplicate 0 / private own cert sidecars + correct proxy --target flags actual upstreams + native early CLI health port numeric 1..65535/noRedirect 5s 4KB (13 actual exporter RASK mounts belong to D-02). Distinguish author wire / native offline proofs.
- **Pre 502**: FINAL ACCEPT native URL focused tests PASS. HTTPS before DB no HTTP fallback + stream clone health client + proxy bounded complete body reject truncation 0 hits / 30 upload / 600 resp header / SSE Write 0 + raw URL error redaction.
- **Sov Architecture 511**: FINAL ACCEPT pure Domain verified Claims stdlib/tokencontract Port/Gateway driver DTO mapper. remote JWT always no local fallback. Native 2 auth/tenant/Lens pkg PASS.
- **RAG / Polish 515**: FINAL ACCEPT actual RAG TLS/health factories used main, backend genuine factory REAL distinct TLS redirect origin 1/dest 0 source proof + author wire. SolScope RAG 519 accepted actualfactory/canonicalorigin/failclosed missingproxyToken+unknownPurpose and native `go test -count=1 -timeout45 ./internal/di` PASS.
- **FrontEnd 516**: author Auth 8 + Logout 4 PASS bun run check 0 errors 0 warnings, native peer earlier Auth 323 accepted source. Biome binary EXEC denied source whitespace instead.
- **Perf (D-04)**: ORIGINAL security source ACCEPT (sol_auth_review): active SYS_ADMIN, seccomp:unconfined, root k6 override 削除、backend_token_secret 署名鍵マウント削除、発行者発行の短期JWTファイル (k6_api_token: /run/secrets/k6_api_token) のみ使用、alt-perf Dockerfile は USER deno (k6実効イメージUIDは実行時未検証)。原初D-04の特権・署名鍵廃止ソース是正は既にACCEPT済。全3ラッパーは安全停止 (TEMPORARILY UNAVAILABLE)。Perf 536 ソース最終判定 sol_auth_review ACCEPT: true empty SQL (余分な外側括弧なし) + atomic owners preflight UNION + db-teardown enum + post-COMMIT guard / source one bounded locked tx / directed feed link foreign subs / stale flags current row truth を反映。全30 tests 56 steps cached-only native PASS。【重要】SQLランタイム未検証 (SQL runtime UNVERIFIED / local Postgres absent / 実際の負荷・実DELETE未実行)。全3ラッパーは一時的利用不可 (TEMPORARILY UNAVAILABLE)。全クロスストア(Meili/Redis/detached rows)の歴史的ゴミクリーンアップは未検証・未完了。
- **Meili (C-01)**: Meili official nested partial merge is now SolScope VERIFIED through full primary decoded blob (89,416 bytes, SHA: 499ab395...): SettingsDiff::from_settings starts with old fields, apply_and_diff each field, and construct result preserves omitted source/model/url/dimensions/request/response (defaults change only on source change). api_key.apply(new_api_key) produces no ReindexAction -> UpdateWithoutReindex (verified via https://github.com/meilisearch/meilisearch/blob/v1.27.0/crates/milli/src/vector/settings.rs#L718 and #L1190, caller update/settings.rs#L1103 verified cached full 103,653 bytes). Primary contract gap closed, ensure_embedder comment fixed peer accepted, BUT real engine execution remains UNVERIFIED.
- **Quality 522**: independent sol_infra_review ACCEPT: スコープ限定 git diff 確認済、4つの不要代入 (dead assignments) 削除、child isolation および proto nesting を維持。フル機能テストの完了は主張しない。

## 4. 読取専用監査エビデンス記録 (`run_e2434334-77e`)

本タスクでの k6 実行・負荷テストデータ生成・実際のDELETE処理は一切行っていない。以下の読取専用スコープにおける安全なSQLスナップショット（4ブロック）のみを確認した。
```sql
BEGIN READ ONLY;
SET LOCAL statement_timeout = 5000;
SELECT count(*) FROM articles WHERE user_id::text LIKE '00000000-0000-4000-a000-%'; =>0
SELECT count(*) FROM article_heads h JOIN articles a ON a.id=h.article_id WHERE a.user_id::text LIKE '00000000-0000-4000-a000-%'; =>0
SELECT count(*) FROM article_summaries s JOIN articles a ON a.id=s.article_id WHERE a.user_id::text LIKE '00000000-0000-4000-a000-%'; =>0
SELECT count(*) FROM feed_links WHERE url ~ '://mock-rss-[0-9]+([:/]|$)'; =>0
ROLLBACK;
```
※ 上記4つのSQL countがゼロであることを明示的に確認。other stores/Meili/Redis/detached rows/historical garbage cleanup NOT verified。NO actual load/DELETE。
※ Original Rejected category の source baseline は new native validation とは異なる点に留意。Runtime D-H01 unresolved, no deploy。
