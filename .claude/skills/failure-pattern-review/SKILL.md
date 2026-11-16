---
name: failure-pattern-review
description: |
  Reviews a diff, a fix, or a plan against the nineteen failure patterns that Alt's ADRs and
  postmortems record as recurring (e.g., silent success, one-sided contract change, projection
  zero-overwrite, non-terminal retry, deploy unit skipping a dependency, tests that miss the
  production path, existence vs freshness confusion, carried-over action items). Returns only
  the gaps that affect correctness, each tied to the recorded precedent. Use before declaring a
  bug fix or a cross-service change done, when reviewing a diff another agent produced, or when
  the user says "failure pattern", "再発しないか見て", "同じ壊れ方をしていないか",
  "過去の障害パターンでレビュー". Not a style review and not a security review (use
  security-auditor), and not the layer check (use clean-architecture).
allowed-tools: Bash, Read, Glob, Grep
---

# Failure Pattern Review

Alt の ADR と `docs/postmortems/` の障害記録を突き合わせると、壊れ方は 19 の型に収束し、
同じ型が新しい境界ごとに再発している。このスキルは、差分がその型のどれかをもう一度作って
いないかを、記録された ADR・ポストモーテムの実例と突き合わせて確かめる。

型の定義・実例・問い・確認コマンドは [references/patterns.md](references/patterns.md) にある。
SKILL.md には経路表と報告の形だけを置く。該当する型の節だけを読む。

## 進め方

```
Failure Pattern Review:
- [ ] 1. 差分の範囲を確定する
- [ ] 2. 経路表で該当する型を選ぶ
- [ ] 3. 型ごとの問いに、コードを読んで答える
- [ ] 4. 既存の機械的ガードが見ている範囲を除く
- [ ] 5. 報告する
```

1. **範囲** — `git diff --stat <base>...HEAD` か渡されたファイル一覧。差分の外は、差分が前提に
   している呼び出し元・読み手・設定だけを読む。
2. **型を選ぶ** — 下の経路表。当てはまらない型は読まない。19 個すべてを当てにいかない。
3. **問いに答える** — `references/patterns.md` の該当節の問いに、`file:line` を示して答える。
   「無い」と言う前に、その節の確認コマンドを実行する。読んでいないものは「未確認」と書く。
4. **ガード済みを除く** — 各節の「機械的に見ているもの」に載っている検査が走る範囲は指摘しない。
   その検査の対象外（新しいディレクトリ、新しいサービス）に出たときだけ指摘する。
5. **報告** — 下の形。

## 経路表

| 差分が触るもの | 読む型 |
|---|---|
| DI・`main`・オプション依存・feature flag・`if x == nil` | FP-01 |
| `/health`・healthcheck・起動時設定・secret の読み込み | FP-01, FP-06, FP-13 |
| proto・Pact・フィールド名・REST と RPC の対・環境変数名・リスナ | FP-02, FP-09 |
| projector・merge / upsert SQL・checkpoint・migration・read model | FP-03 |
| プロンプト・チャットテンプレート・`num_predict`・出力検証器・モデル差し替え | FP-04, FP-12 |
| キュー・再試行・タイムアウト・ジョブ状態・`$effect` | FP-05 |
| `compose/**`・Dockerfile・init container・volume・依存の版指定 | FP-06, FP-12 |
| `.github/workflows/**`・テストのスタブ・E2E の skip・Pact の対象 | FP-07 |
| 直前の修正の続き・認証やガードの撤去・UI のリアクティブ修正 | FP-08, FP-14 |
| ID・UUID・URL・`user_id` を含む一意制約・同名テーブル・keyed `{#each}` | FP-09 |
| `docs/ADR/**` の `supersedes`・frontmatter の一括編集 | FP-10 |
| 文書や規約にある数値の床・「必ず〜する」を実装する変更 | FP-11 |
| `.env*`・イメージタグ・Dockerfile・モデル名・複数チェックアウト用 secrets | FP-12 |
| 証明書・トークン・モデル/学習成果物のロード・再読み込み・bind mount | FP-13 |
| 共通ヘルパ・契約・複数サービスが使うクライアント・データ書き込み経路に触る修正 | FP-14 |
| migration・絞り込みフィルタ追加・backfill CLI | FP-15 |
| `observability/**`・アラートルール・メトリクス・ダッシュボード・プローブ | FP-16 |
| セマフォ・プール・ロック・`CancelledError`・並行資源の獲得/解放 | FP-17 |
| デプロイスクリプト・掃除スクリプト・コンテナ操作・復旧 runbook | FP-18 |
| `docs/postmortems/**`・ADR の `Cons` / Consequences・アクションアイテム | FP-19 |

## 報告の形

```markdown
## Failure Pattern Review: <対象>

範囲: <base>...HEAD、<N> ファイル。読んだ型: FP-xx, FP-yy。

### 指摘
1. **FP-xx <型の名前>** — `path/file.go:123`
   何が起きるか: <入力・状態 → 誤った結果。1〜2 文>
   先例: <ADR-000NNN または PM-2026-NNN>（<その記録で起きたこと。1 文>）
   直し方: <この差分でやること。1 文>

### 該当なし
- FP-yy: <確認した内容と、実行したコマンド>

### 未確認
- <読めなかった範囲と理由>
```

## 守ること

- **正しさに効く指摘だけを書く。** 命名・書式・好みは書かない。指摘 0 件は有効な結果で、
  探せば何かは見つかるという前提で埋めない。
- **先例の無い指摘は型に載せない。** 19 の型に当てはまらない欠陥を見つけたら「型の外」と
  明記して 1 行で書く。型を広げて当てはめない。
- **「無い」は実行して確かめる。** 「テストが無い」「呼び出し元が無い」には `rg` の結果を
  添える。実在するものを「無い」と報告すると、それを受けた修正が丸ごと無駄になる。
- **直さない。** 読み取り専用で報告する。修正は呼び出し側が決める。
- **修正の続きなら直前の ADR と関連 PM を読む。** `docdag context <id> --depth 2 --budget 1500` と
  `docs/postmortems/` で、その修正が置き換えた決定・まだ残る隣の前提・未完了アクションアイテムを確かめる（FP-08, FP-14, FP-19）。
