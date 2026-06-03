# Knowledge Loop — Context Glossary

Alt の primary surface である **Knowledge Loop** 境界の用語集。実装詳細は持たず、概念の正準語を固定する。
本コンテキストは knowledge-sovereign / alt-backend / alt-frontend-sv にまたがる。

> この monorepo には他にも境界 (Acolyte / Recap / Ingestion 等) があるが、本ファイルは
> Knowledge Loop の語彙のみを扱う。他境界を grill する際は CONTEXT-MAP.md へ昇格する。

## Language

### Core

**Knowledge Loop**:
取り込んだ記事を観察し、既存の理解に関係づけ、行動し、その意図を append-only event として蓄積して
次の浮上に反映する、閉じた認知フィードバック循環。`/loop` を primary surface とする。
_Avoid_: feed, timeline, recommendation list

**Relation-set**:
Knowledge Loop projection の **主軸**。1 エントリを「ユーザーの既存の orientation」へ結びつける
typed な関係 (Relation) の集合。bucket でも単一ラベルでもない。
_Avoid_: bucket (主軸の意味では), category, surface label

**Relation**:
エントリから既存 orientation への 1 本の typed edge。kind・対象・度合い・state・why を持つ。

**Orient surface**:
relation-set を常時描画する synthesis 面。「なぜこれが *今あなたに* 関係するか」を示す。
「進む stage」ではなく always-present な面。
_Avoid_: orient stage, orient step, orient tab

### Relation kinds

関係種は **「新規エントリを situ できるか」「自己参照か」** で峻別する。これを畳むと single-axis collapse になる。

**Continuation**:
ユーザーが既に着手したスレッド (開いた / 問うた / 再訪した) を当該エントリが延長する関係。
**自己参照** — 同一エントリへの過去の関与が要るため、新規エントリでは構造的に点かない。

**Cluster**:
当該エントリ (新規を含む) がユーザーの追っているトピック / タグ群に連なる関係。
**新規エントリを既存関心へ situ できる**数少ない種。

**Contradiction**:
当該エントリが、ユーザーが以前持っていた版・見解と食い違う関係。版差分・矛盾から生じ、
**戻り diff (矛盾→解消) が最も明快**。

**Inquiry**:
当該エントリがユーザーの問い / 会話に答える関係。自己参照寄り。

### Loop closure

**Relation state**:
Relation のライフサイクル位置 (open → advancing → advanced → resolved)。event log の純関数。

**Loop closure (return-diff)**:
act の結果として relation state が遷移し、次の observe でその差分が見えること。
**これが「ループ」を feed と分ける唯一の境界**。関係チップの数ではない。
_Avoid_: refresh, reload, live re-orientation animation

**Surface bucket**:
now / continue / changed / review の表層分類。relation-set から導かれる **1 レンズ**であり、
配置の権威ではない。
_Avoid_: bucket as primary axis, stage as gate

### Anti-terms (回避すべき状態)

**Single-axis collapse**:
直交する軸 (relation-set / proposed_stage / surface_bucket) や relation の複数種を、
1 軸・1 ラベルに畳んで捨てる仕様違反。

**Decorated feed**:
relation-set が空で、bucket + why 一文だけが並ぶ状態。Knowledge Loop の失敗形 (現状)。
