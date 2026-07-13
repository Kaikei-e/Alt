# Inference Topology — Context Glossary

GPU 推論ワークロード (要約 / embedding / rerank / Acolyte / TTS) を **どのマシンで動かしてよいか**、
そしてリモートが不在のとき機能がどう振る舞うべきかの正準語を固定する境界。
どのモデルをどこに置くかの個別決定は持たない (それは ADR / wiki/decisions/inference 側)。

> 2026-07-13 grill セッションで定義。北極星: 「リモート GPU は強化であって前提ではない」。

## Language

### Core

**Primary Host (主機)**:
Alt の全 critical path が単体で成立しなければならないマシン (現在は koko-b)。
ingress・DB・auth・コアサービス・観測系は Primary Host の外に出ない。
_Avoid_: 本番マシン (曖昧)

**Enhancement Tier (強化層)**:
あれば品質・スループットを強化し、なくても機能が成立するリモート GPU ノード群。
Enhancement Tier への参加は常にオプションであり、前提であってはならない。
_Avoid_: リモートノード (役割が伝わらない), 分散推論 (常時分散を示唆する)

**Hard Remote Dependency (禁止 anti-term)**:
リモートマシン不在で機能が「不成立」になる構造。本境界が解消対象とする状態そのもの。
劣化契約が未定義のリモート依存は全てこれに該当する。
_Avoid_: 外部依存 (SaaS 等と混同する)

### 劣化契約 (Degradation Contract)

**Degradation Contract (劣化契約)**:
リモート不在時にその機能が何をすれば「成立」かを機能ごとに 1 つ割り当てる契約。
値は Degraded / Deferred / Feature-off の 3 値のみ。契約のない曖昧な中間状態は認めない。
_Avoid_: フォールバック (機構の話に矮小化される), graceful degradation (契約の 3 値を区別しない)

**Degraded**:
Primary Host 上のローカル代替で品質または速度を落として**即時に**成立する。
埋め込みは同一モデルのローカル実体への切替に限る (モデルを変えるとベクトル空間が壊れる)。

**Deferred**:
処理を保留し、リモート復帰後に再開して成立する。結果は遅れて届く。
保留中であることはユーザから可視でなければならない。
_Avoid_: リトライ (無期限保留と有限リトライを混同する)

**Feature-off**:
機能を「利用不可」という可視状態で明示的に停止する。他機能は無傷。
_Avoid_: 無効化 (設定による恒久 off と混同する)

**Loud degradation (可視劣化)**:
契約のどの状態にあるかは常に観測可能 (起動ログ・メトリクス・health) でなければならない。
無言 no-op・成功偽装はサイレントフォールバック (CLAUDE.md Rule 8) 違反として扱う。
_Avoid_: 静かなフォールバック
