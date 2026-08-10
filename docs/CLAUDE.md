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
- ADR参照は必ず `[[000NNN]]` wikilink形式を使う
- ADRが既存ADRを置き換える場合、新ADR側のfrontmatterに `supersedes:` リストを書く（キー省略可。空の `supersedes: -` stub は禁止）。旧ADR側への逆方向記入は不要（DocDag が算出）。循環・dangling・status ドリフト・空 stub は `docdag validate` で検出する（設定はリポジトリルートの `docdag.yaml`）
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
