# PM-2026-043: Inoreader OAuth token volume の空ファイル化による 3 日サイレント停止（PM-2026-042 二次被害 / near-miss）

## メタデータ

| 項目 | 値 |
|------|-----|
| インシデントID | PM-2026-043 |
| 重大度 | SEV-3（near-miss。Inoreader は補完経路でありユーザー操作起点の主経路には影響なし。一方、観測 hook の不在で 3 日間気付けなかった点は SEV-3 相当） |
| 発生日時 | 2026-05-06 07:56 (JST) — Inoreader 取り込みの最終成功時刻（推定の停止開始時刻） |
| 検知日時 | 2026-05-09 昼頃 (JST) — オペレータが host disk の再増加を手動 `df` で観測 |
| 復旧日時 | 2026-05-09 01:01 (JST、auth-token-manager の token 再保存ログ) — その後 16 分以内に circuit breaker CLOSED / sync_state 前進を確認 |
| 影響期間 | 約 65 時間（2026-05-06 07:56 〜 2026-05-09 01:01 JST） |
| 影響サービス | pre-processor-sidecar、auth-token-manager（共有 `oauth_token_data` volume）、間接的に下流 tag-generator / search-indexer |
| 影響機能 | Inoreader 経由の RSS 取り込み（補完経路）。ユーザー操作起点の主経路は無影響 |
| 関連 ADR | [[000895]] Inoreader 取り込みのサイレント停止を検出可能にする、[[000602]] Tier1 入口フィルタ |
| 関連 PM | [[PM-2026-042-staging-projector-content-type-loop-disk-fill]]（一次原因。本 PM は二次被害） |
| 作成者 | オンコール担当者 |
| ステータス | Approved |

## サマリー

2026-05-09 昼頃、オペレータが host disk 使用量を手動 `df` で確認したところ、前日のクリーンアップ後ベースライン約 560GB に対して 633GB と 24 時間以内に 73GB 増えていた。調査の結果、増分は ClickHouse の OOM-restart ループによるコンテナログ肥大であったが、**より深刻なのは Inoreader 取り込みが 2026-05-06 07:56 (JST) を最後に 65 時間サイレントで停止していた** 事実が同時に判明した点である。直接原因は、`oauth_token_data` Docker volume 上の `oauth2_token.env` が **0 byte (空ファイル)** になっていたこと。auth-token-manager が `/api/token` で `HTTP 404 {"error":"No token data found"}` を返し続け、pre-processor-sidecar の circuit breaker が `OPEN ↔ HALF_OPEN` を 16 分間隔で永久往復していた。ファイル消失は PM-2026-042 の disk full イベント (2026-05-04 〜 05-07) の最中に永続化 write が ENOSPC で失敗した結果と推定される。データ損失・ユーザー影響なし。

## 影響

- **影響を受けたサービス**: pre-processor-sidecar (Inoreader 取り込み)、auth-token-manager (token 配信)、間接的に Inoreader 経由の下流 tag-generator / search-indexer
- **影響を受けたユーザー数 / 割合**: 0（ユーザー操作起点の主経路は無影響、Inoreader は補完経路）
- **機能への影響**: Inoreader 経由の自動取り込みが完全停止。`sync_state.last_sync` は 2026-05-06 07:56 JST で凍結
- **データ損失**: なし（`sync_state.continuation_token` は 12 文字でそのまま保持されており、再認証後に当該カーソルから再開可能）
- **SLO/SLA 違反**: なし（Inoreader は補完経路、SLO 外）
- **副次影響**: ClickHouse の OOM-restart ループが log cap 未設定のため 24h で約 73GB を消費していた（disk 流出は本 incident と同根の PM-2026-042 二次被害の片割れ）
- **潜在的影響**: 観測 hook が不在のまま放置されれば 取り込み停止が **無期限に継続** し、Inoreader 側の `continuation_token` が rotate される段階でカーソル整合性も失う可能性があった

## タイムライン

| 時刻 (JST) | イベント | 確度 |
|---|---|---|
| 2026-05-04 〜 05-07 | PM-2026-042: ステージング slice (識別子伏字) の alt-backend Knowledge Projector が content-type ループで Docker host のコンテナログを推定 148GB 生成、disk full 直前まで圧迫 | 確定（[[PM-2026-042-staging-projector-content-type-loop-disk-fill]]） |
| 2026-05-06 07:56 (推定) | `oauth_token_data:/app/secrets/oauth2_token.env` への write が ENOSPC で失敗、ファイル truncate された状態で 0 byte 化（auth-token-manager の write 経路は当時 atomic でなかった） | 推定（disk 解放後にファイル size 0 を確認、write タイミングの直接ログは消失） |
| 2026-05-06 07:56 | pre-processor-sidecar の最終成功 fetch（`sync_state.last_sync` が以降凍結） | 確定（pre-processor-db `sync_state` テーブル） |
| 2026-05-06 07:56 〜 2026-05-09 01:01 | auth-token-manager が `"No valid refresh token found - waiting for user authorization"` を 5 分ごとに warn 出力、pre-processor-sidecar の circuit breaker が OPEN ↔ HALF_OPEN を 16 分ごとに往復。観測者ゼロ | 確定（両コンテナのログから） |
| 2026-05-07 | PM-2026-042 の P-1 / P-2 修正適用（commit `980588543` / `ca1b4ba86`）。disk 流出は止まったが、token volume の空ファイル化は誰も気付かないまま継続 | 確定 |
| 2026-05-08 | オペレータが手動で host disk 整理、約 560GB まで縮減 | 確定（オペレータ報告） |
| 2026-05-09 昼頃 | **検知** — オペレータが host disk が 633GB に再増加していることを `df` で観測。「直近のホストマシンの Disk 容量が満杯になった」を仮説として調査開始 | 確定 |
| 2026-05-09 昼頃 | 並行調査で次が判明: (a) ClickHouse コンテナの RestartCount=85 / json-file driver options=`{}` で log 流出継続中、(b) auth-token-manager `/api/token` が 404、(c) `oauth2_token.env` が 0 byte、(d) pre-processor-sidecar が circuit breaker OPEN ループ、(e) 既存 `-health-check` は `token_manager_available: true` を返し続けていた | 確定 |
| 2026-05-09 午後 | **緩和策適用**: ClickHouse compose に `logging` cap (json-file 10m × 3) と `mem_limit: 2g → 4g` を反映、`docker compose up -d clickhouse` で適用（commit `65e6836b2`） | 確定 |
| 2026-05-09 午後 | TDD で RED テスト追加（`remote_token_service_test.go` 5 ケース、`admin_api_handler_health_test.go` 4 ケース）、`service.ErrTokenUnavailable` typed sentinel と `IsDegraded()` 60s × 3 window、`/admin/health` を実装、`go test ./...` GREEN 確認、pre-processor-sidecar コンテナ再ビルド・再起動（commits `4ade8f77b` / `46fb99dd4`） | 確定 |
| 2026-05-09 01:01 | **復旧**: ブラウザマシンから SSH port-forward (`-L 9201:localhost:9201`) でトンネリングし、auth-token-manager の `/` から OAuth 開始、Inoreader で再承認、`/callback` がトンネル経由で着弾、`oauth2_token.env` 再生成。auth-token-manager ログに `"Authorization completed - tokens stored"` を確認 | 確定 |
| 2026-05-09 01:01 直後 | 手動 `/admin/trigger/article-fetch` で fetch 再開、3 streams × 100 articles を取得、`/admin/health` が `status=ok / token_available=true / circuit_breaker_state=CLOSED / ingestion_silent=false` に flip | 確定 |
| 2026-05-09 | ADR-000895 として恒久対策を記録、本 PM 起票 | 確定 |

## 検知

- **検知方法**: 手動 `df` によるオペレータ気付き。アラート駆動ではない
- **検知までの時間 (TTD)**: 約 65 時間（取り込み停止開始から検知まで）
- **検知の評価**: 著しく遅い。次の 3 つの観測 hook が不在だった
  1. `pre-processor-sidecar -health-check` は token ファイルが空でも `token_manager_available: true` を返し続けた（既存実装は「token 配信サービスが起動しているか」しか見ておらず、ファイル中身の妥当性を検証していなかった）
  2. `sync_state.last_sync` のスタックを能動的に監視する metric / alert が未整備
  3. auth-token-manager 404 が `auth-token-manager returned status: 404` という opaque な string error として伝搬しており、test でも runtime でも「token 消失」を分岐できなかった
- **副次的な気付き**: PM-2026-042 修正後も ClickHouse の log cap 未設定経路が残っていたことが、本 incident の disk 再増加（=偶然の検知トリガー）として機能した。ある意味で「悪い observability が良い observability の代用になった」が、これは依存すべきではない

## 根本原因分析

### 直接原因

`oauth_token_data` Docker volume 上の `/app/secrets/oauth2_token.env` が 0 byte（空ファイル）になっていた。auth-token-manager の `EnvFileSecretManager.getTokenSecret()` が空内容を「`{access_token,refresh_token}` 双方欠如」と解釈して `null` を返し、`/api/token` ハンドラがそれを `HTTP 404 {"error":"No token data found"}` に変換。pre-processor-sidecar はこの 404 を opaque な string error で受け取り、`circuit breaker` は `FailureThreshold=3` で OPEN、`Timeout=60s` で HALF_OPEN に遷移するが次の試行も同じ 404 で再 OPEN、を 16 分間隔で永久反復。

### Five Whys

1. **なぜ Inoreader 取り込みが 3 日間止まっていたか？** → auth-token-manager が token を配信できず、pre-processor-sidecar が circuit breaker OPEN で永久ループに入っていたから
2. **なぜ auth-token-manager は token を配信できなかったか？** → 永続化先の `oauth2_token.env` が 0 byte で、access_token / refresh_token の両方が欠如していたから
3. **なぜ token ファイルが 0 byte 化したか？** → PM-2026-042 の disk full イベントの最中に、token refresh の write が ENOSPC で truncate された後に rename / fsync されない実装で、ファイルだけ「開いて空にした」状態が永続化されたと推定される（write の atomicity 不足）
4. **なぜ 3 日間誰も気付かなかったか？** → 取り込み停止を観測する手段が 3 つともサイレントだった: (a) 既存 health check が token ファイル中身を見ていない、(b) `sync_state.last_sync` の staleness を監視する metric がない、(c) auth-token-manager 404 が typed error 化されておらずユニットテストでも runtime でもアラート条件を作れなかった
5. **なぜそうした観測 hook が不在のまま放置されていたか？** → Inoreader が「補完経路（ユーザー操作主経路）」と位置付けられ、サイレント停止しても直接ユーザーに見えないため、観測投資の優先度が下がっていた。同時に「token が消失する」という failure mode が運用設計時に想定されておらず、token 配信失敗パターンを test で直接 exercise する習慣がなかった

### 根本原因

二層構造で記述する:

- **永続化境界の write atomicity 不足**: `oauth2_token.env` の更新が tmpfile + rename / fsync の組み合わせになっておらず、ENOSPC や crash 時にファイルだけ truncate される（中身が消える）状態が永続化される設計だった
- **取り込み層の観測ギャップ**: 「取り込みがサイレントに止まる」シナリオに対する観測 hook（typed sentinel、staleness metric、health endpoint）が一つも存在せず、3 日間気付けない構造になっていた

### 寄与要因

- **PM-2026-042 の二次被害**: disk full イベントが永続化境界の脆弱性を露出させた。PM-2026-042 自体は staging 限定の near-miss だったが、共有 host のため `oauth_token_data` ボリュームへの write も巻き込まれた
- **ClickHouse 側の log cap 未設定**: PM-2026-042 修正後も `compose/db.yaml` の clickhouse サービスは `logging` 未指定のままで、OOM-restart ループによる log 流出が続いていた。今回の incident 検知の偶発的トリガーになった一方、それ自体が独立した disk 流出源だった
- **`-health-check` の責務の曖昧さ**: 既存 health check は「依存サービスが起動しているか」のみを見ていた。「依存サービスが正しい応答を返すか」は別の責務として未分離だった

## 対応の評価

### うまくいったこと

- **TDD ファースト**: 復旧過程で typed sentinel と health endpoint を入れる際、RED テストを先に書いて GREEN にする workflow を厳守できた。9 ケース全 GREEN を確認後にコンテナを差し替えた
- **コミット粒度**: chore (compose) → fix (typed error) → feat (health endpoint) → docs (ADR) と意味単位で分けて記録できた
- **PM-2026-042 との連結**: 同じ disk full イベントの二次被害として連結記録でき、Five Whys が両 PM で整合した
- **流出停止を先に**: ClickHouse log cap を OAuth 復旧より先に適用したため、復旧作業が長引いても disk 危機が悪化しない順序で進められた
- **手動 rollback 不要**: コード変更は前方互換のみ（既存 token 経路を破壊しない adapter 設計）、roll-forward だけで完結

### うまくいかなかったこと

- **検知が手動 `df` 駆動**: PM-2026-042 で既に「disk pressure alarm 不在」が指摘されていたが、本 incident でも依存先のままだった。PM-2026-042 の D-1/M-1 が未実装である事実を本 incident が再露出させた
- **OAuth 再認証の手順が runbook に存在しなかった**: SSH port-forward → ブラウザ手作業の経路を即興で組み立てる必要があった。手戻り（Zed の port_forwards が動かず生 SSH に切り替え）が発生
- **`-health-check` の誤検知**: 既存実装が `token_manager_available: true` を返し続けたため、本来「異常を見せるはずの hook」が逆に「正常を見せ続ける hook」として機能していた。観測の信頼性そのものが負債だった

### 運が良かったこと

- **ユーザー操作起点の主経路が無事**: Inoreader が補完経路の位置付けだったため、ユーザー影響に至らなかった。これは設計上の意図というより運用慣行の偶然
- **`continuation_token` の Inoreader 側 TTL が 4 日 (推定) で耐えた**: 65 時間後に再開した際、`sync_state.continuation_token` (12 文字) はそのまま受理され、データロス無しで再開できた。Inoreader 側の token rotate 期間が長かった偶然に救われている
- **検知が「別の disk 流出 (ClickHouse)」由来**: token 消失そのものは無症状だったが、ClickHouse の log 暴発が偶然の disk 増加で気付かせた。観測投資の代用にこれを当てにしてはいけない

## アクションアイテム

| # | カテゴリ | アクション | 担当 | 期限 | ステータス |
|---|----------|-----------|------|------|-----------|
| 1 | 検知 | `/admin/health` を Prometheus blackbox exporter に配線、`ingestion_silent=true` または `token_available=false` で 5 分以内に alert 発報 | オンコール | 2026-05-31 | TODO |
| 2 | 検知 | PM-2026-042 D-1（host disk pressure metric / 閾値 alarm）と M-1（per-container log size alarm）を実装。本 PM は二次被害の典型例として PM-2026-042 のフォローに連結する | オンコール | 2026-05-31 | TODO（PM-2026-042 連結） |
| 3 | 予防 | auth-token-manager の `EnvFileSecretManager.updateTokenSecret` を tmpfile + rename + fsync の atomic write に書き直し、ENOSPC 時にファイルが truncate されない保証を入れる | オンコール | 2026-05-23 | TODO |
| 4 | 予防 | `-health-check` を「依存サービスの応答妥当性検証」まで拡張する。具体的には `/api/token` を呼び、200 + 非空 access_token を期待する。`/admin/health` と機能重複が出るが、CLI 経路は K8s probe 用途で残す | オンコール | 2026-05-31 | TODO |
| 5 | 予防 | ClickHouse の compose 全体を `logging` cap 必須化する lint を追加（`compose/*.yaml` の全 service に json-file rotation を pin する CI チェック） | オンコール | 2026-05-31 | TODO |
| 6 | 緩和 | `compose/db.yaml` の clickhouse に `logging` cap 適用 + `mem_limit: 4g` 引き上げ | オンコール | 2026-05-09 | DONE（commit `65e6836b2`） |
| 7 | 緩和 | `service.ErrTokenUnavailable` typed sentinel + `RemoteTokenService.IsDegraded()` 60s × 3 window 実装 | オンコール | 2026-05-09 | DONE（commit `4ade8f77b`） |
| 8 | 緩和 | `/admin/health` 新設、`InoreaderService.LastSuccessfulFetch()` 追加 | オンコール | 2026-05-09 | DONE（commit `46fb99dd4`） |
| 9 | プロセス | 「Inoreader OAuth 再認証」runbook を `docs/runbooks/` に新規作成。SSH port-forward 手順、auth-token-manager `/` の OAuth 開始、`/callback` 着弾確認、手動 fetch trigger まで含める | オンコール | 2026-05-23 | TODO |
| 10 | プロセス | 「disk full イベント発生時の永続化境界整合性チェック」runbook を新規作成。`oauth_token_data` を含む全 named volume の中身検証手順を含める。PM-2026-042 / 本 PM のような二次被害を初動でつぶす | オンコール | 2026-05-31 | TODO |
| 11 | プロセス | ClickHouse の `DROP COLUMN` ミューテーション（`010_alter_otel_traces_nested_events.sql`）が完了していない件を別保守ウィンドウで対処。本 incident では disk 影響だけ閉じた | オンコール | 2026-06-30 | TODO |

## 教訓

### 技術的な学び

- **OAuth token 永続化は atomic write が必須**: ENOSPC や crash の影響を受けるパスに plain `Deno.writeTextFile` 相当を使うと、ファイルだけ truncate されて中身が消える典型的失敗パターンを踏む。tmpfile + rename + fsync の三点セットは妥協できない
- **Health check の責務分離**: 「依存サービスが起動しているか」と「依存サービスが正しい応答を返すか」は別の責務であり、両方を 1 つの bool で表現してはいけない。今回の `-health-check` は前者しか見ていなかったが、後者が壊れていることを 3 日間隠し続けた
- **opaque string error は観測の障害物**: `fmt.Errorf("auth-token-manager returned status: %d", status)` は人間が読むには十分でも、test と alarm の両方で分岐不可能。typed sentinel (`errors.Is`) で受け直す習慣が必要
- **rolling window failure counter は単純で強力**: 60s × 3 のメモリ上カウンタで「持続的失敗」と「散発的瞬断」を分離できる。timer / goroutine 不要、on-demand 評価で済む

### 組織的・プロセス的な学び

- **「補完経路」の観測投資は減らしてはいけない**: ユーザー直結ではないからこそ、サイレント停止しても誰も気付かない。むしろ補完経路ほど自動的な observability が必要
- **PM の二次被害は連結記録する**: PM-2026-042 と本 PM は同じ disk full イベントが起源で、片方を読んだだけでは全体像が見えない。Related PM の双方向リンクで読者がトレースできる構造にする
- **TDD は復旧時にも効く**: 復旧の最中に observability を入れる際、failing test → minimum impl → refactor の順守がそのまま「次の incident で alarm が動作する」保証につながった

## 参考資料

- [[PM-2026-042-staging-projector-content-type-loop-disk-fill]] — 一次原因。本 PM は二次被害
- [[000895]] ADR: Inoreader 取り込みのサイレント停止を検出可能にする
- [[000602]] ADR: Inoreader 記事取り込みに Tier1 入口フィルタを導入
- 関連 commit: `65e6836b2`（ClickHouse log cap）、`4ade8f77b`（typed token error）、`46fb99dd4`（/admin/health）、`6c2d5fdca`（ADR-000895）

---

> **Blameless Postmortem の原則:** 本ドキュメントは個人の過失を追及するためではなく、システムの脆弱性とプロセスの改善機会を特定するために作成されている。「誰が悪いか」ではなく「システムのどこが改善できるか」に焦点を当てている。
