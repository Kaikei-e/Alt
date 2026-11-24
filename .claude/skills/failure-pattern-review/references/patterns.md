# 失敗の型（FP-01 〜 FP-19）

`docs/ADR/` と `docs/postmortems/` が記録している不具合・障害・回帰・差し戻しから抽出した型。各節は同じ並びで書く:
機構、先例、問い、確認コマンド、機械的に見ているもの。先例の ADR は `docdag context <id>` で、
ポストモーテムは `docs/postmortems/` 配下の該当ファイルで読める。

型は排他ではない。ADR-000928 は未配線（FP-02）であり、nil の無音（FP-01）であり、e2e が緑のまま
（FP-07）である。重なったら、検知を一番遅らせた型で報告する。

---

## FP-01 失敗を成功・空・healthy に写す

**機構.** 失敗や未配線を `return nil`、空配列、`Ok(())`、HTTP 200、`/health` 200、checkpoint 前進に
変換する。防御的な nil 判定は「意図的に無効」と「配線し忘れ」を同じ形にする。プロセスは生きて見え、
仕事は止まっている。

**先例.**
- ADR-000928 — `if neighborSearcher == nil { return nil }`。DI で渡し忘れ、本番の Related は空。e2e 313 件は全通過。
- ADR-000273 — `Enabled` の既定は true、secret が空で usecase は nil。画像プロキシは本番で一度も動かなかった。
- ADR-000593 — 取得失敗をログだけで握り、checkpoint が進んで二度と再試行されない。
- ADR-000113 — UPDATE が 0 行でも `Ok(())`。
- ADR-000825 — コンテナは `Up (healthy)`、`/health` は 200 のまま、分類が 8 日止まった。
- ADR-000895 — トークンファイルが空になり取り込みが 3 日凍結。health は available を返し続けた。
- ADR-000340 — 失敗を「200 OK + 透過 PNG」で返し、`onerror` が発火しない。
- PM-2026-033 / PM-2026-035 / PM-2026-036 / PM-2026-037 — 同一の `classification returned 0 results` 文言が、mTLS スキーム拒否・learning_machine artifacts 欠落・joblib 空ディレクトリ・init container 未起動という 4 つの別原因を隠した。
- PM-2026-038 — `rust-bert` キャッシュの読み取り専用エラーを `None` に丸め、keyword-only の縮退経路のままジョブが `completed` で閉じた（30 ジャンル中 2〜5 バケットのみ）。
- PM-2026-049 — 根拠検索が空のままでも finalizer が品質を検証せず、空洞版（v11: 415 字・引用 0 件、v12）を `run_status=succeeded` で正式版として永続化した。
- PM-2026-062 — Morning Letter の `"..."` 出力が `min_length=1` 検証を通り、126 日分中 113 日分が `is_degraded=false` の成功扱いで保存された。

**問い.**
1. 追加・変更した依存が未設定のとき、起動ログに `*_enabled` / `*_disabled` が出るか。業務経路で未配線の枝に入ったら止まるか。
2. 空の結果は「0 件だった」と「取れなかった」を呼び出し側が区別できるか。
3. 副作用が失敗したあとに checkpoint・ACK・ジョブ状態が進む経路は無いか。
4. health は、このプロセスの仕事の前提（listener の bind、モデルや分類器の実体、トークンの中身、直近成功時刻）を見ているか。
5. 初期化失敗をテスト用の縮退経路へ逃がしたり、異なる原因を一つの汎用エラーや最小限の成功判定（空でない・長さ 1 以上等）で丸めていないか。

**確認コマンド.**
```bash
rg -n 'if .* == nil \{' <changed-dir> -A2 | rg -B2 'return nil|return \[\]|return ""'
rg -n '\.ok\(\)|unwrap_or_default\(\)|let _ = ' <changed-dir>
rg -n 'except .*:\s*$' -A1 <changed-dir> | rg 'pass|return None|return \[\]'
rg -n '_enabled|_disabled' <service>/
```

**機械的に見ているもの.** `scripts/tests/test-health-deep-contract.py`（deep health の契約）。
nil 判定そのものを落とす検査は無い。ここはレビューでしか止まらない。

---

## FP-02 境界の片側だけを新契約にする

**機構.** 移行・分割・フィールド改名で、書く側と読む側、REST と RPC、proto の package、環境変数、
リスナの片方だけが新契約に載る。呼び出しは 200 や空文字で成功し、画面は「ゼロ件」「常に false」「404」になる。

**先例.**
- ADR-000867 — 再投影後にカード 55,210 件すべての link が空。producer 2 経路は `"link"` を書き、consumer は `"url"` を読んでいた。
- ADR-000144 — search-indexer の LIMIT 固定、proto のフィールド欠落、backend が offset を渡さない、の三層が同時に欠けた。
- ADR-000720 — REST には認証を必須にし、Connect-RPC のポートは無防備なまま残った。
- ADR-000760 — REST ルートが mTLS listener と不一致で 4 日間 404。
- ADR-000792 — 列追加の migration が本番未適用で 48,795 件が既定値のまま。
- ADR-000942 — admin JSON が PascalCase のまま、CLI は零値を成功として表示した。
- PM-2026-033 — `recap-worker` の outbound だけ `MTLS_ENFORCE=true` にし、下流 `recap-subworker` と `news-creator` が plain HTTP のまま残って 8 件のジョブが送信前で失敗した。
- PM-2026-041 — この事象の初回診断では書く側・読む側の向きを取り違えて 1 経路だけを直し、consumer 自身の struct で往復するテストも緑のまま通った。後に ADR-000867 が本番データで向きを訂正した。
- PM-2026-026 — `search-indexer` が `X-Service-Token` を必須化したあと、`rag-orchestrator` と `alt-backend` のクライアントにヘッダが無く全検索が 401 になった。
- PM-2026-062 — `recap-subworker` が全 `/v1/*` に管理トークン必須を入れた一方、`recap-worker` の分類・完了待ち GET・本文抽出・評価の呼び出しがヘッダ無しで 401 になり日次バッチが止まった。

**問い.**
1. 変更したフィールド・キー・package・環境変数を**書く箇所すべて**と**読む箇所すべて**を列挙したか。同じ変更に入っているか。
2. 過去に書かれた payload（旧キー、キー無し）を新しい読み手はどう扱うか。再投影で全件が空にならないか。
3. producer の変更なら、Pact の失敗テストが実装より先にあるか。
4. REST と Connect-RPC、内部と外部のリスナで、同じ制御が両方に掛かっているか。

**確認コマンド.**
```bash
rg -n '"<old_key>"|"<new_key>"' --type go --type py --type rust --type ts
rg -n '<FieldName>' proto/ <producer-service>/ <consumer-service>/
rg -n 'map\[string\]any\{' <producer-service>/ -A6 | rg '"<key>"'
```

**機械的に見ているもの.** `.github/workflows/proto-contract.yaml`（buf と Pact）。map リテラルで
組み立てた payload のキーと、migration の適用状態は見ていない。

---

## FP-03 投影の部分更新と、誤った DB への着地

**機構.** upsert が、そのイベントが変えるつもりのない列に Go のゼロ値（空文字、nil slice → JSON `null`）を
書き、merge SQL がそれを本物の更新として採用する。projector が payload ではなく最新行や壁時計を読む。
checkpoint だけ進む。表を所有者でない DB に作る。

**先例.**
- ADR-000529 — nil slice が `"null"` になり、`'null'::jsonb != '[]'::jsonb` が真で既存タグを上書き。13,386 件。
- ADR-000465 — 非要約イベントの空文字が、既存の `ready` を上書き。
- ADR-000457 — 後続 upsert が `dismissed_at` を NULL に戻し、dismiss したカードが戻る。
- ADR-000447 — projector が event の version id を無視して最新版を読み、再投影で過去が最新本文に置き換わる。
- ADR-000468 — projector の壁時計で再投影が非決定になり、Recall の予定時刻がすべて未来になった。
- ADR-000944 — タグ版の取得失敗をログだけで続行し、checkpoint が進んでイベントが落ちる。
- ADR-000940 — Trail の表が別 DB の migration ディレクトリにあり、projector も read handler も実行時に落ちる。
- ADR-000960 — 同名の表が 2 つの DB にあり、インデックス追加が死んだコピーに入った。
- PM-2026-040 — `StartReproject` が未初期化の `json.RawMessage`（`nil`）を流し、`pgx` が `NULL` を明示送信して `JSONB NOT NULL DEFAULT '{}'` 制約に違反し 502 が続いた。

**問い.**
1. このイベントが触らない列を、upsert はどの値で書くか。空文字・`null`・未設定は「変更なし」として扱われるか。
2. projector は event payload と版つきリソース以外を読んでいないか（最新行、壁時計、active な投影）。
3. 同じ event log を 2 回投影して行が一致するテストがあるか。本番に残る旧 payload の形を食わせているか。
4. 追加した表・列・インデックスの migration ディレクトリは、それを読むプロセスが接続する DB のものか。
5. INSERT / UPDATE の SQL に未初期化の `nil` バイト列やゼロ値をそのまま渡して、`NOT NULL DEFAULT` 制約違反や意図しない `NULL` 送信を起こしていないか。

**確認コマンド.**
```bash
rg -n 'ON CONFLICT' -A12 <changed-sql-or-driver>
rg -n 'time\.(Now|Since|Until)\(|GetLatest[A-Z]' knowledge-sovereign/app/usecase/
rg -n 'CREATE TABLE|CREATE INDEX' <migration-dir>/ | rg '<table>'
```

**機械的に見ているもの.** `scripts/altharness/alt-harness guard-event-time`（`scripts/altharness/internal/guards/eventtime.go` — projector への壁時計と
`GetLatest*` の追加を編集時に拒否）。merge SQL のゼロ値上書きと DB の所有は見ていない。
詳細な監査は `immutable-design-guard` スキル。

---

## FP-04 ローカル LLM の入出力契約とトークン予算

**機構.** 思考レベル、チャット区切り、`num_predict`、JSON / XML の文法、few-shot、検証器が、実際の
モデル出力と一致しない。思考トークンが予算を使い切り本文が空になる。検証器が妥当な日本語を棄却する。
呼び出し側は空・fallback・degraded を完了として通す。

**先例.**
- ADR-000024 — 思考レベルを上げた直後、短いタスクの出力が長さ 0 になった。
- ADR-000640 — 旧モデルのチャット区切りのまま差し替え、停止トークンが効かず上限までゴミを出力。
- ADR-000608 — 日本語を空白で分割して 1 トークン扱いになり、カバレッジ 0% で全回答が棄却された。
- ADR-000632 — 構造化出力の配線後、golden が 6/12 から 1/12 に落ちた。
- ADR-000888 — 散文を JSON としてパースして空になり、文脈なしで実在しない事実が生成された。
- ADR-001000 — 互換 gateway が長いプロンプトを黙って切り詰め、本文が空になった。

**問い.**
1. 思考トークンと出力上限の合計は、本文の下限を残すか。golden で確かめたか。
2. モデルを替えたなら、チャット区切りと停止トークンはそのモデルのものか。旧区切りの文字列が残っていないか。
3. 日本語を ASCII 空白や ASCII 句読点で分割していないか。
4. 空回答・パース失敗・degraded は、完了ではなく失敗として上に伝わるか。
5. 経路上の gateway やプロキシは、プロンプト長の上限を超えたとき切り詰めずにエラーを返すか。

**確認コマンド.**
```bash
rg -n 'num_predict|max_tokens|num_ctx|think|reasoning_effort' <service>/
rg -n 'start_of_turn|im_start' <service>/
rg -n 'strings\.Fields|\.split\(\)|split_whitespace' <service>/
```

**機械的に見ているもの.** 無い。golden / eval は手動実行。ここはレビューと eval でしか止まらない。

---

## FP-05 終端しない失敗と、外側が先に切れる待ち

**機構.** タイムアウト、403、参照先の欠落を「未完了」に戻す。処理できない項目が候補の先頭に残り、
有効な仕事が飢える。0 件保存を完了にする。クライアントの期限がサーバの処理より短く、再試行が待ち行列を増やす。

**先例.**
- ADR-000504 — 負荷試験の残骸が pending 約 191 万件。`not found → failed → pending` のループで新規ジョブが処理されない。
- ADR-000296 — 403 を永続失敗から外した副作用で、無限にリクエストし続けた。
- ADR-000551 / PM-2026-002 — 要約できない 63 記事が placeholder を書かず、1349 回再投入された。
- ADR-000149 — 保存 0 件・55 ジャンル失敗のジョブが Completed で閉じた。
- ADR-000512 — upsert でも作成イベントを出し、処理済みが再びキューに積まれた。
- ADR-000320 — `$effect` が自分の代入で再発火し、5 分でストリーム開始 9,181 回。
- ADR-001013 / ADR-001014 — 永久に弾かれる記事が一覧の枠を使い切る。版を作らない placeholder で候補から外した。
- PM-2026-027 — `ResponseHeaderTimeout` 20 秒が Map-Reduce 推論（60〜120 秒以上）より短く、バックオフ無しで再試行して重複実行を招き、元リクエスト成功後も `article_summaries` を再確認せず `dead_letter` に送った。
- PM-2026-024 — `status='running'` で落ちた `morning_update` ジョブが `trigger_source` 未書き分けのまま起動時の `find_resumable_job()` に毎回拾われ、409 で落ちて再起動ごとに 5 回以上ループした。
- PM-2026-039 — `onFrame` / `onExpired` の無条件 `invalidateAll()` が SSR `data` を差し替えて `data.lensModeId` を追う `$effect` を再発火させ、短寿命 JWT の即 `stream_expired` と合わせて秒間 30+ 件の `__data.json` 嵐を起こした。
- PM-2026-045 — `BACKEND_TOKEN_TTL=5m` が `streamStaleTimeout=30m` より短く 5 分ごとに切断され、さらに自前 `disconnect()` の abort でも `catch` が `scheduleReconnect()` を呼んでストリームが並走した。

**問い.**
1. 再試行しても成功しない失敗（参照先なし、継続する 403、入力が規約外）は、候補集合から外れる終端状態を持つか。
2. その終端状態は、下流に「完了」と誤読されないか（版・イベント・完了フラグを作らないか）。
3. 0 件処理は成功と区別されるか。
4. 外側の期限（クライアント、プロキシ、JWT）は内側の処理時間より長いか。
5. `$effect` は自分が書く state を依存に取っていないか。
6. 再接続や `invalidateAll()` が、`$effect` の依存参照や自前 `abort()` の `catch` 経由で自分自身を再発火させないか。

**確認コマンド.**
```bash
rg -n 'pending|PENDING' <service>/ | rg -i 'update|set|retry'
rg -n 'timeout|Timeout|deadline' <caller>/ <callee>/ compose/
rg -n '\$effect' <changed-svelte-files>
```

**機械的に見ているもの.** `python3 scripts/kisage-env.py verify`（外部 API 間隔の床）。状態機械の
終端は見ていない。stream consumer の ACK 順序は `.claude/rules/event-stream-consumer.md`。

---

## FP-06 デプロイ単位が実行時の依存を外す

**機構.** Compose 単体では正しい構成が、サービス単位の rolling（依存を起動しない経路）では一度も
動かない。init container、名前付き volume の populate、restart policy 無し、古い netns を掴む sidecar、
マウントされていない設定ファイル、学習時と違う版のライブラリ。

**先例.**
- ADR-000826 / PM-2026-037 — 依存を起動しない rolling は `service_completed_successfully` の init を起動も create もしない。compose 上は healthy、volume は空。
- ADR-000703 — restart policy が無く、コンテナが約 36 時間不在。
- ADR-000892 — ルールファイルの glob がコンテナ内で空。全ルールが無音で load 失敗。
- ADR-000952 — 下限だけの版指定で minor が上がり、学習済み成果物と不一致で worker が初期化できない。
- ADR-000784 / ADR-000802 — healthcheck が証明書の鮮度だけを見て、listener が bind していない状態を healthy とした。
- PM-2026-030 / PM-2026-031 / PM-2026-034 — 親コンテナの `--force-recreate` 時に `network_mode: service:<parent>` の `pki-agent` が旧 netns に取り残され（`depends_on.restart: true` でも追随せず、自己終了後も `No such container` で再起動不能）、Acolyte 502 を繰り返した。
- PM-2026-052 — 上限なしの `scikit-learn>=1.5.0` 更新で 1.8.0 から 1.9.0 に上がり、ホスト bind の分類器アーティファクトと版が割れて 3 週間以上 `recap_outputs` が 0 行になった。

**問い.**
1. この変更は、依存を起動しない rolling でそのサービスだけを入れ替えても成立するか。
2. 常駐サービスに restart policy があるか。
3. コンテナが読むファイル（モデル、ルール、証明書）は、空ディレクトリや未マウントのとき起動を失敗させるか。
4. 学習済み成果物と対になるライブラリは、版が固定されているか。

**確認コマンド.**
```bash
rg -n 'service_completed_successfully' compose/ -B6
rg -n 'restart:' compose/<file>.yaml
rg -n '>=' <service>/pyproject.toml
```

**機械的に見ているもの.** `scripts/compose-init-edge-audit.py`、`scripts/compose-file-bind-audit.py`、
`scripts/compose-netns-cascade-audit.py`、`scripts/compose-restart-policy-audit.py`（常駐サービスの
restart policy 必須、`compose-audit.yaml` 系統）。依存ライブラリの版固定は見ていない。

---

## FP-07 検証が本番の失敗形を見ない

**機構.** CI の握りつぶし、空の結果ファイル、存在しない route をスタブした契約テスト、コピーを叩く
単体テスト、実行環境に無い API、path skip で、失敗が緑になる。

**先例.**
- ADR-000616 — provider 検証が `|| true`。失敗しても CI は green。
- ADR-001004 — 脆弱性スキャンの失敗が `|| true` と空の SARIF で「検出 0」になっていた。
- ADR-000736 — consumer pact が必須ヘッダを assert せず、24 時間の無音劣化。
- ADR-000952 — 存在しない route をスタブして pact が緑。
- ADR-000984 — テスト環境に `matchMedia` が無く、壊れていた読み方を一度も実行していなかった。
- ADR-000193 — 実ハンドラーではなくコピーをテストしていた。
- PM-2026-032 — 起動直後に走る smoke テストと Pact は、24 時間後の証明書ローテ跨ぎで `reqwest` が古い in-memory 証明書を出し続ける失敗を見られなかった。
- PM-2026-055 — テストがストリームの成立と内容だけを見て継続時間を測らず、Go 1.26.6 の `ReadHeaderTimeout`（10 秒）で h2c 接続が切れる欠陥が素通りした。
- PM-2026-040 — 単体テストが `mockCreateReprojectRunPort` で DB 境界をモックしたため、`JSONB NOT NULL DEFAULT '{}'` への `NULL` 送信による 502 が見えなかった。
- PM-2026-041 — `TestKnowledgeProjectorJob_ArticleCreated_LinkPropagation` が consumer 自身の struct で marshal/unmarshal する自己 round-trip だったため、`"url"` と `"link"` のタグ不一致が通っていた。
- PM-2026-061 — SQL テストが文字列の形だけを見、CitationRail テストは重複しない `RefID` だけを使っていたため、owner backfill 後の HNSW 不使用と `each_key_duplicate` を逃した。
- PM-2026-051 — CI がコンパイルとリンクの成功だけを見てビルドしたイメージを起動しなかったため、ビルダーとランタイムの glibc 世代不整合による起動即死（`GLIBC_2.38 not found`）が 6 日間潜伏した。

**問い.**
1. このテストは、本番で動くハンドラ・配線済みのコンテナ・実在する route を通るか。
2. 追加した CI ステップは、失敗したとき job を落とすか。握りつぶしに理由が書かれているか。
3. consumer が要求するヘッダ・認証を、契約テストは assert しているか。
4. 修正した不具合を、修正前のコードで実行して赤になることを確かめたか。
5. 証明書のローテや長寿命ストリームなど経過時間が効く経路で、起動直後だけでなくタイムアウトや期限を跨ぐ検証をしているか。
6. テストのデータ分布や重複（本番相当の偏り、同一記事の複数 chunk 等）が、SQL 実行計画や UI 反復キーを本番と同じ条件に晒しているか。
7. Dockerfile やベースイメージを変更したとき、CI がビルドだけでなく生成したイメージを実際に起動する smoke を通しているか。

**確認コマンド.**
```bash
rg -n -F '|| true' .github/workflows/
rg -n 'continue-on-error' .github/workflows/
rg -n 'route\(|page\.route|mock|stub' <test-dir>/ | rg '<path>'
```

**機械的に見ているもの.** `scripts/altharness/alt-harness workflow`（`scripts/altharness/internal/workflow/rules.go` — 握りつぶしには
`# swallow-ok: <理由>` が必須、ほか構造検査）。Pact のヘッダ assert とスタブ route の実在は見ていない。

---

## FP-08 局所修正と決定の置換が、同じ穴を戻す

**機構.** 直前の修正が隣の不変条件を壊す。正しい理由で制御を外したあと、その制御が塞いでいた穴が
別の形で開く。保持できない多軸モデルが単軸に潰れる。

**先例.**
- ADR-000221 → ADR-000224 → ADR-000226 — `$effect` のガードを直すたびに、初回と追加ロードでスピナーが交互に固着した。
- ADR-000290 → ADR-000296 — 403 の扱いを緩めた修正が無限リクエストを作った。
- ADR-000717 → ADR-000743 → ADR-000954 — mTLS 一本化でアプリ層認証を外し、内部リスナーが「到達できれば通る」に戻った。ADR-000954 は同じ欠陥クラスが 2 度発生したと書いている。
- ADR-000601 〜 ADR-000612 — スロット返却の修正が、プリエンプション、遅延完了、キャンセル待ちの順で別のリークを残した。
- ADR-000940 — 3 軸のモデルが実装でも運用でも単軸に退化し続け、表層ごと退役した。
- PM-2026-031 — c2quay 移行（ADR-000758）で旧デプロイスクリプトごと sidecar の連鎖再作成処理が消え、compose 側への移植が未完のまま Acolyte 502 が再発した。
- PM-2026-034 — `ADR-000784` が Docker の `network_mode: service:<parent>` と `restart: unless-stopped` の挙動を実機検証せずに通った結果、自己終了した sidecar が `No such container` で再起動不能な zombie になった。

**問い.**
1. これは連鎖の何件目か。直前の ADR が直した経路の次段（初回の次の追加ロード、返却の次のキャンセル）にテストがあるか。
2. 制御（認証、検証、ガード）を外す変更なら、その制御が塞いでいた条件を名前にしたテストが残るか。
3. 修正した画面・経路の隣接状態（初回、追加、空、失敗）を同じ変更のテストに含めたか。
4. ツールやデプロイ基盤を置き換えるとき、旧スクリプトが担っていたガードを新経路へ移し忘れていないか。ランタイム仕様の前提を実環境で反証確認したか。

**確認コマンド.**
```bash
docdag context <直前の ADR id> --depth 2 --budget 1500
docdag query <直前の ADR id> --ancestors --fields id,title,status
git log --oneline -8 -- <changed-file>
```

**機械的に見ているもの.** 編集前の ADR preflight が、そのディレクトリの現行契約を提示する。
連鎖の検出そのものは機械化されていない。

---

## FP-09 識別子と所有の取り違え

**機構.** 利用者から見て一つのものが、保存キーでは別物である。URL と記事 ID、feed と article、
複合一意の片方だけで引く SELECT、同名で別 DB にある表。コンパイルも単体テストも通る。

**先例.**
- ADR-000300 — `feed_links.id` を `feeds.id` として返し、外部キー違反で新規記事が保存されなくなった。
- ADR-000040（withdrawn）— feed と article を同一視した移行で、約 7,000 件の既読が落ちた。
- ADR-000027 / ADR-000158 — UUID 列に URL や空文字を渡して保存が失敗。
- ADR-000552 — 一意制約は `(url, user_id)`、SELECT には `user_id` が無く、別記事の要約が表示された。
- ADR-000085 — proto の package 名の差だけで 404。
- PM-2026-061 — `CitationRail` が chunk 単位の引用一覧で記事単位の `cite.RefID || cite.URL` を `{#each}` の key に使ったため、同一記事の複数 chunk 引用で `each_key_duplicate`（画面 500）になった。

**問い.**
1. この ID はどの表の主キーか。関数の引数名と型は、隣の表の ID と取り違えられないか。
2. 複合一意の列すべてが、読む SQL の WHERE にあるか。
3. `string` や `uuid.UUID` のまま、意味の違う ID を同じ引数で渡せる関数を増やしていないか。
4. UI の keyed `{#each}` や一覧マップのキーに、要素の粒度（chunk 等）より粗い外部 ID（記事 ID や URL）を使っていないか。

**確認コマンド.**
```bash
rg -n 'UNIQUE|PRIMARY KEY' <migration-dir>/ | rg '<table>'
rg -n 'FROM <table>' -A6 <driver-dir>/
rg -n 'feed_id|feed_link_id|article_id' <changed-files>
```

**機械的に見ているもの.** `.claude/skills/clean-architecture/scripts/check_layers.sh`（層の越境）。
ID の意味の取り違えは見ていない。

---

## FP-10 契約グラフの修復が、現行契約を落とす

**機構.** ADR の `supersedes` を広げると、退役と無関係な決定まで `superseded` になり、
`docdag query --binding` と編集前の ADR ブリーフから消える。`docdag validate` は、辺と status が
整合していれば通る。

**先例.**
- ADR-000940 — frontmatter の一括修復で `supersedes` が 6 件から 60 件に広がり、54 件が binding から落ちた。
- ADR-001011 / ADR-001012 — 落ちた CI 規約と取り込み監視の契約を、新しい ADR として立て直した。

**問い.**
1. 追加した `supersedes` の各 ID について、新 ADR の本文はその決定を置き換える理由を名指しで述べているか。
2. 前提にする・拡張する・一部を直すだけなら、`depends-on` ではないか。
3. この変更で binding から落ちる ADR の一覧を出して、1 件ずつ意図どおりか確かめたか。

**確認コマンド.**
```bash
./scripts/altharness/alt-harness adr-gate --base origin/main
docdag query --binding --fields id,title
docdag validate --immutable-since origin/main
```

**機械的に見ているもの.** `scripts/altharness/alt-harness adr-gate`（`scripts/altharness/internal/ledgers/adr.go` — 新しい `supersedes` 辺は、
本文に `[[ID]]` が無ければ CI で失敗）。本文に名前があっても置換が妥当かどうかは、レビューで判断する。

---

## FP-11 文書の床が、コードの床になっていない

**機構.** 規約や ADR が「最低 5 秒」「列は末尾に足す」と書いていても、検証がそれを拒まない。
違反は設定値とレビューの記憶に依存する。

**先例.**
- ADR-000982 — 5 秒ルールは書いてあったが、validation は 1 秒超を通していた。床を守らせていたのは配布時の設定値だけだった。
- ADR-001011 — view の列順を見る専用 lint は無く、検出は全 migration を実 DB に適用する統合テストに依存している。

**問い.**
1. この変更が実装する規約には、違反したときに落ちる検査（validation、テスト、lint）があるか。
2. 床を設定値だけで守っていないか。設定を変えたら通ってしまわないか。
3. 新しく文書に書いた「必ず」「禁止」は、どの検査が拒むのか。無いなら、検査を足すか、文書にそう書いたか。

**確認コマンド.**
```bash
rg -n '<規約の数値や名前>' <service>/ scripts/ .github/workflows/
python3 scripts/kisage-env.py verify
./scripts/altharness/alt-harness selfcheck
```

**機械的に見ているもの.** `python3 scripts/kisage-env.py verify`（外部 API 間隔）、
`scripts/altharness/alt-harness selfcheck`（`scripts/altharness/internal/selfcheck/selfcheck.go` — エージェント向けハーネス自身の床: フックの入力、
ガード対象パスの実在、CLAUDE.md と AGENTS.md の共有ブロック一致）。

---

## FP-12 実行中の実体がソースと違う

**機構.** `.env` が compose の `${VAR:-default}` を上書きする、コンパイル済みサービス（Go / Rust / TS）がリビルドされない、複数チェックアウトや相対パスで secrets の実体が分岐する、片側にしか無いモデル名を要求する。リポジトリの記述は正しく、動いているプロセスだけが古い設定やバイナリを掴んでいる。

**先例.**
- ADR-000189 — `altctl` が `-f` に絶対パスを渡したため Compose V2 が `compose/.env` を探し、symlink の無い環境では `.env` が読まれず空の DB 設定で起動した。
- ADR-000499 / ADR-000571 — compose 側の既定値を変えても `.env` の `AUGUR_EXTERNAL` 上書きが勝ち続け、存在しないモデル要求（404）やセマフォ迂回が続いた。
- PM-2026-007 / PM-2026-013 — `.env` に残った `DISTRIBUTED_BE_MODEL_OVERRIDES` や `OLLAMA_NUM_PARALLEL=2`・`gemma3-4b-8k` が compose 既定値を上書きし、BE 要約遅延（163 秒）やスロット構成ずれを起こした。
- PM-2026-014 — モデル統一コミット `4e99146d` のあとも `pre-processor` が 4 日前のバイナリのまま稼働して `gemma3-4b-8k` を送り続け、610 回の `COLD_START` を起こした。
- PM-2026-016（ADR-000614）— Gemma4 移行後に `rag-orchestrator` がリビルドされず旧モデル名 `gemma3-4b-12k` を送り、Ask Augur が 2 連続即時 EOF で失敗した。
- PM-2026-048 / PM-2026-050 — 相対パス `../secrets/*.txt` で 2 系統のチェックアウトが別の検索キー実体を持ち 24 時間 30 分全文検索が 403 で全滅し、正準設定移行でも古いコピーが昇格して `Created` コンテナが旧 env を凍結した。

**問い.**
1. `compose/*.yaml` の `${VAR:-default}` を変えたとき、環境側の `.env` や正準設定に古い上書き値が残っていないか。
2. Go / Rust / TS のコンパイル済みサービスが読む定数や環境変数を変えたとき、そのサービス自身もリビルド対象に含まれているか。
3. 相対パスや複数チェックアウト、`Created` 滞留コンテナにより、実行コンテキストで読まれる secrets・`.env` の実体が分岐しないか。
4. リクエスト先のモデル名や接続先が、呼び出し元と呼び出し先の双方に存在して同期しているか。

**確認コマンド.**
```bash
rg -n '\$\{[A-Z0-9_]+:-' compose/
python3 scripts/check-env-overrides.py --env-file .env.template
rg -n 'MODEL|AUGUR_EXTERNAL|IMAGE_TAG' compose/ .env.template
```

**機械的に見ているもの.** `scripts/compose-image-audit.py`（`build:` と `image:` の組を検査し pull デプロイで古いローカルビルドが残る構成を拒否）。`scripts/check-env-overrides.py`（`deploy/host-prereqs.yaml` に登録された `.env` 上書き検査）はホスト側の env ファイルを要するため運用者が手動実行したときにだけ動き、CI が回すのはその単体テストのみ。実行中バイナリとソースの差分や、別経路での secrets 分岐は見ていない。

---

## FP-13 存在と鮮度の取り違え

**機構.** ファイルやディレクトリの「存在」だけを見て（`[ -f ]`、`Path.exists()`、プロセス起動の有無）、期限切れ・0 byte・Docker が自動生成した空ディレクトリを通す。あるいはディスク上の成果物が差し替わっても、常駐プロセスが起動時にメモリへ読み込んだコピーを握り続ける。

**先例.**
- PM-2026-028 — `cert-init` が `if [ -f /certs/svc-cert.pem ]; then exit 0; fi` で早期終了するため期限切れ証明書が再発行されず、`alt-backend` の mTLS 証明書失効で Knowledge Home 等の BFF 経由 RPC が約 58 分停止した。
- PM-2026-029 — `pki-agent` がディスク上の証明書を差し替えても、nginx TLS sidecar が起動時に読み込んだ旧証明書をメモリに固定し続け Acolyte が停止した。
- PM-2026-032 — `pki-agent-recap-worker` のローテ後も、`recap-worker` の `reqwest::Client` が起動時の `Identity::from_pem` をメモリに保持し続け、3days Recap が `CertificateExpired` で落ちた（他 5 サービスにも潜在）。
- PM-2026-035 — `.gitignore` 除外でホスト側に実体の無い `learning_machine/artifacts/` が空ディレクトリのまま bind され、初回分類呼び出しまで 4 ヶ月潜伏した。
- PM-2026-036 — ホスト側の `.joblib` が消え Docker のファイル単位 bind が空ディレクトリを自動生成したのに、`Path.exists()` が `True` を通して 8 日間（計 34 ジョブ）分類が失敗し続けた。
- PM-2026-043 — ENOSPC で `oauth2_token.env` が 0 byte になり `/api/token` は 404 を返すのに、`-health-check` がプロセスの起動だけを見て `token_manager_available: true` を約 65 時間返し続けた。

**問い.**
1. ファイル・証明書の検査は「パスが存在するか」ではなく「通常のファイルであり、空でなく、期限内か」まで見ているか（`is_file()`、サイズ、NotAfter）。
2. ファイル単位の bind mount を増やしていないか。ホスト側に実体が無いとき Docker が空ディレクトリを作らない構成（`create_host_path: false`、ディレクトリ単位 bind、`configs:`）になっているか。
3. 証明書・トークン・設定・モデルをロードする長期稼働プロセスは、ディスク上の更新やローテに追随して再読み込みするか。

**確認コマンド.**
```bash
rg -n '\.exists\(\)|-f /' <service>/ compose/
rg -n 'from_pem|load_cert_chain|ssl_certificate|GetCertificate|GetClientCertificate' <service>/
python3 scripts/compose-file-bind-audit.py
```

**機械的に見ているもの.** `scripts/compose-file-bind-audit.py`（file bind への `create_host_path: false` と `deploy/host-prereqs.yaml` 整合）。`Path.exists()` によるディレクトリ誤認、0 byte トークン、起動時ロードのメモリ固定は見ていない。

---

## FP-14 修正が兄弟経路と既存の被害に届かない

**機構.** ある箇所の不具合や契約変更を 1 経路・1 サービス・1 言語・将来の書き込みだけに適用し、同じ部品や契約を使う兄弟経路・他言語クライアント・既に壊れている既存行や削除済みコンテナをそのまま残す。直したはずの翌日に別の入り口から同じ障害が発火する。

**先例.**
- PM-2026-003 / PM-2026-009 — `handleFetchFullArticle` にだけ入れた stale-response guard や Answer 本文だけに適用した `sanitizeUTF8()` が、兄弟ハンドラ（`handleSummarize`・モバイル版・Citation `PublishedAt` / `morning_letter`）に届かなかった。
- PM-2026-006 / PM-2026-026 — Query Expansion だけをセマフォ経由にし Ask Augur 本体の直結を残した、また `acolyte-orchestrator` にだけ `X-Service-Token` を足して翌日に `rag-orchestrator` と `alt-backend` が 401 で落ちた。
- PM-2026-015 — `HybridPrioritySemaphore` の `release()` 側リークだけを直し、同じ転送経路の反対側である `acquire()` の `CancelledError` 経路を見落として最大 551 秒待ちが続いた。
- PM-2026-024 — `ADR-000709` で新規行への `trigger_source` 書込を直したが、既に `status='running'` で残っていた既存行を閉じなかったため起動時 resume ループが止まらなかった。
- PM-2026-032 — Go 6 サービスにだけ `certReloader` を入れ、同じ 24 時間証明書を使う Rust（`recap-worker`）・他 Go・Python 4 サービスへ移植しなかったためローテ跨ぎで失敗した。
- PM-2026-036 / PM-2026-038 — `learning_machine` 側だけを起動時検証して既定の `joblib` 側を監査せず、さらに隣接する上流 `recap-worker` の `rust-bert` キャッシュも同じ作り（空のホスト bind）のまま未監査で、同じ日に壊れていることが判明した。
- PM-2026-054 — `ADR-000968` が前日に prune スクリプトの将来発火を止めたが、既に削除されていた `pki-agent` サイドカー 5 個を復元せず `alt-data-hub` 証明書が期限切れになった。

**問い.**
1. 直した構成・関数・認証・リロード・ガードと同じもの（同じ RPC、同じテーブル、同じクライアント、別言語の同役割）を使っている兄弟箇所を `rg` で全件洗ったか。
2. 変更内容やその説明に、すでに壊れている既存の行（`running` 滞留、`link=''`、`NULL`）・削除済みコンテナ・旧成果物をどう扱うかが書かれているか。
3. 対になる経路（`release` と `acquire`、producer と全 consumer、1 つのバックエンドと既定バックエンド）の反対側を通るテストが差分または既存テストにあるか。

**確認コマンド.**
```bash
rg -n '<修正した関数名・設定キー・エンドポイント>' .
git log --oneline -15 -- docs/postmortems/
```

**機械的に見ているもの.** 無い。兄弟経路への横展開と既存被害の棚卸しは、検索結果とパッチの突き合わせでしか止まらない。

---

## FP-15 自動で壊し、手で戻す

**機構.** 既存データを消す migration や可視範囲を狭めるフィルタはデプロイパイプラインで自動適用されるのに、埋め戻す backfill はリポジトリ外・手動起動・一回限りのコマンドに置かれる。一回限りのコマンドは mTLS や pgbouncer の変化に追随できずに腐り、既存データは無音で検索や集計から消え続ける。

**先例.**
- PM-2026-057 — `rag-db-migrator` がデプロイ時に `USING NULL::vector(1024)` を自動実行して全 embedding を消去した一方、backfill は git 管理外の古い手動スクリプトのまま実行されず、約 30 日間 57,058 文書中 1,105 件（1.94%）しか検索対象にならなかった。
- PM-2026-060 — テナント分離で nullable な `user_id` 列と `user_id = $owner` フィルタが自動で有効になった一方、既存行を埋める `rag_owner_backfill` は手動コマンドのまま（後の mTLS・pgbouncer 変更で実行不能化）約 47 日間実行されず、既存文書の大半がベクトル検索から無音で消えた。

**問い.**
1. データを消す migration（`USING NULL`、列・表の削除）や結果を絞るフィルタ（`WHERE user_id = ...` 等）を入れたとき、埋め戻しコマンドもリポジトリ内に置かれ、自動または完了確認付きのゲートで走るか。
2. 一回限りの backfill CLI は、本番と同じ接続経路（mTLS、pgbouncer simple protocol 等）で動作確認されているか。
3. backfill 完了までのあいだ、NULL 除外や `NULLS LAST` で既存データが消えて見えないか。完了判定クエリとカバレッジ監視に新しい絞込条件が入っているか。

**確認コマンド.**
```bash
rg -n 'USING NULL|DROP COLUMN|DROP TABLE|TRUNCATE' migrations-atlas/ knowledge-sovereign/migrations/ rag-migration-atlas/ recap-migration-atlas/ acolyte-migration-atlas/ pre-processor-migration-atlas/ clickhouse/migrations/
rg -n 'backfill|rebuild' scripts/ rag-orchestrator/ alt-backend/
git diff --name-only <base>...HEAD -- '**/migrations/**'
```

**機械的に見ているもの.** `scripts/altharness/alt-harness migration-gate`（`scripts/altharness/internal/ledgers/migration.go` — 破壊的文を含む新規追加 migration に理由付きの `-- destructive-ok:` コメントを要求する。理由の中身は読まず、復旧手順が実在するかは判定しない）。絞り込みフィルタ追加と手動バックフィルの組や、バックフィル CLI の本番接続経路までは見ていない。

---

## FP-16 検知が届かない

**機構.** アラートルールやプローブは書いたが、配送経路（Alertmanager / provisioning）に繋がっていない、対象サービスや時系列が消滅したときに `absent()` が無く発火しない、1 つのゲージに複数の producer が混ざり片方の死を隠す、または実トラフィックでは届かない最小件数条件が閾値を塞ぐ。結果、検知がユーザー報告頼みになる。

**先例.**
- ADR-000286 — アラートルールの datasource UID 不一致、存在しないテーブル名参照、プロビジョニングパス外への配置により、全 Grafana アラートが無言で失敗していた。
- PM-2026-054（ADR-000969）— `pki-agent-alerts.yml` に `page` ルールがあったが Alertmanager と `alerting:` が存在せず配送されず、さらにルールが時系列の存在を前提にしていてサイドカー消滅を検知できなかった（scrape 対象も 14 中 10 個で 2 個は誤 DNS）。
- PM-2026-056 — `SummaryVersionCreated` のゲージにオンデマンド経路とバッチ経路が混ざっていたため、バッチ要約が約 11 日停止しても生きている側がゲージを満たし続け、アラートが鳴らなかった。
- PM-2026-060 — `AugurEmbeddingCoverageLow` が所有者フィルタを含まず、`AugurFallbackRateHigh` 等の最小件数条件（30 分で 10 件以上等）が低トラフィック下で満たされず、約 47 日間無検知だった。
- PM-2026-062 — `recap-subworker` は生きたまま 401 を返していたため死活監視 `RecapSubworkerDown` は発報せず、`PM-2026-052` #8 に挙げられていた日次ジョブ連続失敗アラートも未実装のままで、さらにトピックスナップショット途絶アラートも未調査の理由で通知に届かなかった。

**問い.**
1. 新しいアラートルールは、プロビジョニングパスと scrape 対象に含まれ、Alertmanager 等の通知先まで届く E2E 経路が検証されているか。
2. 監視対象のプロセスやコンテナが消滅したとき（時系列の不在）、`absent()` や `absent_over_time()`、あるいは外側の死活プローブで発火するか。
3. 生存を見るメトリクスやカバレッジの分母に、別の producer（on-demand と batch 等）や新しい絞り込み条件が混ざって死角を作っていないか。
4. アラートの最小件数条件（`N 件/30m` 等）が、この環境の実際のトラフィック量で到達可能か。

**確認コマンド.**
```bash
python3 scripts/observability-config-audit.py
rg -n 'alerting:|receivers:|rule_files:' observability/
rg -n 'absent\(|absent_over_time\(' observability/
```

**機械的に見ているもの.** `scripts/observability-config-audit.py`（Prometheus `rule_files` の glob 漏れ、Grafana provisioning、Alertmanager マウント整合）。`scripts/observability-drift-check.py`（ディスク上設定と稼働中 Prometheus / Alertmanager の差分検査）は稼働中の Prometheus を要するため運用者が手動実行したときにだけ動き、CI が回すのはその単体テストのみ。複数 producer の混在、実トラフィックに対する閾値到達性、`absent()` 欠落は見ていない。

---

## FP-17 並行資源の会計が全出口で保存されない

**機構.** セマフォ・プール・ロックの解放が、発行されたトークンの所有元ではなく呼び出し側の種別から返却先を推定する。あるいは待機中のキャンセル、プリエンプション、`set_result()` 転送直後の `CancelledError`、`be_slots=0` などの境界出口で回収処理が抜け、「空き + 使用中 == 総数」の保存則が破れて資源が永久に消失する。

**先例.**
- PM-2026-012 — `release()` がスロットの出身プールではなく呼び出し元の優先度で返却先を推定したため、プリエンプションで BE に渡った RT スロットが消失し、GPU が約 10 分間遊休のまま RT リクエストがタイムアウトした。
- PM-2026-013 — `be_slots=0`（`total_slots=1, rt_reserved=1`）構成での獲得計上不備により、`SLOT INVARIANT VIOLATION`（空き 0・使用中 0）が 10 件以上発生した。
- PM-2026-014 — `release()` 内の `call_soon_threadsafe` による遅延 `set_result()` の前に waiter の future がキャンセルされて `InvalidStateError` となり、回収処理が無く 109 件の違反と最大 281 秒待ちを起こした。
- PM-2026-015 — `_try_wake_waiter()` で `set_result()` 済みのスロットを転送した直後、`_track_acquire()` 前に `CancelledError` が入ると回収が走らず最大 551.59 秒待ちになり、`release()` 末尾の整合性検査も転送中の過渡状態を誤検知して 86 件の `ERROR` ログに真のリークと誤報が混在した。

**問い.**
1. 解放処理は呼び出し元の属性から返却先を推定せず、獲得時に発行した明示的なトークンや `slot_id` に紐づく所有元（`home_pool`）へ返しているか。
2. 獲得・解放のすべての出口（即時取得、待機完了、プリエンプション、タイムアウト、`CancelledError`、転送直後の途中キャンセル、`be_slots=0` 等の縮退構成）で「空き + 使用中 == 総数」が保存されるか。
3. 保存則が破れたときに握りつぶさず `ERROR` とメトリクスを出しているか。

**確認コマンド.**
```bash
rg -n 'acquire\(|release\(|CancelledError' news-creator/
rg -n 'SLOT INVARIANT|home_pool|slot_id' news-creator/
rg -n 'Semaphore|Mutex|TryAcquire' <service>/
```

**機械的に見ているもの.** 無い。すべての終了経路と中断タイミングでの保存則は、単体・状態機械テストとレビューでしか止まらない。

---

## FP-18 復旧・掃除・手動操作が二次障害を作る

**機構.** 障害復旧やディスク掃除・E2E の後始末・手動 `docker compose` 操作が、正規のデプロイ経路や原子的な更新を迂回し、依存コンテナ・停止中サイドカー・イメージタグ・並列ジョブのネットワーク・secrets や設定を壊す。復旧作業そのものが次の障害の引き金になる。

**先例.**
- PM-2026-001 / PM-2026-005 — `docker compose up` が `depends_on` 先の `recap-db` や LLM バックエンドまで再作成し、DB リカバリ切断や VRAM クリアによるコールドスタート（TTFT 120 秒）の二次障害を起こした。
- PM-2026-028 — 全 8 サービス分の `*-cert-init` / `*-cert-renewer` サイドカーが消えており、原因ログは残らず過去の `docker compose rm` や部分クリーンアップによる除去と推測され、mTLS 証明書が期限切れになった。
- PM-2026-054 — ディスク掃除スクリプトの無差別 prune（`docker container prune -f` / `image prune -a -f`）が停止していた `pki-agent` 5 個とイメージタグを消し、`alt-data-hub` の 24h mTLS 証明書が期限切れになった。
- PM-2026-031 — 障害対応中に正規のデプロイ経路（Pact ゲート付き deploy）を外れて `docker compose up --build -d alt-backend` を実行し、ビルド失敗（`go build ./main.go`）でデプロイ自体は進まなかったが、この経路は契約検証・環境ロック・監査ログをすべてスキップする。
- PM-2026-048 — `PM-2026-047` 復旧でボリューム再作成した際のキーローテが対話用チェックアウト側のみ更新され、デプロイランナー側が旧検索キーのまま残ったため、後のデプロイ後に全文検索が `403` で約 24 時間 30 分停止した。
- PM-2026-046 — E2E の掃除スクリプトが `name=^alt-staging-` の広い prefix で network を消し、並列 17 leg matrix の兄弟ジョブがコンテナを attach する前に network を奪った。
- PM-2026-050 — 正準設定への一回きり移行スクリプトが鮮度比較なしに古いコピーを昇格させ、権限 `0640` で DB マイグレータ群が secrets 読取不可で `Exit(1)` し、`search-indexer` は無効なフォールバック鍵で 403 クラッシュループに入った。

**問い.**
1. 掃除や後片付け（`prune`、`rm`、network/volume 削除）の対象条件は「自プロジェクト・経過時間・ラベル」で狭く閉じているか。停止中の常駐サイドカーや未 attach リソースを巻き込まないか。
2. 復旧や設定反映のコマンドは、`--no-deps`・`--no-recreate` や正規のデプロイ経路を外れて、`depends_on` 先の DB や VRAM 常駐サービス・`network_mode: service:` サイドカーを意図せず再作成しないか。
3. 状態ファイルや secrets の更新は、容量不足や途中失敗で既存値を壊さない原子的手順（`tmpfile` + `fsync` + `rename`）と dry-run diff を通しているか。

**確認コマンド.**
```bash
rg -n 'prune|network rm|docker rm|compose down' scripts/ e2e/ deploy/
rg -n 'writeFile|WriteFile|rename|fsync' auth-token-manager/ scripts/
```

**機械的に見ているもの.** `scripts/altharness/alt-harness guard-ops`（`scripts/altharness/internal/guards/ops.go` — コーディングエージェントが発行するシェルコマンドだけを対象とし、Codex では `.codex/hooks.json` に登録されて該当コマンドを拒否し、Claude Code では利用者のローカル設定に登録されている場合にのみ確認を求める。スクリプト内のコマンドや人間が直接入力するコマンドは見ない）。掃除スクリプト内部の広すぎる prefix 条件や、一回きり移行スクリプトの鮮度・権限検証までは見ていない。

---

## FP-19 持ち越された宿題が次の障害になる

**機構.** ポストモーテムのアクションアイテムや ADR の Consequences に書いた警告が、担当・期限・追跡無しのまま持ち越される。あるいは設定ファイルを作っただけで配送や実経路の E2E 確認なしに「完了」にされる。次に同じ領域を触る変更が、その開いた穴をもう一度踏む。

**先例.**
- PM-2026-010 / PM-2026-041 — `PM-2026-002` の未要約滞留監視と `PM-2026-010` の swap 後健全性検証が未着手のまま持ち越され、reproject 後の状態不整合と全行 `link=''` を本番で踏み直した。
- PM-2026-014 / PM-2026-016 / PM-2026-020 — `PM-2026-012 / PM-2026-013` の `be_slots=0` 監査や `PM-2026-008` の Ask Augur アラートが未実装のまま再発・3 度目の指摘に至った。
- PM-2026-031 — `ADR-000758` の Consequences に書かれた警告に担当と期限が無く、予告していた sidecar 孤立（Acolyte 502）が再発した。
- PM-2026-031 / PM-2026-032 / PM-2026-033 — `PM-2026-031` アクションアイテム #5（recap ジョブ連続失敗アラート）が未着手のまま `PM-2026-032`・`PM-2026-033` へ持ち越された。
- PM-2026-033 / PM-2026-035 / PM-2026-036 / PM-2026-037 — 3 項目（`classify-runs` 実データ smoke、`recap_failed_tasks` の Prometheus 出力、エラー文言区分化）が 4 本のポストモーテムを連続して未着手のまま継承された。
- PM-2026-054 — `PM-2026-028` のアラート作成が配送経路の実地確認なしに「完了」と記録され、次回の証明書失効時に 1 通も通知が届かなかった。
- PM-2026-062 — `PM-2026-052` の日次ジョブ連続失敗アラート（#8）が未実装のまま残り、約 6 日間の日次バッチ全停止が同じ形の無検知で再発した。

**問い.**
1. 新規に書く ADR の `Cons` やポストモーテムのアクションアイテムに、「注意する」だけで担当・期限・検証可能な完了条件の無い宿題を残していないか。
2. 変更対象サービス・機能に関連する過去ポストモーテムの未完了（`open` / `TODO` / `unknown`）アクションアイテムを確認したか。
3. 「完了（`Done`）」とした検知・予防策は、ファイル作成や単体テストだけでなく、実経路で発火・配送・拒否されることまで確認したか。

**確認コマンド.**
```bash
./scripts/altharness/alt-harness postmortem --open
rg -n 'TODO|未着手|OPEN|unknown' docs/postmortems/
```

**機械的に見ているもの.** `scripts/altharness/alt-harness postmortem`（`scripts/altharness/internal/ledgers/postmortem.go` — 新規ポストモーテムの検査と記録済みアクションアイテムの照会）。ADR の `Cons` にだけ書かれた警告や、「完了」とした項目の E2E 実動作までは見ていない。
