# Event Sourcing — General Patterns and Anti-patterns

kawasima 理論とは独立した角度での裏取り用（SKILL.md ワークフロー Step 4）。
一般論の解説ではなく、**Alt のレビューで実際に効く帰結だけ**を残してある。

> 出典: Pat Helland "Immutability Changes Everything" (CIDR 2015)
> <https://www.cidrdb.org/cidr2015/Papers/CIDR15_Paper16.pdf> /
> Oskar Dudycz "Property Sourcing" <https://event-driven.io/en/property-sourcing/> /
> Microsoft Learn "Event Sourcing pattern" /
> Greg Young の event sourcing 系資料

## Contents

- [1. 派生はすべて投影 (Pat Helland)](#1-派生はすべて投影-pat-helland)
- [2. Property Sourcing アンチパターン](#2-property-sourcing-アンチパターン)
- [3. Eventual consistency を隠さない](#3-eventual-consistency-を隠さない)
- [4. Replay の冪等性](#4-replay-の冪等性)
- [5. Read model は cheap, event log は expensive](#5-read-model-は-cheap-event-log-は-expensive)
- [6. Idempotency key と exactly-once illusion](#6-idempotency-key-と-exactly-once-illusion)
- [レビューでの 1 行チェック](#レビューでの-1-行チェック)

## 1. 派生はすべて投影 (Pat Helland)

Source of truth は事実 (event)、それ以外（materialized view / 非正規化 / index）はすべて
「不変データの最適化された投影」= キャッシュ。過去は変更できず、補正は新 event の追記で表現する。

Alt に効く帰結:

- read model に「絶対に消さないでね」という暗黙の依存があれば、それは projection の
  source-of-truth 化。違反。
- read 最適化のための非正規化はいくらしてもよいが、必ず「event log から再構築できる」前提で。

## 2. Property Sourcing アンチパターン

`UserNameChanged` / `UserEmailChanged` / `UserAddressChanged` のように、**property の変更そのものを
event 名にする**スタイル。CRUD の Update を細かく書いただけで業務的な意図を表しておらず、
「なぜ変わったか」を監査 / 分析時に復元できない。1 つの意図（引越し）が `AddressChanged` +
`PostalCodeChanged` + `CityChanged` の 3 本に分裂もする。

代わりに業務述語で名付ける: `UserMoved`, `UserChangedJob`。property の差分は payload で表現する。

- [ ] event 名が `Xxx<Property>Changed` の連発になっていないか
- [ ] 1 ユーザ意図が複数 event に分裂していないか（Single emission 違反でもある）

## 3. Eventual consistency を隠さない

event store と projection の間には常に eventual consistency がある。これを隠す試み
（write path から projection を直接更新して即時整合に見せる / 未到達 event 用の placeholder を
projection に持たせる / projection から event store に書き戻す）は、必ず別の不変条件を破る。

Alt の正しい扱い:

- `projection_seq_hiwater` を露出して「どこまで反映済みか」を読み手に渡す
- UI 側は `projection_revision` を optimistic lock 鍵として使う
- write path が即時 read を必要とするなら、別 read API を作るのではなく、対応する event を
  待ってから読む / stream で更新を受ける

## 4. Replay の冪等性

event sourcing が成立する核。同じ event 列を任意順序で食わせても projection が一致し、
同じ event を 2 回処理しても結果が変わらないこと。

よく壊す例: projector が wall clock を使う（順序非依存だが時刻依存で再現不能）/
projector が `Get<Latest>` 系で外部 state を読む / swap 時に checkpoint がリセットされず
gap が発生する（PM-2026-010）。

```go
// テストパターン (pseudo-code)
events := loadAllEventsForArticle(articleID)
shuffle(events); projection1 := replay(events)
shuffle(events); projection2 := replay(events)
assertEqual(projection1, projection2)
```

## 5. Read model は cheap, event log は expensive

1 つの event log に対し用途別の projection をいくつ作ってもよく、各 projection は disposable。
新しい read 要求には、既存 projection を mutate せず新 projection を作って reproject する。

Alt の実例（互いに正本を主張しない独立投影）: `knowledge_home_items` (Home foreground) /
`today_digest_view` (TodayBar) / `recall_candidate_view` (Recall) / `knowledge_loop_*` (Loop UI)。

## 6. Idempotency key と exactly-once illusion

network 上で exactly-once は不可能。producer 側に idempotency key (UUIDv7 等) を持たせ、
consumer 側で `(producer_id, idempotency_key)` dedupe。短期は TTL 付き dedupe table で速く、
長期は event store の unique index で slow path。Alt の `client_transition_id` がこれに当たる。

## レビューでの 1 行チェック

- [ ] event 名は業務述語か (Property Sourcing 違反でない)
- [ ] read model は disposable か (rebuild 手順があるか)
- [ ] write path は read model を mutate していないか
- [ ] projector は replay 冪等か (wall clock / `Get<Latest>` を使っていない)
- [ ] eventual consistency を隠そうとして別 invariant を壊していないか
- [ ] idempotency key が producer 側に存在し、consumer で dedupe されているか

[← back to SKILL.md](../SKILL.md)
