# Visual Preview — Context Glossary

デスクトップで feed 記事を OG 画像サムネイルのカードグリッドとして閲覧する surface の用語集。
記事画像の取得・表示・欠落をめぐる正準語を固定する。実装詳細 (キュー・retry・閾値) は持たず、
それらは ADR 側に置く。alt-frontend-sv (表示) と alt-backend の image proxy (配信) にまたがる。

> 2026-06-22 grill セッションで定義開始。既読時の OG 画像 fallback バグを起点に語彙を鋭利化した。

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
補充とそれに伴う画像取得が新たな取得負荷を生む点が、既読と画像欠落が相関して見える由来。
_Avoid_: 閲覧済み, 読了
