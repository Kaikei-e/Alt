# Context Map

Alt は 20+ microservices の monorepo。各 bounded context の正準語 (ubiquitous language) は
それぞれの glossary に固定する。本マップは **context の所在と関係のみ** を持ち、用語定義は持たない。

> 2026-06-17: 単一 `CONTEXT.md` (Knowledge Trail glossary) を本マップへ昇格。
> 以降、新しい境界を grill するごとに `contexts/` 配下へ glossary を 1 つ追加する。

## Contexts

- [Knowledge Trail](./contexts/knowledge-trail.md) — Alt の primary surface。footprint の連なりを背骨に、
  型付き branch を提案する認知フィードバック循環。knowledge-sovereign / alt-backend / alt-frontend-sv にまたがる。
- [Resource Efficiency](./contexts/resource-efficiency.md) — _(定義中, 2026-06-17 grill)_ 構成・アーキテクチャを
  変えずに、ログと実測値からランタイムのリソース消費を right-size する **運用境界**。全サービス横断。
- [Visual Preview](./contexts/visual-preview.md) — _(定義中, 2026-06-22 grill)_ feed 記事を OG 画像カードグリッドで
  閲覧する surface。画像の取得・表示・欠落の語彙 (transient fallback vs absent image) を固定。alt-frontend-sv / alt-backend image proxy にまたがる。

## Relationships

- **Resource Efficiency は全 context に対し cross-cutting**: 各 context が吐く観測信号
  (OTEL ログ・Prometheus metrics・nginx access log・`docker stats`) を消費し、それらのランタイム footprint を
  制約する。ドメイン語彙は共有しない (Knowledge Trail の footprint と Resource Efficiency の measurement は別物)。
- **Knowledge Trail は旧 Knowledge Loop を置換** (2026-06-10)。イベント基盤・relation 語彙・evidence 供給機構を継承。
- **Resource Efficiency → Visual Preview**: image proxy の per-host レート制限 (上流ホスト保護) と OG 画像 age-gate は
  Resource Efficiency の保持・cadence 規律に従う。Visual Preview はその制約下で transient fallback を retry で吸収する。
