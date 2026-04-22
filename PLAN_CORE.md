以下、**Knowledge Loop State Machine Definition v1** としてそのまま使える形で書きます。
ADR 本文ではなく、**ADR の前提となる状態機械定義書**です。これを先に固定し、その後に schema / proto / UI を派生させる前提です。現行 Alt の Knowledge Sovereignty は append-first、versioned artifacts、why-first、disposable projections を中核原則としており、既存投影には `updated_at` が残っているため、本定義ではその曖昧さを解消する方向で整理します。

また、Knowledge Loop UI は 3D 的な奥行き感を許可しますが、Apple の spatial design 原則に従い、**重要コンテンツは中心視野に置き、遷移は smooth and predictable で、テキストは flat のまま保つ**ことを前提とします。Reduced Motion 有効時は、depth simulation や parallax を無効化または代替遷移に置き換える必要があります。 ([Apple Developer][1])

---

# Knowledge Loop State Machine Definition v1

* Status: Draft
* Date: 2026-04-23
* Scope: Knowledge Loop UI / Command Surface / Session State / Projection Contract
* Non-scope: DB migration SQL, proto syntax finalization, concrete Svelte implementation
* Related:

  * ADR-000398
  * ADR-000424
  * ADR-000430
  * ADR-000434
  * ADR-000446
  * ADR-000463
  * ADR-000475
  * ADR-000749

## 0. Purpose

Knowledge Loop は、Knowledge Home を単なる一覧画面ではなく、**知識理解を循環させる UI 状態機械**として再定義するための中核モデルである。

本定義書の目的は次の4つである。

1. UI が使う語彙を固定する
2. event / projection / session の責務境界を固定する
3. `updated_at` 依存を排除し、seq / revision / freshness / artifact ref に分解する
4. Observe → Orient → Decide → Act の遷移を、見た目ではなく契約として定義する

Knowledge Loop は CQRS + Event Sourcing と整合する必要がある。イベントストアを write-side の single source of truth とし、read-side は materialized view / projection として再構築可能でなければならない。CQRS と Event Sourcing を組み合わせる場合、event store を真実源泉にし、read model はそれから materialized view を構築するのが基本であり、read 側は eventual consistency を持つため stale state や再描画契約を明示的に扱う必要がある。 ([Microsoft Learn][2])

## 1. Core model

Knowledge Loop は 3 つの独立した軸を持つ。
この 3 軸は混同してはならない。

### 1.1 Proposed Stage

各 entry が、ユーザーにとって**どの局面に最も適した提案か**を表す。
projection が決める。

```text
observe | orient | decide | act
```

### 1.2 Current Session Stage

ユーザーが、現在のセッションで**実際にどの局面にいるか**を表す。
session state が持つ。

```text
observe | orient | decide | act
```

### 1.3 Surface Bucket

画面のどこに entry を置くかを表す。
これは UI の情報設計上の区分であり、stage とは別概念である。

```text
now | continue | changed | review
```

この分離は必須である。`loop_stage` ひとつにこれらを押し込むと、entry 適性、ユーザー現在地、画面配置が混線し、状態機械として破綻する、というレビュー指摘は正しい。

## 2. User-facing interpretation

上の3軸は内部契約であり、ユーザーには次のように見せる。

* **Now**: いま最優先で観測または判断すべきもの
* **Continue**: すでに文脈に入っており、継続が必要なもの
* **Changed**: 前回認識以後に意味のある差分が発生したもの
* **Review**: 緊急ではないが、短時間で理解を前進させるもの

ただし、Now = Observe、Continue = Orient のような 1 対 1 対応ではない。
たとえば Now に属する entry が Proposed Stage = `decide` を持つことは自然であるし、Changed に属する entry が Proposed Stage = `observe` または `orient` を持つことも自然である。

## 3. Canonical invariants

Knowledge Loop は以下の不変条件を満たす。

### 3.1 Event-source invariant

Knowledge Loop の真実源泉は event store である。
session state も entry projection も source of truth ではない。再投影で再構築可能でなければならない。これは現行の Knowledge Sovereignty 原則とも一致する。  ([Microsoft Learn][2])

### 3.2 No `updated_at` invariant

Knowledge Loop projection に `updated_at` を置かない。
代わりに以下を分離して持つ。

* `projection_seq_hiwater`
* `projection_revision`
* `source_event_seq`
* `freshness_at`
* `source_observed_at`
* `artifact_version_ref`

### 3.3 Flat text invariant

奥行き表現は tile / container / transition にのみ適用し、テキストは interface element として flat に保つ。Apple は 3D テキストが読みづらく distractive になりうるため、interface element として使う場合は text を flat に保つよう示している。 ([Apple Developer][1])

### 3.4 Predictable transition invariant

Observe → Orient → Decide → Act の遷移方向、速度、意味付けは常に一貫していなければならない。Apple は immersive / spatial 体験の状態遷移を smooth and predictable にし、continuity を持たせることを求めている。 ([Apple Developer][1])

### 3.5 Reduced motion invariant

Reduced Motion 有効時、depth simulation、parallax、animated blur、depth-of-field 的表現、scaling 中心の遷移は停止または置換する。Apple はこれらを motion trigger として扱い、必要に応じて変更または代替アニメーションを求めている。 ([Apple Developer][3])

## 4. State spaces

## 4.1 Entry state

`KnowledgeLoopEntry` は「UI が提案する一単位」である。
これは source item と 1 対 1 とは限らない。ひとつの article から複数 entry が派生してもよい。

```ts
type LoopStage = "observe" | "orient" | "decide" | "act";
type SurfaceBucket = "now" | "continue" | "changed" | "review";
type DismissState = "active" | "deferred" | "dismissed" | "completed";

type WhyPayload = {
  kind: "source_why" | "pattern_why" | "recall_why" | "change_why";
  text: string;
  confidence?: number;
  evidence_ref_ids: string[];
};

type KnowledgeLoopEntry = {
  userId: string;
  lensModeId: string;
  entryKey: string;
  sourceItemKey: string;

  proposedStage: LoopStage;
  surfaceBucket: SurfaceBucket;

  projectionRevision: number;      // row-local monotonic counter
  projectionSeqHiwater: number;    // max reflected event seq
  sourceEventSeq: number;          // representative source event
  freshnessAt: string;             // reflected-world freshness
  sourceObservedAt?: string;       // article published/observed time
  artifactVersionRef: unknown;     // summary/tag/lens refs

  whyPrimary: WhyPayload;
  whyEvidenceRefs: string[];

  changeSummary?: unknown;
  continueContext?: unknown;
  decisionOptions?: unknown[];
  actTargets?: unknown[];

  supersededByEntryKey?: string;
  dismissState: DismissState;

  renderDepthHint: 0 | 1 | 2 | 3;
  loopPriorityLabel: "最重要" | "継続中" | "確認推奨" | "参照のみ";
};
```

`projectionRevision` は**行単位**の monotonic counter とする。
レビューにある通り、テーブル単位ではなく row-local でないと stream update の楽観ロック基準として使えない。

## 4.2 Session state

ユーザーの現在地は別 projection に持つ。

```ts
type KnowledgeLoopSessionState = {
  userId: string;
  lensModeId: string;

  currentStage: LoopStage;
  currentStageEnteredAt: string;

  focusedEntryKey?: string;
  foregroundEntryKey?: string;

  lastObservedEntryKey?: string;
  lastOrientedEntryKey?: string;
  lastDecidedEntryKey?: string;
  lastActedEntryKey?: string;

  projectionRevision: number;
  projectionSeqHiwater: number;
};
```

これにより「entry は Observe 向きだが、ユーザーは今 Orient 中」という状態を表現できる。
これが状態機械としての最低条件である。

## 4.3 Surface state

画面全体は 3 層の表示面を持つ。

```text
foreground plane
mid-context plane
deep-focus plane
```

* foreground plane: いま処理すべき 1〜3 件
* mid-context plane: 近接文脈、差分、継続理由、関連手掛かり
* deep-focus plane: article, ask, recap, diff, related cluster の深掘り

NN/g の progressive disclosure の原則に従い、初期画面では重要な少数の選択肢だけを見せ、特殊・詳細な選択肢は要求時にだけ開示する。さらに視覚的優先度は color/contrast, scale, grouping で作るべきであり、すべてを均等に並べてはならない。 ([Nielsen Norman Group][4])

## 5. Stage semantics

## 5.1 Observe

目的: 新規性、差分、注意喚起、要確認情報を認識する。

典型 entry:

* 新着で why が強い
* 重要 summary 完了
* weekly recap available
* need_to_know が高い
* supersede 発生直後

典型 UI:

* 短い title
* whyPrimary
* evidence jump
* 単一 primary CTA

## 5.2 Orient

目的: 新情報を既存理解・興味・過去行動に接続する。

典型 entry:

* 以前見た item の続き
* changed item の before/after 要約
* recall candidate の文脈
* recent interest match

典型 UI:

* changeSummary
* continueContext
* related items
* why の由来種別

## 5.3 Decide

目的: 次に取るべき行動を選ぶ。

典型 entry:

* open
* ask
* save
* compare
* revisit
* snooze

典型 UI:

* decisionOptions
* 2〜4 個の短い CTA
* 代替案

## 5.4 Act

目的: 実際の行動を遂行し、Loop を前進させる。

典型 entry:

* article open
* Ask 実行
* recap open
* diff confirm
* defer / dismiss

Act は終端ではない。Act の後、結果に応じて Observe または Continue に再流入する。

## 6. Surface bucket assignment rules

`surfaceBucket` は scoring で決めるが、ルールの骨格は固定する。

### 6.1 Now

条件の例:

* urgency が高い
* novelty または change importance が高い
* blocked user flow の解除に寄与
* primary CTA が明快

Now の foreground には最大 1 件、secondary を含めても 3 件まで。

### 6.2 Continue

条件の例:

* 既に entry に接触済み
* currentSessionStage と親和性が高い
* 継続価値 > 新規価値

### 6.3 Changed

条件の例:

* supersede chain あり
* why 差分あり
* ranking 再浮上
* summary/tag version change

### 6.4 Review

条件の例:

* recall
* recap
* low urgency / high utility
* 学習・再定着価値あり

## 7. Transition model

Knowledge Loop は次の遷移を許可する。

```text
observe -> orient
observe -> decide
orient  -> decide
decide  -> act
act     -> observe
act     -> continue
orient  -> review
review  -> orient
changed -> observe   // bucket conceptually routes into stage
```

ただしこれは UI bucket 遷移ではなく、**session stage 遷移**である。

### 7.1 Forbidden transitions

以下は直接遷移禁止とする。

* `observe -> act`
* `review -> act`
* `act -> act`
* `decide -> observe` without explicit cancel/return event

理由は、UI の意味が飛びすぎて continuity が崩れるため。

## 8. Events

## 8.1 Canonical transition events

```text
KnowledgeLoopObserved
KnowledgeLoopOriented
KnowledgeLoopDecisionPresented
KnowledgeLoopActed
KnowledgeLoopReturned
KnowledgeLoopDeferred
KnowledgeLoopSessionReset
KnowledgeLoopLensModeSwitched
```

## 8.2 Firing conditions

レビューの指摘どおり、ここを曖昧にすると event storm になる。
よって v1 では以下に固定する。

### `KnowledgeLoopObserved`

発火条件:

* tile が viewport 中央 50% に 1.5 秒以上留まる
* 同一 entry について同一 session で 60 秒以内に再発火しない
* scroll 通過だけでは発火しない

### `KnowledgeLoopOriented`

発火条件:

* expand / hover-peek / tap-preview / keyboard focus により文脈展開が起こった
* 単なる viewport 進入では発火しない

### `KnowledgeLoopDecisionPresented`

発火条件:

* decisionOptions が UI に実際に描画された
* projection に options が存在するだけでは発火しない

### `KnowledgeLoopActed`

発火条件:

* open / ask / save / archive / confirm diff / dismiss のいずれか完了

### `KnowledgeLoopReturned`

発火条件:

* deep-focus plane から mid-context または foreground へ戻った
* browser back と explicit close の両方を含む

### `KnowledgeLoopDeferred`

発火条件:

* later / snooze / skip / passive dismiss が行われた

### `KnowledgeLoopSessionReset`

発火条件:

* lens mode 切替
* user context hard reset
* projection incompatibility

### `KnowledgeLoopLensModeSwitched`

発火条件:

* lens mode が変わり、新 contiguous loop segment を開始した

レビューの提案どおり、**lens mode 切替は loop session の新しい contiguous 区間**として扱う。

## 8.3 Idempotency

`TransitionKnowledgeLoop` 系イベントは client-generated idempotency key を必須化する。

```ts
type TransitionRequest = {
  clientTransitionId: string;   // UUIDv7
  entryKey: string;
  fromStage: LoopStage;
  toStage: LoopStage;
  trigger: "user_tap" | "dwell" | "keyboard" | "programmatic";
  observedProjectionRevision: number;
};
```

server は `(userId, clientTransitionId)` を dedupe し、二重送信で同一イベントを重複 append しない。

これは CQRS/Event Sourcing における messaging duplicates / retries 対応と整合する。 Microsoft も CQRS では duplicate や retry を考慮すべきとしている。 ([Microsoft Learn][2])

## 9. Stream semantics

`StreamKnowledgeLoopUpdates` は次の update kind を持つ。

```ts
type StreamUpdate =
  | { kind: "EntryAppended"; entryKey: string; revision: number }
  | { kind: "EntryRevised"; entryKey: string; revision: number }
  | { kind: "EntrySuperseded"; entryKey: string; newEntryKey: string; revision: number }
  | { kind: "EntryWithdrawn"; entryKey: string; revision: number }
  | { kind: "SurfaceRebalanced"; surfaceBucket: SurfaceBucket; revision: number };
```

## 9.1 Client lock contract

client は foreground entry の `(entryKey, projectionRevision)` を保持する。

## 9.2 Server behavior

* `EntryRevised`: silent update。前景を奪わない
* `EntrySuperseded`: badge or inline notice。自動遷移しない
* `EntryWithdrawn`: foreground の場合は「変更されました。確認しますか？」を表示し、即差し替えしない
* `SurfaceRebalanced`: background buckets のみ即反映可

この設計は、eventual consistency 環境で stale read によるユーザーの行動を壊さないために必要である。 CQRS の分離 read model では stale data への user action をどう扱うかを明示的に設計すべきと Microsoft は述べている。 ([Microsoft Learn][2])

## 10. Projection fields and meaning

### 10.1 `projectionSeqHiwater`

その row が反映済みの最大 event seq。
ordering と completeness の基準。

### 10.2 `projectionRevision`

その row の局所版。
楽観ロックと stream discard の基準。

### 10.3 `sourceEventSeq`

entry を代表する主要イベント seq。
why / diff / traceability 用。

### 10.4 `freshnessAt`

その row が反映している「世界」の鮮度。
UI 表示用。

### 10.5 `sourceObservedAt`

article / source 情報の原始観測時刻。
published_at 等。

### 10.6 `projectedAt`

内部運用用。UI 非露出。
`updated_at` の代替ではなく、debugging only。

## 11. Why contract

`whyPrimary` は自由 JSON にしない。最低でも次の構造を持つ。

```ts
type WhyPayload = {
  kind: "source_why" | "pattern_why" | "recall_why" | "change_why";
  text: string;
  confidence?: number;
  evidence_ref_ids: string[];
};
```

理由は recommendation copy ではなく、**source-backed explanation** である。
現行 Knowledge Sovereignty でも Why as First-Class が重視されており、理由は装飾ではなくドメイン概念として扱われている。

## 12. Spatial render contract

### 12.1 Rule

**Depth lives on tiles, not on glyphs.**

tile が奥行きを持つ。
glyph は持たない。

### 12.2 Tile depth

projection は具体 px ではなく `renderDepthHint: 0|1|2|3` だけ返す。
view 層がこれを Z offset / shadow / saturation / brightness に写像する。

### 12.3 Centering

最重要 content は center field に置く。
Apple は center が最も見やすく、主要 content はそこに置くべきと述べている。 ([Apple Developer][1])

### 12.4 Predictability

Observe→Orient→Decide→Act の遷移方向は固定。
たとえば:

* deeper focus: 奥へ入る
* return: 手前に戻る
* changed diff: 横スライドではなく層分離で見せる

### 12.5 Reduced Motion mapping

Reduced Motion 時:

* `renderDepthHint` は視覚強度にのみ使う
* Z movement を停止
* parallax / animated blur / depth-of-field を停止
* dissolve / highlight fade / color shift に置換

Apple は depth simulation や parallax を Reduced Motion 時に変更または無効化すべきとし、意味を伝える遷移は dissolve や highlight fade に置換することを推奨している。 ([Apple Developer][3])

## 13. Accessibility contract

奥行きは視覚障害者には伝わらない。
したがって、各 entry は `loopPriorityLabel` を持ち、`aria-description` として読み上げ可能でなければならない。
例:

* 最重要
* 継続中
* 変更確認推奨
* 参照のみ

これはレビューの指摘をそのまま採る。

## 14. Empty state contract

空の Now は異常ではなく、**正常な終端状態**である。
空状態では無理に新規 item を押し込まず、次のどれかを出す。

* 「今日はここまでです」
* 「Continue を確認」
* 「Review で短く振り返る」
* 「新しい観測を待っています」

これは visual hierarchy を守るためにも必要。何もない時に埋め草を入れると、foreground の意味が崩れる。 ([Nielsen Norman Group][5])

## 15. Migration rules

Phase A では `updated_at` 依存の既存消費者を棚卸しする。
最低限必要なのは次の3つ。

1. `updated_at` 参照箇所 inventory
2. `updated_at -> freshnessAt / sourceObservedAt` の読み替えマップ
3. 互換 endpoint の一時並走

これはレビューの指摘を採る。ここを飛ばすと、旧クライアントが無音で壊れる。

## 16. Acceptance criteria

1. `proposedStage`, `currentSessionStage`, `surfaceBucket` が分離されている
2. `updated_at` が Knowledge Loop projection に存在しない
3. row-local `projectionRevision` がある
4. `renderDepthHint` は projection にあるが、具体 px は UI にのみある
5. `whyPrimary.kind/text/evidence_ref_ids` が proto 契約として固定される
6. `KnowledgeLoopObserved` は dwell threshold 付きである
7. `TransitionKnowledgeLoop` は idempotency key を必須とする
8. `StreamKnowledgeLoopUpdates` は silent update と forced replace を区別する
9. Reduced Motion で depth simulation が無効化される
10. 空の Now を正常状態として扱う

---

この v1 の核心は 2 つです。

ひとつは、**Knowledge Loop を見た目ではなく状態機械として定義したこと**。
もうひとつは、**`updated_at` を捨てて、seq / revision / freshness / ref に分解したこと**です。これは現行の append-first / reproject-safe / versioned artifact / disposable projection 原則と整合します。

次はこの順番で進めるのが妥当です。

**1. この定義書をベースに状態遷移図を1枚起こす**
**2. その後に schema 差分**
**3. その後に proto / RPC**
**4. 最後に SvelteKit の UI 構成**

必要なら次に、そのまま実装に入れるように
**PostgreSQL schema draft** と **proto draft** まで続けて書きます。

[1]: https://developer.apple.com/videos/play/wwdc2023/10072/ "Principles of spatial design - WWDC23 - Videos - Apple Developer"
[2]: https://learn.microsoft.com/en-us/azure/architecture/patterns/cqrs "CQRS Pattern - Azure Architecture Center | Microsoft Learn"
[3]: https://developer.apple.com/help/app-store-connect/manage-app-accessibility/reduced-motion-evaluation-criteria/ "Reduced Motion evaluation criteria - Manage App Accessibility - App Store Connect - Help - Apple Developer"
[4]: https://www.nngroup.com/articles/progressive-disclosure/ "Progressive Disclosure - NN/G"
[5]: https://www.nngroup.com/articles/visual-hierarchy-ux-definition/ "Visual Hierarchy in UX: Definition - NN/G"
