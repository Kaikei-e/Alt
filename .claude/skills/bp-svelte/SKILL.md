---
name: bp-svelte
description: Svelte 5 & SvelteKit ベストプラクティス。Svelte 5 Runes ベースのコンポーネント設計規約。
  TRIGGER when: .svelte ファイルを編集・作成する時、Svelte コンポーネントを書く時、alt-frontend-sv を実装する時。
  DO NOT TRIGGER when: テストの実行のみ、ファイルの読み取りのみ、他言語の作業時。
---

# Svelte 5 & SvelteKit Best Practices

このスキルが発動したら、`docs/best_practices/svelte.md` を Read ツールで読み込み、
記載されたベストプラクティス（DECREE）に従ってコードを書くこと。

## 重要原則

1. **Svelte 5 Runes**: `$state` / `$derived` / `$effect` を使用。レガシー `$:` リアクティブ宣言は禁止
2. **$props() でコンポーネント Props**: `export let` ではなく `$props()` でデストラクチャリング
3. **$state.raw で大規模データ**: 置換のみのデータは `.raw` で proxy オーバーヘッド回避。`.snapshot()` でシリアライズ
4. **$effect は副作用専用**: DOM 操作・外部ライブラリ連携・ネットワークのみ。状態導出は `$derived` を使う
5. **SvelteKit load 関数**: `+page.ts` / `+page.server.ts` の `load` でデータ取得。コンポーネント内で直接 fetch しない
6. **cleanup 関数を返す**: `$effect` 内の observer/listener/connection は return で cleanup
7. **snippet でコンポーネント合成**: `{#snippet}` + `{@render}` を使用。slot は非推奨

## 参照

完全なベストプラクティスは `docs/best_practices/svelte.md` を参照。
セクション: Svelte 5 Runes, Component Design, SvelteKit Routing, Data Loading, Form Actions, Styling, Testing
