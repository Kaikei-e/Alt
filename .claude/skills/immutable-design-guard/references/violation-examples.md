# Violation Examples — Case Studies and Fixes

実在のレビュー / ポストモーテムから抽出した違反パターン。
**症状 → 該当原則 → 是正** のテンプレで書く。

これは「Knowledge Home / Loop の文脈」で書かれているが、構造は他の
event-sourced サブシステムにそのまま適用できる。

## Case 1: Reproject-unsafe projector — latest を引いて投影してしまう

### 症状

`SummaryVersionCreated` event の projector が、event payload の
`summary_version_id` を使わず、`article_id` から `GetLatestSummaryVersion()`
を呼んでいた。replay 時に「古い event が新しい summary 内容で投影される」。

### 該当原則

- **Reproject-safe projector** — projector が latest state に依存
- **Versioned artifacts** — event が stable な version id を参照していない

### 是正

1. event payload に `summary_version_id` を含めることを契約として固定
2. `summary_version_port` に `GetSummaryVersionByID(id)` を追加
3. projector を `event.summary_version_id` で読むよう変更
4. `GetLatestSummaryVersion()` 依存を projector から外す
5. test に「旧 version event を replay しても旧 excerpt が使われる」
   ケースを追加

### 一般化

「projector に `Get<Latest>` / `Find<Active>` を呼ぶ箇所があれば、
ほぼ常に reproject-unsafe」。event payload に stable id を持たせ、
projector はそれを直接読む形に倒す。

> 一次出典: `docs/review/knowledge-home-phase1-5-remediation-directives-2026-03-18.md`
> §3 (P0-1 Summary projection の reproject-safe 化)

---

## Case 2: Backend-non-authoritative read — projection に availability が無い

### 症状

`weeklyRecapAvailable` / `eveningPulseAvailable` が UI / proto には存在
するが、`today_digest_view` の read/write に実装されていない。
TodayBar は backend contract を見ているように見えて、実際は
read result の zero-value に依存していた。

### 該当原則

- **Disposable projection** — projection の field 不足で UI 側が
  zero-value 解釈に逃げ、契約が backend 側で完結していない
- **Why as first-class** — availability の根拠 / 不可状態の理由が
  payload に表現されていない

### 是正

1. `today_digest_view` schema に `weekly_recap_available` /
   `evening_pulse_available` 列を追加
2. projector / usecase が availability を更新する
3. availability の source of truth (どの event 由来で true / false に
   遷移するか) を明文化
4. unavailable の理由を将来持てるよう拡張余地を残す
5. contract test で `(weeklyRecapAvailable=false, eveningPulseAvailable=true)`
   等のケースを fix

### 一般化

「UI が zero-value で挙動を変えている」「`!= null` で機能 on/off を
判定している」場合、**契約側 (event + projection)** に明示 field を
持たせて backend authoritative にする。

> 一次出典: `docs/review/knowledge-home-phase1-5-remediation-directives-2026-03-18.md`
> §4 (P0-2 TodayDigest availability の backend-authoritative 化)

---

## Case 3: Reproject swap でチェックポイントギャップ

### 症状

V3 reproject が `event_seq` 1116081 まで処理した後、約 22 時間遅れて
swap (`ActivateVersion(3)`) を実行。だが LIVE projector の checkpoint は
swap 時にリセットされず、seq 1116082 〜 ~1142000 のイベントが version 3
に未投影のまま運用継続。結果として 326 件の記事が表示欠落、1,947 件で
`summary_state` が stale。

### 該当原則

- **Reproject-safe projector** — swap operation が「activation +
  checkpoint reset」の不可分な単位として設計されていなかった
- **Disposable projection** (補強) — disposable であったおかげで
  checkpoint リセット 1 つで完全修復できた

### 是正

1. `SwapReproject()` に checkpoint reset ロジックを追加 (reproject の
   `checkpoint_payload.last_event_seq` を projector checkpoint に書く)
2. DI で `WithUpdateCheckpointPort()` を配線
3. swap 後の自動 sanity check (v_old / v_new の item count / state
   分布の比較) を追加
4. checkpoint テーブルに `projection_version` 列を入れることを設計
   レビューで検討
5. monitoring: version 間の `summary_state` 分布乖離アラート

### 一般化

「reproject + swap が分離された operation」ならば、活性化と checkpoint
の整合性が必ず別ステップで担保されている必要がある。activation 単独
は不完全な操作。

> 一次出典: `docs/postmortems/PM-2026-010-knowledge-home-reproject-checkpoint-gap.md`

---

## Case 4: Merge-unsafe upsert — snapshot 上書きで負数 / loss

### 症状

`today_digest_view` の `unsummarized_count` を、event 差分 (delta) では
なく snapshot 上書きで更新。並行 event があると COALESCE すべき列が
NULL に戻り、`current - 1` で counter が負数になりうる。

### 該当原則

- **Merge-safe upsert** — monotonic + COALESCE が守られていない
- **No business logic in SQL** — count 判定が SQL CASE に依存

### 是正

1. UPSERT を `GREATEST(0, current + delta)` 形式に変える
2. 既存の他列は `COALESCE(EXCLUDED.x, table.x)` で保持
3. monotonicity を `WHERE EXCLUDED.seq > table.seq_hiwater` で担保
4. business 判定 (どの event が +1 / -1 になるか) は Go 側で行い、
   SQL は単純な merge に留める

### 一般化

「同じ projection 行に複数 projector が触る」「event の順序が前後しうる」
場合は、必ず `GREATEST(0, current + delta)` + `seq_hiwater` ガード。
これを忘れると、稀に発生する race で負数や missing が出続け、
原因特定が極めて遅くなる。

> 関連: memory `feedback_merge_safe_upsert.md`

---

## レビュー Findings の書き方サンプル

これらケースを参考に、SKILL.md の出力テンプレを埋めると次のように
なる:

```markdown
## Immutable Design Findings

### 1. [high] reproject 時に latest summary が読まれて古い event が新しい内容で投影される
- 該当箇所: `alt-backend/app/job/knowledge_projector.go:128`
- 破っている原則: Reproject-safe projector / Versioned artifacts
- なぜ危険か: replay 順序が変わると read model 結果が変わる。
  shadow projection の検証が無効化される。
- 代替案:
  1. event payload に `summary_version_id` を必須化 (proto 変更)
  2. projector を `summary_version_id` 直読に変更
  3. `GetLatestSummaryVersion()` 依存を projector から外す
- 既知の類例: violation-examples.md Case 1
```

---

## このリストの育て方

新しい違反パターンを発見したら、上のテンプレに沿って追加する:

- 症状 (実例から anonymized)
- 該当原則 (alt-invariants.md の名前で参照)
- 是正手順
- 一般化 (他サービスでも適用できる教訓)
- 一次出典 (review / postmortem / ADR への wikilink)

[← back to SKILL.md](../SKILL.md)
