# Alt Obsidian Vault

ボールトルートは `docs/` ディレクトリ。ADR/ と services/ はシンボリックリンクなしで直接アクセスできる。

## 構造
- `ADR/` — Architecture Decision Records（直接アクセス）
- `services/` — マイクロサービスドキュメント（直接アクセス）
- `daily/` — デイリーノート（YYYY-MM-DD.md）
- `blog/`, `perf/`, `proposals/`, `review/`, `runbooks/` — その他ドキュメント

## ルール
- frontmatter必須: title, date, tags
- 内部リンクは `[[ノート名]]` 形式
- タグ: #alt #performance #zenn #idea
- ADRへのリンク追加（Related ADRsのwikilink化）は可。Decision 本文の内容改変は不可。
- **例外（status 投影）**: inbound `supersedes` がある旧 ADR の frontmatter `status` だけは `superseded` に更新してよい（binding の正本は reverse グラフ。`status` はその投影）。
- **append-only が保護するのは「確定した記録の意味」**: 見出し表記をテンプレート綴りに正規化する、本文が既に述べている関係を frontmatter の edge として書き足す、といった**意味を変えない編集**は append-only 違反ではない。Decision が何を決めたかを変える編集は引き続き禁止。`--immutable-since` はバイト単位で比較するため、この種の一括正規化 PR は `immutable_violation` を出す — 意図してレビューし merge する例外であり、通常の PR はこの finding をゼロに保つ
- ADR参照は必ず `[[000NNN]]` wikilink形式を使う
- 新ADRが既存ADRの決定を丸ごと置き換えるなら frontmatter に `supersedes:` を、既存ADRの決定を前提にする・拡張する・部分的に修正するなら `depends-on:` を書く（本文で「〜を前提に」と述べたら frontmatter の edge も必ず対応させる。単なる言及・背景・対比・却下した代替案は `[[000NNN]]` wikilink だけでよい）。どちらも新ADR側にだけ書き、旧ADR側への逆方向記入は不要（DocDag が算出する）。キーは省略可、空の `supersedes: -` / `depends-on: -` stub は `empty_edge` エラー
- `docdag validate` が検出するのは、循環（`supersedes` と `depends-on` の和集合をまたぐ循環も含む）・dangling 参照・status ドリフト・`empty_edge`・本文の見出し欠落/順序違い（`missing_section` / `section_order`: `## Status` → `## Context` → `## Decision` → `## Consequences` の順で必須）・`unmanaged_file`（`docs/ADR/` 直下で `NNNNNN.md` でも README.md/index.md/template.md/`_`・`.`始まりでもない野良 Markdown）・本文や frontmatter の `[[000NNN]]` wikilink が存在しない ADR を指しているケース・append-only 違反（`--immutable-since <rev>`）。このリポジトリでは `unstructured_supersedes`（旧来の `status: "superseded by X"` という自由記述）・`unmanaged_file`・`superseded_orphan` を含め warning ではなく error。設定はリポジトリルートの `docdag.yaml`
- 後継 ADR を書かずに決定を取り下げる場合は `status: withdrawn`。`superseded` は「置き換えた ADR が存在する」ことの投影なので、誰も置き換えていない文書に付けると `superseded_orphan` エラーが残り続ける（このリポジトリでは error に昇格済み）
- ADR グラフを読むのは grep ではなく DocDag。`docdag context <id>` が起点 ADR + 後継 + 近傍（`supersedes`・`depends-on` 両方の typed edge）を Decision 冒頭つきで返す。ある決定が何の上に立っているか（前提）を追うのは `docdag query <id>`（既定 = `--descendants`）、逆にその決定に依存している側（変更の波及先）を追うのは `docdag query <id> --ancestors`。`docdag query --binding` が現行契約の一覧。`docdag resolve <id>` は non-binding な後継（例: `proposed`）で止まり、それを stderr の note（text）/ `pending` 配列（JSON）で示す — 後継が確定済みとは限らない。いずれも `--fields id,title,status,path` で必要な列だけ取れる
- frontmatter は厳密 YAML。バッククォートや `: ` を含む値（特に `affected_services` の項目）はシングルクォートで囲む — `docdag validate` が invalid_frontmatter ERROR で検出する
- **このリポジトリは public。新規に書く文書に private リポジトリの内部識別子を書かない** — デプロイ側の workflow 名 / job 名 / 変数名 / ゲート条件など。振る舞い（何が起きるか、運用者が何をすべきか）で記述する。ホスト名・ハードウェア構成・本番ドメイン・絶対パス・認証情報も同様に書かない。既存文書の遡及修正はしない（ADR 本文は改変不可であり、git 履歴にも残るため実効性が薄い）

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
| Projector / Reproject | [[wiki/services/knowledge-sovereign]], runbooks の reproject 系 |
| 是正・未達事項（historical audit） | [[knowledge-home-phase0-4-audit-2026-03-18]], [[knowledge-home-phase1-5-remediation-directives-2026-03-18]] |
| Knowledge Loop（historical） | [[wiki/architecture/knowledge-loop]], [[000940]] — 現行契約として開かない |
| Acolyte 全般 | [[acolyte/README]], [[acolyte-design-evolution]], ADR 000653-000700 |
| Acolyte パイプライン | [[acolyte/data-flow]], [[acolyte-checkpoint-resume]] |
| Acolyte 運用 | runbooks/acolyte-*.md |
| 運用手順 | runbooks/ 配下 |
| 直近の作業文脈 | daily/ の最新エントリ |
