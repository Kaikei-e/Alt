# Alt Obsidian Vault

ボールトルートは `docs/` ディレクトリ。ADR/ と services/ はシンボリックリンクなしで直接アクセスできる。

## 構造
- `ADR/` — Architecture Decision Records
- `wiki/` — ナビゲーション層（gitignored、`wiki/HOME.md` が起点）
- `services/` — マイクロサービス仕様・レジストリ（`MICROSERVICES.md` + サービス個別文書）
- `runbooks/` — 運用手順書・ランブック（`runbooks/README.md` インデックス）
- `postmortems/` — 障害振り返り・ポストモーテム（`postmortems/README.md` インデックス）
- `review/` — レビュー・監査記録（gitignored、`review/README.md` インデックス）
- `plan/` — 設計計画・正準契約書（gitignored、Trail / Home / active plans）
- `case-studies/` — ケーススタディ・設計進化の記録
- `perf/` — 性能測定・負荷試験ドキュメント
- `features/` — 機能仕様・アーキテクチャドキュメント
- `evaluations/` — 評価・検証ドキュメント
- `best_practices/` — 設計原則・プラクティス集
- `glossary/` — 用語集
- `images/` — 構成図・スクリーンショット等の静的画像
- `blog/` — ブログ下書き（gitignored）
- `daily/` — デイリーノート（gitignored、YYYY-MM-DD.md）
- `proposals/` — 提案書（コードから参照される歴史的記録）
- `testing-strategy.md` — テスト戦略ドキュメント（ルート直下保持）

## ルール
- frontmatter必須: title, date, tags
- 内部リンクは `[[ノート名]]` 形式
- タグ: #alt #performance #zenn #idea
- ADRへのリンク追加（Related ADRsのwikilink化）は可。Decision 本文の内容改変は不可。
- **例外（status 投影）**: inbound `supersedes` がある旧 ADR の frontmatter `status` だけは `superseded` に更新してよい（binding の正本は reverse グラフ。`status` はその投影）。
- **append-only が保護するのは「確定した記録の意味」**: 見出し表記をテンプレート綴りに正規化する、本文が既に述べている関係を frontmatter の edge として書き足す、といった**意味を変えない編集**は append-only 違反ではない。Decision が何を決めたかを変える編集は引き続き禁止。`--immutable-since` はバイト単位で比較するため、この種の一括正規化 PR は `immutable_violation` を出す — 意図してレビューし merge する例外であり、通常の PR はこの finding をゼロに保つ
- ADR参照は必ず `[[000NNN]]` wikilink形式を使う
- 新ADRが既存ADRの決定を丸ごと置き換えるなら frontmatter に `supersedes:` を、既存ADRの決定を前提にする・拡張する・部分的に修正するなら `depends-on:` を書く（本文で「〜を前提に」と述べたら frontmatter の edge も必ず対応させる。単なる言及・背景・対比・却下した代替案は `[[000NNN]]` wikilink だけでよい）。どちらも新ADR側にだけ書き、旧ADR側への逆方向記入は不要（DocDag が算出する）。キーは省略可、空の `supersedes: -` / `depends-on: -` stub は `empty_edge` エラー
- `supersedes` に足す ID は、新 ADR の本文が `[[000NNN]]` で名指しして置き換える理由を述べているものだけ。辺を 1 本足すと相手は binding 一覧（`docdag query --binding`）から落ちる。`docdag validate` は辺と status が整合していれば通るので、本文に無い辺は `./scripts/altharness/alt-harness adr-gate --base origin/main` が CI で落とす（2026-10-24 の一括修復で無関係な 54 件が binding から落ちた経緯は [[001011]]）
- `docdag validate` が検出するのは、循環（`supersedes` と `depends-on` の和集合をまたぐ循環も含む）・dangling 参照・status ドリフト・`empty_edge`・本文の見出し欠落/順序違い（`missing_section` / `section_order`: `## Status` → `## Context` → `## Decision` → `## Consequences` の順で必須）・`unmanaged_file`（`docs/ADR/` 直下で `NNNNNN.md` でも README.md/index.md/template.md/`_`・`.`始まりでもない野良 Markdown）・本文や frontmatter の `[[000NNN]]` wikilink が存在しない ADR を指しているケース・append-only 違反（`--immutable-since <rev>`）。このリポジトリでは `unstructured_supersedes`（旧来の `status: "superseded by X"` という自由記述）・`unmanaged_file`・`superseded_orphan` を含め warning ではなく error。設定はリポジトリルートの `docdag.yaml`
- 後継 ADR を書かずに決定を取り下げる場合は `status: withdrawn`。`superseded` は「置き換えた ADR が存在する」ことの投影なので、誰も置き換えていない文書に付けると `superseded_orphan` エラーが残り続ける（このリポジトリでは error に昇格済み）
- ADR グラフを読むのは grep ではなく DocDag。`docdag context <id>` が起点 ADR + 後継 + 近傍（`supersedes`・`depends-on` 両方の typed edge）を Decision 冒頭つきで返す。ある決定が何の上に立っているか（前提）を追うのは `docdag query <id>`（既定 = `--descendants`）、逆にその決定に依存している側（変更の波及先）を追うのは `docdag query <id> --ancestors`。`docdag query --binding` が現行契約の一覧。`docdag resolve <id>` は non-binding な後継（例: `proposed`）で止まり、それを stderr の note（text）/ `pending` 配列（JSON）で示す — 後継が確定済みとは限らない。いずれも `--fields id,title,status,path` で必要な列だけ取れる
- frontmatter は厳密 YAML。`affected_services` には正準のサービス名・コンポーネント識別子のみを列挙し（例: `[alt-backend, knowledge-sovereign]`）、言語注記や変更概要の散文は本文の `## Affected Services` セクションに記述する（YAML の辞書化やクォート漏れによる `invalid_frontmatter` エラーと集計分裂を防ぐ）
- **このリポジトリは public。新規に書く文書に private リポジトリの内部識別子を書かない** — デプロイ側の workflow 名 / job 名 / 変数名 / ゲート条件など。振る舞い（何が起きるか、運用者が何をすべきか）で記述する。ホスト名・ハードウェア構成・本番ドメイン・絶対パス・認証情報も同様に書かない。既存文書の遡及修正はしない（ADR 本文は改変不可であり、git 履歴にも残るため実効性が薄い）
- 新規ポストモーテムのファイル名は `docs/postmortems/PM-YYYY-NNN-<slug>.md` とし、`docs/postmortems/README.md` の一覧表へ同 ID の行を必ず追加する（`README.md` 以外の非標準名 Markdown や実体の無い行は CI ゲートが落とす。既存ポストモーテムは遡及修正しない）
- 新規ポストモーテムでは `## 根本原因分析` の直後の `##` 見出しとして `## 失敗の型と再発` を置き、リスト項目 `- 失敗の型:` / `- 再発:` / `- 持ち越し:` / `- 止められたはずの検査:` の 4 行を空欄なしで書く。`失敗の型` は `failure-pattern-review` スキルの `FP-NN` ID 列挙、または `型の外` の後に 10 文字以上の理由を記す。`再発`・`持ち越し` に書く `PM-YYYY-NNN` は `docs/postmortems/` に実在するものに限る（該当なしは `なし`）
- アクションアイテムを書く前に `./scripts/altharness/alt-harness postmortem --open --match <service or keyword>` を実行し、過去件の再演となる未完了項目を `持ち越し:` に列挙する。2 度目の持ち越しとなる項目は転記で済ませず優先度を引き上げて扱う
- アクションアイテムは `## アクションアイテム` 配下の Markdown テーブル（`#`・`カテゴリ`・`アクション`/`内容`・`担当`・`期限`・`ステータス`、少なくともアクション列必須。チェックリストのみ・列対応不能な表はゲート不可）で書く。`担当` は空およびプレースホルダ（`要アサイン` / `TBD` / `未定` / `-`）禁止、`期限` は `YYYY-MM-DD`、ステータスは `TODO` / `進行中` / `完了` / `破棄`（太字 `*` 囲み可）のみ。`完了` はアクション欄またはステータス欄に根拠（実在する `docs/ADR/NNNNNN.md` への `[[NNNNNN]]`、`PR #N` / `PR#N` / `/pull/N` URL、英字 a-f と数字を含み前後に英数字・ハイフンが付かない 7〜40 桁 commit hash、実在するリポジトリ内パスを囲むバッククォート表記）を記し、`破棄` はアクション欄に `破棄理由:` に続けて 10 文字以上の理由を書く。ゲートは `./scripts/altharness/alt-harness postmortem --check --base origin/main`

## 検索ガイドライン
- **まず `wiki/HOME.md` を見る** — 結晶化された navigation layer。ADR / runbook / plan の入口
- vault内のノート検索にはObsidian MCPツールを優先して使うこと
- ADRのキーワード検索は grep でも可だが、タグやリンク関係の探索にはMCPを使うこと
- vault外のファイル（ソースコード等）には直接ファイルアクセスを使うこと


## 計画コンテキストガイド

| 計画対象 | 必読ドキュメント |
|---|---|
| Knowledge Trail | [[knowledge-trail-core-concept]], [[knowledge-trail-implementation-plan]], [[wiki/architecture/knowledge-trail]] |
| Knowledge Home（今日の入口） | [[knowledge-home-value-position-plan]], [[wiki/architecture/immutable-data-model]] |
| イミュータブルデータモデル | [[wiki/architecture/immutable-data-model]], Trail §C |
| Projector / Reproject | [[services/knowledge-sovereign]], runbooks の reproject 系 |
| 是正・未達事項（historical audit） | [[review/knowledge-home-phase0-4-audit-2026-03-18]], [[review/knowledge-home-phase1-5-remediation-directives-2026-03-18]] |
| Knowledge Loop（historical） | [[wiki/architecture/knowledge-loop]], [[000940]] — 現行契約として開かない |
| Acolyte 全般 | [[features/acolyte/README]], [[case-studies/acolyte-design-evolution]], ADR 000653-000700 |
| Acolyte パイプライン | [[features/acolyte/data-flow]], [[runbooks/acolyte-checkpoint-resume]] |
| Acolyte 運用 | runbooks/acolyte-*.md |
| 障害分析・ポストモーテム | [[postmortems/README]] |
| レビュー・監査記録 | [[review/README]]（gitignored） |
| 運用手順 | [[runbooks/README]] |
| 直近の作業文脈 | daily/ の最新エントリ（gitignored） |
