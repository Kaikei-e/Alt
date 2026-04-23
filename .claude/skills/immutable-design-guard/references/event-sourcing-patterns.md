# Event Sourcing — General Patterns and Anti-patterns

kawasima 理論の補強として、event sourcing / CQRS 一般理論からのアンチパターン
集。レビュー時に「kawasima 観点」とは独立した別角度の裏取りに使う。

> 主たる出典:
> - Pat Helland, "Immutability Changes Everything", CIDR 2015
>   <https://www.cidrdb.org/cidr2015/Papers/CIDR15_Paper16.pdf>
> - Greg Young の event sourcing 系資料
> - Oskar Dudycz, "Anti-patterns in event modelling — Property Sourcing"
>   <https://event-driven.io/en/property-sourcing/>
> - Microsoft Learn, "Event Sourcing pattern"
>   <https://learn.microsoft.com/azure/architecture/patterns/event-sourcing>

## 1. Immutability Changes Everything (Pat Helland)

### Key claims

- **不変データはレプリケーションを単純化する**: 値が一意なので、stale 読み
  という概念がない。
- **派生はすべて投影**: DataSet, materialized view, denormalization, index,
  column store はすべて「不変データの最適化された投影」。
- **Source of truth は事実 (event)、それ以外はキャッシュ**: 投影は
  rebuild 可能でなければならない。
- **過去は変更できない**: 補正は新しい event の追記で表現する。

### Alt のレビューに効く帰結

- read model に「絶対に消さないでね」と書いてある暗黙の依存があれば、
  それは projection の source-of-truth 化。違反。
- 「DB スナップショットから restore できない」状態の event は
  source of truth として弱い。
- read 最適化のための非正規化はいくらでもしてよいが、
  必ず「event log から再構築できる」前提で。

## 2. Property Sourcing アンチパターン (Oskar Dudycz)

### 何が起きるか

「`UserNameChanged`」「`UserEmailChanged`」「`UserAddressChanged`」のように、
**property の変更そのものを event 名にしてしまう** スタイル。

### なぜ悪いか

- event が業務的な意図を表していない (CRUD の Update を細かく書いただけ)
- ビジネスルールが「どの property を変えたか」のレベルでしか追えない
- 監査 / 分析時に「なぜ変わったか」を復元できない
- 同じ意図の更新が複数 event に分裂 (例: 引越しは
  `AddressChanged` + `PostalCodeChanged` + `CityChanged` の 3 本に)

### 代わりに

- 業務イベントとして名付ける: `UserMoved`, `UserChangedJob`,
  `UserMarriedAndRenamed`
- event は「何が起きたか」を表す述語
- property の差分は payload で表現

### Alt のチェック

- [ ] event 名が `Xxx<Property>Changed` 連発になっていないか
- [ ] 1 ユーザ意図が複数 event に分裂していないか (Single emission 違反でも
      ある)

## 3. Materialized view / Live projection は read 最適化

### 公式 (Microsoft Learn / Kurrent)

> Materialized views are read-only projections of the event store that are
> optimized for querying.

read model は **読みやすさのための optimization** であり、
書き込み先ではない。

### Alt のチェック

- [ ] write path が read model を直接 update していないか
- [ ] projection が他サービスから読まれているとき、それは event 経由で
      正規化されているか
- [ ] 「read model を rebuild する手順」が存在するか (runbook に書かれているか)

## 4. Eventual consistency between event store and projection

event store と projection の間には常に eventual consistency が存在する。
これを隠そうとすると別の不変条件を破る。

### よくある hide
- write path から projection を直接更新して「即時整合性」に見せる
- projection 側に未到達 event 用の placeholder を持たせる
- projection から event store に書き戻す

### Alt の正しい扱い
- `projection_seq_hiwater` を露出して「どこまで反映済みか」を読み手に渡す
- UI 側で `projection_revision` を optimistic lock 鍵として使う
- write path が即時 read を必要とするなら別 read API を作るのではなく、
  対応する event を待ってから読む / stream で更新を受ける

## 5. Replay の冪等性

event sourcing が成立する核は **replay 冪等性**。

### 守るべき性質

- 同じ event 列を任意順序で食わせても projection が一致する
- ある event を 2 回処理しても結果が変わらない (idempotent projection)
- event store の順序が変わっても、business meaning が壊れない

### よく壊す例

- projector が `time.Now()` を使っている → 順序非依存だが時刻依存で再現不能
- projector が `Get<Latest>` 系で外部 state を読む → 別 projection が
  進んでいるかで結果が変わる
- swap 時に checkpoint がリセットされない → gap が発生 (PM-2026-010)

### テストパターン

```go
// pseudo-code
events := loadAllEventsForArticle(articleID)
shuffle(events)                      // 順序を任意に並べ替え
projection1 := replay(events)
shuffle(events)
projection2 := replay(events)
assertEqual(projection1, projection2)
```

## 6. CQRS 文脈での "Read model は cheap, Event log は expensive"

> 「Read model はいくつ作ってもよい。だが event log は 1 つ」

### 正しい運用

- 1 つの event log に対し、用途別に複数 projection を持ってよい
- 各 projection は disposable
- 新しい read 要求が来たら、既存 projection を mutate するのではなく
  新 projection を作って reproject

### Alt の例

- `knowledge_home_items` (Knowledge Home の foreground 用)
- `today_digest_view` (TodayBar 用)
- `recall_candidate_view` (Recall 用)
- `knowledge_loop_*` (Loop UI 用)

これらは独立した投影で、互いに正本を主張しない。

## 7. Idempotency key と "exactly-once illusion"

network 上で exactly-once は不可能。代わりに:

- producer 側に idempotency key (UUIDv7 等) を持たせる
- consumer 側で `(producer_id, idempotency_key)` で dedupe
- TTL を設けて短期は速い dedupe、長期は event store unique index で slow path

これは Alt の `client_transition_id` パターンに対応する。

## レビューでの 1 行チェック

- [ ] event 名は業務述語か (Property Sourcing 違反でない)
- [ ] read model は disposable か (rebuild 手順があるか)
- [ ] write path は read model を mutate していないか
- [ ] projector は replay 冪等か (`time.Now()`, `Get<Latest>` を使っていない)
- [ ] eventual consistency を隠そうとして別 invariant を壊していないか
- [ ] idempotency key が producer 側に存在し、consumer で dedupe されているか

[← back to SKILL.md](../SKILL.md)
