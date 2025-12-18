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

    # 絶対パスかどうかで分岐
    if os.path.isabs(normalized):
        full_path = Path(normalized)
    else:
        # 相対パスの場合は最初の許可ディレクトリをベースとして使用
        full_path = Path(base_dirs[0]) / normalized
        full_path = Path(os.path.normpath(str(full_path)))

    # resolve() してシンボリックリンクなども解決したうえでチェック
    full_path_resolved = full_path.resolve()

    for base_dir in base_dirs:
        base_dir_resolved = base_dir.resolve()
        try:
            # パスがベースディレクトリ内にあるか確認
            full_path_resolved.relative_to(base_dir_resolved)
            return full_path_resolved
        except ValueError:
            # このベースディレクトリには含まれていない
            continue

    # どの許可ディレクトリにも含まれていない
    raise ValueError(
        f"Path '{user_path}' is not within allowed directories: "
        f"{[str(d) for d in base_dirs]}"
    )


