# 失敗の型（FP-01 〜 FP-11）

`docs/ADR/` の本文が自分で記録している不具合・回帰・差し戻しから抽出した型。各節は同じ並びで書く:
機構、先例、問い、確認コマンド、機械的に見ているもの。先例の ADR は `docdag context <id>` で読める。

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

**問い.**
1. 追加・変更した依存が未設定のとき、起動ログに `*_enabled` / `*_disabled` が出るか。業務経路で未配線の枝に入ったら止まるか。
2. 空の結果は「0 件だった」と「取れなかった」を呼び出し側が区別できるか。
3. 副作用が失敗したあとに checkpoint・ACK・ジョブ状態が進む経路は無いか。
4. health は、このプロセスの仕事の前提（listener の bind、モデルや分類器の実体、トークンの中身、直近成功時刻）を見ているか。

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

**問い.**
1. このイベントが触らない列を、upsert はどの値で書くか。空文字・`null`・未設定は「変更なし」として扱われるか。
2. projector は event payload と版つきリソース以外を読んでいないか（最新行、壁時計、active な投影）。
3. 同じ event log を 2 回投影して行が一致するテストがあるか。本番に残る旧 payload の形を食わせているか。
4. 追加した表・列・インデックスの migration ディレクトリは、それを読むプロセスが接続する DB のものか。

**確認コマンド.**
```bash
rg -n 'ON CONFLICT' -A12 <changed-sql-or-driver>
rg -n 'time\.(Now|Since|Until)\(|GetLatest[A-Z]' knowledge-sovereign/app/usecase/
rg -n 'CREATE TABLE|CREATE INDEX' <migration-dir>/ | rg '<table>'
```

**機械的に見ているもの.** `scripts/agent-harness/guard_event_time.py`（projector への壁時計と
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
- ADR-000551 — 要約できない 63 記事が placeholder を書かず、1349 回再投入された。
- ADR-000149 — 保存 0 件・55 ジャンル失敗のジョブが Completed で閉じた。
- ADR-000512 — upsert でも作成イベントを出し、処理済みが再びキューに積まれた。
- ADR-000320 — `$effect` が自分の代入で再発火し、5 分でストリーム開始 9,181 回。
- ADR-001013 / ADR-001014 — 永久に弾かれる記事が一覧の枠を使い切る。版を作らない placeholder で候補から外した。

**問い.**
1. 再試行しても成功しない失敗（参照先なし、継続する 403、入力が規約外）は、候補集合から外れる終端状態を持つか。
2. その終端状態は、下流に「完了」と誤読されないか（版・イベント・完了フラグを作らないか）。
3. 0 件処理は成功と区別されるか。
4. 外側の期限（クライアント、プロキシ、JWT）は内側の処理時間より長いか。
5. `$effect` は自分が書く state を依存に取っていないか。

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
- ADR-000826 — 依存を起動しない rolling は `service_completed_successfully` の init を起動も create もしない。compose 上は healthy、volume は空。
- ADR-000703 — restart policy が無く、コンテナが約 36 時間不在。
- ADR-000892 — ルールファイルの glob がコンテナ内で空。全ルールが無音で load 失敗。
- ADR-000952 — 下限だけの版指定で minor が上がり、学習済み成果物と不一致で worker が初期化できない。
- ADR-000784 / ADR-000802 — healthcheck が証明書の鮮度だけを見て、listener が bind していない状態を healthy とした。

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
`scripts/compose-netns-cascade-audit.py`（`.github/workflows/compose-audit.yaml`）。常駐サービスの
restart policy 欠落と、依存の版固定は見ていない。

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

**問い.**
1. このテストは、本番で動くハンドラ・配線済みのコンテナ・実在する route を通るか。
2. 追加した CI ステップは、失敗したとき job を落とすか。握りつぶしに理由が書かれているか。
3. consumer が要求するヘッダ・認証を、契約テストは assert しているか。
4. 修正した不具合を、修正前のコードで実行して赤になることを確かめたか。

**確認コマンド.**
```bash
rg -n -F '|| true' .github/workflows/
rg -n 'continue-on-error' .github/workflows/
rg -n 'route\(|page\.route|mock|stub' <test-dir>/ | rg '<path>'
```

**機械的に見ているもの.** `.github/workflows/scripts/workflow-safety-check.py`（握りつぶしには
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

**問い.**
1. これは連鎖の何件目か。直前の ADR が直した経路の次段（初回の次の追加ロード、返却の次のキャンセル）にテストがあるか。
2. 制御（認証、検証、ガード）を外す変更なら、その制御が塞いでいた条件を名前にしたテストが残るか。
3. 修正した画面・経路の隣接状態（初回、追加、空、失敗）を同じ変更のテストに含めたか。

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

**問い.**
1. この ID はどの表の主キーか。関数の引数名と型は、隣の表の ID と取り違えられないか。
2. 複合一意の列すべてが、読む SQL の WHERE にあるか。
3. `string` や `uuid.UUID` のまま、意味の違う ID を同じ引数で渡せる関数を増やしていないか。

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
python3 scripts/agent-harness/adr_supersede_gate.py --base origin/main
docdag query --binding --fields id,title
docdag validate --immutable-since origin/main
```

**機械的に見ているもの.** `scripts/agent-harness/adr_supersede_gate.py`（新しい `supersedes` 辺は、
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
python3 scripts/agent-harness/harness_selfcheck.py
```

**機械的に見ているもの.** `python3 scripts/kisage-env.py verify`（外部 API 間隔）、
`scripts/agent-harness/harness_selfcheck.py`（エージェント向けハーネス自身の床: フックの入力、
ガード対象パスの実在、CLAUDE.md と AGENTS.md の共有ブロック一致）。
