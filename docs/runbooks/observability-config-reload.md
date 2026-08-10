---
title: Observability 設定の変更・反映・検証 — Runbook
type: runbook
affected_services:
  - prometheus
  - alertmanager
  - grafana
owner: platform
date: 2026-08-10
last_updated: 2026-08-10
tags:
  - runbook
  - observability
  - prometheus
  - alertmanager
  - grafana
---

# Observability 設定の変更・反映・検証 — Runbook

`observability/` 配下のファイルを編集してから、それが **稼働中のプロセスに実際に
読み込まれるまで** の手順。関連: [[admin-observability]], [[deploy]],
[[pki-agent-recovery]]

> **この Runbook が存在する理由**
> Prometheus は設定を **起動時と明示的な reload 時にしか読まない**。そして
> デプロイパイプラインのどの段階も reload を行わない (§1)。結果として
> 2026-08-02 に起動したプロセスが、2026-08-09 に追加された 9 本のアラートと
> 2 本の scrape job を **一度も読まないまま 8 日間走り続けた**。ファイルを
> main に入れることと、それが有効になることは別の事象である。

## TL;DR

```bash
# 1. 編集後、まず静的検証 (promtool / amtool / 構造監査)
make observability-validate

# 2. 稼働中の設定とディスク上の設定の差分を見る
make observability-drift-check

# 3. 反映する (検証 → reload → 収束確認 まで自動)
make observability-reload

# 4. 二度と手動運用に戻さない (5 分ごとの自動 reconcile)
make observability-reload-install
```

---

## 0. 前提

| 前提 | 確認方法 | 根拠 |
|---|---|---|
| Prometheus の lifecycle API が有効 | `compose/observability.yaml` の prometheus `command:` に `--web.enable-lifecycle` があること | これが無いと `POST /-/reload` は **HTTP 405** を返す |
| 管理ポートは loopback 限定 | `compose/observability.yaml` の `ports:` が `127.0.0.1:9090` / `127.0.0.1:9093` | reload / silence API は無認証。LAN に開けない |
| 設定はホスト側の bind mount | `docker inspect <container> --format '{{json .Mounts}}'` | ファイル編集は **即座に** コンテナ内へ反映される。反映されないのは *読み直し* だけ |

### どのプロセスが何を読んでいるか

| ファイル | 読む側 | 反映の方法 |
|---|---|---|
| `observability/prometheus/prometheus.yml` | Prometheus | reload |
| `observability/prometheus/rules/*.yml` | Prometheus (`rule_files` glob 経由) | reload |
| `observability/prometheus/tests/*_test.yml` | promtool のみ。**Prometheus は読まない** | — (rules/ に置くと config 全体が壊れる) |
| `observability/alertmanager/alertmanager.yml` | Alertmanager | reload |
| `observability/grafana/dashboards/*.json` | Grafana | 自動ポーリング (`updateIntervalSeconds: 30`) |
| `observability/grafana/provisioning/datasources/*.yaml` | Grafana | **起動時のみ** → コンテナ再作成が必要 |
| `observability/grafana/provisioning/alerting/*.yaml` | Grafana | **起動時のみ** → コンテナ再作成が必要 |

---

## 1. なぜ手動運用になっているのか (2026-08-10 時点の構造)

`observability/` 配下の変更は **デプロイパイプラインのどこにも到達しない**。

1. `.github/workflows/dispatch-deploy.yaml` は push 差分を全部列挙してデプロイ側へ
   通知する。ここまでは `observability/prometheus/rules/` も含まれている。
2. デプロイ側は受け取った変更パス集合をサービスへ解決する。
   **`observability/` 配下に解決されるサービスは 1 つも定義されていない**ため、
   対象サービスが空になる。
3. 対象が空だと build / gate / e2e / **deploy の全 job が丸ごと skip** される。
4. 仮に deploy が走っても、roll される対象は解決されたサービスだけであり、
   `prometheus` / `grafana` / `alertmanager` はどのサービスにも属していない。
5. 手動デプロイ経路 (`scripts/deploy.sh` → c2quay) も同じで、`c2quay.yml` の
   `environments.production.services` に observability 系は存在しない。

さらに、稼働中の `prometheus` / `grafana` コンテナと、CI が roll するサービスとでは
**compose の working directory が異なる**。前者は開発者のチェックアウトを bind mount
しており、後者は runner のワークスペースを bind mount している。つまりデプロイ側に
observability のサービス定義を足すだけでは不十分で、「どのチェックアウトが本番の
observability 設定なのか」を先に一本化する必要がある (§7 参照)。

→ **結論**: 反映はホスト側の責務である。だからこの Runbook と
`scripts/observability-reload.sh` がある。

---

## 2. 変更のワークフロー

### 2.1 編集する

- アラート追加 → `observability/prometheus/rules/<name>.yml`
  **必ず `rules/` 直下に置く**。glob は `rule_files: /etc/prometheus/rules/*.yml`
  であり、サブディレクトリは読まれない。
- promtool ユニットテスト → `observability/prometheus/tests/<name>_test.yml`
  **`rules/` に置いてはいけない**。テストファイルはルールグループとして
  パースされ、config 全体が fatal error になる (1 ファイルのミスで全ルールが落ちる)。
- 通知先の変更 → `observability/alertmanager/alertmanager.yml`

### 2.2 静的検証する (反映前に必ず)

```bash
make observability-validate
```

内部では以下を順に実行する。1 つでも落ちたらそこで止まる。

| 段 | 内容 | 落ちるとどうなるか |
|---|---|---|
| 1 | `promtool check config` | prometheus.yml の構文エラー |
| 2 | `promtool check rules` | ルールファイルの構文 / PromQL エラー |
| 3 | `promtool test rules` | アラートの発火条件がテストと食い違う |
| 4 | `amtool check-config` | alertmanager.yml の route / receiver 不整合 |
| 5 | `scripts/observability-config-audit.py` | glob に載らないルールファイル、Grafana provisioning の YAML 破損、dashboard の uid 衝突、Alertmanager の未マウント |

同じ内容が CI (`.github/workflows/observability-validate.yaml`) でも走る。
promtool / amtool は `compose/observability.yaml` に固定されているバージョンを
GitHub releases から取得するので、CI と本番で解釈が食い違うことはない。

> **段 1 が緑でもルールの検証にはならない。** `rule_files` はコンテナ内の絶対パス
> (`/etc/prometheus/rules/*.yml`) なので、リポジトリ上ではマッチ 0 件になる。
> promtool はマッチ 0 件をエラーにしない。ルールの中身は段 2、glob に載っているか
> どうかは段 5 が見ている。

### 2.3 反映する

```bash
make observability-reload
```

これは以下を行う。

1. `make observability-validate` 相当の事前検証 (promtool がある場合)
2. `POST /-/reload` を Prometheus に送る
3. `prometheus_config_last_reload_success_timestamp_seconds` が **前進したこと** を確認
   (HTTP 200 は「要求を受け付けた」であって「読み込めた」ではない)
4. Alertmanager が起動していれば同様に reload
5. ディスクと稼働中の設定が一致したことを再確認
6. Grafana の provisioning に起動時のみ反映のファイル変更があれば警告を出す
   (**自動では再作成しない**)

#### `POST /-/reload` と `kill -HUP` の使い分け

| | `POST /-/reload` | SIGHUP |
|---|---|---|
| Prometheus | `--web.enable-lifecycle` が必要。無いと 405 | 常に有効。フラグ不要 |
| Alertmanager | フラグ不要で常に有効 | 常に有効 |
| 結果の可視性 | HTTP ステータスが返る | 返らない。metric で確認するしかない |
| 使いどころ | **既定**。スクリプト / 自動化はこちら | lifecycle API が無効、ポートが塞がっている、HTTP が死んでいる時のフォールバック |

どちらも **成否は metric でしか分からない**。`POST` が 200 を返しても、設定が
壊れていれば Prometheus は **前の設定のまま走り続ける** (これは安全側の設計だが、
黙って何も変わらない、という形で現れる)。

```bash
# 既定の経路
curl -fsS -X POST http://127.0.0.1:9090/-/reload

# フォールバック (SIGHUP)。コンテナ名は compose から引く
PROM_CID=$(docker compose -f compose/compose.yaml -p alt ps -q prometheus)
docker kill -s HUP "$PROM_CID"
```

#### reload では反映されないもの (再作成が必要)

| 変更内容 | 必要な操作 |
|---|---|
| `compose/observability.yaml` の `command:` (retention, query 制限, lifecycle) | コンテナ再作成 |
| `ports:` / `volumes:` / `mem_limit:` | コンテナ再作成 |
| Grafana の datasource / alerting provisioning | Grafana 再作成 |
| Grafana の dashboard JSON | 不要 (30 秒でポーリング) |
| Prometheus の scrape job / ルール / alerting ブロック | reload で足りる |

```bash
# 再作成が必要な場合 (compose の port 再 bind の都合上 --force-recreate を付ける)
docker compose -f compose/compose.yaml -p alt up -d --force-recreate prometheus
```

---

## 3. 反映確認

### 3.1 最終 reload 時刻と成否

```bash
curl -s http://127.0.0.1:9090/metrics | grep '^prometheus_config_last_reload'
```

| metric | 期待値 |
|---|---|
| `prometheus_config_last_reload_successful` | `1`。`0` なら**設定が読めていない**。前の設定のまま走っている |
| `prometheus_config_last_reload_success_timestamp_seconds` | 直前に reload した時刻。UNIX 秒 |

```bash
# 人が読める形に
curl -s http://127.0.0.1:9090/metrics \
  | awk '/^prometheus_config_last_reload_success_timestamp_seconds /{print strftime("%F %T", $2)}'
```

### 3.2 読み込まれた設定そのもの

```bash
# Prometheus が「今」評価している設定 (コメントは落ち、default が補われた形)
curl -s http://127.0.0.1:9090/api/v1/status/config | python3 -c 'import sys,json; print(json.load(sys.stdin)["data"]["yaml"])'

# scrape job 名の一覧 — ディスク上の prometheus.yml と件数・名前が一致するか
curl -s 'http://127.0.0.1:9090/api/v1/targets?state=any' \
  | python3 -c 'import sys,json; d=json.load(sys.stdin); print("\n".join(sorted({t["labels"]["job"] for t in d["data"]["activeTargets"]})))'
```

### 3.3 ルールの本数

```bash
curl -s http://127.0.0.1:9090/api/v1/rules | python3 -c '
import sys, json, collections
d = json.load(sys.stdin)
c = collections.Counter()
for g in d["data"]["groups"]:
    c[g["file"]] += len(g["rules"])
for k, v in sorted(c.items()):
    print(v, k)
print("TOTAL", sum(c.values()))
'
```

ディスク側の本数は `python3 scripts/observability-config-audit.py` の
サマリ行 (`prometheus rules`) が出す。**両者が一致しない = 反映されていない**。

### 3.4 Alertmanager へ繋がっているか

```bash
# Prometheus が Alertmanager を discover できているか
curl -s http://127.0.0.1:9090/api/v1/alertmanagers | python3 -m json.tool

# 通知の送信数とエラー数
curl -s http://127.0.0.1:9090/metrics \
  | grep -E '^prometheus_notifications_(sent|errors)_total'
```

`activeAlertmanagers` が空の場合、ルールは評価も発火もするが **通知はどこへも行かない**。
Prometheus の UI 上は "FIRING" と表示されるため、UI だけを見ていると気付けない。

---

## 4. ドリフト点検

### 4.1 手動

```bash
make observability-drift-check
```

ディスク上の設定と稼働中の設定を突き合わせ、差分を 1 行ずつ列挙する。差分があれば
exit 1。比較対象:

- scrape job 名の集合
- `alerting.alertmanagers` のターゲット
- `global` に明示されているキー
- アラート / 記録ルールを `(ファイル名, グループ名, ルール名)` の集合として比較
  (「11 本のはずが 4 本」ではなく「どの 7 本が落ちているか」が出る)
- Alertmanager 設定の本文一致 (`/api/v2/status` の `config.original`)
- 最終 reload からの経過時間

### 4.2 自動 (推奨)

```bash
make observability-reload-install     # systemd timer を導入 (5 分間隔)
make observability-reload-status      # 直近の実行と journal
make observability-reload-uninstall   # 撤去
```

`scripts/observability-reload.sh --if-changed` を 5 分ごとに実行する。差分が無ければ
何もしない。差分があれば検証 → reload → 収束確認まで行う。加えて **26 時間 reload が
無ければ差分の有無に関わらず reload する**。これにより
`prometheus_config_last_reload_success_timestamp_seconds` が「最後に誰かが手で
reload した時刻」ではなく **「reload 経路が生きていることの心拍」** になり、
§5 のアラートが意味を持つようになる。

Prometheus が停止中の場合、このユニットは何もせず正常終了する (計画的な再起動の
たびに timer が failed になるのを避けるため)。Prometheus 自体の死活は別の監視の担当。

---

## 5. ドリフト検知アラート (提案)

`observability/prometheus/rules/` は本 Runbook の管轄外なので、**定義は提案に留める**。
導入する場合は `prometheus-meta-alerts.yml` などの名前で `rules/` 直下に置き、
対応する promtool テストを `tests/` に添える。

```yaml
groups:
  - name: prometheus_config_health
    interval: 1m
    rules:
      # 設定が読めていない。プロセスは前の設定のまま走っているので「動いている」
      # ように見えるが、意図した設定ではない。
      - alert: PrometheusConfigReloadFailed
        expr: prometheus_config_last_reload_successful == 0
        for: 5m
        labels:
          severity: page
        annotations:
          summary: "Prometheus が設定を読み込めていない"
          description: |
            直近の reload が失敗し、以前の設定のまま評価を続けている。
            `make observability-validate` で原因を特定し、修正後に
            `make observability-reload` を実行する。

      # reload 経路そのものの死活。§4.2 の timer が 26h 心拍を打つ前提。
      # timer 未導入の環境では常時発火するため、導入とセットで入れる。
      - alert: PrometheusConfigReloadStale
        expr: time() - prometheus_config_last_reload_success_timestamp_seconds > 100800
        for: 30m
        labels:
          severity: ticket
        annotations:
          summary: "Prometheus の設定 reload が 28h 以上行われていない"
          description: |
            observability-reload.timer が停止しているか、reload が失敗し続けている。
            `make observability-reload-status` を確認する。

      # 通知経路が消えた。ルールは評価も発火もするが誰にも届かない状態。
      - alert: PrometheusNoAlertmanagerDiscovered
        expr: prometheus_notifications_alertmanagers_discovered < 1
        for: 10m
        labels:
          severity: page
        annotations:
          summary: "Prometheus が Alertmanager を 1 台も認識していない"
          description: |
            `alerting:` ブロックが読み込まれていないか、Alertmanager が落ちている。
            `curl -s http://127.0.0.1:9090/api/v1/alertmanagers` で確認する。

      # 通知の投函自体が失敗している。
      - alert: PrometheusNotificationsFailing
        expr: |
          rate(prometheus_notifications_errors_total[10m])
            / clamp_min(rate(prometheus_notifications_sent_total[10m]), 1) > 0.1
        for: 15m
        labels:
          severity: page
        annotations:
          summary: "Prometheus から Alertmanager への通知が 10% 以上失敗している"
```

閾値の根拠:

| アラート | 閾値 | 理由 |
|---|---|---|
| `PrometheusConfigReloadFailed` | `== 0` / `for: 5m` | 二値。5 分は reload 直後の一時的な観測を除くための最小値 |
| `PrometheusConfigReloadStale` | `> 100800s` (28h) | timer の心拍が 26h。余裕 2h を取る。timer 未導入なら常時発火するので導入とセット |
| `PrometheusNoAlertmanagerDiscovered` | `< 1` / `for: 10m` | Alertmanager の再起動 (数十秒) を拾わない長さ |
| `PrometheusNotificationsFailing` | `> 0.1` / `for: 15m` | 一過性の 5xx を拾わず、経路の恒常的な破損だけを拾う |

**ルール本数のドリフト** (「ディスクに 35 本あるのに 24 本しか読まれていない」) は
PromQL では表現できない。Prometheus はディスクを見ないため、ディスク側の期待値を
知っているのはホスト側だけである。これは §4 の drift-check の担当であり、
アラート化するなら `observability-reload.service` の exit code を監視する形になる。

---

## 6. Alertmanager 導入後のエンドツーエンド発火テスト

「アラートが鳴るはず」を「鳴った」に変えるための手順。3 層を **下から順に** 確認する。
上から試すと、どの層で切れているのか分からなくなる。

### 層 A: Alertmanager → 通知先

```bash
# ルーティングのドライラン。実際には何も送らない。
AM_CID=$(docker compose -f compose/compose.yaml -p alt ps -q alertmanager)
docker exec "$AM_CID" amtool config routes test \
  --config.file=/etc/alertmanager/alertmanager.yml \
  severity=page stack=push

# 実際に 1 件流す。5 分で自動的に解決する合成アラート。
curl -fsS -X POST http://127.0.0.1:9093/api/v2/alerts \
  -H 'Content-Type: application/json' \
  -d "[{
    \"labels\": {
      \"alertname\": \"AlertmanagerDeliveryTest\",
      \"severity\": \"page\",
      \"stack\": \"push\"
    },
    \"annotations\": {\"summary\": \"end-to-end delivery test — ignore\"},
    \"startsAt\": \"$(date -u +%Y-%m-%dT%H:%M:%SZ)\",
    \"endsAt\": \"$(date -u -d '+5 min' +%Y-%m-%dT%H:%M:%SZ)\"
  }]"

# Alertmanager が受け取ったか
curl -s 'http://127.0.0.1:9093/api/v2/alerts?filter=alertname%3D%22AlertmanagerDeliveryTest%22' \
  | python3 -m json.tool
```

**確認すること**: 通知先 (Pushover 等) に実際に届いたか。届かない場合は
receiver の設定か資格情報。`docker logs` に `notify retry canceled` / HTTP ステータスが出る。

### 層 B: Prometheus → Alertmanager

```bash
curl -s http://127.0.0.1:9090/api/v1/alertmanagers | python3 -m json.tool
```

`activeAlertmanagers` に URL が 1 件以上あること。空なら `alerting:` ブロックが
読み込まれていない → §2.3 で reload。

### 層 C: ルール → 通知 (全経路)

カナリアルールを一時的に置いて、評価から通知までを通す。

```bash
# 1. 常時発火するルールを rules/ 直下に置く (テスト後に必ず消す)
cat > observability/prometheus/rules/_canary.yml <<'YAML'
groups:
  - name: canary
    interval: 15s
    rules:
      - alert: ObservabilityCanary
        expr: vector(1)
        for: 0m
        labels:
          severity: ticket
        annotations:
          summary: "canary — end-to-end alert path test"
YAML

# 2. 反映
make observability-reload

# 3. 発火を確認 (30 秒ほど待つ)
curl -s http://127.0.0.1:9090/api/v1/alerts \
  | python3 -c 'import sys,json;print([a["labels"]["alertname"] for a in json.load(sys.stdin)["data"]["alerts"]])'

# 4. 通知先に届いたことを確認

# 5. 後始末 — 消して再反映するまでカナリアは鳴り続ける
rm observability/prometheus/rules/_canary.yml
make observability-reload
```

> カナリアファイルは **コミットしない**。`_canary.yml` が main に入ると
> `promtool test rules` に対応するテストが無いまま CI が警告を出し、
> かつ本番で恒久的に発火し続ける。

---

## 7. 本 Runbook では閉じていない課題

ホスト側の手順で塞げるのは「反映」までであり、以下は別リポジトリ / 別ファイルの変更が要る。

| 課題 | 必要な変更 | 所在 |
|---|---|---|
| observability 変更がパイプラインに届かない | `observability/` 配下が解決されるサービス定義をデプロイ側に追加する | デプロイ側リポジトリ |
| 本番の observability 設定がどのチェックアウトか曖昧 | `prometheus` / `grafana` / `alertmanager` の bind mount 元を一本化する。現状 CI が roll するサービスとは working directory が異なる | デプロイ側 + 運用取り決め |
| reload がパイプラインに無い | deploy の最後に `scripts/observability-reload.sh` を呼ぶ step を足す (timer と併用しても冪等) | デプロイ側リポジトリ |
| ドリフト検知アラート | §5 のルールを `rules/` に追加し、`tests/` に promtool テストを添える | `observability/prometheus/` |

---

## 8. 症状別トラブルシューティング

| 症状 | 最初に見るもの | 対処 |
|---|---|---|
| 追加したアラートが `/api/v1/rules` に出てこない | `make observability-drift-check` | reload 忘れ。`make observability-reload` |
| `POST /-/reload` が 405 | `compose/observability.yaml` の `command:` | `--web.enable-lifecycle` が消えている。SIGHUP で暫定対応し、compose を戻す |
| reload 後も本数が変わらない | `prometheus_config_last_reload_successful` | `0` なら設定が壊れている。`make observability-validate` |
| ルールを足したのに 1 本も読まれなくなった | `docker logs` の起動直後 | `rules/` にテストファイルが混入していないか。1 ファイルの fatal error で config 全体が落ちる |
| ルールファイルを足したのにそれだけ読まれない | `python3 scripts/observability-config-audit.py` | `rules/` 直下ではなくサブディレクトリに置いていないか (glob は 1 階層のみ) |
| アラートは FIRING なのに通知が来ない | `/api/v1/alertmanagers` | 空なら `alerting:` 未反映。空でなければ §6 層 A へ |
| Grafana のダッシュボードが全部消えた | provisioning YAML の構文 | 1 ファイルの破損で provisioning 全体が中断する。`make observability-validate` の段 5 |
| Grafana の datasource 変更が効かない | — | 起動時のみ反映。`up -d --force-recreate grafana` |
| scrape target が UI に出てこない | `/api/v1/status/config` の `scrape_configs` | 読み込まれた設定に job が無い = reload 忘れ |

---

## 関連

- `scripts/observability-validate.sh` — 静的検証の入口
- `scripts/observability-config-audit.py` — 構造監査 (glob 到達性 / Grafana provisioning / uid 衝突)
- `scripts/observability-drift-check.py` — ディスク vs 稼働中の差分
- `scripts/observability-reload.sh` — 検証 → reload → 収束確認
- `.github/workflows/observability-validate.yaml` — CI での静的検証
- [[admin-observability]] — Admin UI 側の Prometheus 利用と allowlist
- [[deploy]] — 本番デプロイ全体の流れ
