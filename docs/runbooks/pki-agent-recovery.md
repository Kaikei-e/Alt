---
title: pki-agent / mTLS cert 期限切れ緊急対応
date: 2026-04-16
tags:
  - runbook
  - mtls
  - pki
  - incident
affected_services:
  - pki-agent
  - alt-backend
  - alt-harvester
  - alt-data-hub
  - alt-butterfly-facade
  - auth-hub
  - pre-processor
  - search-indexer
  - tag-generator
  - recap-worker
  - recap-subworker
  - news-creator
  - rag-orchestrator
  - acolyte-orchestrator
---
# pki-agent / mTLS cert 期限切れ緊急対応

[[ADR-000747]] で導入された `pki-agent` サイドカーの障害時 runbook。本番で
「BFF ログに `certificate has expired`」や「Knowledge Home が空」が出たときの
手順を上から順に実行する。

## Provisioner 構成 (2026-07-31 更新)

step-ca には 2 本の JWK provisioner が登録されている。

| Provisioner | 用途 | 使用者 |
|---|---|---|
| `pki-agent` | 平常運用。pki-agent サイドカーが OTT mint に使う | 13 本の `alt-pki-agent-*` |
| `bootstrap` | **緊急時フォールバックのみ** (pki-agent provisioner が壊れたとき) | 本 runbook Step 3 の手動発行 |

両 provisioner には authority-level の X.509 CN allowlist が適用される。
許可される **13 の east-west subject** + `localhost`:

| subject | 備考 |
|---|---|
| `alt-backend` | ユーザ向け API |
| `alt-harvester` | [[000954]] で追加。定期ジョブ側。mTLS **クライアント**（outbox-worker → rag-orchestrator、全ジョブ → alt-data-hub:9443） |
| `alt-data-hub` | [[000954]] で追加。データプレーン。mTLS **サーバ**（`:9443` が唯一のリスナー、publish ゼロ） |
| `alt-butterfly-facade` | BFF |
| `auth-hub` | |
| `pre-processor` | |
| `search-indexer` | |
| `tag-generator` | nginx-TLS サイドカー型（下の落とし穴を参照） |
| `recap-worker` | cert の chown uid が `999`（他は `65532`） |
| `recap-subworker` | |
| `news-creator` | |
| `rag-orchestrator` | |
| `acolyte-orchestrator` | nginx-TLS サイドカー型（下の落とし穴を参照） |

これ以外の CN は `step ca certificate` 段階で `not allowed` と拒否される。
正本は `pki-agent/scripts/bootstrap-pki-provisioner.sh` の `SUBJECTS`
（`verify-cn-allowlist.sh` の `EXPECTED_CNS` と lockstep）。

> **新サービスを足すときは CN allowlist が先**。allowlist に無い CN では leaf を
> 発行できず、サイドカーが unhealthy になり、`depends_on` でスタック全体が
> 起動しない（[[000954]] Wave 0 が先行必須だった理由）。

フレッシュ install や step-ca volume 再作成後は、次のコマンドで provisioner と
policy を復元する (冪等):

```bash
bash pki-agent/scripts/bootstrap-pki-provisioner.sh
```

検証のみを走らせるなら:

```bash
bash pki-agent/scripts/verify-cn-allowlist.sh
```

## 症状からの分岐

| 症状 | 最初に見る場所 |
|---|---|
| BFF 経由の任意 RPC が `tls: failed to verify certificate: x509: certificate has expired` | `docker logs alt-pki-agent-<subject>-1` |
| pki-agent 自身が down | `docker ps --filter name=pki-agent` |
| Prometheus `PkiAgentCertExpirySoon` アラート | `docker logs alt-pki-agent-<subject>-1 --tail 30` |
| Prometheus `PkiAgentRenewalFailing` アラート | step-ca が健全か、allowlist 衝突か |

## Step 1: ステート把握

```bash
docker ps --filter label=rask.group=pki --format 'table {{.Names}}\t{{.Status}}'
docker exec alt-prometheus-1 wget -qO- 'http://localhost:9090/api/v1/query?query=pki_agent_cert_remaining_seconds' \
  | python3 -c "import json,sys;d=json.load(sys.stdin);[print(r['metric']['subject'], round(float(r['value'][1])/3600,2),'h') for r in d['data']['result']]"
```

残時間が負値または極端に小さい `subject` が復旧対象。

## Step 2: pki-agent ログで原因特定

```bash
docker logs alt-pki-agent-<subject>-1 --tail 50
```

- `tick failed` + `CA rejected request` → step-ca の allowlist に CN が無い、もしくは
  provisioner 設定ミス。次の Step 3 へ
- `tick failed` + `CA unreachable` → step-ca コンテナ or ネットワーク障害。
  `docker logs alt-step-ca-1` を確認
- ログが流れていない → pki-agent コンテナ自体が落ちている。`docker start`

## Step 3: 緊急 cert 再発行 (pki-agent が動かない場合のフォールバック)

pki-agent が復旧不能な場合、旧 shell 経路で一時的に発行する。compose 履歴から復元:

```bash
# 対象ボリュームから期限切れ cert を消す
docker run --rm -v alt_<subject>_certs:/c alpine rm -f /c/svc-cert.pem /c/svc-key.pem

# step-cli で直接発行
docker run --rm --network alt_alt-network \
  -v alt_<subject>_certs:/certs \
  -v alt_pki_trust_bundle:/trust:ro \
  -v /home/koko/Documents/dev/Alt/secrets/step_ca_root_password.txt:/run/secrets/step_ca_root_password:ro \
  smallstep/step-ca:0.27.5 sh -c '
    TOKEN=$(step ca token "<subject>" --san <subject> --san localhost \
      --ca-url https://step-ca:9000 --root /trust/ca-bundle.pem \
      --provisioner bootstrap \
      --password-file /run/secrets/step_ca_root_password)
    step ca certificate "<subject>" /certs/svc-cert.pem /certs/svc-key.pem \
      --ca-url https://step-ca:9000 --root /trust/ca-bundle.pem \
      --token "$TOKEN" --force
    chown 65532:65532 /certs/svc-cert.pem /certs/svc-key.pem
    chmod 0444 /certs/svc-cert.pem && chmod 0400 /certs/svc-key.pem
  '

# recap-worker は uid 999 を使用
#   chown 999:999 /certs/...
```

発行後、consumer service は `auth-hub/tlsutil/tlsutil.go` の certReloader が mtime を
見て自動リロードする (再起動不要)。

## Step 4: pki-agent 再起動

```bash
docker compose -f /home/koko/Documents/dev/Alt/compose/compose.yaml -p alt \
  up -d --force-recreate pki-agent-<subject>
docker logs alt-pki-agent-<subject>-1 --tail 20
```

`initial tick ok state=fresh` を確認できれば復旧。

## Step 5: 全 subject で健全性確認

```bash
for s in alt-backend alt-harvester alt-data-hub alt-butterfly-facade auth-hub \
         pre-processor search-indexer tag-generator recap-worker \
         recap-subworker news-creator rag-orchestrator acolyte-orchestrator; do
  echo -n "$s: "
  docker exec alt-pki-agent-$s-1 wget -qO- http://127.0.0.1:9510/healthz 2>&1
  echo
done
```

全部 `{"state":"fresh"}` になれば完了。

## Step 6: 事後

- 障害の事象を [[alt-adr-writer]] の慣例で ADR 追記対象か評価 ([[ADR-000747]] を更新)
- Postmortem が必要なら [[postmortem-writer]]

## 全サービス一括復旧 (2026-04-15 と同種の複数期限切れ)

```bash
# 1. 期限切れを全 volume から削除 (volume 名は compose/base.yaml が正本)
for v in alt_alt_backend_certs alt_alt_harvester_certs alt_alt_data_hub_certs \
         alt_alt_butterfly_facade_certs alt_auth_hub_certs \
         alt_pre_processor_certs alt_search_indexer_certs alt_tag_generator_certs \
         alt_recap_worker_certs alt_recap_subworker_certs alt_news_creator_certs \
         alt_rag_orchestrator_certs alt_acolyte_orchestrator_certs; do
  docker run --rm -v "$v:/c" alpine rm -f /c/svc-cert.pem /c/svc-key.pem
done

# 2. pki-agent を全部 force-recreate
docker compose -f compose/compose.yaml -p alt up -d --force-recreate \
  pki-agent-alt-backend pki-agent-alt-harvester pki-agent-alt-data-hub \
  pki-agent-alt-butterfly-facade \
  pki-agent-auth-hub pki-agent-pre-processor pki-agent-search-indexer \
  pki-agent-tag-generator pki-agent-recap-worker pki-agent-recap-subworker \
  pki-agent-news-creator pki-agent-rag-orchestrator pki-agent-acolyte-orchestrator

# 3. 消費側 restart (Go サービスは certReloader で不要だが安全側)
#    alt-data-hub を先に立てる: alt-backend / alt-harvester / 各 worker は
#    alt-data-hub:9443 を叩くので、逆順だと接続エラーのリトライで騒がしくなる。
docker compose -f compose/compose.yaml -p alt restart alt-data-hub
docker compose -f compose/compose.yaml -p alt restart \
  alt-backend alt-harvester alt-butterfly-facade auth-hub pre-processor \
  search-indexer tag-generator recap-worker recap-subworker \
  news-creator rag-orchestrator acolyte-orchestrator

# 4. 検証: BFF から cert expired エラーが消えたか
docker logs alt-alt-butterfly-facade-1 --since 1m 2>&1 | grep -c "certificate has expired"
# -> 0 が期待値

# 5. データプレーン側も見る: alt-data-hub の leaf が腐ると peer allowlist 検証が
#    通らず、consumer 側は「接続できるが全 RPC が TLS エラー」になる。
docker logs alt-alt-data-hub-1 --since 1m 2>&1 | grep -ci "tls\|certificate"
```

## よくある落とし穴

- **Python/nginx-TLS sidecar (tag-generator, acolyte-orchestrator) は SIGHUP が要る**:
  現状 pki-agent が rotation しても nginx は古い cert を掴み続ける可能性がある。
  Phase 2.5 の対応まで、これらの subject は `docker restart alt-<subject>-tls-sidecar-1`
  を手動で叩くこと
- **chown uid の取り違え**: recap-worker は `999`、他は `65532`。間違えると consumer が
  読めなくなる (compose/pki.yaml の `CERT_OWNER_UID` 環境変数を参照)
- **step-ca の provisioner password ファイル**: `secrets/step_ca_root_password.txt` は
  Git に追跡されていない (secrets/.gitignore で守られている)。復旧直後の host で
  見つからない場合は 1Password / secrets backup から復元
