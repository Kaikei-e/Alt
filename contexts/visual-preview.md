# Visual Preview — Context Glossary

feed 記事を OG 画像つきカードで閲覧する surface の用語集。デスクトップはカードグリッド、
モバイルは 1 枚カードのスワイプ面 (Photo Wire Dispatch, ADR-000688) の 2 形態を持つ。
記事画像の取得・表示・欠落をめぐる正準語を固定する。実装詳細 (キュー・retry・閾値) は持たず、
それらは ADR 側に置く。alt-frontend-sv (表示) と alt-backend の image proxy (配信) にまたがる。

> 2026-06-22 grill セッションで定義開始。既読時の OG 画像 fallback バグを起点に語彙を鋭利化した。
> 2026-07-18 grill セッションでモバイルスワイプ面の操作語彙を追加 (誤タップ再設計を起点)。

## Language

### 画像

**OG image (Open Graph 画像)**:
記事の social-preview 画像。1 記事につき高々 1 枚。RSS 由来または scrape 由来。
_Avoid_: thumbnail (UI 上の見た目を指すときのみ可), サムネ画像

**OG image proxy URL**:
ブラウザの `<img src>` が唯一取得する HMAC 署名付きの内部 URL。
原画像 URL を base64 で内包し、上流ホストへの取得・キャッシュ・配信を仲介する。
_Avoid_: image URL (原画像 URL と紛れる), 画像リンク

**Age-gate (7 日保持)**:
OG 画像を著作権上 7 日のみ保持し、以降 purge する保持規則。
purge 後は backend が画像 URL を NULL 返却し、再ロードで再取得され得る。
_Avoid_: TTL (曖昧), キャッシュ期限

### Fallback の 2 つの意味 (最重要・撃ち分ける)

同じ gradient placeholder でも原因と正しい対応が異なる。UI は両者を区別する。

**Transient fallback (一過性フォールバック)**:
OG 画像は存在するのに proxy 取得が一時失敗して gradient を出している状態。
**回復可能** — 取得をやり直せば本来の画像が出る。既読バグの本体はこれ。
_Avoid_: 画像エラー (恒久と紛れる), 画像なし

**Absent OG image (恒久的な画像なし)**:
そもそも表示すべき OG 画像が無い (age-gate purge 済み / 未 scrape) ために gradient を出している状態。
**回復不能** — やり直しても出ない。設計通りの正常表示。
_Avoid_: フォールバック (単独では一過性と紛れる)

**Image fallback (gradient)**:
画像が出せないときに見せる gradient の placeholder。上記 2 状態の*見た目上*の共通帰着点。
それ自体は原因を語らないので、必ず transient / absent のどちらかと併せて使う。
_Avoid_: プレースホルダ (loading 中の shimmer と紛れる)

**Shimmer**:
proxy 取得が**進行中**であることを示すロード中アニメーション。
transient fallback (失敗) とも absent (画像なし) とも別の、第三の状態。
_Avoid_: ローディング (曖昧), placeholder

### 操作

**Mark as read (既読化)**:
この surface では、当該カードを未読グリッドから外し、後続記事を 1 枚補充する操作。
モバイルスワイプ面では左右どちらのスワイプも等しくこの操作である (方向に別の意味を持たせない)。
補充とそれに伴う画像取得が新たな取得負荷を生む点が、既読と画像欠落が相関して見える由来。
_Avoid_: 閲覧済み, 読了

### モバイルスワイプ面 (Photo Wire Dispatch)

> 2026-07-18 grill セッションで確定。ボトムナビ・フッター 3 ボタンの誤タップ再設計を起点に定義。

**Immersive mode (没入モード)**:
スワイプ面がボトムナビを退け、全画面で 1 枚のカードに集中する表示状態。
ボトムナビ persistent 原則 (ADR-000716) に対する、スワイプ面限定の明示的例外。
出口は必ず Dispatch header の戻るで見える形に置く (mystery meat 禁止, ADR-000639)。
_Avoid_: フルスクリーンモード (OS の Fullscreen API と紛れる), キオスクモード

**Dispatch header (ディスパッチヘッダー)**:
没入モード上部の編集部風ミニヘッダー。戻る導線・kicker (面の名前)・進捗・Undo を担う。
カードの外にあり、面全体 (セッション) に属する操作だけを載せる。
_Avoid_: ナビバー, アプリバー, ツールバー

**Keep stamp (検印スタンプ)**:
カード右上に密着する角型の保存操作 (☆)。不透明・sharp edges の「写真電送の検印」であり、
半透明サークルやフローティングボタンではない。保存判断は個々のカードに属するため、
面に属する Dispatch header ではなくカード上に置く。
_Avoid_: お気に入りボタン (機能名としては可, UI 部位名としては不可), FAB, オーバーレイボタン

**Reading actions (読み方の選択)**:
カードフッターの ARTICLE / SUMMARY の 2 大ボタン。「この記事をどう読むか」を選ぶ操作で、
「この記事を残すか」を決める Keep stamp とは異質。ゆえに同列に並べない (誤タップ再設計の核心)。
_Avoid_: アクションボタン (Keep stamp を含んでしまう)

**Undo (直前既読の取り消し)**:
直前にスワイプで既読化した 1 枚をカード列に戻す操作。誤スワイプの回復手段として
Dispatch header に置く。スワイプ方向への意味付け (Tinder 型) は採らない。
_Avoid_: 巻き戻し, リワインド
