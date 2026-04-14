# Python Best Practices — Alt

対象: Python 3.14+、uv、FastAPI、pytest。一次ソース: [PEP 8](https://peps.python.org/pep-0008/), [PEP 257](https://peps.python.org/pep-0257/), [PEP 484](https://peps.python.org/pep-0484/), [Ruff](https://docs.astral.sh/ruff/), [mypy](https://mypy.readthedocs.io/), [Pydantic v2](https://docs.pydantic.dev/)。

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

- `mypy --strict` または `pyright --strict` を CI 必須化
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

## 6. Async Patterns

- Python 3.11+ の `asyncio.TaskGroup` を使う（例外を束ねて伝播）
- 複数非同期 I/O は `asyncio.gather` より `TaskGroup` 優先（キャンセル伝播が正しい）
- ブロッキング I/O は `asyncio.to_thread` に退避
- タイムアウトは `asyncio.timeout` コンテキスト

```python
# ✅
async with asyncio.TaskGroup() as tg:
    t1 = tg.create_task(fetch(a))
    t2 = tg.create_task(fetch(b))
result = (t1.result(), t2.result())

# ❌ 例外時の他タスクキャンセルが曖昧
await asyncio.gather(fetch(a), fetch(b))
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
- **mypy**: `--strict` を CI で必須化
- **uv**: 依存管理と仮想環境。`pip install` 禁止
- **pre-commit**: Ruff + mypy を pre-commit フックで走らせる

```toml
# pyproject.toml (抜粋)
[tool.ruff]
line-length = 100
target-version = "py314"

[tool.ruff.lint]
select = ["E", "W", "F", "I", "B", "UP", "SIM", "N", "ANN", "S", "PTH", "C4"]
ignore = ["ANN101", "ANN102"]  # self/cls の注釈は不要

[tool.mypy]
strict = true
```

## 11. Security

- **SQL injection**: 必ずパラメータバインド。f-string で SQL 組み立て禁止
- **eval / exec 禁止**: 動的評価は設計ミスのサイン
- **pickle 警戒**: 外部入力由来のデータを `pickle.load` しない（RCE リスク）
- **subprocess**: `shell=True` 禁止。`shlex.quote` または `list[str]` で渡す
- **秘匿情報**: コードに書かない。`.env` + Docker secrets（CLAUDE.md 参照）
- **Ruff `S` ルール** で機械的に検出。CI で違反を fail させる

```python
# ✅ パラメータバインド
await conn.fetch("SELECT * FROM articles WHERE id = $1", article_id)

# ❌ SQL インジェクション脆弱
await conn.fetch(f"SELECT * FROM articles WHERE id = '{article_id}'")
```

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
