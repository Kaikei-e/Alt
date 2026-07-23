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
- ADRが既存ADRを置き換える場合、新ADR側のfrontmatterに `supersedes:` リストを書く（キー省略可。空の `supersedes: -` stub は禁止）。旧ADR側への逆方向記入は不要（`scripts/adr_graph.py` が算出）。循環・dangling・status ドリフト・空 stub は `python3 scripts/adr_graph.py check` で検出する

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
