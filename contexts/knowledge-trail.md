# Knowledge Trail — Context Glossary

Alt の primary surface である **Knowledge Trail** 境界の用語集。実装詳細は持たず、概念の正準語を固定する。
本コンテキストは knowledge-sovereign / alt-backend / alt-frontend-sv にまたがる。

> 本境界は旧 **Knowledge Loop** 境界を置換する (2026-06-10)。Loop の概念層 (3軸直交モデル・
> surface bucket・stage) は retire。イベント基盤・relation 語彙・evidence 供給機構は継承する。
> 2026-06-17: 単一 `CONTEXT.md` を `CONTEXT-MAP.md` へ昇格し、本 glossary を `contexts/knowledge-trail.md`
> へ移設。他境界 (Acolyte / Recap / Ingestion / Resource Efficiency 等) は map に context を追加して定義する。

## Language

### Core

**Knowledge Trail**:
ユーザの認知の軌跡 (footprint の連なり) を背骨とし、システムが型付き分岐 (branch) を提案する
閉じた認知フィードバック循環。`/knowledge/trail` に住む**調査の再開点 (resume surface)** であり、
低頻度・高価値の rediscovery を仕事とする。毎日の入口は Knowledge Home が担い、Trail は
「中断した調査に戻る・過去に読んだものを再発見する」瞬間に開かれる (2026-07-18 grill で
primary surface の位置づけを改訂)。成功の尺度は訪問頻度ではなく再開・再発見の成功。
_Avoid_: feed, timeline, recommendation list, loop (旧世代の意味では), primary surface (改訂済み),
daily destination

**Footprint (足跡)**:
ユーザの 1 回の認知行為 (読んだ・問うた・戻った・聴いた・捨てた) の、event log からの純粋な射影。
trail spine を構成する唯一の要素。システムは footprint を生成できない。
_Avoid_: history item, activity entry, log row

**Trail spine (背骨)**:
全 footprint の時系列一本の構造。Knowledge Trail が持つ**唯一の構造**であり、
テーマ・進行度などの追加軸を実体として持たない。
_Avoid_: bucket, lane, stage column, second spine

**Episode (調査の流れ)**:
時間的近接 × トピック的近接で footprint を畳んだ「一続きの調査」。spine の**デフォルト表示単位**
(2026-07-18 grill)。path wear と同格の**純導出値**であり、event log から再生可能・ユーザの
キュレーション操作を要求しない。episode は spine の見え方であって第二の構造ではない —
展開すれば個々の footprint に戻る。同一記事の再読は episode 内で count として畳まれ、
行を増やさない。日付は grouping の第一軸ではなく timeline 上の目盛り (landmark) に降格する。
帰属規則: **同一記事への全接触 (日をまたぐ再読・ask・listen) は無条件に同一 episode**。
記事間の連なりは浄化済みタグの重なり (時間減衰付き) で判定する。
_Avoid_: session (実体としての), folder, named trail (手動キュレーション), day bucket (第一軸としての)

**Branch (分岐)**:
trail 上の地点にシステムが提案する「次の一歩」。relation kind (型)・why・evidence refs・
calibrated confidence を**必須で**持つ。いずれかが欠けた branch は surface されない。
surface する場所は **patch 離脱点** — 第一に記事読了点 (「いま読み終えた記事」がアンカー)、
第二に Trail の episode ヘッダ (「この調査の次の一歩」)。Trail ページ上部への
無文脈な一覧表示はしない (2026-07-18 grill)。
_Avoid_: recommendation, suggestion card, untyped hint, branch inbox (無文脈一覧)

**Anchored why**:
branch の why 文が満たすべき規律 — ユーザが実際に読んだ / 問うた**具体アンカー**
(記事タイトル・問いの文言) への明示参照を必ず含む。アンカーを埋められない branch は
surface しない (2026-07-18 grill)。scent はユーザ自身の足跡への参照からしか生まれない。
_Avoid_: generic why (「あなたの興味に基づき」型 — placebo 信頼を生むだけのノイズ)

**Branch resolution**:
branch に対するユーザの応答。`taken` (踏んだ → footprint 化) | `dismissed` (捨てた)。
dismiss には scrutability (「このテーマは追っていない」等の理由 1 タップ) を添えられる。
branch の効果測定は CTR ではなく **踏んだ先の engaged dwell 率** (trail.act_outcome.v1) で行う。

**Path wear (踏み固め)**:
同一テーマへの再訪・深い滞在・問いの累積を「道の濃さ・太さ」に翻訳した純粋な導出値。
event log の純関数であり、数値・レベル・ラベルとしては表出しない。
_Avoid_: depth score, level, progress bar, stage

**Trail search**:
spine 全体に対する狙い撃ちの rediscovery 手段 (タイトル・拝読本文・タグ横断)。結果は
リストではなく **spine 上の位置にアンカーして**返す (時間文脈ごと再発見させる)。
テーマの塊は episode が既に見せているため、Trail のフィルタ UI はこれ一本 (2026-07-18 grill)。
_Avoid_: tag chip bar, facet list, theme lens (retired)

### Relation kinds (Loop から継承)

関係種は **「新規エントリを situ できるか」「自己参照か」** で峻別する。これを畳むと single-axis collapse になる。
Trail では branch の型として用い、UI には平易な英語 (例: "Continues your thread") で表出する。

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

### Evidence supply (Loop から継承)

**Evidence accumulator**:
イベントを 1 件ずつ畳み込んで保持する、関係導出のための累積 evidence。branch の relations は
この累積の純関数として導出される。イベント履歴から再生可能 (disposable)。
_Avoid_: window re-scan, evidence query, resolver (旧方式の意味では)

**Co-projection**:
補助的な導出状態 (evidence accumulator) を主 projection と同一のイベント順序で更新し、
導出結果が「その時点までのイベント履歴の決定的関数」であることを保つパターン。
「最新状態を覗かない」不変条件の正当な例外はこの形に限る。

**Late fuel (遅延燃料)**:
footprint が刻まれた後に到着する evidence (タグ付け・版差し替え・topic snapshot)。
到着した時点で関連 branch を即時再導出する。次回接触まで放置しない。

### Trail closure

**Trail closure (return-diff)**:
branch を踏んだ結果が、次に trail を見たとき footprint と path wear の**差分として見える**こと。
**これが trail を単なる履歴ログと分ける唯一の境界**。提案チップの数ではない。
_Avoid_: refresh, reload, live re-orientation animation

### Anti-terms (回避すべき状態)

**Single-axis collapse**:
直交する概念 (relation kinds の複数種・footprint と branch の区別) を 1 軸・1 ラベルに
畳んで捨てる仕様違反。

**Untyped branch**:
relation kind も evidence refs も持たず、why 一文だけで surface される branch。
Loop の失敗形 **decorated feed** の Trail における再演。構造的に禁止する。

**Second spine**:
時系列一本以外の構造 (テーマ実体・stage レーン・bucket) を spine と並ぶ実体として導入すること。
誰も保持できない複数軸モデルへの回帰。

**Stage exposure**:
OODA (Observe / Orient / Decide / Act) のステージ語彙を UI・ユーザ向け文言に露出させること。
OODA は提案エンジンの内部設計原理にのみ存在する。

**Push delivery**:
SSE / WebSocket / ポーリングによる自発的な画面更新。Trail は pull のみ
(ナビゲーション時 load + 明示 refresh)。PM-2026-039 / PM-2026-045 の直接の教訓。

**Window re-scan**:
footprint / branch を surface するたびに過去 window 全体の evidence を問い直す pull 型の取得方式。
ログ密度に比例して破綻し、切り捨て (truncation) による無音の evidence 全損を誘発した失敗形。

**Silent truncation**:
エラーを発生させずに evidence を取りこぼす劣化。fail-loud では捕捉できず、
密度を再現したテストと実測 coverage の計測でのみ検知できる。

## Retired terms (Knowledge Loop 期 — 使用禁止)

`proposed_stage` / `session_stage` / `surface_bucket` (now・continue・changed・review) /
`orient surface` / `loop entry` / `loop session`。
**`theme lens`** (2026-07-18 retire — 生タグ union のチップバーは「死んだタグクラウド」の再演だった。
テーマ絞りは episode が、狙い撃ちは trail search が担う。Knowledge Home の saved lens
(ADR-000409) は Home 境界の概念として存続し、Trail はもう再利用しない)。
これらが新規コード・文書に現れたら Loop 概念層の漏出であり、retire の不完全を意味する。
過去イベント (`knowledge_loop.*`) の payload 内に現れるのは歴史であり問題ない。
