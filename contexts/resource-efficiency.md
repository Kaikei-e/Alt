# Resource Efficiency — Context Glossary

構成・アーキテクチャ (20+ microservices / Docker Compose / Clean Architecture) を変えずに、
ログと実測値からランタイムのリソース消費 (memory / CPU / storage / 消費電力) を right-size する運用境界の用語集。
実装詳細は持たず、概念の正準語を固定する。全サービスに cross-cutting に作用する。

> 2026-06-17 grill セッションで定義中。用語は決着したものから順に追記する。

## Language

### Core

**富豪的 (lavish) baseline** (anti-term / 出発点):
idle 主体の実測ワークロードが決して行使しないスケールに合わせた provisioning。
本境界が解消対象とする状態そのもの。
_Avoid_: over-provisioning, 余裕を持った構成

**Right-sizing**:
worst-case の当て推量ではなく、実測された需要に割り当てを合わせる調整。
_Avoid_: optimization (曖昧), チューニング (単独では曖昧)

**Measurement-first**:
目標値の設定も knob 変更も、実測 baseline (実測値) を取ってからのみ行う原則。
停止条件は推測ではなく測定から来る。
_Avoid_: 勘でチューニング

### 2 つの犯人 (goal ごとに撃ち分ける)

**Idle churn (背景チャーン)**:
トラフィックの有無に関係なく CPU/GPU を deep-idle から起こし続ける背景仕事
(flush ループ・polling・cron・health-check・ログ書き込み)。**idle 電力 (#1)** の主犯。
_Avoid_: idle load, 背景ノイズ

**Resident footprint (常駐フットプリント)**:
トラフィックに関係なく steady-state で居座る RAM/VRAM (ML モデル・バッファ・heap 下限)。
**RAM 天井 (#2)** の主犯。idle 電力にはほぼ無関係。
_Avoid_: メモリ使用量 (曖昧)

**Health-check noise**:
観測価値を持たないのにトラフィック・ログ量を支配する health-check polling (baseline で全 HTTP の 80%)。
idle churn とログ I/O の両方を膨らませる。
_Avoid_: 監視トラフィック

### スコープ境界

**Tuning (in-scope)**:
本境界が変更してよいレバー — runtime knobs (mem_limit / GOMEMLIMIT / GC / pool / replica)・
背景 cadence (health-check interval / cron / flush interval)・log retention/verbosity (TTL / sampling / LOG_LEVEL)。
app サービスの分割・Clean Architecture 層・サービス間契約は **out-of-scope**。
_(観測基盤の構造変更が in か out かは grill 中: [[#Measurement substrate]] 参照)_

**Measurement substrate**:
実測値を生む観測コンポーネント (ClickHouse / Prometheus / docker stats)。tuning が依存する情報源のため、
**最初に外すものにはできない**。
_Avoid_: ログ基盤 (曖昧)

### 測定 (method)

**Idle window**:
ユーザ操作ゼロの観測窓。steady-state (idle) 消費を測る。peak window とは別取りで、idle と peak は別 goal。
#1 はまず idle window から。
_Avoid_: 平常時 (曖昧)

**Peak window**:
load-test / k6 で人工的に負荷をかけた観測窓。#2 RAM 天井の根拠。idle window と混ぜない。

**CPU proxy**:
watts を直接計装せず、既存 cadvisor の per-container CPU% と host idle CPU を idle 電力の代理指標とする。
wakeup↓ = deep-idle 滞在↑ = 電力↓ と読む。
_Avoid_: 電力測定 (watts は測っていない)

**Non-富豪的 measurement** (原則):
効率化の測定自体が footprint を増やしてはならない。新規 exporter 常駐より、既存計装でのクエリ・proxy を優先する。
