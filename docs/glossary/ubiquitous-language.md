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

| 文脈 | canonical 名 | 該当箇所 (本 PR で執行) | 備考 |
|---|---|---|---|
| Article URL (Knowledge Home item の article 元 URL) | `URL` (Go field), `url` (proto / db column / wire) | `domain.KnowledgeHomeItem.URL`, `proto/.../knowledge_home.proto` field 13, `knowledge_home_items.url` 列 | **本 PR 1 で rename**。歴史的に `Link` と呼んでいたが PM-2026-041 / 今回バグの真因 |
| Article URL (event payload) | `URL` (Go field), `url` (wire key) | `domain.ArticleCreatedPayload.URL`, `domain.ArticleUrlBackfilledPayload.URL` | 本 PR 1 で確立。canonical |
| Feed Subscription URL (ユーザが購読登録した RSS feed の URL) | `URL` (Go field), `url` (proto / db / wire) | `domain.Feed.URL` (現状 `Feed.Link`)、`feeds.url` 列 (現状 `feeds.link`) | **PR 2 で rename** schedule。命名ミスだが本 PR scope 外 |
| Article Source URL (article scrape 元の URL) | `URL` | `articles.url`, `domain.Article.URL` | 既に canonical |
| Recap Topic URL (Recap snapshot の HTTP route) | `Route` または `URL` | act_target.route | 既に canonical |

### なぜ `Link` を捨てるか

- HTML の `<link>` element と意味重複: HTML の `<link>` は metadata 記述子 (stylesheet 等) で URL ではない。命名に持ち込むと混同を生む
- 「link」は汎用名詞 (relation / hyperlink / connection) で domain noun として弱い
- PM-2026-041 で確認: producer / consumer / projector / DB / FE の 5 層で「link」と「url」が drift して silent failure を生んだ

---

## RSS Item Link カテゴリ (例外)

**RSS spec が定める `<link>` element** は wire schema を spec に合わせる必要があるため例外。

| 文脈 | canonical 名 | 該当箇所 | 備考 |
|---|---|---|---|
| RSS XML `<link>` element | `Link` (Go field、RSS parser 内に閉じる) | `domain.RSSFeed.Link`, `domain.RSSItem.Link` | RSS 2.0 / Atom spec 準拠で `Link` 維持。**ただし parser 出口で `URL` に rename して以降は URL** |
| RSS feed XML 本体の URL | `FeedURL` (Go field) | 現状 `FeedLink` → PR 2 で rename | meta vs item の混同を避ける |

**境界規約**: RSS XML を parse する driver / gateway 層では `Link` を使ってよい。**それ以外 (handler / usecase / domain entity / DB / FE) では URL に rename して扱う**。境界は driver/gateway の return 文。

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

| PR | scope | 執行タイミング |
|---|---|---|
| **PR 1 (本セッション)** | Article URL カテゴリ全層: KnowledgeHomeItem.Link → URL (Go / proto / DB / BFF / FE) + 共通 ArticleCreatedPayload + ArticleUrlBackfilled corrective event | 即 |
| **PR 2** | Feed Subscription URL: Feed.Link → URL (Go / DB / handler / gateway / FE) + 命名違反 lint 追加 | follow-up (`/schedule` で teeup 予定) |
| **PR 3** (将来) | RSS parser 出口で Link → URL の rename を強制する layer 設計 | optional |

---

## 参考

- ADR-000865 (Superseded) — wire form `"url"` 統一の前段、producer 1/3 のみ修正だった不完全 fix
- ADR-000867 (本 PR で起こす) — Article URL ubiquitous-language 統一 + corrective event
- PM-2026-041 — wire-form drift 同型ヒヤリハット
- DDD: Eric Evans "Domain-Driven Design" Ch. 14 Ubiquitous Language
