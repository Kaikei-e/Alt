---
title: "Ubiquitous Language Glossary"
date: 2026-04-28
tags:
  - glossary
  - ddd
  - canonical
  - knowledge-home
---

# Ubiquitous Language Glossary

このドキュメントは Alt 全体で **同じ概念を同じ名前で呼ぶ** ための DDD ubiquitous-language 規約。
PM-2026-041 / ADR-000865 / 2026-04-28 セッションの根本原因は **「同じ概念を Link と URL の 2 通りで呼んでいた」** ことだった。命名の二重化は wire form / projection / FE の各層で silent drift を生む。本文書はこのクラスバグを構造的に閉じる。

新規コードはこの語彙に従う。既存コードは段階的に追従する (執行は ADR-000867 が schedule する)。

---

## URL カテゴリ

「HTTP(S) location を持つ resource」は **常に `URL` と呼ぶ**。
Go field 名・proto field 名・DB column 名・JSON wire key・FE TS 型・UI 文言、すべての層で `URL` (大文字 / `url` / `URL` のケース変種は文脈に従う)。

| 文脈 | canonical 名 | 該当箇所 | 備考 |
|---|---|---|---|
| Article URL (Knowledge Home item の article 元 URL) | `URL` (Go field), `url` (proto / db column / wire) | `domain.KnowledgeHomeItem.URL`, `proto/.../knowledge_home.proto` field 13, `knowledge_home_items.url` 列 | **PR 1 (ADR-000867) で rename 完了**。歴史的に `Link` と呼んでいたが PM-2026-041 の真因 |
| Article URL (event payload) | `URL` (Go field), `url` (wire key) | `domain.ArticleCreatedPayload.URL`, `domain.ArticleUrlBackfilledPayload.URL` | PR 1 (ADR-000867) で確立 |
| Website URL (RSS `<channel><link>` 値、サイト本体の URL) | `WebsiteURL` (Go field), `website_url` (db column) | `domain.Feed.WebsiteURL`, `models.Feed.WebsiteURL`, `feeds.website_url` 列 | **PR 2 (ADR-000868) で rename 完了**。「Link」は HTML/RSS 両方で使われ Article URL とも紛らわしい |
| RSS Subscription URL (ユーザが購読登録した RSS XML feed file の URL) | `URL` (Go field), `url` (db column) | `domain.Feed.URL` (`feed_links.url` から JOIN populate)、`feed_links.url` 列 | 既に canonical (`feed_links` table 側) |
| Article Source URL (article scrape 元の URL) | `URL` | `articles.url`, `domain.Article.URL` | 既に canonical |
| Recap Topic URL (Recap snapshot の HTTP route) | `Route` または `URL` | act_target.route | 既に canonical |

### なぜ `Link` を捨てるか

- HTML の `<link>` element と意味重複: HTML の `<link>` は metadata 記述子 (stylesheet 等) で URL ではない。命名に持ち込むと混同を生む
- 「link」は汎用名詞 (relation / hyperlink / connection) で domain noun として弱い
- PM-2026-041 で確認: producer / consumer / projector / DB / FE の 5 層で「link」と「url」が drift して silent failure を生んだ

---

## RSS Item Link カテゴリ (例外)

**RSS spec が定める `<link>` element** は wire schema を spec に合わせる必要があるため例外。Item-level (個別記事の URL) のみが対象 — Channel-level (`<channel><link>`) は Website URL カテゴリで `WebsiteURL` に rename 済 (PR 2)。

| 文脈 | canonical 名 | 該当箇所 | 備考 |
|---|---|---|---|
| RSS Item `<link>` element | `Link` (Go field、driver/gateway 内に閉じる) | `domain.RSSItem.Link`, `domain.FeedItem.Link`, `domain.TagTrail.Link`, gofeed `*Item.Link` | Item-level の RSS spec 準拠で `Link` 維持 |
| RSS Channel `<link>` element の値 | `WebsiteURL` (Go field、driver/gateway 出口で rename) | `domain.RSSFeed.Link` (RSS parser 内のみ) → `domain.Feed.WebsiteURL` (driver 出口以降) | RSS parser 内では Link、domain layer 以降は WebsiteURL に rename |

**境界規約**:
- RSS XML を parse する gofeed / driver / gateway 層 (例: `convertGofeedToDomain`) では `Link` を使ってよい
- domain entity / handler / usecase / DB / FE では:
  - **Item-level URL** → `Link` 維持 (RSS Item Link カテゴリ)
  - **Channel-level / Article URL** → `WebsiteURL` または `URL` に rename (Website URL / Article URL カテゴリ)

---

## 命名違反検出

CI で以下を検出する linter を追加する (PR 2 で執行):

```bash
# domain / handler / usecase / FE で "link" / "Link" がフィールド名・JSON キーとして出ていないか
grep -nE '"link"|\.Link\b|Link\s+string' \
  --include="*.go" --include="*.svelte" --include="*.ts" \
  alt-backend/app/domain/ \
  alt-backend/app/usecase/ \
  alt-backend/app/connect/ \
  alt-frontend-sv/src/lib/ \
  alt-frontend-sv/src/routes/
# RSS parser (driver/gateway) は除外
```

違反時は CI fail。例外はコメント `// allow:link-rss-spec` で抑止可能。

---

## マイグレーション ロードマップ

| PR | scope | status |
|---|---|---|
| **PR 1** ADR-000867 | Article URL カテゴリ全層: KnowledgeHomeItem.Link → URL (Go / proto / DB / BFF / FE) + 共通 ArticleCreatedPayload + ArticleUrlBackfilled corrective event | **完了** |
| **PR 2** ADR-000868 | Website URL カテゴリ: Feed.Link → WebsiteURL + feeds.link → website_url (Atlas migration + index/constraint rename) + 全 consumer 追従 + 命名違反 lint script | **完了** |
| **PR 3** (将来) | RSS parser 出口で driver→domain の境界マッピングを enforce する layer test | optional |

---

## 参考

- ADR-000865 (Superseded) — wire form `"url"` 統一の前段、producer 1/3 のみ修正だった不完全 fix
- ADR-000867 (本 PR で起こす) — Article URL ubiquitous-language 統一 + corrective event
- PM-2026-041 — wire-form drift 同型ヒヤリハット
- DDD: Eric Evans "Domain-Driven Design" Ch. 14 Ubiquitous Language
