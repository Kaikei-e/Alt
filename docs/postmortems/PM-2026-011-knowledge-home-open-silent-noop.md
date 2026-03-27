# PM-2026-011: Knowledge Home Open ボタンがサイレントに no-op になりナビゲーションしない

## メタデータ

| 項目 | 値 |
|------|-----|
| 重大度 | SEV-4（一部記事でナビゲーション不可、サービス停止なし） |
| 影響期間 | 旧データ由来のため正確な開始時期は不明。検知は 2026-03-26 23:50 (JST) |
| 影響サービス | alt-frontend-sv |
| 影響機能 | Knowledge Home の記事 Open アクション |
| 関連 ADR | [[000598]], [[000599]] |
| 関連 PM | [[PM-2026-010-knowledge-home-reproject-checkpoint-gap]] |

## サマリー

Knowledge Home で記事カードの Open をクリックしても記事ページに遷移しない問題が報告された。DevTools では `TrackHomeAction` API が 200 で成功しているにもかかわらず、ページ遷移が発生しなかった。直接原因は FE の `handleAction()` で `item.link` が空の場合にサイレントに `return` していたこと。根本原因は旧イベント発行パス（`actor_id = "article-store"`）で `ArticleCreated` イベントに `link` フィールドが含まれておらず、`knowledge_home_items.link` が空のまま read model に残っていたことである。projection version 3 で 2,045 件が空 link であった。

## 影響

- **ナビゲーション不可**: 2,045 件の記事で Open ボタンが無反応（全 44,149 件中、約 4.6%）
- **ユーザー体験**: ボタン押下に対してエラーメッセージも遷移も発生せず、壊れているように見える
- **TrackHomeAction の無駄呼び出し**: Open 押下のたびに tracking API が呼ばれるが、ナビゲーションは発生しない
- **データ損失**: なし

## タイムライン

| 時刻 (JST) | イベント |
|---|---|
| 不明 | 旧イベント発行パス（`article-store`）で `ArticleCreated` payload に `link` なしのイベントが 14,397 件蓄積 |
| 2026-03-26 23:50 | **検知**: ユーザーが Knowledge Home で Open 押下してもナビゲーションしないことを報告 |
| 2026-03-26 23:50 | **対応開始**: FE コード・DB・イベントログの調査を開始 |
| 2026-03-26 23:55 | FE の `handleAction()` で `item.link` が空の場合にサイレント `return` していることを特定 |
| 2026-03-27 00:00 | sovereign-db で `knowledge_home_items` の v3 で 2,045 件が空 link であることを確認 |
| 2026-03-27 00:05 | `knowledge_events` の `ArticleCreated` イベントを actor_id 別に集計。`article-store` の 14,397 件全てが link なしであることを確認 |
| 2026-03-27 00:07 | **原因特定**: 旧イベント発行パスで `link` フィールドが payload に含まれていなかったことが根本原因。現行の outbox-worker / pre-processor パスでは正しく設定されている |
| 2026-03-27 00:12 | **緩和策適用**: FE の `handleAction()` に toast フォールバックを追加。サイレント no-op を廃止 |
| 2026-03-27 00:14 | alt-frontend-sv 再ビルド・起動確認 |

## 検知

- **検知方法**: ユーザーによる手動報告
- **検知までの時間 (TTD)**: 旧データ由来のため正確な TTD は不明。link フィールドの投影が有効になった後も既存の空 link 行が残り続けた
- **検知の評価**: FE で no-op が発生する条件がログにも console にも出力されていなかったため、サーバー側のモニタリングでは検知不可能だった。さらに、`knowledge_home_items.link = ''` の件数監視と replay 中の checkpoint 進捗監視がなければ、復旧が進んでいるかも見えにくかった

## 根本原因分析

### 直接原因

FE の `handleAction()` で `item.link` が空の場合にナビゲーションをスキップし、エラーフィードバックも出さずに `return` していた。

### Five Whys

1. **なぜ Open ボタンを押してもナビゲーションしなかったか？**
   → `handleAction()` の `if (item.link)` チェックが falsy で、`goto()` が呼ばれなかったため

2. **なぜ `item.link` が空だったか？**
   → `knowledge_home_items.link` カラムが空文字列のまま read model に残っていたため

3. **なぜ read model の link が空だったか？**
   → projector が `ArticleCreated` イベントの `payload.link` から投影するが、旧イベント（`actor_id = "article-store"`）の payload に `link` フィールドが含まれていなかったため

4. **なぜ旧イベントに link がなかったか？**
   → `link` フィールドは後から追加された機能であり、旧イベント発行パス（現在はコードベースに存在しない `article-store`）では未実装だった

5. **なぜ空 link がユーザーに見えるまで放置されたか？**
   → FE がサイレント no-op を行い、エラーフィードバックがなかったため、問題が顕在化しなかった

### 寄与要因

- **サイレント no-op パターン**: `if (condition) { action } return;` というパターンが else 節なしで使われ、失敗が見えない設計になっていた
- **旧データの残存**: `knowledge_events` テーブルの append-first 原則により旧イベントは永続的に残り、reproject / replay のたびに空 link が再投影される
- **`/articles/[id]` の url 必須依存**: `url` クエリパラメータなしではコンテンツ取得も要約もできないため、link の欠損は実質的にナビゲーション不能を意味する

### Replay と Repair の切り分け

空 link の修復は 2 種類に分かれる。

1. **replay で直るもの**: `knowledge_events` に `ArticleCreated.payload.link` が存在するのに、checkpoint gap や古い projection の影響で `knowledge_home_items.link` に反映されていない行
2. **repair が必要なもの**: `article-store` 由来などで `ArticleCreated.payload.link` 自体が空の行

判定用 SQL:

```sql
WITH empty_items AS (
  SELECT item_key, primary_ref_id
  FROM knowledge_home_items
  WHERE item_type = 'article'
    AND link = ''
)
SELECT
  count(*) FILTER (
    WHERE EXISTS (
      SELECT 1
      FROM knowledge_events ke
      WHERE ke.aggregate_type = 'article'
        AND ke.aggregate_id = empty_items.primary_ref_id::text
        AND ke.event_type = 'ArticleCreated'
        AND COALESCE(ke.payload->>'link', '') <> ''
    )
  ) AS replayable,
  count(*) FILTER (
    WHERE NOT EXISTS (
      SELECT 1
      FROM knowledge_events ke
      WHERE ke.aggregate_type = 'article'
        AND ke.aggregate_id = empty_items.primary_ref_id::text
        AND ke.event_type = 'ArticleCreated'
        AND COALESCE(ke.payload->>'link', '') <> ''
    )
  ) AS repair_needed
FROM empty_items;
```

- `replayable > 0`: まず replay を再実行する
- `repair_needed > 0`: replay 後も残る分だけを一時 repair script に流す

## 対応の評価

### うまくいったこと

- FE コードの調査から DB・イベントログの突合まで約 20 分で根本原因を特定できた
- `actor_id` 別の集計で、旧パス（`article-store`）と現行パス（`outbox-worker`, `pre-processor`）の差異が明確になった
- canonical contract と実装の整合確認により、現行パスでは問題がないことを確認できた

### うまくいかなかったこと

- 初期調査で projector に `ArticleUpdated` ハンドラを追加し enrichment イベントを sovereign-db に直接 INSERT する方向に逸れた。プランの方針（contract は正しい、stale row の切り分けが先）に立ち返る必要があった
- サイレント no-op の設計は、Knowledge Home 実装初期から存在しており、レビューで見逃された

### 運が良かったこと

- 影響は全記事の約 4.6%（2,045 / 44,149）に限定されていた
- 現行の記事作成パスでは link が正しく設定されるため、新規記事には影響しない

## アクションアイテム

### 予防（Prevent）

| # | アクション | 担当 | 期限 | 状態 |
|---|---|---|---|---|
| P-1 | FE の `handleAction()` に toast フォールバックを追加（サイレント no-op 廃止） | 開発担当者 | 2026-03-27 | **完了** |

### 検知（Detect）

| # | アクション | 担当 | 期限 | 状態 |
|---|---|---|---|---|
| D-1 | `knowledge_home_items.link` が空の件数を定期計測し、閾値超過でアラート | 開発担当者 | 2026-04-10 | 未着手 |
| D-2 | replay 実行中に `knowledge_projection_checkpoints.last_event_seq` が一定時間進まない場合にアラート | 開発担当者 | 2026-04-10 | 未着手 |

### 緩和（Mitigate）

| # | アクション | 担当 | 期限 | 状態 |
|---|---|---|---|---|
| M-1 | replayable rows を replay で修復し、`repair_needed` のみ repair script で埋める | 開発担当者 | 2026-04-10 | 未着手 |

### プロセス（Process）

| # | アクション | 担当 | 期限 | 状態 |
|---|---|---|---|---|
| R-1 | FE のアクションハンドラで else 節なしの早期 return パターンを検出する lint ルールの検討 | 開発担当者 | 2026-04-17 | 未着手 |

## 教訓

### 技術的教訓

1. **サイレント no-op は最も見つけにくいバグ**: ユーザーがボタンを押しても何も起きない場合、サーバーログにもコンソールにも痕跡が残らない。空条件での早期 return には必ずフィードバック（toast, console.warn 等）を付与すべき
2. **append-first イベントログの旧データは永続的**: 旧フォーマットのイベントは永久に残る。read model が旧イベントに依存するフィールドを追加する場合、既存イベントの補償戦略が必要
3. **canonical contract と実装の乖離は stale data で顕在化する**: 現行コードが正しくても、旧データが残っていれば問題は発生する。contract 更新時に既存データの移行計画を含めるべき

### 組織的教訓

1. **計画の赤入れに従うこと**: 調査中に見つけた問題に対して即座にコード修正に飛ぶのではなく、プランの方針（contract は正しい、stale row の切り分けが先）に沿って進めることが重要

## 参考資料

- `alt-frontend-sv/src/routes/(app)/home/+page.svelte` — handleAction 修正箇所
- `alt-frontend-sv/src/routes/(app)/articles/[id]/+page.svelte` — `/articles/[id]` の url 必須依存
- [[000598]] SwapReproject チェックポイントリセット
- [[000599]] Knowledge Home Open アクション toast フォールバック
- [[PM-2026-010-knowledge-home-reproject-checkpoint-gap]] — 関連 PM（reproject ギャップ）
- `plan/knowledge-home-phase0-canonical-contract.md` — canonical contract

---

> **Blameless Postmortem の原則:** このドキュメントは個人の過失を追及するためではなく、
> システムの脆弱性とプロセスの改善機会を特定するために作成されています。
> 「誰が悪いか」ではなく「システムのどこが改善できるか」に焦点を当ててください。
