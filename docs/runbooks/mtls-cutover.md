---
title: mTLS 全面移行 cutover runbook
date: 2026-04-15
tags:
  - runbook
  - security
  - mtls
  - networking
---
# mTLS 全面移行 cutover runbook

X-Service-Token の shared secret から mTLS peer identity へ全面切替するための
手順集。ADR-000737 (search-indexer REST peer-identity 化 + Acolyte sidecar
VERIFY_CLIENT=on) 完了時点を起点にする。

## フェーズ状態表

| Phase | 内容 | 状態 |
|-------|------|------|
| A1 | search-indexer REST on :9443 with peer-identity middleware | DONE (ADR-000737) |
| A2 | 残 Python/Go HTTP-only サーバの mTLS listener 追加 | TODO (6 Python + 2 Go) |
| A3 | recap-worker (Rust) 送受信 mTLS | TODO |
| A4 | Acolyte sidecar `VERIFY_CLIENT=on` に昇格 | DONE (ADR-000737) |
| B  | 全クライアントを X-Service-Token から cert 提示に切替 | TODO |
| C  | X-Service-Token / SERVICE_SECRET / ServiceAuthMiddleware を完全撤去 | TODO |
| D5 | recap-worker Option-token 撤去 + Pact regen | Blocked by A3 |
| D7 | recap-worker Rust provider verify 基盤 | TODO |

## CI guard: `scripts/check-no-service-token.sh`

- **現状 (pre-cutover)**: 実行すると 527+ refs を列挙、exit 0 (報告のみ)。削除対象の全カウントが見える。
- **cutover 当日**: 全削除 PR を merge → `./scripts/check-no-service-token.sh` が `PASS` を返す状態にする
- **cutover 後**: `.github/workflows/proto-contract.yaml` に step を追加して `--strict` モードで実行、branch protection の required status check に登録

```yaml
# Add to proto-contract.yaml after Phase C merges:
- name: Guard against X-Service-Token reintroduction
  run: ./scripts/check-no-service-token.sh --strict
```

## cutover 当日の手順 (hard-cutover)

**前提**: A1-A4 + B が main に入っており、Pact CDC で全 consumer が cert 提示
していることを確認済 (`./scripts/pact-check.sh --broker` 15+/N pass)。

1. 本 runbook を画面に表示した状態で始める
2. cutover 30 分前に `#ops-alerts` に事前告知
3. `docker compose -f compose/compose.yaml -p alt ps` で全サービス healthy を確認
4. Phase C PR を merge (SERVICE_SECRET / SERVICE_TOKEN / ServiceAuthMiddleware の物理削除)
5. deploy.yaml 経由で deploy — `release-gate` が can-i-deploy を通せば deploy 進行
6. 全サービスを rolling rebuild:

   ```bash
   # 順序重要: provider 側から先に再起動すると caller が一時的に 401 を見る窓を短くできる
   for svc in mq-hub search-indexer alt-backend pre-processor news-creator \
              tag-generator recap-subworker recap-evaluator rag-orchestrator \
              acolyte-orchestrator alt-butterfly-facade auth-hub; do
     docker compose -f compose/compose.yaml -p alt up --build -d --force-recreate \
       ${svc} ${svc}-cert-init ${svc}-cert-renewer
     # nginx sidecar は netns 共有のため同時 recreate が必要 (ADR-000729)
     if docker ps -a --format '{{.Names}}' | grep -q "alt-${svc}-tls-sidecar-1"; then
       docker compose -f compose/compose.yaml -p alt up -d --force-recreate ${svc}-tls-sidecar
     fi
     sleep 10
     docker logs alt-${svc}-1 2>&1 | tail -5
   done
   ```

7. 1 時間の観察窓 — 以下を 5 分毎にチェック:
   - `docker compose ps` — 全サービス healthy
   - `curl https://curionoah.com/api/v2/health` — user path が 302 or 200
   - Acolyte: UI で report 生成 → 全 section に本文があることを確認 (PM-2026-025 の回帰 smoke)
   - Broker: `curl -u pact:... http://localhost:9292/matrix` に全 pacticipant が居る
   - alt-backend container logs: `"peer":"<cn>"` が出現する (peer-identity が伝播している)
8. 全 green なら Phase C DONE を `#ops` に通知
9. 24h 観察期間後、`scripts/check-no-service-token.sh` を CI に組み込み回帰保護を永続化

## ロールバック (Phase C の revert 手順)

X-Service-Token 削除は 1 PR 1 サービスで分割している。401/403 が大量に出た
場合:

1. 直前の service の revert PR を main に入れる (forward-only revert)
2. `docker compose up --build -d --force-recreate <svc>` で該当サービスのみ再起動
3. cert-renewer は独立 sidecar なので recreate 不要、ただし cert-init は volume が空でなければ no-op
4. 復旧確認: `curl -X POST http://<svc>:<port>/... -H X-Service-Token: <secret>` が 200 に戻る

## 事象別対応フロー

### 症状: 全リクエストが 401 Unauthorized

- 原因候補 1: peer-identity middleware の allowlist に caller CN がない
  - 確認: `docker logs alt-search-indexer-1 | grep peer_identity`
  - 対応: `compose/workers.yaml` の `MTLS_ALLOWED_PEERS` env に caller CN を追加 → `docker compose up -d search-indexer`
- 原因候補 2: cert-init sidecar 未完了 (初回起動時)
  - 確認: `docker compose ps` で `<svc>-cert-init` が `Exited 0` になっているか
  - 対応: `docker compose up -d <svc>-cert-init` で再実行

### 症状: TLS handshake failure: unknown authority

- 原因: `/trust/ca-bundle.pem` が caller 側の trust store に無い
- 確認: `docker exec alt-<caller>-1 cat /trust/ca-bundle.pem | openssl x509 -text -noout | grep Issuer`
- 対応: pki_trust_bundle volume の mount 確認、step-ca が initialize 済か

### 症状: client cert 提示できない / caller 側 "no such file"

- 原因: caller の cert-init sidecar が未実行、または `MTLS_CERT_FILE` env が
  cert volume 内のパスと合ってない
- 対応: `docker compose up -d <caller>-cert-init` → `ls -l /certs/` で
  `svc-cert.pem` と `svc-key.pem` の存在確認

### 症状: PM-2026-025 型の "report に空セクション"

- 原因: acolyte → search-indexer の mTLS が失敗し degraded continue が発動
- 確認:
  ```bash
  docker logs alt-acolyte-orchestrator-1 2>&1 | grep -E "Gatherer|peer_identity|No claims"
  ```
- 対応:
  - search-indexer が peer CN `acolyte-orchestrator` を allowlist に含むか確認
  - acolyte の `_build_mtls_context()` が `MTLS_ENFORCE=true` で ssl_context を返すか main.py ログで確認
  - Pact: `./scripts/pact-check.sh --broker` の `acolyte-orchestrator → search-indexer` が GREEN か確認

### 症状: nginx sidecar が起動できない (`Cannot restart container ... joining network namespace ... No such container`)

- 原因: ADR-000729 の既知問題。`docker compose up --force-recreate <svc>` 後に sidecar の netns が lost
- 対応: `docker compose up -d --force-recreate <svc>-tls-sidecar` で sidecar も再作成

## cert rotation 運用

- 各サービスは `<svc>-cert-renewer` が `step ca renew --daemon --expires-in=8h` で 16h 残るよう自動更新
- cert 有効期限は 24h。step-ca が down していても 16h の safety margin がある
- step-ca ダウン時の対応: `docker compose up -d step-ca` で再起動。renewer は指数バックオフで再試行する

### 手動 rotate が必要なケース

- CA 署名鍵の compromise 疑い → `step ca rekey` 手順で intermediate CA を rotation。leaf は自動追従
- secret が git に誤コミットされた → `secrets/` 配下の該当ファイルを regenerate + `docker compose up -d` で全サービス restart

## 参考

- [[000725]] step-ca mTLS 基盤
- [[000727]] Phase 1 ハードニング + Phase 2 pilot 永続化
- [[000729]] BFF outbound 全 mTLS 化 / sidecar netns 問題
- [[000737]] search-indexer REST peer-identity + Acolyte VERIFY_CLIENT=on
- [[000738]] (Phase C 完成時に作成予定) X-Service-Token 完全撤去
- [[PM-2026-025]] acolyte empty section incident
- [[PM-2026-026]] rag-augur search-indexer auth gap
