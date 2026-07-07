# Python Best Practices — Alt

対象: Python 3.14+、uv、FastAPI、pytest。一次ソース: [PEP 8](https://peps.python.org/pep-0008/), [PEP 257](https://peps.python.org/pep-0257/), [PEP 484](https://peps.python.org/pep-0484/), [Ruff](https://docs.astral.sh/ruff/), [Pyrefly](https://pyrefly.org/), [Pydantic v2](https://docs.pydantic.dev/)。型検査ツールは [[000530]] により Pyrefly ≥ 0.42.0 を採用（mypy は非推奨）。

## 1. Project Structure

- `src/` レイアウトを採用。`pyproject.toml` で単一パッケージ宣言（uv が前提）
- モジュール名は `snake_case`、クラスは `PascalCase`（[PEP 8](https://peps.python.org/pep-0008/#naming-conventions)）
- `__init__.py` は薄く保つ。副作用のある import を入れない
- エントリポイントは `app/main.py` の `main()` のみ。ビジネスロジック禁止

> **Alt:** `news-creator`, `tag-generator`, `metrics`, `recap-subworker`, `recap-evaluator` は全て `src/` 配下に `app/` パッケージを持つ構成。`uv run` で実行。

```
service/
  app/
    handler/      # FastAPI ルーター、入出力整形
    usecase/      # ビジネスロジック（I/O 非依存）
    port/         # 抽象インタフェース (Protocol / ABC)
    gateway/      # 外部サービス呼び出し
    driver/       # DB / HTTP / ファイル I/O 実装
    config.py     # 設定値（環境変数 → Pydantic Settings）
    main.py       # FastAPI app + lifespan
  tests/
  pyproject.toml
```

## 2. Type Hints & Static Analysis

- 公開関数・メソッドは引数と戻り値を完全アノテーション
- コンテナは具象型でなく抽象型（`Iterable`, `Mapping`, `Sequence`）を受ける／具象を返す
- `Any` は境界でのみ。内部に漏らさない
- 型ガードには `typing.TypeGuard` / `TypeIs` を使う

```python
# ✅
from collections.abc import Sequence

def total(amounts: Sequence[int]) -> int:
    return sum(amounts)

# ❌ 具象を強制して柔軟性を失う
def total(amounts: list[int]) -> int: ...
```

- `uv run pyrefly check .` を CI 必須化（ADR-000530 で Pyrefly ≥ 0.42.0 を採用、mypy は非推奨）
- `from __future__ import annotations` は不要（Python 3.14 で PEP 649 により遅延評価が既定）

## 3. Error Handling

- 裸 `except:` / `except Exception:` 禁止。捕捉する例外型を明示
- 再送出は `raise ... from err` で原因チェーン保持
- ドメイン例外はクラス階層で表現。文字列比較禁止
- 外部境界（API/CLI）で拾って整形。内部層で握り潰さない

```python
# ✅
class DomainError(Exception): ...
class ArticleNotFound(DomainError): ...

def load(article_id: str) -> Article:
    try:
        return _repo.get(article_id)
    except KeyError as err:
        raise ArticleNotFound(article_id) from err

# ❌ コンテキストを失う
try:
    return _repo.get(article_id)
except Exception:
    return None
```

> **Alt:** FastAPI ハンドラは `HTTPException` への変換レイヤを `handler/` に集約し、`usecase/` 以下はドメイン例外を投げる。

- **フォールバックには理由の型を付ける**: blanket `except Exception` → fallback は根本原因を消す。
  timeout / json_decode / validation 等を型別に catch し、`error_type` を構造化ログと
  degradation reason の両方に残す。「握り潰して縮退」は復旧経路とアラートを同時に殺す
  （ADR-000706, PM-2026-014）
- **フォールバック条件を truthiness で書かない**: `if keywords:` は 1 件でも truthy になり、
  「十分な結果が得られなければフォールバック」の意図を満たさない。件数閾値
  （`len(keywords) >= MIN_KEYWORDS`）や `x is not None` で明示する（ADR-000321）

## 4. Clean Architecture

依存方向: `handler` → `usecase` → `port` ← `gateway` ← `driver`

`port` は抽象（Protocol/ABC）。`usecase` は `port` にのみ依存し、具体実装（`gateway`/`driver`）を直接 import しない。

```python
# port/article_repo.py
from typing import Protocol

class ArticleRepo(Protocol):
    async def get(self, article_id: str) -> Article: ...

# usecase/summarize.py
class Summarize:
    def __init__(self, repo: ArticleRepo) -> None:
        self._repo = repo

    async def execute(self, article_id: str) -> Summary:
        article = await self._repo.get(article_id)
        return _summarize(article)
```

> **Alt:** `news-creator` ではこの 5 層を厳守。`usecase` 内で `httpx` / `asyncpg` を直接呼ぶレビュー指摘は常に差し戻し。

## 5. Pydantic & Dataclass

- **API 境界**: Pydantic v2 `BaseModel`。`model_config = ConfigDict(strict=True, frozen=True)`
- **内部値オブジェクト**: `@dataclass(frozen=True, slots=True)`
- 生 `dict[str, Any]` をレイヤ間で引き回さない

```python
from dataclasses import dataclass
from pydantic import BaseModel, ConfigDict

class ArticleIn(BaseModel):
    model_config = ConfigDict(strict=True, frozen=True)
    url: str
    title: str

@dataclass(frozen=True, slots=True)
class Article:
    id: str
    url: str
    title: str
```

### 起動時 fail-closed — 必須依存の lazy init 禁止

必須の外部 artefact（モデルファイル、vectorizer 等）を初回リクエストで遅延ロードすると、
欠損が「起動成功 → 全リクエスト 500」として現れる。Pydantic Settings の
`@model_validator` で起動時に検証し、失敗なら即 exit する（PM-2026-035, PM-2026-036, ADR-000825）。

- ファイル存在チェックは `Path.is_file()` を使う。`exists()` は Docker の file-scoped bind が
  作る空ディレクトリでも True になり、guard を素通りする

```python
class Settings(BaseSettings):
    model_path: Path

    @model_validator(mode="after")
    def _validate_artifacts(self) -> "Settings":
        if not self.model_path.is_file():
            raise ValueError(f"model artefact missing: {self.model_path}")
        return self
```

## 6. Async Patterns

- Python 3.11+ の `asyncio.TaskGroup` を使う（例外を束ねて伝播）
- 複数非同期 I/O は `asyncio.gather` より `TaskGroup` 優先（キャンセル伝播が正しい）
- ブロッキング I/O は `asyncio.to_thread` に退避
- タイムアウトは `asyncio.timeout` コンテキスト

### 同期 ML 推論・CPU-bound 処理をイベントループで実行しない

`async def` ハンドラ内で同期推論（transformers / ONNX / psutil サンプリング等）を
直接呼ぶと**イベントループ全体が止まり、health check を含む全リクエストが停止する**
（[FastAPI: async](https://fastapi.tiangolo.com/async/)）。全 Python サービスの
rerank / embedding / 生成 router に共通する頻出違反。

- 同期呼び出ししかしないハンドラは `def` で宣言（FastAPI が threadpool に退避）
- `async def` 内から呼ぶなら `anyio.to_thread.run_sync` / `run_in_threadpool` に退避
- 持続的な CPU-bound 推論は thread では GIL で並列化できない —
  process pool か専用 worker（mq 経由）へ
- 同時推論数は `asyncio.Semaphore` / `anyio.CapacityLimiter` で必ず上限を張る

```python
# ❌ イベントループをブロック
@router.post("/rerank")
async def rerank(req: RerankIn) -> RerankOut:
    return _model.predict(req.pairs)  # 同期推論

# ✅ threadpool へ退避 + 同時実行上限
_limiter = anyio.CapacityLimiter(2)

@router.post("/rerank")
async def rerank(req: RerankIn) -> RerankOut:
    return await anyio.to_thread.run_sync(_model.predict, req.pairs, limiter=_limiter)
```

```python
# ✅
async with asyncio.TaskGroup() as tg:
    t1 = tg.create_task(fetch(a))
    t2 = tg.create_task(fetch(b))
result = (t1.result(), t2.result())

# ❌ 例外時の他タスクキャンセルが曖昧
await asyncio.gather(fetch(a), fetch(b))
```

### async generator の finally は実行保証がない（PEP 525）

クライアント切断で FastAPI `StreamingResponse` の async generator が中断されると、
`aclose()` が呼ばれない限り `finally` は実行されず、確保済みのセマフォスロット等が
永久リークする（ADR-000243, PM-2026-004）。1 レイヤの `finally` を信用せず多層防御にする:

- 呼び出し側で `contextlib.aclosing()` に包んで `aclose()` を保証
- generator 側は `GeneratorExit` / `asyncio.CancelledError` でも解放処理を走らせる
- 取得数と解放数を突き合わせるリーク検知をメトリクスに出す

```python
# ✅ 呼び出し側で aclose を保証
from contextlib import aclosing

async with aclosing(stream_tokens(prompt)) as gen:
    async for token in gen:
        await send(token)
```

### セマフォ / スロットは所有権を明示追跡する

優先度付きセマフォのスロットリークは同一コンポーネントで 5 回のインシデントを起こした
頻出パターン（ADR-000601, ADR-000606, ADR-000612, PM-2026-012, PM-2026-015）:

- `acquire` は `slot_id`（と出身プール `home_pool`）を返し、`release` に必ず渡す。
  呼び出し元の属性から返却先を推論するフォールバックはバグ温床
- release パスに invariant チェック（`available + acquired == total_slots`）を置く。
  ただし waiter へ転送中 (in transit) の transient window を考慮しないと false positive になる
- `CancelledError` ハンドラは「取得済みだが未返却のリソース」を網羅的に棚卸しして返却する。
  waiter へ転送済みのスロットも受け手キャンセル時は回収義務がある

### 同一スレッドで完結する処理に `call_soon_threadsafe` を使わない

`call_soon_threadsafe` は別スレッドからの呼び出し専用。同一イベントループスレッド内で使うと
遅延スケジュールとキャンセルの race でリソースが消失する（ADR-000610, PM-2026-014）。
同一スレッドなら `future.done()` / `future.cancelled()` を事前チェックして直接 `set_result()` する。

### ブロッキング await はプリエンプトできない

cancel フラグを await の前後でチェックしても、30–90 秒かかる HTTP POST の最中は中断できない。
中断可能にするには `asyncio.wait(..., return_when=FIRST_COMPLETED)` で cancel task と競争させ、
未完了側を `task.cancel()` する（ADR-000556）。

```python
req_task = asyncio.create_task(client.post(url, json=payload))
cancel_task = asyncio.create_task(cancel_event.wait())
done, pending = await asyncio.wait(
    {req_task, cancel_task}, return_when=asyncio.FIRST_COMPLETED
)
for task in pending:
    task.cancel()
```

### httpx の timeout は 4 ステージを個別指定する

スカラー `timeout=30` は connect / read / write / pool の 4 ステージ全部に同値展開される
（プール取得待ちだけで 30 秒許容する等、意図と乖離する）。`httpx.Timeout` で個別化し、
全体上限は `asyncio.timeout` を外郭に張ってドメイン例外へ変換する（ADR-000732, ADR-000733）。

```python
# ✅ ステージ別に意図を明示
timeout = httpx.Timeout(connect=5.0, read=30.0, write=10.0, pool=5.0)
```

## 7. Resource Management

- ファイル、DB 接続、ロックは必ず `with` / `async with`
- 自作クラスが資源を持つなら `__enter__` / `__aenter__` を実装
- `contextlib.closing` / `contextlib.asynccontextmanager` を使う

```python
# ✅
async with asyncpg.create_pool(dsn) as pool:
    async with pool.acquire() as conn:
        await conn.execute(...)

# ❌ close 漏れの温床
conn = await asyncpg.connect(dsn)
await conn.execute(...)
```

### 永続化ファイルは tmpfile + rename + fsync

書き込み途中で kill されると半書きファイルが残り、次回起動時に壊れた状態を読み込む。
token 等の状態ファイルは同一ディレクトリの一時ファイルに書き、`fsync` してから
`Path.replace`（同一ファイルシステム内でアトミック）で差し替える（PM-2026-043）。

```python
tmp = path.with_suffix(".tmp")
with tmp.open("w") as f:
    f.write(payload)
    f.flush()
    os.fsync(f.fileno())
tmp.replace(path)
```

## 8. Logging

- 標準 `logging` または `structlog`。`print` 禁止
- 構造化ログ（JSON）を基本。キー名は `snake_case`
- 機密情報（トークン、PII）はロギング前にマスク
- 例外は `logger.exception(...)` でスタック込み記録

```python
import logging
logger = logging.getLogger(__name__)

# ✅
logger.info("article.summarized", extra={"article_id": article.id, "tokens": n})

# ❌ 文字列連結・秘匿情報そのまま
logger.info(f"user={user.email} token={token}")
```

## 9. Testing

- `pytest` + `pytest-asyncio` が標準
- **RED → GREEN → REFACTOR** を厳守。テストと実装を同一コミットにしない
- fixture のスコープは最小に（`function` 既定）
- モックはドライバ層のみ。ユースケース単体テストでは fake implementation を使う
- `parametrize` でテーブル駆動

```python
# ✅ パラメトライズ
@pytest.mark.parametrize(
    ("input_", "expected"),
    [("a", 1), ("bb", 2), ("", 0)],
)
def test_length(input_: str, expected: int) -> None:
    assert len(input_) == expected
```

> **Alt:** FastAPI のモジュールレベル `router = APIRouter()` はプロセス横断状態を持ち、テスト間で汚染される。
> 解決: 各テストで `importlib.reload(module)` してルーターを再構築する（`news-creator` で確立されたパターン）。

```python
import importlib
import app.handler.article_handler as handler_module

@pytest.fixture
def handler():
    importlib.reload(handler_module)
    return handler_module
```

## 10. Tooling

- **Ruff**: linter + formatter を一本化。以下をベースに有効化:
  - `E`, `W` (pycodestyle), `F` (Pyflakes), `I` (isort)
  - `B` (flake8-bugbear), `UP` (pyupgrade), `SIM` (simplify), `N` (pep8-naming)
  - `ANN` (annotations), `S` (bandit), `PTH` (use-pathlib), `C4` (comprehensions)
  - `BLE` (blind-except), `ASYNC` (async best practices), `TRY` (tryceratops), `RUF`, `PL` (pylint)
- **Pyrefly**: `uv run pyrefly check .` を CI で必須化（[[000530]] で mypy から Pyrefly ≥ 0.42.0 に移行）
- **uv**: 依存管理と仮想環境。`pip install` 禁止
- **pre-commit**: Ruff + Pyrefly を pre-commit フックで走らせる
- **Python バージョンは全経路で固定**: `requires-python` 宣言＋ `uv python pin 3.14`
  で `.python-version` をコミットし、CI（`setup-python` の `python-version-file`）・
  lint・型検査を**本番と同一 minor バージョン**で走らせる。
  3.14 専用構文（PEP 758 `except A, B:` 等）は 3.11 のツールチェーンでは
  パース不能になり、解析が丸ごと落ちる
- **Python 3.14 / 依存メジャーアップの既知の罠**（ADR-000563, ADR-000611）:
  - `Mock(spec=)` は instance attribute の検査が厳密化され、既存テストが落ちる
  - numpy 2.0 は暗黙変換が TypeError になり、間接依存にも波及する
  - PyO3 拡張は `PYO3_USE_ABI3_FORWARD_COMPATIBILITY=1` が必要な場合がある
  - spacy / pydantic v1 は PEP 649（遅延アノテーション評価）と非互換

```toml
# pyproject.toml (抜粋)
[tool.ruff]
line-length = 100
target-version = "py314"

[tool.ruff.lint]
select = ["E", "W", "F", "I", "B", "UP", "SIM", "N", "ANN", "S", "PTH", "C4", "BLE", "ASYNC", "TRY", "RUF", "PL"]
ignore = ["ANN101", "ANN102"]  # self/cls の注釈は不要

[tool.pyrefly]
project-includes = ["src"]
python-version = "3.14"
# ML ライブラリは型スタブ不足のため import 解決の失敗のみ抑止（内部コードは厳密検査を維持）
[tool.pyrefly.errors]
missing-import = false
missing-module-attribute = false
```

## 11. Security

- **SQL injection**: 必ずパラメータバインド。f-string で SQL 組み立て禁止
- **eval / exec 禁止**: 動的評価は設計ミスのサイン
- **pickle 警戒**: 外部入力由来のデータを `pickle.load` しない（RCE リスク）
- **subprocess**: `shell=True` 禁止。`shlex.quote` または `list[str]` で渡す
- **秘匿情報**: コードに書かない。`.env` + Docker secrets（CLAUDE.md 参照）
- **Ruff `S` ルール** で機械的に検出。CI で違反を fail させる
- **認証の無言フォールバック禁止**: 認証モジュールの import 失敗や
  トークン未設定時に anonymous / no-op 実装へ差し替えない。**起動時に raise**。
  「認証が効いているつもりで無認証」はレビューで最も危険な HIGH 類型
  （CLAUDE.md ルール8 / `.claude/rules/di-wiring.md`）

```python
# ✅ パラメータバインド
await conn.fetch("SELECT * FROM articles WHERE id = $1", article_id)

# ❌ SQL インジェクション脆弱
await conn.fetch(f"SELECT * FROM articles WHERE id = '{article_id}'")
```

## 12. ML Runtime & Process Pools

- **CUDA は fork した子プロセスで再初期化できない**。Gunicorn の fork worker ではなく
  Uvicorn シングルプロセス + `multiprocessing.get_context("spawn")` の専用プールを使い、
  torch は子プロセス側で遅延 import する（ADR-000048）
- **spawn プールはワーカー数に比例してメモリが線形増加する**（CoW が効かない）。
  「ワーカー数 × モデルサイズ」で見積もり、mem_limit は実測ピーク + ヘッドルームで設定。
  子プロセスが OOM kill されるとエラーログなしで親の `.get()` が永久ブロックする
  **無症状ハング**になる — `.get()` には必ず timeout を付ける（ADR-000550, PM-2026-001）
- **`TOKENIZERS_PARALLELISM=false` を既定化する**。また重量級初期化
  （torch / ProcessPoolExecutor）を FastAPI DI の lazy init に置くとテストが無限ハングする —
  テストでは `dependency_overrides` の autouse stub で隔離する（ADR-000728）
- **Numba は `NUMBA_THREADING_LAYER=tbb` を明示する**。既定フォールバックの `workqueue` は
  非スレッドセーフで、/health を含むプロセス全体がデッドロックする。pip インストールの TBB は
  `LD_LIBRARY_PATH` を明示しないと検出されない（ADR-000575, PM-2026-007）
- **大入力の埋め込み・クラスタリングは固定バッチ + `MiniBatchKMeans`**。一括 encode +
  通常 KMeans はメモリ上限で OOM する。バッチ上限は回帰テストで固定する（ADR-000637）

---

## レビュー時のチェックリスト

- [ ] 公開 API に型ヒントが完全に付いているか
- [ ] `except:` / `except Exception:` が無いか、`raise ... from err` になっているか
- [ ] Clean Architecture の層越境が無いか（`usecase/` から `driver/` 直 import 等）
- [ ] Pydantic/frozen dataclass の代わりに `dict[str, Any]` が引き回されていないか
- [ ] 資源は `with` / `async with` で閉じているか
- [ ] ログに秘匿情報が混入していないか、構造化されているか
- [ ] テストは RED → GREEN の順でコミットされているか（テストと実装が同一コミットでないか）
- [ ] `asyncio.gather` ではなく `TaskGroup` が使えないか
- [ ] Ruff `S`（bandit）ルール違反が無いか、`eval`/`exec`/`pickle`/`shell=True` が無いか
- [ ] モジュールレベル `APIRouter()` 等のグローバル状態がテスト分離を壊していないか
- [ ] 型検査は Pyrefly（mypy ではない）を使っているか、`uv run pyrefly check .` が 0 エラーか
- [ ] `async def` ハンドラ内に同期推論・CPU-bound 呼び出しが直書きされていないか（to_thread / process pool / Semaphore）
- [ ] import 失敗・env 未設定で no-op / anonymous にフォールバックする箇所がないか（起動時 raise になっているか）
- [ ] `.python-version` と CI のバージョンが本番と一致しているか
- [ ] async generator の解放が `finally` 単層頼みになっていないか（`contextlib.aclosing` / `GeneratorExit` 対応があるか）
- [ ] セマフォ/スロットの release が `slot_id` ベースか、`CancelledError` ハンドラが取得済みリソースを棚卸ししているか
- [ ] httpx の timeout がスカラーでなく 4 ステージ個別指定になっているか
- [ ] 必須 artefact が起動時 `@model_validator` + `is_file()` で fail-closed になっているか（lazy init していないか）
- [ ] spawn プールの「ワーカー数 × モデルサイズ」が mem_limit に収まるか、`.get()` に timeout があるか
- [ ] 状態ファイルの書き込みが tmpfile + rename + fsync になっているか
- [ ] フォールバック条件が truthiness でなく件数閾値 / `is not None` で書かれているか
