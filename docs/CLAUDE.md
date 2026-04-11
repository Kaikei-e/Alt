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
- ADRへのリンク追加（Related ADRsのwikilink化）は可。内容改変は不可。
- ADR参照は必ず `[[000NNN]]` wikilink形式を使う

## 検索ガイドライン
- vault内のノート検索にはObsidian MCPツールを優先して使うこと
- ADRのキーワード検索は grep でも可だが、タグやリンク関係の探索にはMCPを使うこと
- vault外のファイル（ソースコード等）には直接ファイルアクセスを使うこと


## 計画コンテキストガイド

| 計画対象 | 必読ドキュメント |
|---|---|
| Knowledge Home 全般 | [[knowledge-home-phase0-canonical-contract]], [[alt_knowledge_home_phase_plan]] |
| イミュータブルデータモデル | IMPL_BASE.md |
| Projector / Reproject | [[knowledge-home-projection-recovery]], IMPL_PHASE1-6 |
| 是正・未達事項 | [[knowledge-home-phase0-4-audit-2026-03-18]], [[knowledge-home-phase1-5-remediation-directives-2026-03-18]] |
| Acolyte 全般 | [[acolyte/README]], [[acolyte-design-evolution]], ADR 000653-000700 |
| Acolyte パイプライン | [[acolyte/data-flow]], [[acolyte-checkpoint-resume]] |
| Acolyte 運用 | runbooks/acolyte-*.md |
| 運用手順 | runbooks/ 配下 |
| 直近の作業文脈 | daily/ の最新エントリ |
