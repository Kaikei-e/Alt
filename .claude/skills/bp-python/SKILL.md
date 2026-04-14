---
name: bp-python
description: Python ベストプラクティス。Python コードの品質を保つための規約とレビュー観点（Python 3.14+）。
  TRIGGER when: .py ファイルを編集・作成・レビューする時、Python コードを書く時、Python サービス（news-creator, tag-generator, metrics, recap-subworker, recap-evaluator）を実装する時。
  DO NOT TRIGGER when: テストの実行のみ、pyproject.toml の確認のみ、ファイルの読み取りのみ、他言語の作業時。
---

# Python Best Practices

このスキルが発動したら、`docs/best_practices/python.md` を Read ツールで読み込み、
記載されたベストプラクティス（DECREE）に従ってコードを書き、レビューすること。

## 重要原則

1. **型ヒント必須**: 公開関数・メソッドは完全アノテーション。`Any` は境界最小限。`uv run pyrefly check .` 通過必須（mypy は ADR-000530 により非推奨）
2. **例外は具体的に**: 裸の `except:` / `except Exception:` 禁止。`raise DomainError("action") from err` で原因チェーン保持
3. **Clean Architecture**: Handler → Usecase → Port → Gateway → Driver（news-creator 準拠）。層越境・逆向き依存禁止
4. **Ruff + Pyrefly が一次ソース**: フォーマット・静的検査はツールで自動化。Pyrefly ≥ 0.42.0 を採用（ADR-000530）。推奨ルール集合 `E,W,F,B,UP,SIM,N,I,ANN,S,PTH,C4,BLE,ASYNC,TRY,RUF,PL`。手動スタイル議論禁止
5. **Pydantic / frozen dataclass で境界保護**: API 入出力は Pydantic v2、内部値オブジェクトは `@dataclass(frozen=True, slots=True)`。生 dict を引き回さない
6. **context manager で資源管理**: `with` / `async with` で確実に close。async 並行は `asyncio.TaskGroup` / `async with`。裸 `open()` 禁止
7. **pytest + TDD**: RED → GREEN → REFACTOR。FastAPI のモジュールレベル `APIRouter()` はテスト分離を壊す → `importlib.reload()` で毎テスト再構築

## 参照

完全なベストプラクティスは `docs/best_practices/python.md` を参照。
セクション: Project Structure, Type Hints & Static Analysis, Error Handling, Clean Architecture, Pydantic & Dataclass, Async Patterns, Resource Management, Logging, Testing, Tooling, Security
