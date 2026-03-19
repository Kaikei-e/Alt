# Knowledge Home Frontend Integration 実装手順

## 目的

Knowledge Home 周辺で、frontend が backend 未実装前提のまま止まっている箇所を順に解消する。対象は以下の 5 項目とする。

1. Knowledge Home 内の Ask 導線と Augur の受け側を正式接続する
2. backend で未対応の `save / unsave` UI を整理する
3. `knowledge_home.ts` に残っている仮フィールドを整理する
4. Recall の unavailable 表現を実際の状態に接続する
5. Ask 周辺 UI を英語に統一し、実装に合う文言へ調整する

## 前提

- 実装は `alt-frontend-sv` を主対象とする
- backend API 追加を前提にせず、既存 backend 契約に接続できる範囲を優先する
- テストは Red -> Green -> Refactor の順で進める
- UI 文言は英語で統一する

## 全体方針

- まず「誤った affordance」を止める
- 次に「既存 backend に繋げられる導線」を接続する
- 最後に「型だけ先行している箇所」を縮退させて契約を明確化する

## 実装ステップ

### Step 1. Augur 受け口の契約を整理する

目的:
- Knowledge Home から遷移した `q` と `context` を Augur 側で正しく解釈できるようにする

作業:
- `src/routes/(app)/augur/+page.svelte` で `q` と `context` の両方を取得する
- `q` がある場合は初回問い合わせメッセージを組み立てる
- `context` だけがある場合は下書きとして入力欄へ残す
- 質問文と文脈の結合ルールを utility に切り出す

期待結果:
- `/augur?q=...&context=...` で遷移したとき、単なる入力初期値ではなく問い合わせとして成立する

### Step 2. Knowledge Home の Ask 導線を接続する

目的:
- `AskSheet` の「未接続」状態を解消し、Knowledge Home からの質問が意味的に成立するようにする

作業:
- `AskSheet` の説明文を、現在の動作に合う英語文言へ変更する
- `Open in Augur` のラベルを実装に合う表現へ変更する
- `submitAsk()` の URL パラメータ設計を Step 1 の utility に合わせる
- item ask と home ask の双方で同じハンドオフルールを使う

期待結果:
- Knowledge Home の Ask は「未実装 UI」ではなく、Augur 連携として完結する

### Step 3. unsupported な Save UI を止める

目的:
- backend が未対応の `save / unsave` を frontend から見えなくする

作業:
- `QuickActionRow` から save/unsave ボタンを除去する
- `home/+page.svelte` の `save / unsave` 分岐を削除する
- `useKnowledgeHome.svelte.ts` の `setSaved()` を削除する
- 関連テストを修正する

期待結果:
- ユーザーに「保存できたように見えるが永続化されない」誤表示がなくなる

### Step 4. Recall unavailable を実状態に接続する

目的:
- Recall が単なる empty state なのか、一時的な取得失敗なのかを UI で区別できるようにする

作業:
- `useRecallRail` の `error` を page から参照する
- `RecallRail` に `unavailable` を明示的に渡す
- mobile 用 `RecallRailCollapsible` にも unavailable 表示を追加する
- home 初回ロード時に recall candidates が空なら、既存の `getRecallRail` で補完取得を試みる
- 補完取得失敗時のみ unavailable 表示へ倒す

期待結果:
- empty と unavailable が UI 上で分離される

### Step 5. Knowledge Home contract 先行フィールドを縮退する

目的:
- backend が返していない値を frontend が仮値で持ち続ける状態を解消する

作業:
- `TodayDigestData` から未使用かつ backend 未提供の項目を整理する
- `KnowledgeHomeItemData` から未接続の項目を整理する
- `convertDigest()` と `convertItem()` の固定値投入を削除する
- もし UI 派生値が必要なら transform 層で派生値であることを明示する

対象候補:
- `totalItems`
- `unreadCount`
- `audioAvailable`
- `sourceName`
- `summaryVersionId`
- `tagVersionId`
- `recapRefs`
- `audioState`
- `isSaved`
- `isRead`
- `isDismissed`

期待結果:
- frontend 型が backend 契約により近づく

### Step 6. Ask UI 文言を英語へ統一する

目的:
- Knowledge Home UI の言語混在を解消する

作業:
- `AskSheet` の suggestion を英語化する
- 説明文と CTA を英語で統一する
- fallback / degraded と同じトーンで、断定しすぎない表現に揃える

期待結果:
- Knowledge Home から Augur への導線が他 UI と同じ英語 UX になる

## テスト計画

### 追加・更新するテスト

- Augur handoff utility の unit test
- `QuickActionRow` の action 構成テスト
- `RecallRail` / `RecallRailCollapsible` の unavailable 表示テスト
- 必要なら `ChatWindow` の初回メッセージ反映テスト

### 実行コマンド

```bash
cd alt-frontend-sv
bunx vitest run src/lib/components/knowledge-home/QuickActionRow.test.ts
bunx vitest run src/lib/components/mobile/search/ChatWindow.svelte.test.ts
bunx vitest run src/lib/components/knowledge-home/recall-rail/RecallRail.svelte.test.ts
VITEST_BROWSER=true bunx vitest run --project=client src/lib/components/knowledge-home/DegradedModeBanner.svelte.test.ts
```

必要に応じて、実装箇所に応じた最小追加テストを同時に実行する。

## 完了条件

- Knowledge Home の Ask 導線に「未接続」表現が残っていない
- Augur 側で `q/context` の受け取りが成立している
- save/unsave UI が消えているか、backend 契約と整合している
- Recall unavailable が empty と区別される
- `knowledge_home.ts` の仮値固定が整理されている
- 追加したテストが通る
