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
  - alt-notifier
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

[[000747]] で導入された mTLS leaf ライフサイクルの障害時 runbook。現行契約は [[000978]]（14 親 in-process、workload sidecar 0）。
Workload は 14 本すべて in-process（pki-agent sidecar fleet は 0）。本番で
「BFF ログに `certificate has expired`」や「Knowledge Home が空」が出たときの
手順を上から順に実行する。

## Provisioner 構成 (in-process fleet, 0 workload sidecars)

Workload PKI は **14 本すべて in-process**。pki-agent の workload sidecar
fleet は **0**。共有 `pki-agent` provisioner は使わない。

| Provisioner | 用途 | 使用者 |
|---|---|---|
| `pki-agent-<subject>` | 平常運用。14 parent の in-process enrollment | 下記 14 subject |
| `bootstrap` | **緊急時フォールバックのみ** (provisioner が壊れたとき) | 本 runbook Step 3 の手動発行 |

`step_ca_root_password` は step-ca / bootstrap 専用。workload 親プロセスには
載せない。TLS は fail-closed。cleartext に落とす経路は無い。

### Operator-created secret files (do not commit)

Cutover **前**にホストへ 14 ファイルを作る。中身は repo に置かない。
`compose/base.yaml` の `file:` は欠落で fail-fast する。

| Host file | In-container secret | Parent |
|---|---|---|
| `secrets/pki-agent-alt-backend-jwk.txt` | `/run/secrets/pki-agent-alt-backend-jwk` | alt-backend |
| `secrets/pki-agent-alt-harvester-jwk.txt` | `/run/secrets/pki-agent-alt-harvester-jwk` | alt-harvester |
| `secrets/pki-agent-alt-data-hub-jwk.txt` | `/run/secrets/pki-agent-alt-data-hub-jwk` | alt-data-hub |
| `secrets/pki-agent-alt-notifier-jwk.txt` | `/run/secrets/pki-agent-alt-notifier-jwk` | alt-notifier |
| `secrets/pki-agent-alt-butterfly-facade-jwk.txt` | `/run/secrets/pki-agent-alt-butterfly-facade-jwk` | alt-butterfly-facade |
| `secrets/pki-agent-auth-hub-jwk.txt` | `/run/secrets/pki-agent-auth-hub-jwk` | auth-hub |
| `secrets/pki-agent-search-indexer-jwk.txt` | `/run/secrets/pki-agent-search-indexer-jwk` | search-indexer |
| `secrets/pki-agent-rag-orchestrator-jwk.txt` | `/run/secrets/pki-agent-rag-orchestrator-jwk` | rag-orchestrator |
| `secrets/pki-agent-pre-processor-jwk.txt` | `/run/secrets/pki-agent-pre-processor-jwk` | pre-processor |
| `secrets/pki-agent-tag-generator-jwk.txt` | `/run/secrets/pki-agent-tag-generator-jwk` | tag-generator |
| `secrets/pki-agent-recap-worker-jwk.txt` | `/run/secrets/pki-agent-recap-worker-jwk` | recap-worker |
| `secrets/pki-agent-acolyte-orchestrator-jwk.txt` | `/run/secrets/pki-agent-acolyte-orchestrator-jwk` | acolyte-orchestrator |
| `secrets/pki-agent-recap-subworker-jwk.txt` | `/run/secrets/pki-agent-recap-subworker-jwk` | recap-subworker |
| `secrets/pki-agent-news-creator-jwk.txt` | `/run/secrets/pki-agent-news-creator-jwk` | news-creator |

Mode `0400`. Bootstrap 順:

1. step-ca が healthy
2. `bash pki-agent/scripts/bootstrap-pki-provisioner.sh`（冪等。SUBJECTS ごとに subject-scoped JWK provisioner を足し、欠落している `secrets/pki-agent-<subject>-jwk.txt` を作る）
3. `bash pki-agent/scripts/verify-cn-allowlist.sh`
4. **新イメージ + 14 JWK ファイル + 14 provisioner を compose より先に**用意する。旧 sidecar compose のまま新イメージを載せても `PKI_ENROLLMENT` 未設定なら disabled で待つが、最終 compose は sidecar が無い

### Rolling compatibility — old images + new compose is unsafe

新 compose は 14 本すべての pki-agent sidecar を消して親に enroll させる。
in-process PKI を持たない **old image** はその env を無視し、sidecar も
いないので cert writer がゼロになる。平文に落とす経路は無い。この組み合わせは
unsafe。

**Deploy order:** new images + 14 JWK files + subject-scoped provisioners **before** compose.

**Rollback stop:** restore sidecars/compose **before** old images。イメージを先に戻すと、新 compose のまま old binary が走り enroll しない。
sidecar と in-process 親を同じ cert volume に同時に載せない（dual writer）。

`docker compose up --remove-orphans` は orphan 削除が親 recreate より
先に終わることを保証しない（同じコマンド内の dual-writer 窓）。
Forward cutover / recovery は下の `retire_alt_pki_agent_leftovers` で
project=`alt` の `pki-agent-*` だけ stop+rm し、docker ps が 0 件になって
から親を `up` する。Prometheus は pki-agent `:9510` を scrape しないので
leftover 検知には使わない。

Rollback では **先に** sidecar 時代の compose で target sidecar を戻し、
**そのあと** 同じ SHA の old parent image。restore の後に retirement sweep を走らせない
（戻した sidecar を消してしまう）。

イメージ pin は SHA-aligned な [[deploy]] / alt-deploy に委譲する。HEAD の親イメージのまま
`docker compose … up -d`（サービス未指定）してはならない。mixed-mode SHA では一部親が
既に `PKI_ENROLLMENT=enabled` なので、現行イメージで全スタック recreate すると writer が二重になる。

```bash
# rollback: sidecar 時代の SHA に compose と親イメージを揃える。
# イメージ pin は SHA-aligned な alt-deploy / docs/runbooks/deploy.md。
# retire_alt_pki_agent_leftovers は呼ぶな。
#
# Phase A — restore sidecar-era compose only. Do not load old parent
# images yet. Start only pki-agent-* (--no-deps). Parents stay stopped or
# on the current enrollment images until sidecars exist.
git checkout <sidecar-era-SHA> -- compose/
mapfile -t PKI_SIDECARS < <(docker compose -f compose/compose.yaml config --services | grep '^pki-agent-')
docker compose -f compose/compose.yaml -p alt up -d --no-deps "${PKI_SIDECARS[@]}"
# Phase B — recreate parents from the SAME sidecar-era SHA (alt-deploy
# image pin). Never whole-stack `up -d` on HEAD images.
```

許可される east-west subject + `localhost`:

| subject | 備考 |
|---|---|
| `alt-backend` | ユーザ向け API |
| `alt-harvester` | [[000954]] で追加。定期ジョブ側。mTLS **クライアント**（outbox-worker → rag-orchestrator、全ジョブ → alt-data-hub:9443） |
| `alt-data-hub` | [[000954]] で追加。データプレーン。mTLS **サーバ**（`:9443` が唯一のリスナー、publish ゼロ） |
| `alt-notifier` | Web Push dispatcher。mTLS **クライアント**（alt-data-hub:9443） |
| `alt-butterfly-facade` | BFF |
| `auth-hub` | |
| `pre-processor` | |
| `search-indexer` | |
| `tag-generator` | Wave 4 in-process mTLS (:9443 in the parent); chown uid `1000` |
| `recap-worker` | cert の chown uid が `999` |
| `recap-subworker` | Wave 4 in-process mTLS; chown uid `999` |
| `news-creator` | Wave 4 in-process mTLS; chown uid `1000` |
| `rag-orchestrator` | |
| `acolyte-orchestrator` | Wave 4 in-process mTLS (:9443 in the parent); chown uid `1000` |

これ以外の CN は `step ca certificate` 段階で `not allowed` と拒否される。
正本は `pki-agent/scripts/bootstrap-pki-provisioner.sh` の `SUBJECTS`
（`verify-cn-allowlist.sh` の `EXPECTED_CNS` と lockstep）。

> **新サービスを足すときは CN allowlist が先**。allowlist に無い CN では leaf を
> 発行できず、親の in-process enrollment が fail-fast し、`depends_on`
> step-ca-bootstrap 待ちのままスタックが揃わない。

フレッシュ install や step-ca volume 再作成後は、次のコマンドで provisioner と
policy を復元する (冪等):

```bash
bash pki-agent/scripts/bootstrap-pki-provisioner.sh
```

検証のみを走らせるなら:

```bash
bash pki-agent/scripts/verify-cn-allowlist.sh
```

### Operator maps (copy into the shell first)

正本は compose の 14 cert volume と各親 `pre_start` の runtime UID
（Go distroless `65532` / Python appuser `1000` / recap `999`）。
以降の Step はこれらの配列がカレントシェルにある前提。0400 の key は
この UID の所有者でなければ親が読めない。

```bash
SUBJECTS=(
  alt-backend alt-harvester alt-data-hub alt-notifier
  alt-butterfly-facade auth-hub pre-processor search-indexer
  tag-generator recap-worker recap-subworker news-creator
  rag-orchestrator acolyte-orchestrator
)

# docker volume 名 = compose project `alt` + compose volume
declare -A CERT_VOLUME=(
  [alt-backend]=alt_alt_backend_certs
  [alt-harvester]=alt_alt_harvester_certs
  [alt-data-hub]=alt_alt_data_hub_certs
  [alt-notifier]=alt_alt_notifier_certs
  [alt-butterfly-facade]=alt_alt_butterfly_facade_certs
  [auth-hub]=alt_auth_hub_certs
  [pre-processor]=alt_pre_processor_certs
  [search-indexer]=alt_search_indexer_certs
  [tag-generator]=alt_tag_generator_certs
  [recap-worker]=alt_recap_worker_certs
  [recap-subworker]=alt_recap_subworker_certs
  [news-creator]=alt_news_creator_certs
  [rag-orchestrator]=alt_rag_orchestrator_certs
  [acolyte-orchestrator]=alt_acolyte_orchestrator_certs
)

declare -A CERT_UID=(
  [alt-backend]=65532
  [alt-harvester]=65532
  [alt-data-hub]=65532
  [alt-notifier]=65532
  [alt-butterfly-facade]=65532
  [auth-hub]=65532
  [pre-processor]=65532
  [search-indexer]=65532
  [tag-generator]=1000
  [recap-worker]=999
  [recap-subworker]=999
  [news-creator]=1000
  [rag-orchestrator]=65532
  [acolyte-orchestrator]=1000
)
```

### Forward-only sidecar retirement (copy into the shell first)

現行 target compose の workload sidecar は **0**。残っている
`pki-agent-*` は dual writer。`com.docker.compose.project=alt` かつ
compose service が `pki-agent-` で始まるコンテナだけ stop+rm する。
他 project・非 pki サービスは触らない。**Forward cutover / recovery だけ。
rollback restore の後には走らせない。**

正本は `scripts/retire-alt-pki-agent-leftovers.sh`（`scripts/deploy.sh` が
親 `up` の前に呼ぶ）。`matching pki-agent=0` は fresh install ではない:

- project=`alt` のアンカー（親 / step-ca など）が見えて sidecar が 0 → steady no-op
- project=`alt` コンテナが **1 件も見えない** → Docker context / rootless の取り違えを疑う。親 `up` を拒否する。本当に空のホストだけ `ALT_ACK_FRESH_INSTALL=1`
- service ラベル欠落 / 不正 → fail closed。ACK では救わない

```bash
# Canonical: scripts/retire-alt-pki-agent-leftovers.sh
# First install on a host that has never run project=alt:
#   ALT_ACK_FRESH_INSTALL=1 ./scripts/deploy.sh production
retire_alt_pki_agent_leftovers() {
  local -a ids=()
  local cid svc leftovers line
  local anchors=0
  local total=0
  while IFS= read -r line || [ -n "$line" ]; do
    [ -n "$line" ] || continue
    cid="${line%%$'\t'*}"
    svc="${line#*$'\t'}"
    if [ -z "$cid" ] || [ "$cid" = "$line" ] || [ -z "$svc" ]; then
      echo "malformed or missing com.docker.compose.service label. Fail closed." >&2
      exit 1
    fi
    total=$((total + 1))
    echo "project=alt id=${cid} service=${svc}"
    case "$svc" in
      pki-agent-*) ids+=("$cid") ;;
      *) anchors=$((anchors + 1)) ;;
    esac
  done < <(docker ps -a \
    --filter 'label=com.docker.compose.project=alt' \
    --format '{{.ID}}\t{{.Label "com.docker.compose.service"}}')
  if [ "$total" -eq 0 ]; then
    if [ "${ALT_ACK_FRESH_INSTALL:-}" = "1" ]; then
      echo "visible=0 pki-agent=0; ALT_ACK_FRESH_INSTALL=1 (genuinely empty host)"
    else
      echo "no project=alt containers visible. matching pki-agent=0 is not a fresh install (wrong Docker context / rootless). Set ALT_ACK_FRESH_INSTALL=1 only if this host has never run project=alt." >&2
      exit 1
    fi
  elif [ "${#ids[@]}" -gt 0 ]; then
    docker stop -- "${ids[@]}"
    docker rm -f -- "${ids[@]}"
  fi
  leftovers=$(docker ps \
    --filter 'label=com.docker.compose.project=alt' \
    --format '{{.ID}}\t{{.Label "com.docker.compose.service"}}' \
    | awk -F '\t' '$2 ~ /^pki-agent-/ { print }')
  if [ -n "$leftovers" ]; then
    echo "pki-agent leftovers still running in project alt (dual writers). Refuse parent recreate." >&2
    printf '%s\n' "$leftovers" >&2
    exit 1
  fi
}
```

## 症状からの分岐

| 症状 | 最初に見る場所 |
|---|---|
| BFF 経由の任意 RPC が `tls: failed to verify certificate: x509: certificate has expired` | `docker logs alt-<subject>-1`（親の in-process enrollment） |
| leftover pki-agent が up / `PkiAgentFleetIncomplete` | `docker ps` + `label=com.docker.compose.project=alt` かつ service `pki-agent-*` — fleet は 0 が正常。Prometheus の pki-agent scrape には頼らない（`:9510` job は無い） |
| Prometheus `PkiEnrollmentCertExpirySoon` / `PkiEnrollmentWorkloadMetricsAbsent` | 14 親の ops `:9110`: `docker logs alt-<subject>-1` |
| Prometheus `PkiEnrollmentRenewalFailing` | step-ca が健全か、subject-scoped JWK provisioner 衝突か |

## Step 1: ステート把握

```bash
docker ps -a \
  --filter 'label=com.docker.compose.project=alt' \
  --format '{{.Names}}\t{{.Label "com.docker.compose.service"}}\t{{.Status}}' \
  | awk -F '\t' '$2 ~ /^pki-agent-/'
# leftover 検知は project+service ラベル。Prometheus は pki-agent を scrape しない。
docker run --rm --network alt_alt-network busybox:1.37 \
  wget -qO- 'http://prometheus:9090/api/v1/query?query=pki_enrollment_cert_remaining_seconds' \
  | python3 -c "import json,sys;d=json.load(sys.stdin);[print(r['metric']['subject'], round(float(r['value'][1])/3600,2),'h') for r in d['data']['result']]"
```

残時間が負値または極端に小さい `subject` が復旧対象。14 本すべて
sidecar ではなく親の ops `:9110` の `pki_enrollment_*`。

## Step 2: 親プロセスの enrollment ログで原因特定

```bash
docker logs alt-<subject>-1 --tail 50
```

- enroll / tick failed + CA rejected → step-ca の allowlist に CN が無い、もしくは
  subject-scoped provisioner 設定ミス。次の Step 3 へ
- CA unreachable → step-ca コンテナ or ネットワーク障害。
  `docker logs alt-step-ca-1` を確認
- `pki_enrollment_disabled` のまま → 新 compose なのに old image。Rollback stop を見よ

## Step 3: 緊急 cert 再発行 (in-process enrollment が動かない場合のフォールバック)

pki-agent が復旧不能な場合、旧 shell 経路で一時的に発行する。compose 履歴から復元:

```bash
# Operator maps の SUBJECTS / CERT_UID / CERT_VOLUME を copy 済み前提
subject=<subject>   # 例: recap-worker
uid="${CERT_UID[$subject]:?unknown subject}"
vol="${CERT_VOLUME[$subject]:?unknown subject}"

# 対象ボリュームから期限切れ cert を消す
docker run --rm -v "$vol":/c alpine rm -f /c/svc-cert.pem /c/svc-key.pem

# step-cli で直接発行。chown は親の runtime UID（blanket 65532 禁止）
docker run --rm --network alt_alt-network \
  -e SUBJECT="$subject" -e CERT_UID="$uid" \
  -v "$vol":/certs \
  -v alt_pki_trust_bundle:/trust:ro \
  -v "$PWD/secrets/step_ca_root_password.txt:/run/secrets/step_ca_root_password:ro" \
  smallstep/step-ca:0.27.5 sh -c '
    TOKEN=$(step ca token "$SUBJECT" --san "$SUBJECT" --san localhost \
      --ca-url https://step-ca:9000 --root /trust/ca-bundle.pem \
      --provisioner bootstrap \
      --password-file /run/secrets/step_ca_root_password)
    step ca certificate "$SUBJECT" /certs/svc-cert.pem /certs/svc-key.pem \
      --ca-url https://step-ca:9000 --root /trust/ca-bundle.pem \
      --token "$TOKEN" --force
    chown "$CERT_UID:$CERT_UID" /certs/svc-cert.pem /certs/svc-key.pem
    chmod 0444 /certs/svc-cert.pem && chmod 0400 /certs/svc-key.pem
  '
```

発行後、consumer service は `auth-hub/tlsutil/tlsutil.go` の certReloader が mtime を
見て自動リロードする (再起動不要)。

## Step 4: 親プロセスを force-recreate（sidecar は無い）

```bash
# Operator maps + retire_alt_pki_agent_leftovers を copy 済み前提
retire_alt_pki_agent_leftovers

docker compose -f compose/compose.yaml -p alt \
  up -d --force-recreate <subject>
docker logs alt-<subject>-1 --tail 20
```

`pki_enrollment_enabled` と `pki_enrollment_healthy 1` を確認できれば復旧。
pki-agent sidecar を再宣言して dual writer にしない。

## Step 5: 全 14 subject で健全性確認

Workload sidecar fleet は 0。ops `:9110` のみ。

```bash
# distroless 親に wget は無い。Compose DNS で :9110 を toolbox から叩く。
for s in "${SUBJECTS[@]}"; do
  echo -n "$s: "
  docker run --rm --network alt_alt-network busybox:1.37 \
    wget -qO- "http://${s}:9110/metrics" \
    | grep pki_enrollment_healthy || echo missing
done
```

`pki_enrollment_healthy{subject="..."} 1` なら完了。

## Step 6: 事後

- 障害の事象を新しい ADR または postmortem として評価する。[[000747]] の Decision は編集しない（immutable）。新規は [[alt-adr-writer]] / [[postmortem-writer]]
- Postmortem が必要なら [[postmortem-writer]]

## 全サービス一括復旧 (2026-04-15 と同種の複数期限切れ)

```bash
# 0. leftover sidecar は dual writer。Prometheus scrape には頼らない。
#    compose up --remove-orphans では順序が保証されない。
retire_alt_pki_agent_leftovers

# 1. 期限切れを全 volume から削除 (CERT_VOLUME は compose 14 本の正本)
for v in "${CERT_VOLUME[@]}"; do
  docker run --rm -v "$v:/c" alpine rm -f /c/svc-cert.pem /c/svc-key.pem
done

# 2. 14 親を force-recreate（workload sidecar は 0。dual writer 禁止）
docker compose -f compose/compose.yaml -p alt up -d --force-recreate \
  "${SUBJECTS[@]}"

# 3. 消費側 restart (Go サービスは certReloader で不要だが安全側)
#    alt-data-hub を先に立てる: alt-backend / alt-harvester / 各 worker は
#    alt-data-hub:9443 を叩くので、逆順だと接続エラーのリトライで騒がしくなる。
docker compose -f compose/compose.yaml -p alt restart alt-data-hub
docker compose -f compose/compose.yaml -p alt restart \
  alt-backend alt-harvester alt-notifier alt-butterfly-facade auth-hub \
  pre-processor search-indexer tag-generator recap-worker recap-subworker \
  news-creator rag-orchestrator acolyte-orchestrator

# 4. 検証: BFF から cert expired エラーが消えたか
docker logs alt-alt-butterfly-facade-1 --since 1m 2>&1 | grep -c "certificate has expired"
# -> 0 が期待値

# 5. データプレーン側も見る: alt-data-hub の leaf が腐ると peer allowlist 検証が
#    通らず、consumer 側は「接続できるが全 RPC が TLS エラー」になる。
docker logs alt-alt-data-hub-1 --since 1m 2>&1 | grep -ci "tls\|certificate"
```

## よくある落とし穴

- **`--force-recreate` a parent does not require a pki-agent cascade.** Inbound TLS now terminates in the parent. Workload sidecars must be 0. A leftover `pki-agent-*` on the same cert volume is a dual writer, not a live cert-only netns topology. `scripts/cascade-pki-sidecars.sh` is a retired tombstone — do not invoke it.
- **`up --remove-orphans` を leftover 掃除だと思わない**: orphan 削除と親 recreate が同一コマンドだと dual-writer 窓が残る。親を enable / recreate する前に `retire_alt_pki_agent_leftovers`（project=`alt` のコンテナを service ラベル付きで列挙し、`pki-agent-*` だけ stop+rm、docker ps 0 件）を走らせる。`matching pki-agent=0` は fresh install ではない（見えるアンカーがあれば steady no-op。コンテナが 0 件なら Docker context / rootless を疑う。本当に空のホストだけ `ALT_ACK_FRESH_INSTALL=1`）。Prometheus の pki-agent scrape には頼らない。rollback restore の後に sweep しない。
- **chown uid の取り違え**: `CERT_UID` を使う。recap-worker / recap-subworker は `999`、Python appuser 系 (tag-generator, acolyte-orchestrator, news-creator) は `1000`、Go distroless 系は `65532`。blanket `65532` のままだと 0400 の key を uid 999/1000 の親が読めない（各親の `pre_start` chown）
- **step-ca の provisioner password ファイル**: `secrets/step_ca_root_password.txt` は
  Git に追跡されていない (secrets/.gitignore で守られている)。復旧直後の host で
  見つからない場合は 1Password / secrets backup から復元
