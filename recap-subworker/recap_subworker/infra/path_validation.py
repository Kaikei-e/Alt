"""Path validation utilities for recap-subworker.

ユーザー入力由来のパスを扱うときに、許可されたベースディレクトリ配下に
正規化・制限するための共通ユーティリティ。
"""

from __future__ import annotations

import os
from pathlib import Path
from typing import List

# 許可されたベースディレクトリ
# NOTE: ここを変更する場合は、APIレイヤなどの仕様とも合わせて見直すこと。
ALLOWED_BASE_DIRS: List[Path] = [
    Path("/app/data"),
    Path("/app/resources"),
]


def validate_path(user_path: str, base_dirs: List[Path] | None = None) -> Path:
    """ユーザー入力のパスを検証し、安全なPathオブジェクトを返す。

    - パスを正規化（.. などを除去）
    - 相対パスは base_dirs[0] をベースに解決
    - resolve() したうえで、いずれかの許可ディレクトリ配下であることを確認

    Args:
        user_path: ユーザーが指定したパス（絶対/相対）
        base_dirs: 許可されたベースディレクトリのリスト。
                   None の場合は ALLOWED_BASE_DIRS を利用。

    Returns:
        Path: 検証済みで、実際のファイルシステム上の絶対パス。

    Raises:
        ValueError: パスが許可されたディレクトリ外にある場合。
    """
    if base_dirs is None:
        base_dirs = ALLOWED_BASE_DIRS

    if not base_dirs:
        raise ValueError("No base directories configured for path validation")

    # パスを文字列として正規化
    normalized = os.path.normpath(user_path)

    # 相対パスの場合は最初の許可ディレクトリをベースとして使用
    if not os.path.isabs(normalized):
        normalized = os.path.normpath(os.path.join(str(base_dirs[0]), normalized))

    # realpath() でシンボリックリンクを解決し正規化
    # NOTE: CodeQL は os.path.realpath() + startswith() を sanitizer として認識する
    real_path = os.path.realpath(normalized)

    for base_dir in base_dirs:
        real_base = os.path.realpath(str(base_dir))
        # startswith に trailing separator を付けて prefix attack を防止
        # (例: /app/data-evil が /app/data にマッチしないようにする)
        if real_path == real_base or real_path.startswith(real_base + os.sep):
            return Path(real_path)

    # どの許可ディレクトリにも含まれていない
    raise ValueError(
        f"Path '{user_path}' is not within allowed directories: "
        f"{[str(d) for d in base_dirs]}"
    )


