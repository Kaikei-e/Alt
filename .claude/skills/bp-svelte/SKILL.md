---
name: bp-svelte
description: Alt の Svelte 5 / SvelteKit 規約を適用する。Runes（$state/$derived/$effect）、$props()、$effect をコールスタック越えで自己再発火させない、snippet でのコンポーネント合成、load 関数でのデータ取得、{@html} の DOMPurify 必須、keyed each の重複キー、非同期コールバックの stale-response guard を扱う。Svelte コンポーネントや SvelteKit のルート・load 関数を書く・直すときに使う。ユーザが「Svelte」に触れなくても alt-frontend-sv の UI 実装に入るなら使う。
paths:
  - "**/*.svelte"
  - "alt-frontend-sv/src/routes/**/*.ts"
---

# Svelte 5 & SvelteKit Best Practices

以下はタスク全体を通じて有効な規約であり、一度読んで終わる手順ではない。Svelte コードを書くたびに適用する。

詳細な根拠とコード例が必要になった時点で `docs/best_practices/svelte.md` の該当セクションだけを Read する
（1221 行あるため全文読み込みはしない）。

## 重要原則

1. **Svelte 5 Runes**: `$state` / `$derived` / `$effect` を使用。レガシー `$:` リアクティブ宣言は禁止
2. **$props() でコンポーネント Props**: `export let` ではなく `$props()` でデストラクチャリング
3. **$state.raw で大規模データ**: 置換のみのデータは `.raw` で proxy オーバーヘッド回避。`.snapshot()` でシリアライズ
4. **$effect は副作用専用**: DOM 操作・外部ライブラリ連携・ネットワークのみ。状態導出は `$derived` を使う
5. **SvelteKit load 関数**: `+page.ts` / `+page.server.ts` の `load` でデータ取得。コンポーネント内で直接 fetch しない
6. **cleanup 関数を返す**: `$effect` 内の observer/listener/connection は return で cleanup
7. **snippet でコンポーネント合成**: `{#snippet}` + `{@render}` を使用。slot は非推奨
8. **{@html} は DOMPurify 必須**: RSS / 上流 API 由来 HTML も user input。SSR とクライアント両方でサニタイズ（→ `.claude/rules/security-boundaries.md`）
9. **$effect の依存追跡はコールスタックを越える**: effect 内で呼ぶ関数の `$state` 読み書きが自己再発火ループを作る。ガード条件は effect 本体に直接書き、依存にしない読み取りは `untrack()`。stream 起点の refresh は無条件 `invalidateAll()` 禁止 — スコープ付き `invalidate(name)` + debounce
10. **keyed `{#each}` の重複キーは警告なしでクラッシュ**: 一意な記事 ID をキーにする。動的 ranking backend の offset pagination は FE 側 dedupe（`appendUniqueById`）必須
11. **非同期コールバックに stale-response guard**: 呼び出し時点の ID をキャプチャして適用前に比較。AbortController だけでは不十分、catch 節でも `signal.aborted` を確認

## 参照

完全なベストプラクティスは `docs/best_practices/svelte.md` を参照。
セクション: Svelte 5 Runes, Component Design, SvelteKit Routing, Data Loading, Form Actions, Styling, Testing
