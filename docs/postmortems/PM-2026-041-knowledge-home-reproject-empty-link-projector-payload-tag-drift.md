# ポストモーテム: Knowledge Home Reproject swap 後に knowledge_home_items.link が全行空となり article カードから記事を開けなくなった潜伏バグ

## メタデータ

| 項目 | 内容 |
|------|------|
| インシデント ID | PM-2026-041 |
| 発生日時 | 不明 — `articleCreatedPayload` に `Link string \`json:"link"\`` が追加された時点 (commit `2718ef416`「remove alt-db sovereign repositories, gateways, and dual-write path」前後、推定 2026-03-23 周辺) から潜伏 |
| 検知日時 | 2026-04-28 (JST) — operator が `/admin/knowledge-home` から Reproject (compare → swap) 直後に Knowledge Home `/home` の article カードを click した瞬間 |
| 復旧日時 | 2026-04-28 (JST) — ローカル fix landed。production 反映は別途 `git push origin main` → dispatch-deploy 経路を要する |
| 影響時間 | 顕在化からローカル fix まで本セッション内に完結。潜伏期間は数週間〜数か月の可能性 |
| 重大度 | SEV-4 (ヒヤリハット — admin operator の Reproject swap 後にしか顕在化しない、データ損失なし、live projection 経由のエンドユーザ機能には影響なし) |
| 作成者 | alt-backend / alt-frontend-sv 担当 |
| レビュアー | (Pending) |
| ステータス | Draft |

## サマリー

`alt-backend/app/driver/mqhub_connect/client.go` の event producer (`ArticleCreatedPayload.URL` フィールド、`json:"url"`) が書き込む payload を、`alt-backend/app/job/knowledge_projector.go` の consumer (`articleCreatedPayload.Link` フィールド、`json:"link"`) は別キーとして読んでおり、`payload.Link` は常に空文字となっていた。`domain.KnowledgeHomeItem.Link = ""` が `knowledge-sovereign/app/driver/sovereign_db/repository.go` の upsert を経て `knowledge_home_items.link` カラム (TEXT NOT NULL DEFAULT '') に空文字として書かれ続けていた。merge-safe upsert の `link = CASE WHEN EXCLUDED.link != '' THEN EXCLUDED.link ELSE knowledge_home_items.link END` が live projection の既存値 (何らかの過去の backfill / 旧 dual-write 由来) を温存していたため live では誰も気づかなかったが、Reproject は `(user_id, item_key, projection_version)` ユニークキーで v_new の新行を作成するため preserve 分岐が空振りし、v_new の article 行が全件 `link=''` になった。swap 後 `getKnowledgeHome` レスポンスで全 article の `link` が空となり FE が `Article link is not available yet.` トースト + `DESK // ERROR` kicker を表示した。本ヒヤリハットは admin 経路依存の症状でデータ損失・エンドユーザ影響はない。projector の struct tag を canonical wire schema (`json:"url"`) に揃え、wire-form contract test と FE 防御層を追加して同型バグの再発を構造的に塞いだ。

## 影響

- **影響を受けたサービス**: `alt-backend` の `KnowledgeProjectorJob`、副次的に `alt-frontend-sv` の `/home` 画面 error toast surface
- **影響を受けたユーザ**: admin operator が swap した後の Knowledge Home ユーザ全員。live projection 経由ではこの不具合は発生しない
- **機能への影響**: Reproject swap 直後の v_new 上で、Knowledge Home の article カードを click すると全件 `Article link is not available yet.` トーストに落ちる。記事閲覧経路 (`/articles/{id}?url=...`) への navigate ができない
- **データ損失**: なし。`knowledge_events` event log は完全 (link は payload に `"url"` キーで全部入っている)、`articles.url` も無傷。projection の disposable 性質により、projector tag fix 後の Reproject 再実行で完全復旧可能
- **SLO/SLA 違反**: なし (admin 経路依存の症状)

## タイムライン

全時刻は JST。バグ混入時刻は不明。

| 時刻 (JST) | イベント |
|------------|---------|
| 不明 (推定 2026-03-23 周辺) | commit `2718ef416` で `articleCreatedPayload` に `Link string \`json:"link"\`` が追加。producer (`mqhub_connect.ArticleCreatedPayload.URL string \`json:"url"\``) との JSON-tag drift が混入。**潜伏バグ混入**。live projection は何らかの履歴値が CASE preserve で温存され症状なし |
| 2026-04-28 セッション中 | operator が `/admin/knowledge-home` で Reproject (compare → swap) を実行。Knowledge Home `/home` で article カードを click → `Article link is not available yet.` トースト + `DESK // ERROR` kicker を観測 → **検知** |
| 同日 即時 | 経路逆引き — FE `+page.svelte:181-188` toast 起点 → BFF `convertHomeItemToProto:125` `Link: item.Link` → 上流 `read_projections.go:103` `COALESCE(khi.link, '') AS link` → projector `knowledge_projector.go:319` `Link: payload.Link` → producer `mqhub_connect/client.go:77` `URL string \`json:"url"\`` の経路を確認。**JSON-tag mismatch を特定** |
| 同日 | 全 event payload struct の producer↔consumer JSON-tag parity を audit。drift があるのは `ArticleCreated` のみ。他は全部 parity OK。`docs/review/knowledge-event-payload-tag-audit-2026-04-28.md` (gitignored local note) に記録 |
| 同日 | TDD で wire-form contract test (`TestArticleCreatedPayloadContract_LinkRoundTrips`) を RED で追加 → projector struct field を `URL string \`json:"url"\`` に rename + site-of-use を `Link: payload.URL` に → GREEN。`knowledge_backfill_job.GenerateBackfillEvent` の struct 初期化子も追従 |
| 同日 | FE 防御層追加 — `KnowledgeCard.svelte` に `linkAvailable` derived state、`Archived · No source URL projected` kicker、`data-testid="kh-card-link-unavailable"`、`console.warn` once-per-item-key 診断を追加 |
| 同日 | Phase 5 CI parity 全 GREEN — alt-backend (gofmt / go vet / golangci-lint / go test ./...)、knowledge-sovereign (gofmt / vet / test)、alt-frontend-sv (lint / format / check / test:client) |
| 2026-04-28 後 (TODO) | ADR-000865 採番後にユーザ承認 → `git push origin main` → `dispatch-deploy.yaml` → `Kaikei-e/alt-deploy` で production 反映。反映後に operator が Reproject を再実行して v_active の link を全件回復 |

## 検知

- **検知方法**: ユーザ (admin operator) によるブラウザ操作中の手動観測 (article カード click → `Article link is not available yet.` トースト)
- **検知までの時間 (TTD)**: バグ混入から数週間〜数か月 (潜伏)。本日の swap 操作からは数秒
- **検知の評価**: 不十分。
  - producer↔consumer の wire-form contract をテストする層が存在しなかった。既存の `TestKnowledgeProjectorJob_ArticleCreated_LinkPropagation` は consumer struct で marshal してから unmarshal する形だったので tag drift では fail しない false-confidence パターン
  - integration / E2E (Hurl) で Reproject swap 後の link が non-empty かを assert するシナリオがなかった
  - admin 系 reproject swap の post-swap sanity check (`SELECT count(*) FROM knowledge_home_items WHERE projection_version=<v_new> AND link='';`) を runbook が要求していなかった (関連: PM-2026-010 D-3 のアクションアイテム未完)
  - Grafana / Prometheus に admin 経路の記事 click 失敗を可視化する dashboard がなかった

## 根本原因分析

### 直接原因

`alt-backend/app/job/knowledge_projector.go` の `articleCreatedPayload` 構造体が `Link string \`json:"link"\`` だったが、producer (`alt-backend/app/driver/mqhub_connect/client.go` および `search-indexer/app/consumer/event_handler.go` の同名 struct) は `URL string \`json:"url"\`` で marshal していた。projector の `json.Unmarshal` は wire 上に存在しない `"link"` キーを探して見つけられず、`payload.Link` は常に Go zero value (`""`) のまま `domain.KnowledgeHomeItem.Link` に流れ、`knowledge_home_items.link` カラムへ空文字が書かれた。

live projection では `repository.go:156` の merge-safe upsert に preserve-on-empty CASE 分岐があり、既存行の何らかの過去の backfill / 旧 dual-write 由来の値を温存していたため、後続の空文字書き込みが既存値を塗り潰さなかった。Reproject は `(user_id, item_key, projection_version)` をユニークキーに持つため、新しい `projection_version` で初めて INSERT される行に preserve 対象は存在しない。結果として v_new の article 行が全件 `link=''` となった。

### Five Whys

1. **なぜ Reproject swap 後に article が開けなくなったか？** → FE が `item.link` 空文字判定で error toast に分岐したから
2. **なぜ `item.link` が空文字だったか？** → BFF / sovereign の `KnowledgeHomeItem.link` proto field が空文字で、`knowledge_home_items.link` カラムが空文字だったから
3. **なぜ v_new 行の `link` カラムが空文字だったか？** → projector が `payload.Link` を空文字で書いたから (CASE preserve は新行作成時に空振り)
4. **なぜ projector の `payload.Link` が空文字だったか？** → producer が JSON wire 上に `"url"` キーで書き、consumer struct は `"link"` キーで unmarshal を試みていたから (struct tag drift)
5. **なぜ tag drift がここまで温存されたか？** → (a) live projection が CASE preserve upsert で過去の何らかの backfill 経由で埋まった link を持ち回っていたため症状が出なかった、(b) 既存 unit test (`TestKnowledgeProjectorJob_ArticleCreated_LinkPropagation`) が consumer struct を marshal してから unmarshal していたので JSON-key の生形を検証できなかった、(c) producer (mqhub_connect) と consumer (knowledge_projector) を跨ぐ wire-form contract test が存在しなかった、(d) Reproject swap 後の post-swap sanity check が runbook 化されていなかった

### 根本原因

複数層に分散した silent contract: producer の struct tag (mqhub_connect) / wire bytes / consumer の struct tag (knowledge_projector) / merge-safe upsert の CASE preserve / live projection 内の "歴史的 backfill 値" / 既存 unit test の自己 round-trip 形式 — のいずれもが個別には正しく動作していたが、5 層を跨ぐ「producer が書いた wire form を consumer が同じキーで読む」という暗黙の契約は誰の責務として明示されていなかった。型システム / コンパイラ / linter / unit test (consumer struct で round-trip するもの) のいずれもこの semantic gap を検出できない。PM-2026-040 (JSONB NOT NULL の DB schema / Go struct / proto bytes / pgx wire 4 層) と同型のクラス問題。

### 寄与要因

- **live projection の CASE preserve upsert が drift を覆い隠す役回りになった**: 本来は重複 event の冪等性向上を意図する merge-safe upsert が、ここでは「歴史的に埋まっていた link の温存装置」として働き、live で症状が出ない仕組みを構造的に提供してしまった
- **既存 unit test が wire-form contract を検証していなかった**: `TestKnowledgeProjectorJob_ArticleCreated_LinkPropagation` という名前の test が consumer struct で marshal してから unmarshal する形だったため、struct tag が drift しても fail せず false-confidence になっていた
- **PM-2026-010 D-3 (post-swap sanity check) と R-1 (projection_version 単位 checkpoint) の action item が未着手のまま**、同型「reproject swap 直後の不整合」が異なる field で再発した
- **Knowledge Home の admin reproject 操作の使用頻度が低い**: projection_version major bump 時にしか実行されない運用機能で、日常的なヘルスチェック対象ではなかったため silent failure が誰にも踏まれずに長期潜伏した
- **producer / consumer の struct を pkg レベルで共有しない設計判断** (`mqhub_connect` / `knowledge_projector` / `search-indexer/consumer` で各々 struct を再宣言する) が drift の温床になっていた

## 対応の評価

### うまくいったこと

- FE の toast 文言 (`Article link is not available yet.`) が user-visible で具体的だったため、operator が即座に「link が無い」と認識し、原因経路 (FE → BFF → sovereign → projector → producer) を逆引きできた
- ブラウザ DevTools Network で `getKnowledgeHome` レスポンス内の `link` field が全行空であることを直接確認でき、症状と原因が一直線で繋がった
- TDD で wire-form contract test (`TestArticleCreatedPayloadContract_LinkRoundTrips`) を RED → projector struct tag fix → GREEN の順で landed させ、再発防止防波堤を CI に積めた
- producer↔consumer JSON-tag parity を全 event 種別で audit し、他に同型 drift が存在しないことを構造的に確認した
- イミュータブルデータモデルの「disposable projection」設計のおかげで、projector fix 後に Reproject を再実行するだけで v_active の link が全件回復可能。event log と `articles.url` は無傷、復旧パスは既存の runbook で吸収可能
- FE 防御層 (`Archived · No source URL projected` kicker + `data-testid` + `console.warn`) を追加することで、同種 regression が将来再発しても black-box でなく可視化される
- 検知から fix まで本セッション内で完結。audit + 修正 + テスト + Phase 5 CI parity (alt-backend / knowledge-sovereign / alt-frontend-sv) すべて GREEN

### うまくいかなかったこと

- バグが数週間〜数か月潜伏した。live で踏まれない silent failure を構造的に検出する仕組みが無かった
- `TestKnowledgeProjectorJob_ArticleCreated_LinkPropagation` という名前の test は存在したのに、wire-form を検証する形になっていなかった (false sense of safety)
- PM-2026-010 で提案された D-3 (post-swap sanity check) と R-1 (projection_version 単位 checkpoint) の action item が未着手のまま、同種「reproject swap 直後の不整合」が異なる field で再発した
- producer と consumer の struct を pkg レベルで共有しない設計判断が drift の温床になっていた

### 運が良かったこと

- 影響範囲が admin operator が swap した後の Knowledge Home article 経路に限定された。live projection は無傷で、エンドユーザ向け機能 (`/feeds` / `/loop` / `/augur` / `/recap` 等) の hot path には一切影響しなかった
- event log と `articles.url` が無傷だったので、disposable projection の rebuild 一発で完全復旧できる構造だった
- 検知から fix まで本セッション内で完結 (audit + 修正 + テスト + Phase 5 CI parity すべて GREEN)
- `af79c171f` (LoopEntryTile.safeRecapRoute open-redirect closure) と同セッションで本件も検知できたため、`/articles/{id}?url=…` 周りの allowlist 整備を follow-up としてスタックできる

## アクションアイテム

| # | カテゴリ | アクション | 担当 | 期限 | ステータス |
|---|----------|-----------|------|------|-----------|
| 1 | 予防 | 本 fix を `git push origin main` → dispatch-deploy 経路で production 反映 | alt-backend / alt-frontend-sv 担当 | 2026-04-29 | TODO |
| 2 | 予防 | production 反映後、operator が `/admin/knowledge-home` から Reproject (dry_run → compare → swap) を再実行し v_active の link を全件回復。compare の diff は ≈100% link 追加が *expected* outcome である旨を runbook に明記 | operator | 2026-04-30 | TODO |
| 3 | 予防 | `docs/runbooks/knowledge-home-reproject-operations.md` に「post-tag-fix backfill」サブセクションを追記し、本件のような latent payload-tag bug を fix した直後は Reproject を強制する手順 + 事前 SQL sanity (`SELECT count(*) FROM knowledge_home_items WHERE projection_version=<v_new> AND item_type='article' AND link='';` が ≪ pre-fix であること) を必須化 | alt-backend / docs 担当 | 2026-05-05 | TODO |
| 4 | 検知 | `e2e/hurl/knowledge-sovereign/` に Reproject 経路 (alt-backend 同梱) で article event → projection.link round-trip を assert するシナリオを追加。現状 knowledge-sovereign 単体 stack なので、alt-backend を含む staging プロファイルで実行可能か確認 | e2e 担当 | 2026-05-12 | TODO |
| 5 | 検知 | Reproject swap 直後に `(v_old, v_new)` で `count(link='') / count(*)` の比を返すジョブを追加し、閾値超過で alert (PM-2026-010 D-3 の再掲) | observability 担当 | 2026-05-19 | TODO |
| 6 | 検知 | projector の event payload struct に対する wire-form contract test を全 event 種別で網羅。今回 `ArticleCreated` にしか積んでいないが、次に producer 側で field 追加 / rename が起こる時にも防波堤が効くように一般化 | alt-backend 担当 | 2026-05-19 | TODO |
| 7 | プロセス | producer struct (`mqhub_connect`) と consumer struct (各 projector) の field 名対応表を `docs/services/alt-backend.md` に常設掲載。audit doc 自体は gitignored なので参照テーブルのみ tracked 領域に集約 | docs 担当 | 2026-05-12 | TODO |
| 8 | プロセス | `/articles/{id}?url=…` の URL allowlist (open-redirect / SSRF defense in depth) を `af79c171f` LoopEntryTile.safeRecapRoute と並列に整備 | alt-frontend-sv 担当 | 2026-05-19 | TODO |
| 9 | プロセス | event payload struct を producer / consumer 共通パッケージに集約する設計を別 ADR で議論。共有パッケージ化の trade-off (proto / Go struct 一元化 vs 各サービスの autonomy) を整理 | アーキテクチャ担当 | 2026-06-02 | TODO |

## 教訓

### 技術的教訓

- **「自己 round-trip テスト」は contract テストではない**: `consumer_struct → json.Marshal → json.Unmarshal → consumer_struct` は struct tag が一致しているかではなく、struct tag が *自分自身と* 一致しているかしか検証しない。Producer↔consumer 間の wire-form contract を検証するには、test 内で raw `map[string]any` か producer 側の struct で marshal して consumer struct で unmarshal する必要がある。`TestKnowledgeProjectorJob_ArticleCreated_LinkPropagation` の名前と振る舞いの乖離は今後発見されるべき false-confidence パターン
- **Merge-safe upsert の CASE preserve は両刃の剣**: 「既存値が非空ならそれを温存」は冪等性 / 順序非依存性に対して正しいが、初回 INSERT 時の payload bug を覆い隠してしまう。Reproject の `(user_id, item_key, projection_version)` ユニークキー設計と組み合わせると、新 projection_version で初回 INSERT が「空文字書き込み」になっても CASE preserve 分岐が空振りで気づけない。**「live でしか preserve が効かないフィールド」は reproject-safety 違反の早期警報**
- **disposable projection は復旧の保険になる**: event log と producer 側 source-of-truth (`articles.url`) が無傷であれば、projector fix → Reproject 再実行で完全復旧できる。本件は immutable invariants §5 (Disposable projection) の恩恵を最大限受けた。projection を source of truth に昇格させない設計判断が後から効いた
- **Silent contract は層を跨ぐ**: producer struct tag / wire bytes / consumer struct tag / DB upsert CASE / live preserve 値 — どの層も単独では正しく動作するが、層を跨ぐ「同じキーで読み書きする」契約は型システムでは表現できない。**Driver / projector layer に defense-in-depth wire-form contract test を積むのが現実解** (PM-2026-040 の `emptyJSONIfNil` helper と同じ思想)

### 組織・プロセス的教訓

- **PM-2026-010 D-3 (post-swap sanity check) の未完了が再発を呼んだ**: 同型「reproject swap 直後の不整合」が異なる field で再発した。アクションアイテムの追跡を強化すべき (これは PM-2026-040 の教訓と同じ)
- **使用頻度の低い admin 機能は腐る**: Reproject swap は projection_version major bump 時にしか実行されない。日常的に動かないコードパスは、CI / production の両方で「実際に動かす」E2E テストが必要
- **「struct field 名と JSON tag が同じ」という暗黙の前提を捨てる**: Go の struct tag は wire schema の真実。フィールド名と tag が乖離していても compiler は何も言わない。レビュー観点として「producer 側 struct と consumer 側 struct の tag を field-by-field で比較する」を毎度持ち込む

## 参考資料

- 修正 commit: TBD (本ヒヤリハットを fix する commit を本 PM landed 後に link)
- 関連 ADR: [[000865]] Knowledge Home projector の event payload wire schema を producer に揃え reproject-safety を回復する
- 関連 ADR: [[000421]] Knowledge Home reproject 運用
- 関連 ADR: [[000859]] canonical-contract 不変条件の防波堤テスト
- 関連 PM: [[PM-2026-040-knowledge-home-start-reproject-jsonb-not-null-violation-latent-since-projection-versions]] (同種 silent multi-layer contract drift)
- 関連 PM: [[PM-2026-010-knowledge-home-reproject-checkpoint-gap]] (同種 live ↔ reproject divergence)
- 関連 audit: `docs/review/knowledge-event-payload-tag-audit-2026-04-28.md` (gitignored local working note — producer↔consumer JSON-tag parity 全 event 種別)
- 関連 fix (本セッション、別件): `af79c171f` LoopEntryTile.safeRecapRoute open-redirect closure (`/articles/{id}?url=…` 経路の allowlist は本件と並列に follow-up)

---

> **Blameless Postmortem の原則:** このドキュメントは個人の過失を追及するためではなく、
> システムの脆弱性とプロセスの改善機会を特定するために作成されています。
> 「誰が悪いか」ではなく「システムのどこが改善できるか」に焦点を当ててください。
