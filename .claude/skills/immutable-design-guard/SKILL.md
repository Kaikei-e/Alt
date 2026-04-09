---
name: immutable-design-guard
description: |
  Knowledge Home のイミュータブルデータモデル不変条件をチェックする。
  knowledge_events, projector, reproject, summary_versions, tag_set_versions,
  knowledge_home_items, today_digest_view, recall_candidate_view に関わる変更で使う。
user-invocable: false
allowed-tools: Bash, Read, Glob, Grep, mcp__obsidian__view
---

# Immutable Design Guard

Knowledge Home の変更が、[kawasima のイミュータブルデータモデル](https://scrapbox.io/kawasima/%E3%82%A4%E3%83%9F%E3%83%A5%E3%83%BC%E3%82%BF%E3%83%96%E3%83%AB%E3%83%87%E3%83%BC%E3%82%BF%E3%83%A2%E3%83%87%E3%83%AB)の考え方と
Alt の canonical contract を壊していないか確認するためのガード。

見るべき中心原則:

- UPDATE を安易に増やさない
- 事実は event として残す
- projector は disposable projection を作るだけに寄せる
- projection は source of truth に昇格させない
- 再投影で同じ結果に戻れることを優先する

## 先に読む文書

必要な節だけ読む。

| 文書 | 目的 |
|---|---|
| `docs/plan/IMPL_BASE.md` | append-first / reproject-safe / projection 原則 |
| `docs/plan/knowledge-home-phase0-canonical-contract.md` | allowed why code と read contract |
| `docs/review/knowledge-home-phase1-5-remediation-directives-2026-03-18.md` | 既知の未達と是正方針 |
| 関連 ADR | 例外が ADR 化されているか確認 |

## チェックリスト

### 1. Append-first

- [ ] `knowledge_events` は INSERT only か
- [ ] 状態変化を event append で表現しているか
- [ ] `dedupe_key` などの冪等性が保たれているか

確認例:

```bash
grep -rn "UPDATE.*knowledge_events\\|DELETE.*knowledge_events" alt-backend/app/
```

### 2. Resource / Event の分離

kawasima 理論では、まず resource と event を分ける。  
日時を持つ業務アクティビティは event として表現する。

- [ ] 「更新日時が欲しい」という要求の背後に、抽出漏れの event がないか
- [ ] resource の変更を 1 本の mutable row に押し込めていないか
- [ ] hidden event を read model の `updated_at` でごまかしていないか

危険サイン:

- `status` と `updated_at` だけを追加して業務変化を表現しようとする
- `XxxUpdated` を generic に増やして意味の違う変更を 1 event に押し込む

### 3. Event は 1 つの業務時刻に寄せる

kawasima 理論では、event entity は原則として 1 つの日時属性に寄せる。  
Alt では event source と projection を区別して考える。

- [ ] source-of-truth の event payload が単一の業務事実を表しているか
- [ ] projector が `time.Now()` ではなく `event.OccurredAt` や payload 時刻で再生できるか
- [ ] 1 event に複数の意味の異なる時刻を持ち込み、更新ルールを増やしていないか

補足:

- projection の `generated_at` / `updated_at` は read model 運用上あってよい
- ただし business fact の再現にそれらを使ってはいけない

確認例:

```bash
grep -rn "time.Now()" alt-backend/app/job/knowledge_projector*.go
```

### 4. Reproject-safe

- [ ] projector が event payload と安定 resource から read model を再構築できるか
- [ ] projector 内で latest state や active projection を暗黙に読んでいないか
- [ ] replay 順序が変わっても最終結果が壊れないか

確認例:

```bash
grep -rn "GetLatest\\|FindCurrent\\|SELECT.*FROM.*knowledge_home_items" \
  alt-backend/app/job/knowledge_projector*.go \
  alt-backend/app/usecase/*knowledge*projector*.go
```

### 5. Projection is disposable

- [ ] `knowledge_home_items` / `today_digest_view` / `recall_candidate_view` を正本扱いしていないか
- [ ] write path が read model を直接 mutate していないか
- [ ] reproject / shadow version 中でも意味が壊れないか

危険サイン:

- tracking usecase から projection table を直接更新する
- `projection_version` を無視した update を足す
- projector lag を write path の副作用更新で隠す

### 6. Versioned artifacts / versioned projections

- [ ] summary/tag は version append で表現しているか
- [ ] projection の更新は `projection_version` を意識しているか
- [ ] active/shadow 両方を誤って一括更新していないか

確認例:

```bash
grep -rn "UPDATE.*summary_versions.*SET\\|UPDATE.*tag_set_versions.*SET" alt-backend/app/
grep -rn "projection_version" alt-backend/app/
```

### 7. Why as first-class

- [ ] Home item の `why_json` は許可コードだけを使っているか
- [ ] why の意味が summary/tag/supersede 状態と矛盾していないか
- [ ] why merge が既存理由を壊していないか

許可コード:

- `new_unread`
- `in_weekly_recap`
- `pulse_need_to_know`
- `tag_hotspot`
- `recent_interest_match`
- `related_to_recent_search`
- `summary_completed`

### 8. Merge-safe upsert

- [ ] `summary_state` は逆行しないか
- [ ] `dismissed_at` は通常 upsert で解除されないか
- [ ] `why_json` は code 単位で merge されるか
- [ ] supersede 情報が後続更新で消えないか

## 違反を見つけたら

次の形式で短く報告する。

```markdown
## Immutable Design Findings

1. [重大度] 何が不変条件に反するか
   - 該当箇所: path:line
   - 破っている原則: append-first / reproject-safe / versioned projection など
   - なぜ危険か: 再投影不能、shadow 汚染、意味論崩壊など
   - 代替案: event append / projector fix / contract update
```

ルール:

- まず違反を指摘する
- 次に、どの原則に反するかを名前で示す
- 代替案は event-first に寄せる
- 例外が必要なら ADR か canonical contract への明示反映を求める
